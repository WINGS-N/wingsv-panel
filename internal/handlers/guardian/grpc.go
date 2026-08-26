// gRPC transport for the Guardian protocol. It carries the same guardianpb.Frame
// envelope the WebSocket endpoint uses and shares its session logic; only the pipe
// differs. Two RPCs, because a device has two very different needs:
//
//   - Session is the live bidirectional channel for a device that is awake.
//   - Sync is one round trip for the background worker. A long-lived socket does
//     not survive doze, so the periodic path stopped trying to hold one open: it
//     posts what it has and takes back whatever the panel had queued.
package guardian

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	"v.wingsnet.org/internal/guardianhub"
)

type GRPCService struct {
	guardianpb.UnimplementedGuardianServer
	handler *Handler
}

func NewGRPCService(h *Handler) *GRPCService {
	return &GRPCService{handler: h}
}

func (s *GRPCService) Register(registrar grpc.ServiceRegistrar) {
	guardianpb.RegisterGuardianServer(registrar, s)
}

// grpcSession adapts a server stream to guardianhub.ClientSink. gRPC forbids
// concurrent SendMsg on one stream and the hub fans out from admin goroutines, so
// every write goes through the mutex.
type grpcSession struct {
	stream  grpc.BidiStreamingServer[guardianpb.Frame, guardianpb.Frame]
	writeMu sync.Mutex
	cancel  context.CancelFunc
	closed  chan struct{}
	once    sync.Once
}

func (s *grpcSession) SendFrame(frame *guardianpb.Frame) error {
	if s == nil {
		return errors.New("nil session")
	}
	select {
	case <-s.closed:
		return errors.New("session closed")
	default:
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.stream.Send(frame)
}

func (s *grpcSession) Close(reason string) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.closed)
		if s.cancel != nil {
			s.cancel()
		}
	})
}

func (s *GRPCService) Session(stream grpc.BidiStreamingServer[guardianpb.Frame, guardianpb.Frame]) error {
	h := s.handler
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	first, err := recvWithTimeout(ctx, stream, helloTimeout)
	if err != nil {
		return status.Error(codes.DeadlineExceeded, "hello timeout")
	}
	hello := first.GetClientHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first frame must be ClientHello")
	}

	client, ok := h.authenticate(hello)
	if !ok {
		_ = stream.Send(&guardianpb.Frame{
			Payload: &guardianpb.Frame_ServerHello{
				ServerHello: &guardianpb.ServerHello{
					Accepted:        false,
					ErrorMessage:    "invalid credentials",
					ProtocolVersion: protocolVersion,
				},
			},
		})
		return status.Error(codes.Unauthenticated, "invalid credentials")
	}

	sess := &grpcSession{stream: stream, cancel: cancel, closed: make(chan struct{})}
	defer sess.Close("session end")

	if err := sess.SendFrame(&guardianpb.Frame{
		Payload: &guardianpb.Frame_ServerHello{
			ServerHello: &guardianpb.ServerHello{Accepted: true, ProtocolVersion: protocolVersion},
		},
	}); err != nil {
		return err
	}

	connectedAt := time.Now()
	log.Printf("guardian: grpc session up client=%s device=%s app=%s",
		client.ID, hello.GetDeviceModel(), hello.GetAppVersion())

	h.markOnline(client, hello)
	h.hub.AttachClient(client.ID, sess)
	defer func() {
		h.hub.DetachClient(client.ID, sess)
		h.markOffline(client)
		log.Printf("guardian: grpc session down client=%s lifetime=%s",
			client.ID, time.Since(connectedAt).Truncate(time.Second))
	}()

	for _, frame := range h.welcomeFrames(client, hello.GetLastAppliedConfigVersion()) {
		if err := sess.SendFrame(frame); err != nil {
			return err
		}
	}

	// No application-level heartbeat ticker here: HTTP/2 keepalive already proves
	// the channel, and a device that still sends Heartbeat frames gets them
	// bounced back by handleClientFrame.
	for {
		frame, recvErr := stream.Recv()
		if recvErr != nil {
			return nil
		}
		h.handleClientFrame(client, sess, frame)
	}
}

func (s *GRPCService) Sync(ctx context.Context, req *guardianpb.SyncRequest) (*guardianpb.SyncResponse, error) {
	h := s.handler
	hello := req.GetHello()
	if hello == nil {
		return nil, status.Error(codes.InvalidArgument, "hello is required")
	}
	client, ok := h.authenticate(hello)
	if !ok {
		return &guardianpb.SyncResponse{
			Hello: &guardianpb.ServerHello{
				Accepted:        false,
				ErrorMessage:    "invalid credentials",
				ProtocolVersion: protocolVersion,
			},
		}, nil
	}

	// A one-shot sync is not a live channel, so presence blips rather than sticking
	// online: the device is reachable at this instant and offline again right after.
	// The write also refreshes last_seen_at and the device info.
	h.markOnline(client, hello)
	defer h.markOffline(client)

	sink := &collectingSink{}
	if state := req.GetState(); state != nil {
		h.storeStateReport(client, state)
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{
			ClientID: client.ID,
			Frame:    &guardianpb.Frame{Payload: &guardianpb.Frame_StateReport{StateReport: state}},
		})
	}
	if apps := req.GetInstalledApps(); apps != nil {
		h.storeInstalledApps(client, apps)
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{
			ClientID: client.ID,
			Frame:    &guardianpb.Frame{Payload: &guardianpb.Frame_InstalledApps{InstalledApps: apps}},
		})
	}
	for _, ack := range req.GetAcks() {
		h.handleClientFrame(client, sink, &guardianpb.Frame{
			Payload: &guardianpb.Frame_CommandAck{CommandAck: ack},
		})
	}
	return &guardianpb.SyncResponse{
		Hello:      &guardianpb.ServerHello{Accepted: true, ProtocolVersion: protocolVersion},
		LogControl: h.logControlFor(client.ID),
		ConfigPush: h.configPushFor(client.ID, hello.GetLastAppliedConfigVersion()),
		Commands:   h.drainCommands(client.ID),
	}, nil
}

// collectingSink satisfies guardianhub.ClientSink for the unary path, where there
// is no channel to write back on: anything the shared handler would have pushed is
// dropped, since the response carries the queued work instead.
type collectingSink struct{}

func (c *collectingSink) SendFrame(*guardianpb.Frame) error { return nil }

func (c *collectingSink) Close(string) {}

func recvWithTimeout(
	ctx context.Context,
	stream grpc.BidiStreamingServer[guardianpb.Frame, guardianpb.Frame],
	timeout time.Duration,
) (*guardianpb.Frame, error) {
	type result struct {
		frame *guardianpb.Frame
		err   error
	}
	done := make(chan result, 1)
	go func() {
		frame, err := stream.Recv()
		done <- result{frame: frame, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("hello timeout")
	case res := <-done:
		return res.frame, res.err
	}
}

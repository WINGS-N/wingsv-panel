// Package fedclient is the panel's gRPC client to the federation head.
//
// The head is a separate process with its own database on purpose: it holds
// hundreds of 1 Hz streams from third-party servers, and that surface has no
// business sharing a process with admin sessions.
package fedclient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/tokenaead"
)

// retryPolicy повторяет вызовы, упавшие на недоступности башки
const retryPolicy = `{
  "methodConfig": [{
    "name": [{"service": "wingsv.headpanel.v1.FederationHead"}],
    "waitForReady": true,
    "retryPolicy": {
      "MaxAttempts": 4,
      "InitialBackoff": "0.2s",
      "MaxBackoff": "2s",
      "BackoffMultiplier": 2.0,
      "RetryableStatusCodes": ["UNAVAILABLE"]
    }
  }]
}`

// ErrDisabled means no head is configured, which is the normal state until the
// operator turns the federation on.
var ErrDisabled = errors.New("fedclient: no federation head configured")

// Client dials the head over tokenaead. Both ends are ours and both are new, so
// this link derives with SHA-512 rather than the SHA-256 the deployed 3x-ui
// nodes and relays are stuck with.
type Client struct {
	endpoint string
	creds    credentials.TransportCredentials

	mu   sync.Mutex
	conn *grpc.ClientConn
}

// Option configures a Client.
type Option func(*Client)

// WithTransportCredentials pins the credentials, used by tests.
func WithTransportCredentials(c credentials.TransportCredentials) Option {
	return func(f *Client) { f.creds = c }
}

// New builds a client. An empty endpoint yields a client whose calls all report
// ErrDisabled, so callers need no separate nil check.
func New(endpoint, secret string, opts ...Option) *Client {
	c := &Client{endpoint: strings.TrimSpace(endpoint)}
	if secret != "" {
		c.creds = tokenaead.ClientVariant(secret, tokenaead.SHA512)
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Enabled reports whether a head is configured.
func (c *Client) Enabled() bool { return c.endpoint != "" && c.creds != nil }

func (c *Client) dial() (headpb.FederationHeadClient, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		// Башка переезжает между репликами, и на время переключения вызов
		// упирается в отказ соединения. Это не поломка федерации, а секунда
		// выката, поэтому повтор берёт на себя транспорт, а не каждый хендлер
		conn, err := grpc.NewClient(c.endpoint,
			grpc.WithTransportCredentials(c.creds),
			grpc.WithDefaultServiceConfig(retryPolicy))
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}
	return headpb.NewFederationHeadClient(c.conn), nil
}

// Close drops the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// PublicCounters is the aggregate the landing page shows.
func (c *Client) PublicCounters(ctx context.Context) (*headpb.PublicCounters, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.GetPublicCounters(ctx, &headpb.PublicCountersRequest{})
}

// ListNodes lists the fleet, or one donor's slice of it.
func (c *Client) ListNodes(ctx context.Context, donorID string) (*headpb.ListNodesResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.ListNodes(ctx, &headpb.ListNodesRequest{DonorId: donorID})
}

// DonorSummary is what one donor may see about their own contribution.
func (c *Client) DonorSummary(ctx context.Context, donorID string) (*headpb.DonorCounters, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.DonorSummary(ctx, &headpb.DonorSummaryRequest{DonorId: donorID})
}

// MintEnrollToken returns the string a donor pastes into the installer.
//
// uses is how many nodes may join on the one token. One is the ordinary case;
// more is a donor enrolling a fleet from a single Secret, which is what a
// Kubernetes DaemonSet does. The head caps it.
func (c *Client) MintEnrollToken(
	ctx context.Context,
	donorID string,
	ttl time.Duration,
	uses uint32,
	budgetGB uint32,
) (*headpb.MintEnrollTokenResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.MintEnrollToken(ctx, &headpb.MintEnrollTokenRequest{
		DonorId:    donorID,
		TtlSeconds: uint32(ttl.Seconds()),
		Uses:       uses,
		BudgetGb:   budgetGB,
	})
}

// EnsureUser gives a free user nodes, or returns what they already have.
// Idempotent, so it is safe to call on every login without moving anybody.
func (c *Client) EnsureUser(ctx context.Context, userID string) (*headpb.UserAllocation, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.EnsureUser(ctx, &headpb.EnsureUserRequest{UserId: userID})
}

// RevokeUser takes a user off every node.
func (c *Client) RevokeUser(ctx context.Context, userID string) error {
	client, err := c.dial()
	if err != nil {
		return err
	}
	_, err = client.RevokeUser(ctx, &headpb.RevokeUserRequest{UserId: userID})
	return err
}

// SetNodeState is the manual override behind automatic rotation.
func (c *Client) SetNodeState(ctx context.Context, nodeID, state, reason string) error {
	client, err := c.dial()
	if err != nil {
		return err
	}
	_, err = client.SetNodeState(ctx, &headpb.SetNodeStateRequest{
		NodeId: nodeID, State: state, Reason: reason,
	})
	return err
}

// SetNodeBudget меняет обещанный на месяц бюджет и возвращает то, на чём
// остановилась башка
func (c *Client) SetNodeBudget(ctx context.Context, nodeID string, bytes uint64) (uint64, error) {
	client, err := c.dial()
	if err != nil {
		return 0, err
	}
	got, err := client.SetNodeBudget(ctx, &headpb.SetNodeBudgetRequest{
		NodeId: nodeID, DeclaredBudgetBytes: bytes,
	})
	if err != nil {
		return 0, err
	}
	return got.GetDeclaredBudgetBytes(), nil
}

// DonorHistory - вклад донора по месяцам
func (c *Client) DonorHistory(ctx context.Context, donorID string, months uint32) ([]*headpb.DonorMonth, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	got, err := client.DonorHistory(ctx, &headpb.DonorHistoryRequest{DonorId: donorID, Months: months})
	if err != nil {
		return nil, err
	}
	return got.GetMonths(), nil
}

// ProbeReports - то, что намеряли точки наблюдения
func (c *Client) ProbeReports(ctx context.Context, limit, offset uint32) (*headpb.ProbeReportsResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.ProbeReports(ctx, &headpb.ProbeReportsRequest{Limit: limit, Offset: offset})
}

// OracleOverview - текущее состояние судьи
func (c *Client) OracleOverview(ctx context.Context, limit, offset uint32, subjectIDs ...string) (*headpb.OracleOverviewResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.OracleOverview(ctx, &headpb.OracleOverviewRequest{
		Limit: limit, Offset: offset, SubjectIds: subjectIDs,
	})
}

// OracleNodes отдаёт доверие к нодам. Судим обе стороны: врать о трафике ноде
// выгоднее всех, а до сих пор она считалась честной по умолчанию
func (c *Client) OracleNodes(ctx context.Context, limit, offset uint32) (*headpb.OracleNodesResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.OracleNodes(ctx, &headpb.OracleNodesRequest{Limit: limit, Offset: offset})
}

// RunProbes просит точки наблюдения замерить прямо сейчас
func (c *Client) RunProbes(ctx context.Context) (uint32, error) {
	client, err := c.dial()
	if err != nil {
		return 0, err
	}
	got, err := client.RunProbes(ctx, &headpb.RunProbesRequest{})
	if err != nil {
		return 0, err
	}
	return got.GetProbes(), nil
}

// OracleSubject - вердикт и сырые сигналы по одному профилю, страницами: у
// активного участника хвост доменов уходит в тысячи строк
func (c *Client) OracleSubject(ctx context.Context, req *headpb.OracleSubjectRequest) (*headpb.OracleSubjectResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.OracleSubject(ctx, req)
}

// PayoutStatement - что донору начислено и куда ему платить
func (c *Client) PayoutStatement(ctx context.Context, donorID string, limit uint32) (*headpb.PayoutStatementResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.PayoutStatement(ctx, &headpb.PayoutStatementRequest{DonorId: donorID, Limit: limit})
}

// SetPayoutAddress записывает кошелёк донора
func (c *Client) SetPayoutAddress(ctx context.Context, donorID, address string) error {
	client, err := c.dial()
	if err != nil {
		return err
	}
	_, err = client.SetPayoutAddress(ctx, &headpb.SetPayoutAddressRequest{DonorId: donorID, Address: address})
	return err
}

// Epochs - закрытые расчётные периоды целиком, это уже владельцу площадки
func (c *Client) Epochs(ctx context.Context, limit uint32) (*headpb.EpochsResponse, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.Epochs(ctx, &headpb.EpochsRequest{Limit: limit})
}

// ReportInviteTree отдаёт башке карту приглашений.
//
// Дерево живёт тут, трафик у башки, и без карты нода, возящая трафик своим же
// приглашённым ради выплаты, неотличима от честной
func (c *Client) ReportInviteTree(ctx context.Context, subjects []*headpb.SubjectAncestry) (uint32, error) {
	client, err := c.dial()
	if err != nil {
		return 0, err
	}
	got, err := client.ReportInviteTree(ctx, &headpb.ReportInviteTreeRequest{Subjects: subjects})
	if err != nil {
		return 0, err
	}
	return got.GetSubjects(), nil
}

// ReportDonation засчитывает занос и возвращает, на сколько очков он греет
// доверие. Нулевая сумма означает "просто скажи текущий кредит"
func (c *Client) ReportDonation(ctx context.Context, subjectID string, amountMicro uint64, atUnix int64, reference string) (float64, error) {
	client, err := c.dial()
	if err != nil {
		return 0, err
	}
	got, err := client.ReportDonation(ctx, &headpb.ReportDonationRequest{
		SubjectId: subjectID, AmountMicro: amountMicro, AtUnix: atUnix, Reference: reference,
	})
	if err != nil {
		return 0, err
	}
	return got.GetCredit(), nil
}

// retryDelay is how long the live loop waits before re-dialing a head that is
// down. Long enough not to hammer it, short enough that the counter comes back
// on its own after a head restart.
const retryDelay = 5 * time.Second

// StreamGlobal keeps a subscription open and calls onUpdate for every frame,
// re-dialing until ctx ends. A head that is down must not take the panel with
// it, so a failure here is a pause and never an error the caller has to handle.
func (c *Client) StreamGlobal(ctx context.Context, onUpdate func(*headpb.LiveUpdate)) {
	if !c.Enabled() {
		return
	}
	for ctx.Err() == nil {
		if err := c.streamOnce(ctx, onUpdate); err != nil && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
		}
	}
}

// StreamDonor - тот же цикл, суженный до одного донора. Показывать ему цифры
// всего флота нельзя: это другой, больший набор, и читать его как свой неверно
func (c *Client) StreamDonor(ctx context.Context, donorID string, onUpdate func(*headpb.LiveUpdate)) {
	if !c.Enabled() {
		return
	}
	for ctx.Err() == nil {
		if err := c.streamScoped(ctx, &headpb.LiveSubscribe{
			Scope: headpb.LiveScope_LIVE_SCOPE_DONOR, DonorId: donorID,
		}, onUpdate); err != nil && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
		}
	}
}

func (c *Client) streamOnce(ctx context.Context, onUpdate func(*headpb.LiveUpdate)) error {
	return c.streamScoped(ctx, &headpb.LiveSubscribe{Scope: headpb.LiveScope_LIVE_SCOPE_GLOBAL}, onUpdate)
}

func (c *Client) streamScoped(ctx context.Context, sub *headpb.LiveSubscribe, onUpdate func(*headpb.LiveUpdate)) error {
	client, err := c.dial()
	if err != nil {
		return err
	}
	stream, err := client.StreamLive(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(sub); err != nil {
		return err
	}
	for {
		update, err := stream.Recv()
		if err != nil {
			return err
		}
		onUpdate(update)
	}
}

// FleetSettings is what the fleet currently serves
func (c *Client) FleetSettings(ctx context.Context) (*headpb.FleetSettings, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.GetFleetSettings(ctx, &headpb.FleetSettingsRequest{})
}

// SetFleetSettings applies the operator's choice to every node
func (c *Client) SetFleetSettings(ctx context.Context, in *headpb.FleetSettings) (*headpb.FleetSettings, error) {
	client, err := c.dial()
	if err != nil {
		return nil, err
	}
	return client.SetFleetSettings(ctx, in)
}

// RestartComponent bounces xray or the relay. An empty nodeID means the fleet.
func (c *Client) RestartComponent(ctx context.Context, component, nodeID string) (uint32, error) {
	client, err := c.dial()
	if err != nil {
		return 0, err
	}
	resp, err := client.RestartComponent(ctx, &headpb.RestartComponentRequest{
		Component: component, NodeId: nodeID,
	})
	if err != nil {
		return 0, err
	}
	return resp.GetNodes(), nil
}

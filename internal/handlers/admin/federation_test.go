package admin

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/fedclient"
	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

type fakeHead struct {
	headpb.UnimplementedFederationHeadServer
	nodesByDonor map[string][]*headpb.NodeSummary
	stateCalls   []string
	budgetCalls  []string
}

func (f *fakeHead) DonorSummary(_ context.Context, req *headpb.DonorSummaryRequest) (*headpb.DonorCounters, error) {
	nodes := f.nodesByDonor[req.GetDonorId()]
	return &headpb.DonorCounters{
		DonorId: req.GetDonorId(),
		Nodes:   uint32(len(nodes)),
		UpBytes: 1000, DownBytes: 9000,
		DeclaredBudgetBytes: 1 << 40,
	}, nil
}

func (f *fakeHead) ListNodes(_ context.Context, req *headpb.ListNodesRequest) (*headpb.ListNodesResponse, error) {
	// Пустой донор значит весь флот - так отвечает и настоящая башка
	if req.GetDonorId() == "" {
		all := make([]*headpb.NodeSummary, 0, len(f.nodesByDonor))
		for _, nodes := range f.nodesByDonor {
			all = append(all, nodes...)
		}
		return &headpb.ListNodesResponse{Nodes: all}, nil
	}
	return &headpb.ListNodesResponse{Nodes: f.nodesByDonor[req.GetDonorId()]}, nil
}

func (f *fakeHead) SetNodeBudget(_ context.Context, req *headpb.SetNodeBudgetRequest) (*headpb.SetNodeBudgetResponse, error) {
	f.budgetCalls = append(f.budgetCalls, req.GetNodeId())
	return &headpb.SetNodeBudgetResponse{DeclaredBudgetBytes: req.GetDeclaredBudgetBytes()}, nil
}

func (f *fakeHead) MintEnrollToken(_ context.Context, req *headpb.MintEnrollTokenRequest) (*headpb.MintEnrollTokenResponse, error) {
	return &headpb.MintEnrollTokenResponse{
		EnrollToken:    "fleet." + req.GetDonorId(),
		ExpiresUnix:    1 << 40,
		InstallCommand: "curl -fsSL https://fed.example/fed/join.sh | sh -s -- fleet." + req.GetDonorId() + " 500",
	}, nil
}

func (f *fakeHead) SetNodeState(_ context.Context, req *headpb.SetNodeStateRequest) (*headpb.SetNodeStateResponse, error) {
	f.stateCalls = append(f.stateCalls, req.GetNodeId())
	return &headpb.SetNodeStateResponse{}, nil
}

func newFedHandler(t *testing.T, head *fakeHead, on bool) *Handler {
	t.Helper()
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "fed.db")})
	if err != nil {
		t.Fatal(err)
	}
	if on {
		if err := st.SetPlatformSetting(storage.SettingFederationEnabled, "1"); err != nil {
			t.Fatal(err)
		}
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	headpb.RegisterFederationHeadServer(gs, head)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	return &Handler{
		cfg:   config.Config{PublicBaseURL: "https://v.wingsnet.org"},
		store: st,
		fed: fedclient.New(lis.Addr().String(), "secret",
			fedclient.WithTransportCredentials(insecure.NewCredentials())),
	}
}

func admin7() storage.Admin { return storage.Admin{ID: 7} }

// The federation is off until an operator turns it on, and then the page is not
// rendered at all rather than rendered empty
func TestSummaryReportsTheFederationOff(t *testing.T) {
	h := newFedHandler(t, &fakeHead{}, false)
	rec := httptest.NewRecorder()
	h.handleFederationSummary(rec, httptest.NewRequest(http.MethodGet, "/api/admin/federation/summary", nil), admin7())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got federationSummaryView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Error("the federation reported itself on without anyone turning it on")
	}
}

// A donor sees their own machines and nothing about who is using them
func TestSummaryCarriesAggregatesOnly(t *testing.T) {
	head := &fakeHead{nodesByDonor: map[string][]*headpb.NodeSummary{
		"admin-7": {{NodeId: "n1", Hostname: "box", State: "active", Online: true,
			Live: &headpb.NodeCounters{UpRateBps: 100, Sessions: 3}}},
	}}
	h := newFedHandler(t, head, true)
	rec := httptest.NewRecorder()
	h.handleFederationSummary(rec, httptest.NewRequest(http.MethodGet, "/api/admin/federation/summary", nil), admin7())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"profile", "email", "uuid", "client", "subscription"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("donor payload mentions %q: %s", forbidden, body)
		}
	}
	var got federationSummaryView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || len(got.NodeList) != 1 || got.NodeList[0].Sessions != 3 {
		t.Errorf("summary = %+v", got)
	}
}

// The head does not check who owns a node, so a panel that skipped this would let
// any admin park anyone else's machine
func TestANodeBelongingToSomebodyElseCannotBeParked(t *testing.T) {
	head := &fakeHead{nodesByDonor: map[string][]*headpb.NodeSummary{
		"admin-7": {{NodeId: "mine"}},
		"admin-9": {{NodeId: "theirs"}},
	}}
	h := newFedHandler(t, head, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/federation/nodes/theirs/state",
		strings.NewReader(`{"state":"parked"}`))
	h.handleFederationNodeState(rec, req, admin7())
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for somebody else's node", rec.Code)
	}
	if len(head.stateCalls) != 0 {
		t.Errorf("the head was asked to change %v", head.stateCalls)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/federation/nodes/mine/state",
		strings.NewReader(`{"state":"parked","reason":"maintenance"}`))
	h.handleFederationNodeState(rec, req, admin7())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(head.stateCalls) != 1 || head.stateCalls[0] != "mine" {
		t.Errorf("state calls = %v", head.stateCalls)
	}
}

// The command a donor pastes has to name the panel they are looking at
func TestEnrollTokenComesWithTheInstallCommand(t *testing.T) {
	h := newFedHandler(t, &fakeHead{}, true)
	rec := httptest.NewRecorder()
	h.handleFederationEnrollToken(rec,
		httptest.NewRequest(http.MethodPost, "/api/admin/federation/enroll-token", strings.NewReader(`{}`)), admin7())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got enrollTokenView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Token != "fleet.admin-7" {
		t.Errorf("token = %q", got.Token)
	}
	// The head builds it, so the panel must pass it through rather than guess
	if !strings.HasPrefix(got.Command, "curl -fsSL https://fed.example/fed/join.sh") ||
		!strings.Contains(got.Command, got.Token) {
		t.Errorf("command = %q", got.Command)
	}
}

// Everything behind the toggle stays shut, not merely hidden in the UI
func TestWritePathsRefuseWhileTheFederationIsOff(t *testing.T) {
	h := newFedHandler(t, &fakeHead{}, false)
	for _, tc := range []struct {
		name string
		call func(*httptest.ResponseRecorder)
	}{
		{"mint", func(rec *httptest.ResponseRecorder) {
			h.handleFederationEnrollToken(rec,
				httptest.NewRequest(http.MethodPost, "/api/admin/federation/enroll-token", strings.NewReader(`{}`)), admin7())
		}},
		{"set state", func(rec *httptest.ResponseRecorder) {
			h.handleFederationNodeState(rec,
				httptest.NewRequest(http.MethodPost, "/api/admin/federation/nodes/n1/state", strings.NewReader(`{}`)), admin7())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 while the federation is off", rec.Code)
			}
		})
	}
}

// Владелец видит весь флот, и править он должен всё, что видит: иначе кнопка
// есть, а в ответ всегда "нет такой ноды"
func TestOwnerEditsAnyNodeBudget(t *testing.T) {
	head := &fakeHead{nodesByDonor: map[string][]*headpb.NodeSummary{
		"admin-7": {{NodeId: "mine"}},
		"admin-9": {{NodeId: "theirs"}},
	}}
	h := newFedHandler(t, head, true)

	owner := admin7()
	owner.Role = storage.RoleOwner
	rec := httptest.NewRecorder()
	h.handleFederationNodeState(
		rec,
		httptest.NewRequest(
			http.MethodPut,
			"/api/app/federation/nodes/theirs/budget",
			strings.NewReader(`{"declared_budget_bytes":1073741824}`),
		),
		owner,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("владельцу не дали править чужую ноду: %d %s", rec.Code, rec.Body.String())
	}

	// Обычному админу чужая нода по-прежнему недоступна
	rec = httptest.NewRecorder()
	h.handleFederationNodeState(
		rec,
		httptest.NewRequest(
			http.MethodPut,
			"/api/app/federation/nodes/theirs/budget",
			strings.NewReader(`{"declared_budget_bytes":1073741824}`),
		),
		admin7(),
	)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужую ноду отдали обычному админу: %d", rec.Code)
	}
}

package storage

import (
	"errors"
	"testing"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func TestPanelModeDefaultAndSet(t *testing.T) {
	st := openTemp(t)
	mode, err := st.GetPanelMode()
	if err != nil {
		t.Fatalf("GetPanelMode: %v", err)
	}
	if mode != PanelModeControlPanel {
		t.Fatalf("default mode = %q, want control_panel", mode)
	}
	if err := st.SetPanelMode(PanelModeWGAWG); err != nil {
		t.Fatalf("SetPanelMode: %v", err)
	}
	if err := st.SetPanelMode(PanelModeXUIAPI); err != nil {
		t.Fatalf("SetPanelMode overwrite: %v", err)
	}
	mode, err = st.GetPanelMode()
	if err != nil {
		t.Fatalf("GetPanelMode: %v", err)
	}
	if mode != PanelModeXUIAPI {
		t.Fatalf("mode = %q, want xui_api", mode)
	}
}

func TestServerNodeCRUD(t *testing.T) {
	st := openTemp(t)
	node, err := st.CreateServerNode(dbmodel.ServerNode{
		ID: "n1", Kind: ServerNodeVKTurnProxy, Name: "relay-a", GRPCEndpoint: "10.0.0.2:2055", CAPin: []byte("pin"),
	})
	if err != nil {
		t.Fatalf("CreateServerNode: %v", err)
	}
	if node.Status != "unknown" || node.CreatedAtUnix == 0 {
		t.Fatalf("defaults not applied: %+v", node)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n2", Kind: ServerNodeXUI, Name: "panel-b"}); err != nil {
		t.Fatalf("CreateServerNode n2: %v", err)
	}

	relays, err := st.ListServerNodes(ServerNodeVKTurnProxy)
	if err != nil {
		t.Fatalf("ListServerNodes: %v", err)
	}
	if len(relays) != 1 || relays[0].ID != "n1" {
		t.Fatalf("filtered list = %+v, want only n1", relays)
	}
	all, err := st.ListServerNodes("")
	if err != nil {
		t.Fatalf("ListServerNodes all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all list len = %d, want 2", len(all))
	}

	if err := st.UpdateServerNodeStatus("n1", "online", 999); err != nil {
		t.Fatalf("UpdateServerNodeStatus: %v", err)
	}
	got, err := st.GetServerNode("n1")
	if err != nil {
		t.Fatalf("GetServerNode: %v", err)
	}
	if got.Status != "online" || got.LastSeenAt != 999 || string(got.CAPin) != "pin" {
		t.Fatalf("node not updated: %+v", got)
	}

	if err := st.DeleteServerNode("n1"); err != nil {
		t.Fatalf("DeleteServerNode: %v", err)
	}
	if _, err := st.GetServerNode("n1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetServerNode after delete = %v, want ErrNotFound", err)
	}
	if err := st.DeleteServerNode("n1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteServerNode missing = %v, want ErrNotFound", err)
	}
}

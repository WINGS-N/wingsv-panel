package admin

import (
	"path/filepath"
	"testing"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/storage"
)

// managedTurnForTest returns a Turn carrying a panel-managed VK-TURN profile,
// the only shape applyAdminVKLinks embeds links into.
func managedTurnForTest() *wingsvpb.Turn {
	return applyManagedTurnProfile(nil, "c1", "Dev", []byte("tok"), "relay.example:56000")
}

func TestApplyAdminVKLinksCapsAndGates(t *testing.T) {
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	admin, err := st.CreateAdmin("adm", "hash", false, "admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := st.SetAdminVKLinks(admin.ID, []string{"https://vk.com/a", "https://vk.com/b", "https://vk.com/c"}); err != nil {
		t.Fatalf("set links: %v", err)
	}
	h := &Handler{store: st}

	// Enrollment link / QR: one link, and one only. The rest of the pool arrives
	// on the DTLS provision that the same enrolment performs, so a denser barcode
	// would buy nothing.
	linkTurn := managedTurnForTest()
	h.applyAdminVKLinks(linkTurn, admin.ID, maxEnrollmentVKLinks)
	if got := len(linkTurn.GetLinks()); got != 1 {
		t.Fatalf("enrollment link should carry exactly 1 VK link, got %d (%v)", got, linkTurn.GetLinks())
	}

	// Stored config: full pool (WS source of truth).
	storedTurn := managedTurnForTest()
	h.applyAdminVKLinks(storedTurn, admin.ID, 0)
	if got := len(storedTurn.GetLinks()); got != 3 {
		t.Fatalf("stored config should carry the full pool of 3, got %d", got)
	}

	// A non-VK-TURN turn (no managed profile) gets no links.
	plainTurn := &wingsvpb.Turn{}
	h.applyAdminVKLinks(plainTurn, admin.ID, 0)
	if len(plainTurn.GetLinks()) != 0 {
		t.Fatalf("a non-VK-TURN turn must not get VK links, got %v", plainTurn.GetLinks())
	}
}

package admin

import "testing"

// An open redirect on a login callback is a phishing primitive: somebody lands
// on our domain, signs in for real, and is bounced to an attacker's page
func TestReturnToStaysInsideThePanel(t *testing.T) {
	for _, bad := range []string{
		"https://evil.example/steal",
		"//evil.example/steal",
		"http://evil.example",
		"",
		"   ",
		"javascript:alert(1)",
	} {
		if got := safeReturnTo(bad); got != "" {
			t.Errorf("safeReturnTo(%q) = %q, want it dropped", bad, got)
		}
	}
	for _, good := range []string{"/admin/clients", "/owner/overview"} {
		if got := safeReturnTo(good); got != good {
			t.Errorf("safeReturnTo(%q) = %q", good, got)
		}
	}
	if got := redirectTarget("//evil.example", "/admin/clients"); got != "/admin/clients" {
		t.Errorf("redirectTarget fell through to %q", got)
	}
}

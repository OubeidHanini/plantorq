package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/identity"
)

const secret = "test-secret-test-secret-test-secret!"

var alice = identity.Principal{UserID: 7, TenantID: 3, TenantSlug: "acme", Role: identity.RoleOperator}

func TestIssueAndVerify(t *testing.T) {
	tokens, err := New(secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, exp, err := tokens.Issue(alice)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) < 59*time.Minute {
		t.Fatalf("expiry too short: %s", exp)
	}
	got, err := tokens.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if got != alice {
		t.Fatalf("principal not preserved: %+v", got)
	}
}

func TestVerifyRejects(t *testing.T) {
	tokens, _ := New(secret, time.Hour)
	other, _ := New(strings.Repeat("x", 32), time.Hour)

	past, _ := New(secret, time.Hour)
	past.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expired, _, _ := past.Issue(alice)

	admin := alice
	admin.Role = identity.RoleAdmin
	forged, _, _ := other.Issue(admin)

	noTenant, _, _ := tokens.Issue(identity.Principal{UserID: 1, Role: identity.RoleAdmin})

	cases := map[string]string{"expired": expired, "wrong key": forged, "garbage": "a.b.c", "no tenant": noTenant}
	for name, tok := range cases {
		if _, err := tokens.Verify(tok); err == nil {
			t.Errorf("%s token accepted", name)
		}
	}
}

func TestShortSecretRejected(t *testing.T) {
	if _, err := New("short", time.Hour); err == nil {
		t.Fatal("short secret accepted")
	}
}

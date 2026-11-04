package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

func uniqueProfile(t *testing.T, first string) identity.Profile {
	t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	return identity.Profile{
		ProviderUserID: "user_test_" + id,
		Email:          "test-" + id + "@example.com",
		FirstName:      first,
	}
}

func cleanup(t *testing.T, db *bun.DB, providerIDs ...string) {
	t.Cleanup(func() {
		for _, id := range providerIDs {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE workos_user_id = ?`, id)
		}
	})
}

func count(t *testing.T, db *bun.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSignInCreatesPersonalWorkspace(t *testing.T) {
	db := testdb.Open(t)
	svc := NewService(db)
	p := uniqueProfile(t, "Ada")
	cleanup(t, db, p.ProviderUserID)

	u, err := svc.SignIn(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == "" || u.Email != p.Email || u.FirstName != "Ada" {
		t.Fatalf("user = %+v", u)
	}

	_, ms, err := svc.Me(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Fatalf("memberships = %+v", ms)
	}
	m := ms[0]
	if !m.Personal || m.Role != RoleOwner || m.OrganisationName != "Ada’s studio" {
		t.Errorf("membership = %+v", m)
	}
	if len(m.Workspaces) != 1 || m.Workspaces[0].Name != "Main" {
		t.Errorf("workspaces = %+v", m.Workspaces)
	}
}

func TestSignInIsIdempotentAndUpdatesProfile(t *testing.T) {
	db := testdb.Open(t)
	svc := NewService(db)
	p := uniqueProfile(t, "Ada")
	cleanup(t, db, p.ProviderUserID)

	first, err := svc.SignIn(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	p.Email = "renamed-" + p.Email
	p.FirstName = "Adaeze"
	second, err := svc.SignIn(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Email != p.Email || second.FirstName != "Adaeze" {
		t.Errorf("second sign-in = %+v (first %+v)", second, first)
	}
	if n := count(t, db, `SELECT count(*) FROM organisations WHERE personal_owner_user_id = ?`, first.ID); n != 1 {
		t.Errorf("personal organisations = %d, want 1", n)
	}
	if n := count(t, db, `SELECT count(*) FROM memberships WHERE user_id = ?`, first.ID); n != 1 {
		t.Errorf("memberships = %d, want 1", n)
	}
}

func TestConcurrentFirstSignInCreatesOneOrganisation(t *testing.T) {
	db := testdb.Open(t)
	svc := NewService(db)
	p := uniqueProfile(t, "")
	cleanup(t, db, p.ProviderUserID)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.SignIn(context.Background(), p); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent sign-in: %v", err)
	}
	if n := count(t, db, `
		SELECT count(*) FROM organisations o JOIN users u ON u.id = o.personal_owner_user_id
		WHERE u.workos_user_id = ?`, p.ProviderUserID); n != 1 {
		t.Errorf("personal organisations = %d, want 1", n)
	}
	if n := count(t, db, `SELECT count(*) FROM users WHERE workos_user_id = ?`, p.ProviderUserID); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

// Tenant isolation: Me never returns organisations the user doesn't belong to.
func TestMeOnlyReturnsOwnOrganisations(t *testing.T) {
	db := testdb.Open(t)
	svc := NewService(db)
	a, b := uniqueProfile(t, "Ana"), uniqueProfile(t, "Ben")
	cleanup(t, db, a.ProviderUserID, b.ProviderUserID)

	ua, err := svc.SignIn(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	ub, err := svc.SignIn(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	_, msA, _ := svc.Me(context.Background(), ua.ID)
	_, msB, _ := svc.Me(context.Background(), ub.ID)
	if len(msA) != 1 || len(msB) != 1 || msA[0].OrganisationID == msB[0].OrganisationID {
		t.Fatalf("A=%+v B=%+v", msA, msB)
	}
	if msA[0].OrganisationName != "Ana’s studio" || msB[0].OrganisationName != "Ben’s studio" {
		t.Errorf("A=%q B=%q", msA[0].OrganisationName, msB[0].OrganisationName)
	}
}

func TestMeUnknownUser(t *testing.T) {
	db := testdb.Open(t)
	_, _, err := NewService(db).Me(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != ErrUserNotFound {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestPersonalOrgName(t *testing.T) {
	for in, want := range map[string]string{"": "My studio", "  ": "My studio", "Ada": "Ada’s studio"} {
		if got := personalOrgName(in); got != want {
			t.Errorf("personalOrgName(%q) = %q, want %q", in, got, want)
		}
	}
}

package account

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

type world struct {
	t   *testing.T
	db  *bun.DB
	svc *Service
	ctx context.Context
}

func newWorld(t *testing.T) *world {
	db := testdb.Open(t)
	return &world{t: t, db: db, svc: NewService(db), ctx: context.Background()}
}

// user signs in a fresh, verified user and returns their ID and profile.
func (w *world) user(first string) (string, identity.Profile) {
	w.t.Helper()
	p := uniqueProfile(w.t, first)
	p.EmailVerified = true
	cleanup(w.t, w.db, p.ProviderUserID)
	u, err := w.svc.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	return u.ID, p
}

// team creates a team organisation owned by owner.
func (w *world) team(owner, name string) string {
	w.t.Helper()
	org, err := w.svc.CreateOrganisation(w.ctx, owner, name)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() {
		_, _ = w.db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org.ID)
	})
	return org.ID
}

// join invites and accepts, making userID a member with role.
func (w *world) join(orgID, inviter, userID, email, role string) {
	w.t.Helper()
	inv, err := w.svc.CreateInvitation(w.ctx, inviter, orgID, email, role)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.svc.AcceptInvitation(w.ctx, userID, inv.ID); err != nil {
		w.t.Fatal(err)
	}
}

func wantErr(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
}

func TestCreateOrganisation(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	orgID := w.team(ana, "  Night Shift Studio ")

	org, err := w.svc.Organisation(w.ctx, ana, orgID)
	if err != nil {
		t.Fatal(err)
	}
	if org.Name != "Night Shift Studio" || org.Personal || org.Role != authz.RoleOwner || len(org.Permissions) == 0 {
		t.Errorf("org = %+v", org)
	}
	_, ms, _ := w.svc.Me(w.ctx, ana)
	if len(ms) != 2 || !ms[0].Personal || ms[1].Workspaces[0].Name != "Main" {
		t.Errorf("memberships = %+v", ms)
	}
	var n int
	_ = w.db.QueryRowContext(w.ctx, `SELECT count(*) FROM audit_events WHERE organisation_id = ? AND action = 'organisation.created'`, orgID).Scan(&n)
	if n != 1 {
		t.Errorf("audit events = %d", n)
	}

	for _, bad := range []string{"", "   ", string(make([]rune, 121))} {
		if _, err := w.svc.CreateOrganisation(w.ctx, ana, bad); err == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
}

// Tenant isolation: outsiders get ErrNotFound for everything.
func TestOutsidersCannotSeeOrTouchAnOrganisation(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	eve, _ := w.user("Eve")
	orgID := w.team(ana, "Private Co")

	_, err := w.svc.Organisation(w.ctx, eve, orgID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Members(w.ctx, eve, orgID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.RenameOrganisation(w.ctx, eve, orgID, "Pwned")
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.CreateInvitation(w.ctx, eve, orgID, "x@example.com", "owner")
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Invitations(w.ctx, eve, orgID)
	wantErr(t, err, ErrNotFound)
	wantErr(t, w.svc.ChangeRole(w.ctx, eve, orgID, ana, "viewer"), ErrNotFound)
	wantErr(t, w.svc.RemoveMember(w.ctx, eve, orgID, ana), ErrNotFound)
	_, err = w.svc.Organisation(w.ctx, eve, "not-a-uuid")
	wantErr(t, err, ErrNotFound)

	org, _ := w.svc.Organisation(w.ctx, ana, orgID)
	if org.Name != "Private Co" {
		t.Error("outsider changed the organisation")
	}
}

func TestInvitationFlowAndRoles(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	ben, benP := w.user("Ben")
	orgID := w.team(ana, "Studio")

	inv, err := w.svc.CreateInvitation(w.ctx, ana, orgID, "  "+benP.Email+" ", authz.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.CreateInvitation(w.ctx, ana, orgID, benP.Email, authz.RoleEditor)
	wantErr(t, err, ErrInvitationExists)

	pending, _ := w.svc.PendingInvitations(w.ctx, ben)
	if len(pending) != 1 || pending[0].OrganisationName != "Studio" || pending[0].InvitedByName == "" {
		t.Fatalf("pending = %+v", pending)
	}
	org, err := w.svc.AcceptInvitation(w.ctx, ben, inv.ID)
	if err != nil || org.Role != authz.RoleViewer {
		t.Fatalf("accept = %+v, %v", org, err)
	}
	_, err = w.svc.AcceptInvitation(w.ctx, ben, inv.ID)
	wantErr(t, err, ErrNotFound) // already used

	_, err = w.svc.CreateInvitation(w.ctx, ana, orgID, benP.Email, authz.RoleViewer)
	wantErr(t, err, ErrAlreadyMember)

	// Viewers can see members but not manage them.
	members, err := w.svc.Members(w.ctx, ben, orgID)
	if err != nil || len(members) != 2 || members[0].Role != authz.RoleOwner {
		t.Fatalf("members = %+v, %v", members, err)
	}
	_, err = w.svc.CreateInvitation(w.ctx, ben, orgID, "z@example.com", authz.RoleViewer)
	wantErr(t, err, ErrForbidden)
	wantErr(t, w.svc.ChangeRole(w.ctx, ben, orgID, ben, authz.RoleAdmin), ErrForbidden)
	_, err = w.svc.RenameOrganisation(w.ctx, ben, orgID, "Mine now")
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Invitations(w.ctx, ben, orgID)
	wantErr(t, err, ErrForbidden)

	// The owner promotes Ben to admin.
	if err := w.svc.ChangeRole(w.ctx, ana, orgID, ben, authz.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	// Admins manage members but cannot touch owners or create owners.
	wantErr(t, w.svc.ChangeRole(w.ctx, ben, orgID, ana, authz.RoleViewer), ErrForbidden)
	wantErr(t, w.svc.ChangeRole(w.ctx, ben, orgID, ben, authz.RoleOwner), ErrForbidden)
	wantErr(t, w.svc.RemoveMember(w.ctx, ben, orgID, ana), ErrForbidden)
	_, err = w.svc.CreateInvitation(w.ctx, ben, orgID, "o@example.com", authz.RoleOwner)
	wantErr(t, err, ErrForbidden)
	if _, err := w.svc.CreateInvitation(w.ctx, ben, orgID, "e@example.com", authz.RoleEditor); err != nil {
		t.Errorf("admin invite editor: %v", err)
	}

	// Members can leave.
	if err := w.svc.RemoveMember(w.ctx, ben, orgID, ben); err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.Organisation(w.ctx, ben, orgID)
	wantErr(t, err, ErrNotFound) // removed members lose access immediately
}

func TestInvitationsAreBoundToTheVerifiedEmail(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	_, benP := w.user("Ben")
	eve, _ := w.user("Eve")
	orgID := w.team(ana, "Studio")

	inv, err := w.svc.CreateInvitation(w.ctx, ana, orgID, benP.Email, authz.RoleEditor)
	if err != nil {
		t.Fatal(err)
	}
	// Someone else with the invitation ID cannot use it.
	_, err = w.svc.AcceptInvitation(w.ctx, eve, inv.ID)
	wantErr(t, err, ErrNotFound)
	if p, _ := w.svc.PendingInvitations(w.ctx, eve); len(p) != 0 {
		t.Error("invitation visible to the wrong user")
	}

	// The right email, unverified, cannot accept either.
	unverified := uniqueProfile(t, "Ben2")
	unverified.Email = "x" + benP.Email
	cleanup(t, w.db, unverified.ProviderUserID)
	u, _ := w.svc.SignIn(w.ctx, unverified)
	inv2, _ := w.svc.CreateInvitation(w.ctx, ana, orgID, unverified.Email, authz.RoleEditor)
	if p, _ := w.svc.PendingInvitations(w.ctx, u.ID); len(p) != 0 {
		t.Error("unverified email sees invitations")
	}
	_, err = w.svc.AcceptInvitation(w.ctx, u.ID, inv2.ID)
	wantErr(t, err, ErrEmailNotVerified)

	// Revoked invitations cannot be accepted.
	if err := w.svc.RevokeInvitation(w.ctx, ana, orgID, inv.ID); err != nil {
		t.Fatal(err)
	}
	ben := w.userIDFor(benP)
	_, err = w.svc.AcceptInvitation(w.ctx, ben, inv.ID)
	wantErr(t, err, ErrNotFound)
	wantErr(t, w.svc.RevokeInvitation(w.ctx, ana, orgID, inv.ID), ErrNotFound)
}

func (w *world) userIDFor(p identity.Profile) string {
	var id string
	if err := w.db.QueryRowContext(w.ctx, `SELECT id FROM users WHERE workos_user_id = ?`, p.ProviderUserID).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func TestDecline(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	ben, benP := w.user("Ben")
	orgID := w.team(ana, "Studio")
	inv, _ := w.svc.CreateInvitation(w.ctx, ana, orgID, benP.Email, authz.RoleViewer)
	if err := w.svc.DeclineInvitation(w.ctx, ben, inv.ID); err != nil {
		t.Fatal(err)
	}
	_, err := w.svc.Organisation(w.ctx, ben, orgID)
	wantErr(t, err, ErrNotFound)
	if p, _ := w.svc.PendingInvitations(w.ctx, ben); len(p) != 0 {
		t.Error("declined invitation still pending")
	}
}

func TestLastOwnerIsProtected(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	orgID := w.team(ana, "Studio")

	wantErr(t, w.svc.RemoveMember(w.ctx, ana, orgID, ana), ErrLastOwner)
	wantErr(t, w.svc.ChangeRole(w.ctx, ana, orgID, ana, authz.RoleAdmin), ErrLastOwner)

	// With two owners, one can step down, but not both at once.
	ben, benP := w.user("Ben")
	w.join(orgID, ana, ben, benP.Email, authz.RoleOwner)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{ana, ben} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = w.svc.RemoveMember(w.ctx, id, orgID, id)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !errors.Is(err, ErrLastOwner) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d owners left concurrently; want exactly 1 (errs %v)", ok, errs)
	}
	var owners int
	_ = w.db.QueryRowContext(w.ctx, `SELECT count(*) FROM memberships WHERE organisation_id = ? AND role = 'owner'`, orgID).Scan(&owners)
	if owners != 1 {
		t.Errorf("owners = %d, want 1", owners)
	}
}

func TestPersonalOrganisationsStayPersonal(t *testing.T) {
	w := newWorld(t)
	ana, _ := w.user("Ana")
	_, ms, _ := w.svc.Me(w.ctx, ana)
	personal := ms[0].OrganisationID

	_, err := w.svc.CreateInvitation(w.ctx, ana, personal, "friend@example.com", authz.RoleEditor)
	wantErr(t, err, ErrPersonalOrg)
	wantErr(t, w.svc.RemoveMember(w.ctx, ana, personal, ana), ErrPersonalOrg)
	if _, err := w.svc.RenameOrganisation(w.ctx, ana, personal, "Ana makes things"); err != nil {
		t.Errorf("renaming personal org: %v", err)
	}
}

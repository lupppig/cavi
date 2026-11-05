package project

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

type world struct {
	t        *testing.T
	db       *bun.DB
	accounts *account.Service
	svc      *Service
	ctx      context.Context
}

func newWorld(t *testing.T) *world {
	db := testdb.Open(t)
	return &world{t: t, db: db, accounts: account.NewService(db), svc: NewService(db), ctx: context.Background()}
}

type person struct{ id, email string }

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_proj_" + id, Email: "proj-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() {
		_, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID)
	})
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "projtest team")
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() {
		_, _ = w.db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org.ID)
	})
	return org.ID
}

func (w *world) join(orgID string, owner, p person, role string) {
	w.t.Helper()
	inv, err := w.accounts.CreateInvitation(w.ctx, owner.id, orgID, p.email, role)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.accounts.AcceptInvitation(w.ctx, p.id, inv.ID); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) create(actor person, orgID, name string) Project {
	w.t.Helper()
	p, err := w.svc.Create(w.ctx, actor.id, orgID, "", name, "")
	if err != nil {
		w.t.Fatal(err)
	}
	return p
}

func (w *world) count(q string, args ...any) int {
	w.t.Helper()
	var n int
	if err := w.db.QueryRowContext(w.ctx, q, args...).Scan(&n); err != nil {
		w.t.Fatal(err)
	}
	return n
}

func wantErr(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	var v *account.ValidationError
	if !errors.As(err, &v) || v.Field != field {
		t.Fatalf("err = %v, want validation error on %q", err, field)
	}
}

func str(s string) *string { return &s }

func personalOrg(w *world, p person) string {
	w.t.Helper()
	_, ms, err := w.accounts.Me(w.ctx, p.id)
	if err != nil {
		w.t.Fatal(err)
	}
	return ms[0].OrganisationID
}

func TestCreateInPersonalStudio(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := personalOrg(w, ana)

	p, err := w.svc.Create(w.ctx, ana.id, org, "", "  Tour teaser  ", " Three cuts for the spring tour. ")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Tour teaser" || p.Description != "Three cuts for the spring tour." || p.WorkspaceName != "Main" ||
		p.CreatedByID != ana.id || p.CreatedByName != "Ana" || p.OrganisationID != org {
		t.Fatalf("project = %+v", p)
	}
	for _, perm := range []authz.Permission{authz.ProjectRead, authz.ProjectUpdate, authz.ProjectDelete, authz.ProjectAccessManage} {
		if !has(p.Permissions, perm) {
			t.Errorf("owner missing %s", perm)
		}
	}
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id = ? AND action = 'project.created'`, p.ID); n != 1 {
		t.Errorf("audit events = %d", n)
	}
	got, err := w.svc.Get(w.ctx, ana.id, org, p.ID)
	if err != nil || got.ID != p.ID {
		t.Fatalf("get = %+v, %v", got, err)
	}
}

func TestValidation(t *testing.T) {
	w := newWorld(t)
	ana, eve := w.user("Ana"), w.user("Eve")
	org := personalOrg(w, ana)

	_, err := w.svc.Create(w.ctx, ana.id, org, "", "   ", "")
	wantField(t, err, "name")
	_, err = w.svc.Create(w.ctx, ana.id, org, "", strings.Repeat("x", 121), "")
	wantField(t, err, "name")
	_, err = w.svc.Create(w.ctx, ana.id, org, "", "ok", strings.Repeat("é", 2001))
	wantField(t, err, "description")
	_, err = w.svc.Create(w.ctx, ana.id, org, "not-a-uuid", "ok", "")
	wantField(t, err, "workspace_id")

	// A workspace belonging to another tenant is rejected like a missing one.
	_, eveMs, err := w.accounts.Me(w.ctx, eve.id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.Create(w.ctx, ana.id, org, eveMs[0].Workspaces[0].ID, "ok", "")
	wantField(t, err, "workspace_id")

	p := w.create(ana, org, "Real")
	_, err = w.svc.Update(w.ctx, ana.id, org, p.ID, Changes{})
	wantField(t, err, "name")
	_, err = w.svc.List(w.ctx, ana.id, org, ListOptions{Limit: MaxPageSize + 1})
	wantField(t, err, "limit")
	_, err = w.svc.List(w.ctx, ana.id, org, ListOptions{Cursor: "garbage!"})
	wantField(t, err, "cursor")
}

func TestTenantIsolation(t *testing.T) {
	w := newWorld(t)
	ana, eve := w.user("Ana"), w.user("Eve")
	anaOrg, eveOrg := personalOrg(w, ana), personalOrg(w, eve)
	p := w.create(ana, anaOrg, "Secret campaign")

	// Eve is not a member of Ana's organisation.
	_, err := w.svc.Get(w.ctx, eve.id, anaOrg, p.ID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.List(w.ctx, eve.id, anaOrg, ListOptions{})
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Update(w.ctx, eve.id, anaOrg, p.ID, Changes{Name: str("pwned")})
	wantErr(t, err, ErrNotFound)
	wantErr(t, w.svc.Delete(w.ctx, eve.id, anaOrg, p.ID), ErrNotFound)
	_, err = w.svc.Create(w.ctx, eve.id, anaOrg, "", "intruder", "")
	wantErr(t, err, ErrNotFound)

	// Using her own organisation ID with Ana's project ID doesn't help.
	_, err = w.svc.Get(w.ctx, eve.id, eveOrg, p.ID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Update(w.ctx, eve.id, eveOrg, p.ID, Changes{Name: str("pwned")})
	wantErr(t, err, ErrNotFound)
	wantErr(t, w.svc.Delete(w.ctx, eve.id, eveOrg, p.ID), ErrNotFound)
	page, err := w.svc.List(w.ctx, eve.id, eveOrg, ListOptions{})
	if err != nil || len(page.Projects) != 0 {
		t.Fatalf("eve's list = %+v, %v", page, err)
	}

	// The database refuses a project filed in another tenant's workspace.
	_, eveMs, _ := w.accounts.Me(w.ctx, eve.id)
	_, err = w.db.ExecContext(w.ctx, `INSERT INTO projects (organisation_id, workspace_id, name) VALUES (?, ?, 'x')`,
		anaOrg, eveMs[0].Workspaces[0].ID)
	if err == nil {
		t.Fatal("cross-tenant workspace insert succeeded")
	}

	got, err := w.svc.Get(w.ctx, ana.id, anaOrg, p.ID)
	if err != nil || got.Name != "Secret campaign" {
		t.Fatalf("project changed: %+v, %v", got, err)
	}
}

func TestRolePermissions(t *testing.T) {
	w := newWorld(t)
	ana, vic, cat, ed := w.user("Ana"), w.user("Vic"), w.user("Cat"), w.user("Ed")
	org := w.team(ana)
	w.join(org, ana, vic, authz.RoleViewer)
	w.join(org, ana, cat, authz.RoleCreator)
	w.join(org, ana, ed, authz.RoleEditor)
	anas := w.create(ana, org, "Ana's")

	// Viewers read but can't change anything.
	if _, err := w.svc.Get(w.ctx, vic.id, org, anas.ID); err != nil {
		t.Fatal(err)
	}
	_, err := w.svc.Create(w.ctx, vic.id, org, "", "nope", "")
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Update(w.ctx, vic.id, org, anas.ID, Changes{Name: str("nope")})
	wantErr(t, err, ErrForbidden)
	wantErr(t, w.svc.Delete(w.ctx, vic.id, org, anas.ID), ErrForbidden)

	// Creators edit their own projects only, and can't delete.
	cats := w.create(cat, org, "Cat's")
	if !has(cats.Permissions, authz.ProjectUpdate) || has(cats.Permissions, authz.ProjectDelete) {
		t.Errorf("creator perms on own = %v", cats.Permissions)
	}
	if _, err := w.svc.Update(w.ctx, cat.id, org, cats.ID, Changes{Description: str("mine")}); err != nil {
		t.Fatal(err)
	}
	asCat, _ := w.svc.Get(w.ctx, cat.id, org, anas.ID)
	if has(asCat.Permissions, authz.ProjectUpdate) {
		t.Error("creator can update someone else's project")
	}
	_, err = w.svc.Update(w.ctx, cat.id, org, anas.ID, Changes{Name: str("taken")})
	wantErr(t, err, ErrForbidden)
	wantErr(t, w.svc.Delete(w.ctx, cat.id, org, cats.ID), ErrForbidden)

	// Editors edit anyone's, but can't delete or share.
	upd, err := w.svc.Update(w.ctx, ed.id, org, cats.ID, Changes{Name: str("Cat's (edited)")})
	if err != nil || upd.Name != "Cat's (edited)" || upd.Description != "mine" {
		t.Fatalf("editor update = %+v, %v", upd, err)
	}
	wantErr(t, w.svc.Delete(w.ctx, ed.id, org, cats.ID), ErrForbidden)
	_, err = w.svc.Access(w.ctx, ed.id, org, cats.ID)
	wantErr(t, err, ErrForbidden)

	// Owners delete; the project is then gone for everyone.
	if err := w.svc.Delete(w.ctx, ana.id, org, cats.ID); err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.Get(w.ctx, cat.id, org, cats.ID)
	wantErr(t, err, ErrNotFound)
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id = ? AND action IN ('project.updated','project.deleted')`, cats.ID); n != 3 {
		t.Errorf("audit events = %d, want 3", n)
	}
}

func TestClientsSeeOnlySharedProjects(t *testing.T) {
	w := newWorld(t)
	ana, cli, other, vic := w.user("Ana"), w.user("Cli"), w.user("Ola"), w.user("Vic")
	org := w.team(ana)
	w.join(org, ana, cli, authz.RoleClient)
	w.join(org, ana, other, authz.RoleClient)
	w.join(org, ana, vic, authz.RoleViewer)
	shared, private := w.create(ana, org, "For the client"), w.create(ana, org, "Internal")

	page, err := w.svc.List(w.ctx, cli.id, org, ListOptions{})
	if err != nil || len(page.Projects) != 0 {
		t.Fatalf("client sees %+v, %v before sharing", page.Projects, err)
	}
	_, err = w.svc.Get(w.ctx, cli.id, org, shared.ID)
	wantErr(t, err, ErrNotFound)

	// Only clients can be granted access, only by managers and up, and
	// only to members of this organisation.
	wantField(t, w.svc.GrantAccess(w.ctx, ana.id, org, shared.ID, vic.id), "user_id")
	wantField(t, w.svc.GrantAccess(w.ctx, ana.id, org, shared.ID, w.user("Zed").id), "user_id")
	wantErr(t, w.svc.GrantAccess(w.ctx, vic.id, org, shared.ID, cli.id), ErrForbidden)

	if err := w.svc.GrantAccess(w.ctx, ana.id, org, shared.ID, cli.id); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.GrantAccess(w.ctx, ana.id, org, shared.ID, cli.id); err != nil {
		t.Fatalf("second grant: %v", err)
	}
	grants, err := w.svc.Access(w.ctx, ana.id, org, shared.ID)
	if err != nil || len(grants) != 1 || grants[0].UserID != cli.id {
		t.Fatalf("grants = %+v, %v", grants, err)
	}

	page, _ = w.svc.List(w.ctx, cli.id, org, ListOptions{})
	if len(page.Projects) != 1 || page.Projects[0].ID != shared.ID {
		t.Fatalf("client list = %+v", page.Projects)
	}
	got, err := w.svc.Get(w.ctx, cli.id, org, shared.ID)
	if err != nil || len(got.Permissions) != 1 || got.Permissions[0] != authz.ProjectRead {
		t.Fatalf("client get = %+v, %v", got, err)
	}
	_, err = w.svc.Get(w.ctx, cli.id, org, private.ID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Update(w.ctx, cli.id, org, shared.ID, Changes{Name: str("client edit")})
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Access(w.ctx, cli.id, org, shared.ID)
	wantErr(t, err, ErrForbidden)

	// Another client of the same agency sees none of it.
	page, _ = w.svc.List(w.ctx, other.id, org, ListOptions{})
	if len(page.Projects) != 0 {
		t.Fatalf("other client sees %+v", page.Projects)
	}
	_, err = w.svc.Get(w.ctx, other.id, org, shared.ID)
	wantErr(t, err, ErrNotFound)

	// Revoking removes access immediately.
	if err := w.svc.RevokeAccess(w.ctx, ana.id, org, shared.ID, cli.id); err != nil {
		t.Fatal(err)
	}
	wantErr(t, w.svc.RevokeAccess(w.ctx, ana.id, org, shared.ID, cli.id), ErrNotFound)
	_, err = w.svc.Get(w.ctx, cli.id, org, shared.ID)
	wantErr(t, err, ErrNotFound)

	// Removing a client from the organisation removes their grants too, so
	// rejoining later doesn't silently restore access.
	if err := w.svc.GrantAccess(w.ctx, ana.id, org, shared.ID, cli.id); err != nil {
		t.Fatal(err)
	}
	if err := w.accounts.RemoveMember(w.ctx, ana.id, org, cli.id); err != nil {
		t.Fatal(err)
	}
	if n := w.count(`SELECT count(*) FROM project_access WHERE project_id = ?`, shared.ID); n != 0 {
		t.Errorf("grants left after removal = %d", n)
	}
	w.join(org, ana, cli, authz.RoleClient)
	_, err = w.svc.Get(w.ctx, cli.id, org, shared.ID)
	wantErr(t, err, ErrNotFound)
}

func TestPagination(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := personalOrg(w, ana)
	want := map[string]bool{}
	for _, n := range []string{"one", "two", "three", "four", "five"} {
		want[w.create(ana, org, n).ID] = true
	}

	seen := map[string]bool{}
	var sizes []int
	cursor := ""
	var last Project
	for {
		page, err := w.svc.List(w.ctx, ana.id, org, ListOptions{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(page.Projects))
		for _, p := range page.Projects {
			if seen[p.ID] {
				t.Fatalf("duplicate %s", p.ID)
			}
			if last.ID != "" && p.CreatedAt.After(last.CreatedAt) {
				t.Fatalf("not newest first")
			}
			seen[p.ID], last = true, p
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 5 || len(sizes) != 3 || sizes[2] != 1 {
		t.Fatalf("seen %d in pages %v", len(seen), sizes)
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("missing %s", id)
		}
	}

	// Filtering by workspace.
	_, ms, _ := w.accounts.Me(w.ctx, ana.id)
	page, err := w.svc.List(w.ctx, ana.id, org, ListOptions{WorkspaceID: ms[0].Workspaces[0].ID})
	if err != nil || len(page.Projects) != 5 {
		t.Fatalf("workspace filter = %d, %v", len(page.Projects), err)
	}
}

// Package project owns projects: the creative production units that media,
// edits and renders will hang off. Every operation resolves the caller's
// membership through package account and authorises itself.
//
// Visibility rules:
//   - Members with project.read see every project in the organisation,
//     except clients, who see only projects explicitly shared with them.
//   - Projects a caller cannot see are reported as not found.
//   - Creators may update only projects they created.
package project

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
)

// Errors are shared with package account so the HTTP layer maps them once.
var (
	ErrNotFound  = account.ErrNotFound
	ErrForbidden = account.ErrForbidden
)

// Limits.
const (
	MaxNameLength        = 120
	MaxDescriptionLength = 2000
	DefaultPageSize      = 30
	MaxPageSize          = 100
)

// Project is a project as seen by one caller. Permissions lists the
// project-level actions the caller may take on it.
type Project struct {
	ID             string
	OrganisationID string
	WorkspaceID    string
	WorkspaceName  string
	Name           string
	Description    string
	CreatedByID    string
	CreatedByName  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Permissions    []authz.Permission
}

// Page is one page of projects, newest first. NextCursor is empty on the
// last page.
type Page struct {
	Projects   []Project
	NextCursor string
}

// ListOptions filter and page a project listing.
type ListOptions struct {
	WorkspaceID string
	Cursor      string
	Limit       int
}

// Changes are the fields an update may set. Nil fields are left alone.
type Changes struct {
	Name        *string
	Description *string
}

// Grant is a person a project has been explicitly shared with.
type Grant struct {
	UserID    string
	Email     string
	FirstName string
	LastName  string
	AvatarURL string
	GrantedAt time.Time
}

// Service manages projects.
type Service struct {
	db *bun.DB
}

// NewService returns a project service backed by db.
func NewService(db *bun.DB) *Service { return &Service{db: db} }

func invalid(field, msg string) error { return &account.ValidationError{Field: field, Message: msg} }

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > MaxNameLength {
		return "", invalid("name", fmt.Sprintf("Name must be between 1 and %d characters.", MaxNameLength))
	}
	return name, nil
}

func cleanDescription(d string) (string, error) {
	d = strings.TrimSpace(d)
	if len([]rune(d)) > MaxDescriptionLength {
		return "", invalid("description", fmt.Sprintf("Description must be at most %d characters.", MaxDescriptionLength))
	}
	return d, nil
}

// permissions returns what a caller may do with a project they can see.
func permissions(a account.Access, actorID, createdBy string) []authz.Permission {
	out := []authz.Permission{authz.ProjectRead}
	if authz.Can(a.Role, authz.ProjectUpdate) && (a.Role != authz.RoleCreator || createdBy == actorID) {
		out = append(out, authz.ProjectUpdate)
	}
	for _, p := range []authz.Permission{authz.ProjectDelete, authz.ProjectAccessManage} {
		if authz.Can(a.Role, p) {
			out = append(out, p)
		}
	}
	return out
}

func has(ps []authz.Permission, p authz.Permission) bool {
	for _, x := range ps {
		if x == p {
			return true
		}
	}
	return false
}

// visibleClause restricts p.* rows to those the caller may see. Clients
// need an explicit grant; everyone else with project.read sees all.
const visibleClause = `(? <> 'client' OR EXISTS (
	SELECT 1 FROM project_access pa WHERE pa.project_id = p.id AND pa.membership_id = ?))`

const projectColumns = `
	p.id, p.organisation_id, p.workspace_id, w.name, p.name, p.description,
	coalesce(p.created_by_user_id::text, ''),
	coalesce(nullif(trim(coalesce(u.first_name, '') || ' ' || coalesce(u.last_name, '')), ''), u.email, ''),
	p.created_at, p.updated_at`

const projectFrom = `
	FROM projects p
	JOIN workspaces w ON w.id = p.workspace_id
	LEFT JOIN users u ON u.id = p.created_by_user_id`

type scanner interface{ Scan(...any) error }

func scanProject(row scanner) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.OrganisationID, &p.WorkspaceID, &p.WorkspaceName, &p.Name, &p.Description,
		&p.CreatedByID, &p.CreatedByName, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// access resolves the caller and checks they can read projects at all.
func access(ctx context.Context, db bun.IDB, actorID, orgID string) (account.Access, error) {
	a, err := account.ResolveAccess(ctx, db, actorID, orgID, false)
	if err != nil {
		return account.Access{}, err
	}
	if !authz.Can(a.Role, authz.ProjectRead) {
		return account.Access{}, ErrForbidden
	}
	return a, nil
}

// load returns a project the caller can see, optionally locking it.
func load(ctx context.Context, db bun.IDB, a account.Access, actorID, orgID, projectID string, forUpdate bool) (Project, error) {
	if !account.ValidUUID(projectID) {
		return Project{}, ErrNotFound
	}
	q := `SELECT` + projectColumns + projectFrom + `
		WHERE p.id = ? AND p.organisation_id = ? AND ` + visibleClause
	if forUpdate {
		q += " FOR UPDATE OF p"
	}
	p, err := scanProject(db.QueryRowContext(ctx, q, projectID, orgID, a.Role, a.MembershipID))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("load project: %w", err)
	}
	p.Permissions = permissions(a, actorID, p.CreatedByID)
	return p, nil
}

// Create makes a project (project.create). workspaceID may be empty, in
// which case the organisation's first workspace is used.
func (s *Service) Create(ctx context.Context, actorID, orgID, workspaceID, name, description string) (Project, error) {
	name, err := cleanName(name)
	if err != nil {
		return Project{}, err
	}
	description, err = cleanDescription(description)
	if err != nil {
		return Project{}, err
	}
	if workspaceID != "" && !account.ValidUUID(workspaceID) {
		return Project{}, invalid("workspace_id", "Choose a workspace in this organisation.")
	}
	var p Project
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		a, err := account.ResolveAccess(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if !authz.Can(a.Role, authz.ProjectCreate) {
			return ErrForbidden
		}
		// Resolve the workspace inside the organisation; a workspace from
		// another tenant is indistinguishable from a missing one.
		q := `SELECT id FROM workspaces WHERE organisation_id = ? AND (? = '' OR id::text = ?) ORDER BY created_at, id LIMIT 1`
		if err := tx.QueryRowContext(ctx, q, orgID, workspaceID, workspaceID).Scan(&workspaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return invalid("workspace_id", "Choose a workspace in this organisation.")
			}
			return fmt.Errorf("resolve workspace: %w", err)
		}
		var id string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO projects (organisation_id, workspace_id, name, description, created_by_user_id)
			VALUES (?, ?, ?, ?, ?) RETURNING id`,
			orgID, workspaceID, name, description, actorID).Scan(&id); err != nil {
			return fmt.Errorf("insert project: %w", err)
		}
		if p, err = load(ctx, tx, a, actorID, orgID, id, false); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "project.created", "project", id, nil)
	})
	return p, err
}

// cursor encodes the position after the last item of a page.
func encodeCursor(p Project) string {
	return base64.RawURLEncoding.EncodeToString([]byte(p.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + p.ID))
}

func decodeCursor(c string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok || !account.ValidUUID(id) {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	return t, id, err
}

// List returns the projects the caller can see, newest first.
func (s *Service) List(ctx context.Context, actorID, orgID string, opts ListOptions) (Page, error) {
	limit := opts.Limit
	switch {
	case limit == 0:
		limit = DefaultPageSize
	case limit < 1 || limit > MaxPageSize:
		return Page{}, invalid("limit", fmt.Sprintf("Limit must be between 1 and %d.", MaxPageSize))
	}
	if opts.WorkspaceID != "" && !account.ValidUUID(opts.WorkspaceID) {
		return Page{}, invalid("workspace_id", "Choose a workspace in this organisation.")
	}
	var after time.Time
	var afterID string
	if opts.Cursor != "" {
		var err error
		if after, afterID, err = decodeCursor(opts.Cursor); err != nil {
			return Page{}, invalid("cursor", "The page cursor is invalid. Start again from the first page.")
		}
	}
	a, err := access(ctx, s.db, actorID, orgID)
	if err != nil {
		return Page{}, err
	}
	q := `SELECT` + projectColumns + projectFrom + `
		WHERE p.organisation_id = ? AND ` + visibleClause + `
		AND (? = '' OR p.workspace_id::text = ?)
		AND (? = '' OR (p.created_at, p.id) < (?::timestamptz, ?::uuid))
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT ?`
	var afterTS any
	if afterID != "" {
		afterTS = after
	}
	rows, err := s.db.QueryContext(ctx, q, orgID, a.Role, a.MembershipID,
		opts.WorkspaceID, opts.WorkspaceID, afterID, afterTS, nullUUID(afterID), limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	page := Page{Projects: []Project{}}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return Page{}, fmt.Errorf("scan project: %w", err)
		}
		p.Permissions = permissions(a, actorID, p.CreatedByID)
		page.Projects = append(page.Projects, p)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Projects) > limit {
		page.Projects = page.Projects[:limit]
		page.NextCursor = encodeCursor(page.Projects[limit-1])
	}
	return page, nil
}

func nullUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// Get returns one project the caller can see.
func (s *Service) Get(ctx context.Context, actorID, orgID, projectID string) (Project, error) {
	a, err := access(ctx, s.db, actorID, orgID)
	if err != nil {
		return Project{}, err
	}
	return load(ctx, s.db, a, actorID, orgID, projectID, false)
}

// Update changes a project's name or description (project.update; creators
// only for projects they created).
func (s *Service) Update(ctx context.Context, actorID, orgID, projectID string, ch Changes) (Project, error) {
	if ch.Name == nil && ch.Description == nil {
		return Project{}, invalid("name", "Provide a name or description to change.")
	}
	if ch.Name != nil {
		n, err := cleanName(*ch.Name)
		if err != nil {
			return Project{}, err
		}
		ch.Name = &n
	}
	if ch.Description != nil {
		d, err := cleanDescription(*ch.Description)
		if err != nil {
			return Project{}, err
		}
		ch.Description = &d
	}
	var p Project
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		a, err := access(ctx, tx, actorID, orgID)
		if err != nil {
			return err
		}
		cur, err := load(ctx, tx, a, actorID, orgID, projectID, true)
		if err != nil {
			return err
		}
		if !has(cur.Permissions, authz.ProjectUpdate) {
			return ErrForbidden
		}
		changed := []string{}
		if ch.Name != nil && *ch.Name != cur.Name {
			changed = append(changed, "name")
		}
		if ch.Description != nil && *ch.Description != cur.Description {
			changed = append(changed, "description")
		}
		if len(changed) == 0 {
			p = cur
			return nil
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET name = coalesce(?, name), description = coalesce(?, description), updated_at = now()
			WHERE id = ?`, ch.Name, ch.Description, projectID); err != nil {
			return fmt.Errorf("update project: %w", err)
		}
		if p, err = load(ctx, tx, a, actorID, orgID, projectID, false); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "project.updated", "project", projectID,
			map[string]any{"fields": changed})
	})
	return p, err
}

// Delete removes a project (project.delete).
func (s *Service) Delete(ctx context.Context, actorID, orgID, projectID string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		a, err := access(ctx, tx, actorID, orgID)
		if err != nil {
			return err
		}
		cur, err := load(ctx, tx, a, actorID, orgID, projectID, true)
		if err != nil {
			return err
		}
		if !has(cur.Permissions, authz.ProjectDelete) {
			return ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID); err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		return account.Audit(ctx, tx, orgID, actorID, "project.deleted", "project", projectID,
			map[string]any{"name": cur.Name})
	})
}

// manageable loads a project and checks the caller may manage its access.
func manageable(ctx context.Context, tx bun.IDB, actorID, orgID, projectID string) (account.Access, Project, error) {
	a, err := access(ctx, tx, actorID, orgID)
	if err != nil {
		return a, Project{}, err
	}
	p, err := load(ctx, tx, a, actorID, orgID, projectID, false)
	if err != nil {
		return a, Project{}, err
	}
	if !has(p.Permissions, authz.ProjectAccessManage) {
		return a, Project{}, ErrForbidden
	}
	return a, p, nil
}

// Access lists the people a project is explicitly shared with
// (project.access.manage).
func (s *Service) Access(ctx context.Context, actorID, orgID, projectID string) ([]Grant, error) {
	if _, _, err := manageable(ctx, s.db, actorID, orgID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, coalesce(u.first_name, ''), coalesce(u.last_name, ''), coalesce(u.avatar_url, ''), pa.created_at
		FROM project_access pa
		JOIN memberships m ON m.id = pa.membership_id
		JOIN users u ON u.id = m.user_id
		WHERE pa.project_id = ? AND pa.organisation_id = ?
		ORDER BY pa.created_at`, projectID, orgID)
	if err != nil {
		return nil, fmt.Errorf("list project access: %w", err)
	}
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.UserID, &g.Email, &g.FirstName, &g.LastName, &g.AvatarURL, &g.GrantedAt); err != nil {
			return nil, fmt.Errorf("scan project access: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GrantAccess shares a project with a client of the organisation
// (project.access.manage). Other roles already see every project, so only
// clients can be granted access. Granting twice is a no-op.
func (s *Service) GrantAccess(ctx context.Context, actorID, orgID, projectID, userID string) error {
	notClient := invalid("user_id", "Choose a client of this organisation. Other roles already see every project.")
	if !account.ValidUUID(userID) {
		return notClient
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, _, err := manageable(ctx, tx, actorID, orgID, projectID); err != nil {
			return err
		}
		target, err := account.ResolveAccess(ctx, tx, userID, orgID, true)
		if errors.Is(err, account.ErrNotFound) || (err == nil && target.Role != authz.RoleClient) {
			return notClient
		}
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO project_access (project_id, membership_id, organisation_id, granted_by_user_id)
			VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`, projectID, target.MembershipID, orgID, actorID)
		if err != nil {
			return fmt.Errorf("grant project access: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		return account.Audit(ctx, tx, orgID, actorID, "project.access_granted", "project", projectID,
			map[string]any{"user_id": userID})
	})
}

// RevokeAccess stops sharing a project with someone (project.access.manage).
func (s *Service) RevokeAccess(ctx context.Context, actorID, orgID, projectID, userID string) error {
	if !account.ValidUUID(userID) {
		return ErrNotFound
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, _, err := manageable(ctx, tx, actorID, orgID, projectID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			DELETE FROM project_access pa USING memberships m
			WHERE pa.membership_id = m.id AND pa.project_id = ? AND pa.organisation_id = ? AND m.user_id = ?`,
			projectID, orgID, userID)
		if err != nil {
			return fmt.Errorf("revoke project access: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return account.Audit(ctx, tx, orgID, actorID, "project.access_revoked", "project", projectID,
			map[string]any{"user_id": userID})
	})
}

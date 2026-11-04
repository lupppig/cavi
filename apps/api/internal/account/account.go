// Package account owns CAVI's users, organisations, workspaces and
// memberships: the source of truth for who belongs to which tenant.
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
)

// ErrUserNotFound is returned when a user ID has no matching user.
var ErrUserNotFound = errors.New("account: user not found")

// RoleOwner is the role given to an organisation's creator.
const RoleOwner = authz.RoleOwner

// User is a person with a CAVI account.
type User struct {
	ID        string
	Email     string
	FirstName string
	LastName  string
	AvatarURL string
	CreatedAt time.Time
}

// Workspace is a working area inside an organisation.
type Workspace struct {
	ID   string
	Name string
}

// Membership is a user's role in one organisation.
type Membership struct {
	OrganisationID   string
	OrganisationName string
	Personal         bool
	Role             string
	Workspaces       []Workspace
}

// Service manages accounts and tenancy.
type Service struct {
	db *bun.DB
}

// NewService returns an account service backed by db.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// SignIn records a verified sign-in. On the first sign-in it creates the
// user together with a personal organisation, a first workspace and an
// owner membership, all in one transaction. It is safe to call repeatedly
// and concurrently for the same person.
func (s *Service) SignIn(ctx context.Context, p identity.Profile) (User, error) {
	if p.ProviderUserID == "" || p.Email == "" {
		return User{}, errors.New("account: profile missing provider user id or email")
	}
	var u User
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO users (workos_user_id, email, email_verified, first_name, last_name, avatar_url, last_sign_in_at)
			VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), now())
			ON CONFLICT (workos_user_id) DO UPDATE SET
				email = EXCLUDED.email,
				email_verified = EXCLUDED.email_verified,
				first_name = EXCLUDED.first_name,
				last_name = EXCLUDED.last_name,
				avatar_url = EXCLUDED.avatar_url,
				last_sign_in_at = now(),
				updated_at = now()
			RETURNING id, email, coalesce(first_name, ''), coalesce(last_name, ''), coalesce(avatar_url, ''), created_at`,
			p.ProviderUserID, p.Email, p.EmailVerified, p.FirstName, p.LastName, p.AvatarURL)
		if err := row.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.AvatarURL, &u.CreatedAt); err != nil {
			return fmt.Errorf("upsert user: %w", err)
		}

		// The unique personal_owner_user_id makes this idempotent under
		// concurrent first sign-ins: only one insert wins.
		var orgID string
		err := tx.QueryRowContext(ctx, `
			INSERT INTO organisations (name, personal_owner_user_id)
			VALUES (?, ?)
			ON CONFLICT (personal_owner_user_id) DO NOTHING
			RETURNING id`, personalOrgName(u.FirstName), u.ID).Scan(&orgID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // personal organisation already exists
		}
		if err != nil {
			return fmt.Errorf("create personal organisation: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO workspaces (organisation_id, name) VALUES (?, 'Main')`, orgID); err != nil {
			return fmt.Errorf("create first workspace: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memberships (organisation_id, user_id, role) VALUES (?, ?, ?)`,
			orgID, u.ID, RoleOwner); err != nil {
			return fmt.Errorf("create owner membership: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// Me returns a user and every organisation they belong to, with their
// role and that organisation's workspaces. It only ever returns
// organisations the user is a member of.
func (s *Service) Me(ctx context.Context, userID string) (User, []Membership, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, coalesce(first_name, ''), coalesce(last_name, ''), coalesce(avatar_url, ''), created_at
		FROM users WHERE id = ?`, userID).
		Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.AvatarURL, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, nil, ErrUserNotFound
	}
	if err != nil {
		return User{}, nil, fmt.Errorf("load user: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.name, o.personal_owner_user_id IS NOT NULL, m.role,
		       coalesce(w.id::text, ''), coalesce(w.name, '')
		FROM memberships m
		JOIN organisations o ON o.id = m.organisation_id
		LEFT JOIN workspaces w ON w.organisation_id = o.id
		WHERE m.user_id = ?
		ORDER BY (o.personal_owner_user_id IS NOT NULL) DESC, o.created_at, w.created_at`, userID)
	if err != nil {
		return User{}, nil, fmt.Errorf("load memberships: %w", err)
	}
	defer rows.Close()

	var out []Membership
	index := map[string]int{}
	for rows.Next() {
		var m Membership
		var wsID, wsName string
		if err := rows.Scan(&m.OrganisationID, &m.OrganisationName, &m.Personal, &m.Role, &wsID, &wsName); err != nil {
			return User{}, nil, fmt.Errorf("scan membership: %w", err)
		}
		i, ok := index[m.OrganisationID]
		if !ok {
			m.Workspaces = []Workspace{}
			out = append(out, m)
			i = len(out) - 1
			index[m.OrganisationID] = i
		}
		if wsID != "" {
			out[i].Workspaces = append(out[i].Workspaces, Workspace{ID: wsID, Name: wsName})
		}
	}
	if err := rows.Err(); err != nil {
		return User{}, nil, fmt.Errorf("read memberships: %w", err)
	}
	return u, out, nil
}

func personalOrgName(firstName string) string {
	name := strings.TrimSpace(firstName)
	if name == "" {
		return "My studio"
	}
	n := name + "’s studio"
	if len([]rune(n)) > 120 {
		return "My studio"
	}
	return n
}

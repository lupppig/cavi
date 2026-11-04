package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/authz"
)

// Errors returned by organisation operations. Callers that are not
// members of an organisation get ErrNotFound, never ErrForbidden, so
// organisation IDs cannot be probed.
var (
	ErrNotFound            = errors.New("account: not found")
	ErrForbidden           = errors.New("account: not allowed")
	ErrLastOwner           = errors.New("account: an organisation must keep at least one owner")
	ErrPersonalOrg         = errors.New("account: personal organisations cannot have other members")
	ErrAlreadyMember       = errors.New("account: already a member")
	ErrInvitationExists    = errors.New("account: an invitation is already pending for this email")
	ErrEmailNotVerified    = errors.New("account: email address not verified")
	invitationTTL          = 14 * 24 * time.Hour
	uuidPattern            = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	errInvalidOrganisation = &ValidationError{Field: "name", Message: "Name must be between 1 and 120 characters."}
)

// ValidationError reports an invalid input field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Organisation is an organisation as seen by one member.
type Organisation struct {
	ID          string
	Name        string
	Personal    bool
	Role        string
	Permissions []authz.Permission
	CreatedAt   time.Time
}

// Member is a person in an organisation.
type Member struct {
	UserID    string
	Email     string
	FirstName string
	LastName  string
	AvatarURL string
	Role      string
	JoinedAt  time.Time
}

// Invitation is an invitation to join an organisation.
type Invitation struct {
	ID               string
	OrganisationID   string
	OrganisationName string
	Email            string
	Role             string
	InvitedByName    string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type membership struct {
	role     string
	personal bool
	name     string
	created  time.Time
}

// member loads the caller's membership. Missing organisations and
// non-members are indistinguishable (ErrNotFound).
func member(ctx context.Context, db bun.IDB, userID, orgID string, forUpdate bool) (membership, error) {
	if !uuidPattern.MatchString(orgID) {
		return membership{}, ErrNotFound
	}
	q := `SELECT m.role, o.personal_owner_user_id IS NOT NULL, o.name, o.created_at
		FROM memberships m JOIN organisations o ON o.id = m.organisation_id
		WHERE m.organisation_id = ? AND m.user_id = ?`
	if forUpdate {
		q += " FOR UPDATE OF m"
	}
	var m membership
	err := db.QueryRowContext(ctx, q, orgID, userID).Scan(&m.role, &m.personal, &m.name, &m.created)
	if errors.Is(err, sql.ErrNoRows) {
		return membership{}, ErrNotFound
	}
	if err != nil {
		return membership{}, fmt.Errorf("load membership: %w", err)
	}
	return m, nil
}

func require(m membership, p authz.Permission) error {
	if !authz.Can(m.role, p) {
		return ErrForbidden
	}
	return nil
}

func audit(ctx context.Context, db bun.IDB, orgID, actorID, action, targetType, targetID string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO audit_events (organisation_id, actor_user_id, action, target_type, target_id, metadata)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?::jsonb)`,
		orgID, actorID, action, targetType, targetID, string(b))
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > 120 {
		return "", errInvalidOrganisation
	}
	return name, nil
}

func toOrganisation(id string, m membership) Organisation {
	return Organisation{
		ID: id, Name: m.name, Personal: m.personal, Role: m.role,
		Permissions: authz.PermissionsFor(m.role), CreatedAt: m.created,
	}
}

// CreateOrganisation creates a team organisation with a Main workspace and
// makes the creator its owner.
func (s *Service) CreateOrganisation(ctx context.Context, actorID, name string) (Organisation, error) {
	name, err := cleanName(name)
	if err != nil {
		return Organisation{}, err
	}
	var org Organisation
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var m membership
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO organisations (name) VALUES (?) RETURNING id, created_at`, name).
			Scan(&org.ID, &m.created); err != nil {
			return fmt.Errorf("insert organisation: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO workspaces (organisation_id, name) VALUES (?, 'Main')`, org.ID); err != nil {
			return fmt.Errorf("insert workspace: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memberships (organisation_id, user_id, role) VALUES (?, ?, ?)`,
			org.ID, actorID, authz.RoleOwner); err != nil {
			return fmt.Errorf("insert membership: %w", err)
		}
		m.role, m.name = authz.RoleOwner, name
		org = toOrganisation(org.ID, m)
		return audit(ctx, tx, org.ID, actorID, "organisation.created", "organisation", org.ID, nil)
	})
	return org, err
}

// Organisation returns an organisation the actor belongs to.
func (s *Service) Organisation(ctx context.Context, actorID, orgID string) (Organisation, error) {
	m, err := member(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return Organisation{}, err
	}
	return toOrganisation(orgID, m), nil
}

// RenameOrganisation renames an organisation (organisation.manage).
func (s *Service) RenameOrganisation(ctx context.Context, actorID, orgID, name string) (Organisation, error) {
	name, err := cleanName(name)
	if err != nil {
		return Organisation{}, err
	}
	var org Organisation
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		m, err := member(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if err := require(m, authz.OrganisationManage); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE organisations SET name = ?, updated_at = now() WHERE id = ?`, name, orgID); err != nil {
			return fmt.Errorf("rename organisation: %w", err)
		}
		old := m.name
		m.name = name
		org = toOrganisation(orgID, m)
		return audit(ctx, tx, orgID, actorID, "organisation.renamed", "organisation", orgID, map[string]any{"from": old, "to": name})
	})
	return org, err
}

// Members lists an organisation's members (organisation.members.read).
func (s *Service) Members(ctx context.Context, actorID, orgID string) ([]Member, error) {
	m, err := member(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return nil, err
	}
	if err := require(m, authz.OrganisationMembersRead); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, coalesce(u.first_name, ''), coalesce(u.last_name, ''), coalesce(u.avatar_url, ''), m.role, m.created_at
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.organisation_id = ?
		ORDER BY array_position(?::text[], m.role), m.created_at`, orgID, rolesArray())
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var mb Member
		if err := rows.Scan(&mb.UserID, &mb.Email, &mb.FirstName, &mb.LastName, &mb.AvatarURL, &mb.Role, &mb.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		out = append(out, mb)
	}
	return out, rows.Err()
}

func rolesArray() string { return "{" + strings.Join(authz.Roles, ",") + "}" }

// lockOwners locks the organisation's owner memberships and returns how
// many there are, so last-owner checks hold under concurrent changes.
func lockOwners(ctx context.Context, tx bun.Tx, orgID string) (int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT user_id FROM memberships WHERE organisation_id = ? AND role = 'owner' FOR UPDATE`, orgID)
	if err != nil {
		return 0, fmt.Errorf("lock owners: %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	return n, rows.Err()
}

// ChangeRole changes a member's role (organisation.members.manage). Only
// owners can grant or take away the owner role, and the last owner cannot
// be demoted.
func (s *Service) ChangeRole(ctx context.Context, actorID, orgID, targetID, role string) error {
	if !authz.ValidRole(role) {
		return &ValidationError{Field: "role", Message: "Choose a valid role."}
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		actor, err := member(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if err := require(actor, authz.OrganisationMembersManage); err != nil {
			return err
		}
		if actor.personal {
			return ErrPersonalOrg
		}
		owners, err := lockOwners(ctx, tx, orgID)
		if err != nil {
			return err
		}
		target, err := member(ctx, tx, targetID, orgID, true)
		if err != nil {
			return err
		}
		if target.role == role {
			return nil
		}
		if (target.role == authz.RoleOwner || role == authz.RoleOwner) && actor.role != authz.RoleOwner {
			return ErrForbidden
		}
		if target.role == authz.RoleOwner && owners <= 1 {
			return ErrLastOwner
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE memberships SET role = ?, updated_at = now() WHERE organisation_id = ? AND user_id = ?`,
			role, orgID, targetID); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
		return audit(ctx, tx, orgID, actorID, "member.role_changed", "user", targetID,
			map[string]any{"from": target.role, "to": role})
	})
}

// RemoveMember removes someone from an organisation. Members may always
// remove themselves (leave); removing others needs
// organisation.members.manage, and only owners can remove owners. The
// last owner cannot leave or be removed.
func (s *Service) RemoveMember(ctx context.Context, actorID, orgID, targetID string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		actor, err := member(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if actor.personal {
			return ErrPersonalOrg
		}
		self := actorID == targetID
		if !self {
			if err := require(actor, authz.OrganisationMembersManage); err != nil {
				return err
			}
		}
		owners, err := lockOwners(ctx, tx, orgID)
		if err != nil {
			return err
		}
		target, err := member(ctx, tx, targetID, orgID, true)
		if err != nil {
			return err
		}
		if target.role == authz.RoleOwner {
			if !self && actor.role != authz.RoleOwner {
				return ErrForbidden
			}
			if owners <= 1 {
				return ErrLastOwner
			}
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM memberships WHERE organisation_id = ? AND user_id = ?`, orgID, targetID); err != nil {
			return fmt.Errorf("remove member: %w", err)
		}
		action := "member.removed"
		if self {
			action = "member.left"
		}
		return audit(ctx, tx, orgID, actorID, action, "user", targetID, map[string]any{"role": target.role})
	})
}

func normaliseEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	addr, err := mail.ParseAddress(email)
	at := strings.LastIndexByte(email, '@')
	if err != nil || addr.Name != "" || addr.Address != email || len(email) > 254 || at < 1 ||
		!strings.Contains(email[at+1:], ".") {
		return "", &ValidationError{Field: "email", Message: "Enter a valid email address."}
	}
	return strings.ToLower(email), nil
}

// CreateInvitation invites an email address to join a team organisation
// (organisation.members.manage). Only owners can invite owners.
func (s *Service) CreateInvitation(ctx context.Context, actorID, orgID, email, role string) (Invitation, error) {
	email, err := normaliseEmail(email)
	if err != nil {
		return Invitation{}, err
	}
	if !authz.ValidRole(role) {
		return Invitation{}, &ValidationError{Field: "role", Message: "Choose a valid role."}
	}
	var inv Invitation
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		actor, err := member(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if err := require(actor, authz.OrganisationMembersManage); err != nil {
			return err
		}
		if actor.personal {
			return ErrPersonalOrg
		}
		if role == authz.RoleOwner && actor.role != authz.RoleOwner {
			return ErrForbidden
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM memberships m JOIN users u ON u.id = m.user_id
			WHERE m.organisation_id = ? AND lower(u.email) = ?)`, orgID, email).Scan(&exists); err != nil {
			return fmt.Errorf("check membership: %w", err)
		}
		if exists {
			return ErrAlreadyMember
		}
		// Clear an expired pending invitation so the address can be re-invited.
		if _, err := tx.ExecContext(ctx, `
			UPDATE invitations SET status = 'revoked', responded_at = now()
			WHERE organisation_id = ? AND email_normalized = ? AND status = 'pending' AND expires_at <= now()`,
			orgID, email); err != nil {
			return fmt.Errorf("expire old invitation: %w", err)
		}
		err = tx.QueryRowContext(ctx, `
			INSERT INTO invitations (organisation_id, email_normalized, role, invited_by_user_id, expires_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (organisation_id, email_normalized) WHERE status = 'pending' DO NOTHING
			RETURNING id, created_at, expires_at`,
			orgID, email, role, actorID, time.Now().Add(invitationTTL)).
			Scan(&inv.ID, &inv.CreatedAt, &inv.ExpiresAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvitationExists
		}
		if err != nil {
			return fmt.Errorf("insert invitation: %w", err)
		}
		inv.OrganisationID, inv.OrganisationName, inv.Email, inv.Role = orgID, actor.name, email, role
		return audit(ctx, tx, orgID, actorID, "invitation.created", "invitation", inv.ID,
			map[string]any{"role": role})
	})
	return inv, err
}

const invitationColumns = `
	i.id, i.organisation_id, o.name, i.email_normalized, i.role,
	coalesce(nullif(trim(coalesce(u.first_name, '') || ' ' || coalesce(u.last_name, '')), ''), u.email, ''),
	i.created_at, i.expires_at`

func scanInvitations(rows *sql.Rows) ([]Invitation, error) {
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.OrganisationID, &inv.OrganisationName, &inv.Email, &inv.Role,
			&inv.InvitedByName, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Invitations lists an organisation's pending invitations
// (organisation.members.manage).
func (s *Service) Invitations(ctx context.Context, actorID, orgID string) ([]Invitation, error) {
	m, err := member(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return nil, err
	}
	if err := require(m, authz.OrganisationMembersManage); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT`+invitationColumns+`
		FROM invitations i JOIN organisations o ON o.id = i.organisation_id
		LEFT JOIN users u ON u.id = i.invited_by_user_id
		WHERE i.organisation_id = ? AND i.status = 'pending' AND i.expires_at > now()
		ORDER BY i.created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	return scanInvitations(rows)
}

// RevokeInvitation cancels a pending invitation (organisation.members.manage).
func (s *Service) RevokeInvitation(ctx context.Context, actorID, orgID, invitationID string) error {
	if !uuidPattern.MatchString(invitationID) {
		return ErrNotFound
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		m, err := member(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if err := require(m, authz.OrganisationMembersManage); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE invitations SET status = 'revoked', responded_at = now()
			WHERE id = ? AND organisation_id = ? AND status = 'pending'`, invitationID, orgID)
		if err != nil {
			return fmt.Errorf("revoke invitation: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, orgID, actorID, "invitation.revoked", "invitation", invitationID, nil)
	})
}

// PendingInvitations lists invitations addressed to the user's verified
// email. Unverified emails see none.
func (s *Service) PendingInvitations(ctx context.Context, userID string) ([]Invitation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT`+invitationColumns+`
		FROM invitations i
		JOIN organisations o ON o.id = i.organisation_id
		JOIN users me ON me.id = ? AND me.email_verified AND lower(me.email) = i.email_normalized
		LEFT JOIN users u ON u.id = i.invited_by_user_id
		WHERE i.status = 'pending' AND i.expires_at > now()
		ORDER BY i.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list pending invitations: %w", err)
	}
	return scanInvitations(rows)
}

// respond accepts or declines an invitation addressed to the user's
// verified email. Anything else is ErrNotFound.
func (s *Service) respond(ctx context.Context, userID, invitationID string, accept bool) (Organisation, error) {
	if !uuidPattern.MatchString(invitationID) {
		return Organisation{}, ErrNotFound
	}
	var org Organisation
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var verified bool
		var email string
		if err := tx.QueryRowContext(ctx, `SELECT email_verified, lower(email) FROM users WHERE id = ?`, userID).
			Scan(&verified, &email); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("load user: %w", err)
		}
		var orgID, role string
		err := tx.QueryRowContext(ctx, `
			SELECT organisation_id, role FROM invitations
			WHERE id = ? AND status = 'pending' AND expires_at > now() AND email_normalized = ?
			FOR UPDATE`, invitationID, email).Scan(&orgID, &role)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load invitation: %w", err)
		}
		if !verified {
			return ErrEmailNotVerified
		}
		status, action := "declined", "invitation.declined"
		if accept {
			status, action = "accepted", "invitation.accepted"
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO memberships (organisation_id, user_id, role) VALUES (?, ?, ?)
				ON CONFLICT (organisation_id, user_id) DO NOTHING`, orgID, userID, role); err != nil {
				return fmt.Errorf("insert membership: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE invitations SET status = ?, responded_at = now() WHERE id = ?`, status, invitationID); err != nil {
			return fmt.Errorf("update invitation: %w", err)
		}
		if err := audit(ctx, tx, orgID, userID, action, "invitation", invitationID, nil); err != nil {
			return err
		}
		if accept {
			m, err := member(ctx, tx, userID, orgID, false)
			if err != nil {
				return err
			}
			org = toOrganisation(orgID, m)
		}
		return nil
	})
	return org, err
}

// AcceptInvitation joins the organisation an invitation is for.
func (s *Service) AcceptInvitation(ctx context.Context, userID, invitationID string) (Organisation, error) {
	return s.respond(ctx, userID, invitationID, true)
}

// DeclineInvitation turns an invitation down.
func (s *Service) DeclineInvitation(ctx context.Context, userID, invitationID string) error {
	_, err := s.respond(ctx, userID, invitationID, false)
	return err
}

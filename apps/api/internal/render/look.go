package render

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
)

// Look is an organisation's house style for clips: what new montages and
// batches start from, so a channel's clips look like they belong
// together. It is a starting point; each clip can still differ.
type Look struct {
	// Captions are the caption choices; nil for no captions.
	Captions *CaptionOptions `json:"captions"`
	// SignOff is the sign-off card's text; empty for none.
	SignOff string `json:"sign_off"`
	// FollowPeople starts clips with auto framing.
	FollowPeople bool `json:"follow_people"`
	// CutPauses starts montages with pauses cut.
	CutPauses bool `json:"cut_pauses"`
	// SeriesBadge shows PART n/N on the parts of a series.
	SeriesBadge bool `json:"series_badge"`
}

// DefaultLook is the look for organisations that haven't chosen one: what
// short-form clips do best with.
func DefaultLook() Look {
	return Look{
		Captions:     &CaptionOptions{Style: CaptionsPop, Mode: ModeWords, Position: "bottom", Uppercase: true},
		SignOff:      "Follow for more!",
		FollowPeople: true,
		CutPauses:    true,
		SeriesBadge:  true,
	}
}

func (l Look) normalize() (Look, error) {
	if l.Captions != nil {
		c, err := l.Captions.Normalize()
		if err != nil {
			return Look{}, err
		}
		c.SpeakerColors = nil // speakers belong to a file, not the house style
		l.Captions = &c
	}
	l.SignOff = strings.Join(strings.Fields(l.SignOff), " ")
	if !utf8.ValidString(l.SignOff) || utf8.RuneCountInString(l.SignOff) > MaxSignOffRunes {
		return Look{}, invalid("sign_off", fmt.Sprintf("Keep the sign-off to %d characters.", MaxSignOffRunes))
	}
	return l, nil
}

// Look returns an organisation's look (anyone in it can read it) and
// whether it was chosen rather than the default.
func (s *Service) Look(ctx context.Context, actorID, orgID string) (Look, bool, error) {
	if _, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false); err != nil {
		return Look{}, false, err
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT clip_look FROM organisations WHERE id = ? AND clip_look IS NOT NULL`, orgID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultLook(), false, nil
	}
	if err != nil {
		return Look{}, false, fmt.Errorf("load look: %w", err)
	}
	var l Look
	if err := json.Unmarshal(raw, &l); err != nil {
		return Look{}, false, fmt.Errorf("organisation %s look: %w", orgID, err)
	}
	return l, true, nil
}

// SetLook saves an organisation's look (editing.manage).
func (s *Service) SetLook(ctx context.Context, actorID, orgID string, l Look) (Look, error) {
	l, err := l.normalize()
	if err != nil {
		return Look{}, err
	}
	a, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return Look{}, err
	}
	if !authz.Can(a.Role, authz.EditingManage) {
		return Look{}, ErrForbidden
	}
	raw, err := json.Marshal(l)
	if err != nil {
		return Look{}, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE organisations SET clip_look = ?::jsonb, updated_at = now() WHERE id = ?`, string(raw), orgID); err != nil {
		return Look{}, fmt.Errorf("save look: %w", err)
	}
	if err := account.Audit(ctx, s.db, orgID, actorID, "organisation.look_changed", "organisation", orgID, nil); err != nil {
		return Look{}, err
	}
	return l, nil
}

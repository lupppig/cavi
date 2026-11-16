package youtube

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

// Errors returned to callers.
var (
	ErrNotFound      = account.ErrNotFound
	ErrForbidden     = account.ErrForbidden
	ErrNotConnected  = errors.New("youtube: no channel connected")
	ErrSyncing       = errors.New("youtube: already syncing")
	ErrConsentFailed = errors.New("youtube: connecting failed")
)

// Sync statuses.
const (
	SyncIdle    = "idle"
	SyncQueued  = "queued"
	SyncRunning = "running"
	SyncFailed  = "failed"
)

const stateTTL = 10 * time.Minute

// Service connects channels and reads what was synced. Every method
// authorises the actor itself.
type Service struct {
	db     *bun.DB
	client *Client
	key    []byte
	logger *slog.Logger
	onJob  func()
}

// NewService returns a Service. key encrypts refresh tokens.
func NewService(db *bun.DB, client *Client, key []byte, logger *slog.Logger) *Service {
	return &Service{db: db, client: client, key: key, logger: logger}
}

// OnJobQueued registers fn to run after a sync is queued.
func (s *Service) OnJobQueued(fn func()) { s.onJob = fn }

// Connection is an organisation's connected channel.
type Connection struct {
	ChannelID       string
	ChannelTitle    string
	ConnectedAt     time.Time
	ConnectedByName string
	SyncStatus      string
	SyncError       string
	SyncedAt        time.Time
	Videos          int
}

func hashState(state string) []byte {
	h := sha256.Sum256([]byte(state))
	return h[:]
}

// Status returns the organisation's connection; ErrNotConnected if none.
// Anyone in the organisation can see it.
func (s *Service) Status(ctx context.Context, actorID, orgID string) (Connection, error) {
	if _, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false); err != nil {
		return Connection{}, err
	}
	var c Connection
	var synced sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT y.channel_id, y.channel_title, y.connected_at,
			coalesce(nullif(trim(coalesce(u.first_name, '') || ' ' || coalesce(u.last_name, '')), ''), u.email, ''),
			y.sync_status, coalesce(y.sync_error, ''), y.synced_at,
			(SELECT count(*) FROM youtube_videos v WHERE v.organisation_id = y.organisation_id)
		FROM youtube_connections y LEFT JOIN users u ON u.id = y.connected_by_user_id
		WHERE y.organisation_id = ?`, orgID).
		Scan(&c.ChannelID, &c.ChannelTitle, &c.ConnectedAt, &c.ConnectedByName, &c.SyncStatus, &c.SyncError, &synced, &c.Videos)
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, ErrNotConnected
	}
	if err != nil {
		return Connection{}, fmt.Errorf("load youtube connection: %w", err)
	}
	c.SyncedAt = synced.Time
	return c, nil
}

// Connect starts connecting a channel (organisation.manage): it returns
// the Google consent page to send the person to.
func (s *Service) Connect(ctx context.Context, actorID, orgID string) (string, error) {
	a, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return "", err
	}
	if !authz.Can(a.Role, authz.OrganisationManage) {
		return "", ErrForbidden
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(b[:])
	if _, err := s.db.ExecContext(ctx, `DELETE FROM youtube_oauth_states WHERE expires_at < now()`); err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO youtube_oauth_states (state_hash, organisation_id, user_id, expires_at) VALUES (?, ?, ?, ?)`,
		hashState(state), orgID, actorID, time.Now().Add(stateTTL)); err != nil {
		return "", fmt.Errorf("save consent state: %w", err)
	}
	return s.client.ConsentURL(state), nil
}

// Callback finishes connecting when Google sends the person back: the
// state must be one this person started in the last 10 minutes, and they
// must still manage the organisation. It returns the organisation, so the
// person can be sent back to it. The first sync is queued.
func (s *Service) Callback(ctx context.Context, actorID, state, code, googleError string) (string, error) {
	var orgID, userID string
	err := s.db.QueryRowContext(ctx, `
		DELETE FROM youtube_oauth_states WHERE state_hash = ? AND expires_at > now()
		RETURNING organisation_id, user_id`, hashState(state)).Scan(&orgID, &userID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && userID != actorID) {
		return "", fmt.Errorf("%w: unknown or expired request", ErrConsentFailed)
	}
	if err != nil {
		return "", fmt.Errorf("load consent state: %w", err)
	}
	if googleError != "" {
		return orgID, fmt.Errorf("%w: %s", ErrConsentFailed, googleError)
	}
	a, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return "", err
	}
	if !authz.Can(a.Role, authz.OrganisationManage) {
		return orgID, ErrForbidden
	}
	access, refresh, scopes, err := s.client.Exchange(ctx, code)
	if err != nil {
		return orgID, fmt.Errorf("%w: %v", ErrConsentFailed, err)
	}
	for _, want := range Scopes {
		if !strings.Contains(" "+strings.Join(scopes, " ")+" ", " "+want+" ") {
			return orgID, fmt.Errorf("%w: CAVI needs to read your channel and its analytics; tick both on Google's page", ErrConsentFailed)
		}
	}
	ch, err := s.client.MyChannel(ctx, access)
	if err != nil {
		return orgID, fmt.Errorf("%w: %v", ErrConsentFailed, err)
	}
	sealed, err := seal(s.key, orgID, refresh)
	if err != nil {
		return orgID, err
	}
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// A different channel replaces the old one's videos.
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM youtube_videos WHERE organisation_id = ?
			AND NOT EXISTS (SELECT 1 FROM youtube_connections WHERE organisation_id = ? AND channel_id = ?)`, orgID, orgID, ch.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO youtube_connections (organisation_id, channel_id, channel_title, refresh_token, scopes, connected_by_user_id, sync_status)
			VALUES (?, ?, ?, ?, ?, ?, 'queued')
			ON CONFLICT (organisation_id) DO UPDATE SET channel_id = EXCLUDED.channel_id, channel_title = EXCLUDED.channel_title,
				refresh_token = EXCLUDED.refresh_token, scopes = EXCLUDED.scopes, connected_by_user_id = EXCLUDED.connected_by_user_id,
				connected_at = now(), sync_status = 'queued', sync_error = NULL`,
			orgID, ch.ID, ch.Title, sealed, strings.Join(scopes, " "), actorID); err != nil {
			return fmt.Errorf("save youtube connection: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindYouTubeSync, OrganisationID: orgID,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 3}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "youtube.connected", "organisation", orgID,
			map[string]any{"channel_id": ch.ID, "channel_title": ch.Title})
	})
	if err != nil {
		return orgID, err
	}
	s.queued()
	return orgID, nil
}

func (s *Service) queued() {
	if s.onJob != nil {
		s.onJob()
	}
}

// Disconnect removes the channel and its synced numbers
// (organisation.manage), and asks Google to revoke CAVI's access.
func (s *Service) Disconnect(ctx context.Context, actorID, orgID string) error {
	a, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return err
	}
	if !authz.Can(a.Role, authz.OrganisationManage) {
		return ErrForbidden
	}
	var sealed []byte
	err = s.db.QueryRowContext(ctx, `DELETE FROM youtube_connections WHERE organisation_id = ? RETURNING refresh_token`, orgID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotConnected
	}
	if err != nil {
		return fmt.Errorf("remove youtube connection: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM youtube_videos WHERE organisation_id = ?`, orgID); err != nil {
		return err
	}
	if err := account.Audit(ctx, s.db, orgID, actorID, "youtube.disconnected", "organisation", orgID, nil); err != nil {
		return err
	}
	// Best effort: the connection is gone from CAVI either way.
	if token, err := open(s.key, orgID, sealed); err == nil {
		s.client.Revoke(ctx, token)
	}
	return nil
}

// RequestSync queues reading the channel's numbers again (editing.manage).
func (s *Service) RequestSync(ctx context.Context, actorID, orgID string) (Connection, error) {
	a, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false)
	if err != nil {
		return Connection{}, err
	}
	if !authz.Can(a.Role, authz.EditingManage) {
		return Connection{}, ErrForbidden
	}
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx, `SELECT sync_status FROM youtube_connections WHERE organisation_id = ? FOR UPDATE`, orgID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotConnected
		}
		if err != nil {
			return err
		}
		if status == SyncQueued || status == SyncRunning {
			return ErrSyncing
		}
		if _, err := tx.ExecContext(ctx, `UPDATE youtube_connections SET sync_status = 'queued', sync_error = NULL WHERE organisation_id = ?`, orgID); err != nil {
			return err
		}
		_, err = jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindYouTubeSync, OrganisationID: orgID,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 3})
		return err
	})
	if err != nil {
		return Connection{}, err
	}
	s.queued()
	return s.Status(ctx, actorID, orgID)
}

// VideoStats is a synced upload and, if known, the clip it came from.
type VideoStats struct {
	VideoID     string
	Title       string
	PublishedAt time.Time
	DurationMS  int64
	Metrics
	RenderID   string
	RenderName string
	ProjectID  string
}

// Videos lists the synced uploads, newest first. Anyone in the
// organisation can see them.
func (s *Service) Videos(ctx context.Context, actorID, orgID string) ([]VideoStats, error) {
	if _, err := account.ResolveAccess(ctx, s.db, actorID, orgID, false); err != nil {
		return nil, err
	}
	return Stats(ctx, s.db, orgID)
}

// Stats lists an organisation's synced uploads, newest first. The caller
// has authorised access.
func Stats(ctx context.Context, db bun.IDB, orgID string) ([]VideoStats, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT v.video_id, v.title, v.published_at, v.duration_ms, v.views, v.watch_minutes, v.avg_view_seconds,
			v.avg_view_percentage, v.subscribers_gained, v.likes, v.shares,
			coalesce(r.id::text, ''), coalesce(r.name, ''), coalesce(r.project_id::text, '')
		FROM youtube_videos v LEFT JOIN renders r ON r.id = v.render_id AND r.organisation_id = v.organisation_id
		WHERE v.organisation_id = ? ORDER BY v.published_at DESC NULLS LAST, v.video_id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list youtube videos: %w", err)
	}
	defer rows.Close()
	out := []VideoStats{}
	for rows.Next() {
		var v VideoStats
		var published sql.NullTime
		if err := rows.Scan(&v.VideoID, &v.Title, &published, &v.DurationMS, &v.Views, &v.WatchMinutes, &v.AvgViewSeconds,
			&v.AvgViewPercentage, &v.SubscribersGained, &v.Likes, &v.Shares, &v.RenderID, &v.RenderName, &v.ProjectID); err != nil {
			return nil, err
		}
		v.PublishedAt = published.Time
		out = append(out, v)
	}
	return out, rows.Err()
}

// Revoke asks Google to withdraw a token; failures are ignored.
func (c *Client) Revoke(ctx context.Context, token string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.Replace(c.TokenURL, "/token", "/revoke", 1),
		strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resp, err := c.HTTP.Do(req); err == nil {
		resp.Body.Close()
	}
}

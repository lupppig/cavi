package youtube

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

// Messages shown when a sync stops.
const (
	msgRevoked = "YouTube access ended (it was revoked, or Google's testing-mode access expired after 7 days). Connect the channel again."
	msgChanged = "The Google account now has a different channel. Connect the channel again."
	msgGaveUp  = "Reading the channel's numbers kept failing. Try again later."
)

// Syncer handles youtube.sync jobs: it reads the connected channel's
// uploads and their numbers, stores them, and matches them to the clips
// made in CAVI. It runs in the media worker.
type Syncer struct {
	db     *bun.DB
	client *Client
	key    []byte
	logger *slog.Logger
	now    func() time.Time
}

// NewSyncer returns a Syncer.
func NewSyncer(db *bun.DB, client *Client, key []byte, logger *slog.Logger) *Syncer {
	return &Syncer{db: db, client: client, key: key, logger: logger, now: time.Now}
}

var _ jobs.Handler = (*Syncer)(nil)

// Process implements jobs.Handler. It is idempotent.
func (y *Syncer) Process(ctx context.Context, job jobs.Job) error {
	var channelID string
	var sealed []byte
	err := y.db.QueryRowContext(ctx, `SELECT channel_id, refresh_token FROM youtube_connections WHERE organisation_id = ?`,
		job.OrganisationID).Scan(&channelID, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // disconnected since
	}
	if err != nil {
		return fmt.Errorf("load youtube connection: %w", err)
	}
	if _, err := y.db.ExecContext(ctx, `UPDATE youtube_connections SET sync_status = 'running' WHERE organisation_id = ?`, job.OrganisationID); err != nil {
		return err
	}
	refresh, err := open(y.key, job.OrganisationID, sealed)
	if err != nil {
		return jobs.Permanent(err)
	}
	access, err := y.client.Access(ctx, refresh)
	if err != nil {
		return classify(err)
	}
	ch, err := y.client.MyChannel(ctx, access)
	if err != nil {
		return classify(err)
	}
	if ch.ID != channelID {
		return jobs.Permanent(errChannelChanged)
	}
	videos, err := y.client.Uploads(ctx, access, ch)
	if err != nil {
		return classify(err)
	}
	metrics, err := y.client.Analytics(ctx, access, y.now())
	if err != nil {
		return classify(err)
	}
	clips, err := y.clips(ctx, job.OrganisationID)
	if err != nil {
		return err
	}
	matched := Match(videos, clips)
	err = y.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, v := range videos {
			m := metrics[v.ID]
			// Views and likes are live; the rest arrives a day or two late.
			m.Views, m.Likes = max(m.Views, v.Views), max(m.Likes, v.Likes)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO youtube_videos (organisation_id, video_id, title, published_at, duration_ms, views, watch_minutes,
					avg_view_seconds, avg_view_percentage, subscribers_gained, likes, shares, render_id, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, '')::uuid, now())
				ON CONFLICT (organisation_id, video_id) DO UPDATE SET title = EXCLUDED.title, published_at = EXCLUDED.published_at,
					duration_ms = EXCLUDED.duration_ms, views = EXCLUDED.views, watch_minutes = EXCLUDED.watch_minutes,
					avg_view_seconds = EXCLUDED.avg_view_seconds, avg_view_percentage = EXCLUDED.avg_view_percentage,
					subscribers_gained = EXCLUDED.subscribers_gained, likes = EXCLUDED.likes, shares = EXCLUDED.shares,
					render_id = EXCLUDED.render_id, updated_at = now()`,
				job.OrganisationID, v.ID, v.Title, v.PublishedAt, v.DurationMS, m.Views, m.WatchMinutes, m.AvgViewSeconds,
				m.AvgViewPercentage, m.SubscribersGained, m.Likes, m.Shares, matched[v.ID]); err != nil {
				return fmt.Errorf("save youtube video: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE youtube_connections SET sync_status = 'idle', sync_error = NULL, synced_at = now(), channel_title = ?
			WHERE organisation_id = ?`, ch.Title, job.OrganisationID)
		return err
	})
	if err != nil {
		return err
	}
	y.logger.InfoContext(ctx, "youtube synced", slog.String("organisation_id", job.OrganisationID),
		slog.Int("videos", len(videos)), slog.Int("matched", len(matched)))
	return nil
}

var errChannelChanged = errors.New("youtube: the account's channel changed")

func classify(err error) error {
	var api *APIError
	if errors.Is(err, ErrRevoked) || (errors.As(err, &api) && api.Status == 403) {
		return jobs.Permanent(err)
	}
	return err
}

// Failed implements jobs.Handler.
func (y *Syncer) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgGaveUp
	var api *APIError
	switch {
	case errors.Is(err, ErrRevoked):
		msg = msgRevoked
	case errors.Is(err, errChannelChanged):
		msg = msgChanged
	case errors.As(err, &api) && api.Status == 403:
		msg = "YouTube refused to share the channel's numbers: " + api.Message
	}
	_, dbErr := y.db.ExecContext(ctx, `UPDATE youtube_connections SET sync_status = 'failed', sync_error = ? WHERE organisation_id = ?`,
		msg, job.OrganisationID)
	return dbErr
}

// Clip is a finished CAVI render a video may have come from.
type Clip struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	DurationMS int64
}

func (y *Syncer) clips(ctx context.Context, orgID string) ([]Clip, error) {
	rows, err := y.db.QueryContext(ctx, `
		SELECT id, name, created_at, coalesce(output_duration_ms, 0) FROM renders
		WHERE organisation_id = ? AND status = 'ready'`, orgID)
	if err != nil {
		return nil, fmt.Errorf("load clips: %w", err)
	}
	defer rows.Close()
	var out []Clip
	for rows.Next() {
		var c Clip
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt, &c.DurationMS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Match pairs uploads with the clips they were made from: the clip's name
// and the video's title must be the same (ignoring case, punctuation and
// emoji) or one contain the other, the clip made before the video was
// published, and its length within 2 seconds. Several candidates go to
// the closest length; a clip is matched to one video at most.
func Match(videos []Video, clips []Clip) map[string]string {
	out := map[string]string{}
	used := map[string]bool{}
	for _, v := range videos {
		title := normalise(v.Title)
		if title == "" {
			continue
		}
		best, bestDiff := "", int64(2001)
		for _, c := range clips {
			name := normalise(c.Name)
			if used[c.ID] || name == "" || !similar(title, name) {
				continue
			}
			if !v.PublishedAt.IsZero() && c.CreatedAt.After(v.PublishedAt.Add(time.Hour)) {
				continue
			}
			diff := v.DurationMS - c.DurationMS
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				best, bestDiff = c.ID, diff
			}
		}
		if best != "" {
			out[v.ID] = best
			used[best] = true
		}
	}
	return out
}

func similar(a, b string) bool {
	if a == b {
		return true
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	return len([]rune(short)) >= 12 && strings.Contains(long, short)
}

// normalise keeps letters and digits, lower case, single-spaced.
func normalise(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		default:
			space = true
		}
	}
	return b.String()
}

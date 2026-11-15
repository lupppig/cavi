package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/brief"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
)

// Clip length limits for highlight detection, in seconds.
const (
	MinClipSeconds = 5
	MaxClipSeconds = 600
)

// Highlight run statuses match transcript statuses.
const (
	HighlightsQueued  = "queued"
	HighlightsRunning = "running"
	HighlightsReady   = "ready"
	HighlightsFailed  = "failed"
)

// ErrHighlightsInProgress means a highlight run is already queued or running.
var ErrHighlightsInProgress = errors.New("media: highlight detection in progress")

// HighlightRequest is what the user asks highlight detection for.
type HighlightRequest struct {
	// Objective is the creative goal in the user's words; optional.
	Objective  string
	MinSeconds int
	MaxSeconds int
}

// Highlights is a file's current highlight run as seen by a caller.
type Highlights struct {
	ID            string
	MediaAssetID  string
	Status        string
	Provider      string
	Model         string
	ModelLabel    string
	PromptVersion string
	Objective     string
	MinMS         int64
	MaxMS         int64
	// UsedBrief reports whether the run followed the creator brief.
	UsedBrief   bool
	Error       string
	CreatedAt   time.Time
	CompletedAt time.Time
	// Clips are best first.
	Clips []Clip
}

// Clip is a candidate clip and the creator brief's rules (other than
// priorities) it touches, from the brief as it is now.
type Clip struct {
	ai.Highlight
	Flags []ai.BriefRule
	// Priorities are the brief's priority moments the clip features.
	Priorities []ai.BriefRule
}

func (r HighlightRequest) validate(durationMS int64) (HighlightRequest, error) {
	r.Objective = strings.TrimSpace(r.Objective)
	if !utf8.ValidString(r.Objective) || utf8.RuneCountInString(r.Objective) > ai.MaxBriefRunes {
		return r, invalid("objective", fmt.Sprintf("Keep the objective under %d characters.", ai.MaxBriefRunes))
	}
	if r.MinSeconds < MinClipSeconds {
		return r, invalid("min_seconds", fmt.Sprintf("Clips must be at least %d seconds.", MinClipSeconds))
	}
	if r.MaxSeconds > MaxClipSeconds {
		return r, invalid("max_seconds", fmt.Sprintf("Clips can be at most %d minutes.", MaxClipSeconds/60))
	}
	if r.MaxSeconds < r.MinSeconds+MinClipSeconds {
		return r, invalid("max_seconds", "The longest clip must be a few seconds longer than the shortest.")
	}
	if int64(r.MinSeconds)*1000 > durationMS {
		return r, invalid("min_seconds", "The shortest clip is longer than the file.")
	}
	return r, nil
}

// RequestHighlights queues highlight detection on a file's transcript
// (editing.manage). The transcript text, not the media, is sent to the
// organisation's chosen AI provider, which the audit log records.
// Requesting again after a run is ready or failed replaces it.
func (s *Service) RequestHighlights(ctx context.Context, actorID, orgID, projectID, assetID string, req HighlightRequest) (Highlights, error) {
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		a, err := s.load(ctx, tx, acc, actorID, orgID, projectID, assetID, true)
		if err != nil {
			return err
		}
		if a.Status != "uploaded" {
			return ErrNotFound
		}
		if !authz.Can(acc.Role, authz.EditingManage) {
			return ErrForbidden
		}
		var transcriptStatus string
		var lines int
		err = tx.QueryRowContext(ctx, `
			SELECT tr.status, (SELECT count(*) FROM transcript_segments WHERE transcript_id = tr.id)
			FROM transcripts tr WHERE tr.media_asset_id = ? AND tr.organisation_id = ?`, assetID, orgID).
			Scan(&transcriptStatus, &lines)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load transcript: %w", err)
		}
		switch {
		case transcriptStatus == TranscriptQueued || transcriptStatus == TranscriptRunning:
			return invalid("media", "Wait for the transcript to finish, then find highlights.")
		case transcriptStatus != TranscriptReady:
			return invalid("media", "Transcribe this file first. Highlights are found in its transcript.")
		case lines == 0:
			return invalid("media", "No speech was found in this file, so there’s nothing to find highlights in.")
		}
		if req, err = req.validate(a.DurationMS); err != nil {
			return err
		}

		var status string
		err = tx.QueryRowContext(ctx, `SELECT status FROM highlight_runs WHERE media_asset_id = ? FOR UPDATE`, assetID).Scan(&status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load highlights: %w", err)
		}
		if status == HighlightsQueued || status == HighlightsRunning {
			return ErrHighlightsInProgress
		}
		model, err := ai.Resolve(ctx, tx, orgID, ai.TaskHighlights)
		if err != nil {
			return err
		}
		// The brief is copied onto the run, so it runs (and retries) as
		// asked even if the brief changes meanwhile.
		camp, err := brief.Campaign(ctx, tx, orgID, assetID)
		if err != nil {
			return err
		}
		var campaign any
		if camp != nil {
			b, err := json.Marshal(camp)
			if err != nil {
				return err
			}
			campaign = string(b)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO highlight_runs (organisation_id, media_asset_id, status, provider, model, prompt_version,
				objective, min_ms, max_ms, requested_by_user_id, campaign)
			VALUES (?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?, ?::jsonb)
			ON CONFLICT (media_asset_id) DO UPDATE SET
				status = 'queued', provider = EXCLUDED.provider, model = EXCLUDED.model,
				prompt_version = EXCLUDED.prompt_version, objective = EXCLUDED.objective,
				min_ms = EXCLUDED.min_ms, max_ms = EXCLUDED.max_ms, campaign = EXCLUDED.campaign,
				requested_by_user_id = EXCLUDED.requested_by_user_id,
				error = NULL, updated_at = now(), created_at = now(), completed_at = NULL`,
			orgID, assetID, model.Provider, model.ID, ai.HighlightsPromptVersion, req.Objective,
			int64(req.MinSeconds)*1000, int64(req.MaxSeconds)*1000, actorID, campaign); err != nil {
			return fmt.Errorf("queue highlights: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaHighlights, OrganisationID: orgID,
			MediaAssetID: assetID, CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 6}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "media.highlights_requested", "media_asset", assetID,
			map[string]any{"provider": model.Provider, "model": model.ID, "brief": camp != nil})
	})
	if err != nil {
		return Highlights{}, err
	}
	s.jobQueued()
	return s.Highlights(ctx, actorID, orgID, projectID, assetID)
}

// Highlights returns a file's current highlight run, with clips once
// ready. Anyone who can see the project can read it.
func (s *Service) Highlights(ctx context.Context, actorID, orgID, projectID, assetID string) (Highlights, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Highlights{}, err
	}
	a, err := s.load(ctx, s.db, acc, actorID, orgID, projectID, assetID, false)
	if err != nil {
		return Highlights{}, err
	}
	if a.Status != "uploaded" {
		return Highlights{}, ErrNotFound
	}
	var h Highlights
	var completed sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT id, media_asset_id, status, provider, model, prompt_version, objective, min_ms, max_ms,
			campaign IS NOT NULL, coalesce(error, ''), created_at, completed_at
		FROM highlight_runs WHERE media_asset_id = ? AND organisation_id = ?`, assetID, orgID).
		Scan(&h.ID, &h.MediaAssetID, &h.Status, &h.Provider, &h.Model, &h.PromptVersion, &h.Objective,
			&h.MinMS, &h.MaxMS, &h.UsedBrief, &h.Error, &h.CreatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return Highlights{}, ErrNotFound
	}
	if err != nil {
		return Highlights{}, fmt.Errorf("load highlights: %w", err)
	}
	h.CompletedAt = completed.Time
	h.ModelLabel = h.Model
	if m, ok := ai.Lookup(ai.TaskHighlights, h.Provider, h.Model); ok {
		h.ModelLabel = m.Label
	}
	h.Clips = []Clip{}
	if h.Status != HighlightsReady {
		return h, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT first_segment, last_segment, start_ms, end_ms, title, reason, campaign_link
		FROM highlights WHERE highlight_run_id = ? ORDER BY position`, h.ID)
	if err != nil {
		return Highlights{}, fmt.Errorf("load highlights: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Clip
		if err := rows.Scan(&c.FirstSegment, &c.LastSegment, &c.StartMS, &c.EndMS, &c.Title, &c.Reason, &c.CampaignLink); err != nil {
			return Highlights{}, err
		}
		h.Clips = append(h.Clips, c)
	}
	if err := rows.Err(); err != nil {
		return Highlights{}, err
	}
	rules, err := brief.Rules(ctx, s.db, orgID, assetID)
	if err != nil {
		return Highlights{}, err
	}
	for i := range h.Clips {
		h.Clips[i].Flags = brief.Flags(rules, h.Clips[i].StartMS, h.Clips[i].EndMS)
		h.Clips[i].Priorities = brief.Priorities(rules, h.Clips[i].StartMS, h.Clips[i].EndMS)
	}
	return h, nil
}

// Highlighter handles media.highlights jobs in the media worker.
type Highlighter struct {
	db     *bun.DB
	ai     ai.Highlighter
	logger *slog.Logger
}

// NewHighlighter returns a highlight detection job handler.
func NewHighlighter(db *bun.DB, provider ai.Highlighter, logger *slog.Logger) *Highlighter {
	return &Highlighter{db: db, ai: provider, logger: logger}
}

var _ jobs.Handler = (*Highlighter)(nil)

// User-facing failure messages.
const (
	msgHighlightsRejected     = "The AI provider couldn’t find highlights in this transcript."
	msgHighlightsGaveUp       = "Finding highlights kept failing, so we stopped trying. You can try again."
	msgHighlightsNoTranscript = "The transcript changed while we were working. Find highlights again."
)

// errTranscriptGone means the transcript was replaced or removed after
// highlights were requested.
var errTranscriptGone = errors.New("media: transcript is no longer ready")

// Process implements jobs.Handler. It is idempotent: a rerun replaces the
// clips in one transaction.
func (h *Highlighter) Process(ctx context.Context, job jobs.Job) error {
	var runID, provider, modelID, objective string
	var minMS, maxMS int64
	var campaign []byte
	err := h.db.QueryRowContext(ctx, `
		SELECT id, provider, model, objective, min_ms, max_ms, campaign FROM highlight_runs
		WHERE media_asset_id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		job.MediaAssetID, job.OrganisationID).Scan(&runID, &provider, &modelID, &objective, &minMS, &maxMS, &campaign)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted, or superseded; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load highlight run: %w", err)
	}
	// The model was fixed when highlights were requested.
	model, ok := ai.Lookup(ai.TaskHighlights, provider, modelID)
	if !ok {
		return jobs.Permanent(fmt.Errorf("%w: model %s/%s is no longer supported", ai.ErrPermanent, provider, modelID))
	}
	segments, err := h.transcript(ctx, job)
	if err != nil {
		return err
	}
	if _, err := h.db.ExecContext(ctx, `UPDATE highlight_runs SET status = 'running', updated_at = now() WHERE id = ?`, runID); err != nil {
		return err
	}

	req := ai.HighlightBrief{Objective: objective, MinMS: minMS, MaxMS: maxMS}
	if len(campaign) > 0 {
		req.Campaign = &ai.Campaign{}
		if err := json.Unmarshal(campaign, req.Campaign); err != nil {
			return jobs.Permanent(fmt.Errorf("highlight run %s campaign: %w", runID, err))
		}
	}
	clips, usage, err := h.ai.Highlights(ctx, model, segments, req)
	h.recordUsage(ctx, job, model, usage, err == nil)
	if err != nil {
		if errors.Is(err, ai.ErrPermanent) {
			return jobs.Permanent(err)
		}
		return err // includes invalid output, which may pass on retry
	}
	// Clips that include content the creator's brief excludes can't be
	// used, so they aren't offered.
	rules, err := brief.Rules(ctx, h.db, job.OrganisationID, job.MediaAssetID)
	if err != nil {
		return err
	}
	kept := clips[:0]
	for _, c := range clips {
		if _, excluded := brief.Excluded(rules, c.StartMS, c.EndMS); !excluded {
			kept = append(kept, c)
		}
	}
	clips = kept

	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE highlight_runs SET status = 'ready', error = NULL, completed_at = now(), updated_at = now()
			WHERE id = ? AND status IN ('queued', 'running')`, runID)
		if err != nil {
			return fmt.Errorf("save highlights: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // deleted or replaced meanwhile
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM highlights WHERE highlight_run_id = ?`, runID); err != nil {
			return err
		}
		for i, c := range clips {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO highlights (highlight_run_id, position, start_ms, end_ms, first_segment, last_segment, title, reason, campaign_link)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				runID, i, c.StartMS, c.EndMS, c.FirstSegment, c.LastSegment, c.Title, c.Reason, c.CampaignLink); err != nil {
				return fmt.Errorf("save highlight: %w", err)
			}
		}
		return nil
	})
}

// transcript loads the file's ready transcript, in order.
func (h *Highlighter) transcript(ctx context.Context, job jobs.Job) ([]ai.Segment, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT s.start_ms, s.end_ms, s.speaker, s.text
		FROM transcripts tr JOIN transcript_segments s ON s.transcript_id = tr.id
		WHERE tr.media_asset_id = ? AND tr.organisation_id = ? AND tr.status = 'ready'
		ORDER BY s.position`, job.MediaAssetID, job.OrganisationID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	defer rows.Close()
	var segments []ai.Segment
	for rows.Next() {
		var s ai.Segment
		if err := rows.Scan(&s.StartMS, &s.EndMS, &s.Speaker, &s.Text); err != nil {
			return nil, err
		}
		segments = append(segments, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, jobs.Permanent(errTranscriptGone)
	}
	return segments, nil
}

func (h *Highlighter) recordUsage(ctx context.Context, job jobs.Job, model ai.Model, u ai.Usage, ok bool) {
	if _, err := h.db.ExecContext(ctx, `
		INSERT INTO ai_usage (organisation_id, job_id, task, provider, model, input_tokens, output_tokens, succeeded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		job.OrganisationID, job.ID, ai.TaskHighlights, model.Provider, model.ID, u.InputTokens, u.OutputTokens, ok); err != nil {
		h.logger.WarnContext(ctx, "record ai usage failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
	}
}

// Failed implements jobs.Handler.
func (h *Highlighter) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgHighlightsGaveUp
	switch {
	case errors.Is(err, errTranscriptGone):
		msg = msgHighlightsNoTranscript
	case errors.Is(err, ai.ErrPermanent):
		msg = msgHighlightsRejected
	}
	_, dbErr := h.db.ExecContext(ctx, `
		UPDATE highlight_runs SET status = 'failed', error = ?, updated_at = now()
		WHERE media_asset_id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		msg, job.MediaAssetID, job.OrganisationID)
	return dbErr
}

// Package brief holds creator briefs: a project's campaign direction and
// the timed rules every clip is checked against.
//
// A person pastes the creator's brief and says which video its timestamps
// refer to. AI reads it into rules (see ai.BriefReader) in a background
// job; people then review and edit them. Rules are data the backend
// enforces, not instructions to the AI: a clip that overlaps an exclude
// rule can't be made, and clips touching other rules carry flags.
package brief

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
)

// Brief statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusReady   = "ready"
	StatusFailed  = "failed"
)

// Errors returned to callers.
var (
	ErrNotFound   = account.ErrNotFound
	ErrForbidden  = account.ErrForbidden
	ErrInProgress = errors.New("brief: reading in progress")
)

// Rule is a timed rule on one video.
type Rule struct {
	MediaAssetID string
	ai.BriefRule
}

// Brief is a project's brief as seen by a caller.
type Brief struct {
	ID              string
	ProjectID       string
	MediaAssetID    string // empty once the video is deleted
	SourceText      string
	CampaignMessage string
	Guidance        string
	Status          string
	Provider        string
	Model           string
	ModelLabel      string
	PromptVersion   string
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     time.Time
	Rules           []Rule
	// CanEdit reports whether the caller may change the brief.
	CanEdit bool
}

// Service manages briefs. Every method authorises the actor itself.
type Service struct {
	db    *bun.DB
	onJob func()
}

// NewService returns a Service.
func NewService(db *bun.DB) *Service { return &Service{db: db} }

// OnJobQueued registers fn to run after a job is queued (typically
// jobs.Relay.Wake).
func (s *Service) OnJobQueued(fn func()) { s.onJob = fn }

func invalid(field, msg string) error { return &account.ValidationError{Field: field, Message: msg} }

// editable resolves the project for a change to its brief (project.update).
func editable(ctx context.Context, tx bun.Tx, actorID, orgID, projectID string) error {
	_, p, err := project.Resolve(ctx, tx, actorID, orgID, projectID, true)
	if err != nil {
		return err
	}
	if !slices.Contains(p.Permissions, authz.ProjectUpdate) {
		return ErrForbidden
	}
	return nil
}

// video returns the duration of a processed video or audio file in the
// project, or a validation error.
func video(ctx context.Context, db bun.IDB, orgID, projectID, assetID string) (int64, error) {
	bad := invalid("media_asset_id", "Choose a processed video or audio file from this project.")
	if !account.ValidUUID(assetID) {
		return 0, bad
	}
	var kind, status, processing string
	var duration int64
	err := db.QueryRowContext(ctx, `
		SELECT coalesce(kind, ''), status, processing_status, coalesce(duration_ms, 0)
		FROM media_assets WHERE id = ? AND project_id = ? AND organisation_id = ? FOR SHARE`,
		assetID, projectID, orgID).Scan(&kind, &status, &processing, &duration)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, bad
	}
	if err != nil {
		return 0, fmt.Errorf("load brief video: %w", err)
	}
	if (kind != "video" && kind != "audio") || status != "uploaded" || processing != "ready" || duration <= 0 {
		return 0, bad
	}
	return duration, nil
}

// Submit saves a pasted brief and queues it to be read into rules
// (project.update). The brief's text, not the media, is sent to the
// organisation's chosen AI provider. Submitting again replaces the brief
// and its rules.
func (s *Service) Submit(ctx context.Context, actorID, orgID, projectID, text, mediaAssetID string) (Brief, error) {
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > ai.MaxBriefTextRunes {
		return Brief{}, invalid("text", fmt.Sprintf("Paste a brief of up to %d characters.", ai.MaxBriefTextRunes))
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := editable(ctx, tx, actorID, orgID, projectID); err != nil {
			return err
		}
		if _, err := video(ctx, tx, orgID, projectID, mediaAssetID); err != nil {
			return err
		}
		var status string
		var hasVideo bool
		err := tx.QueryRowContext(ctx, `SELECT status, media_asset_id IS NOT NULL FROM briefs WHERE project_id = ? FOR UPDATE`,
			projectID).Scan(&status, &hasVideo)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load brief: %w", err)
		}
		// A brief whose video was deleted mid-read has lost its job: it can
		// be submitted again.
		if (status == StatusQueued || status == StatusRunning) && hasVideo {
			return ErrInProgress
		}
		model, err := ai.Resolve(ctx, tx, orgID, ai.TaskBriefs)
		if err != nil {
			return err
		}
		var id string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO briefs (organisation_id, project_id, media_asset_id, source_text, status, provider, model,
				prompt_version, requested_by_user_id, updated_by_user_id)
			VALUES (?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?)
			ON CONFLICT (project_id) DO UPDATE SET
				media_asset_id = EXCLUDED.media_asset_id, source_text = EXCLUDED.source_text,
				campaign_message = '', guidance = '', status = 'queued', provider = EXCLUDED.provider,
				model = EXCLUDED.model, prompt_version = EXCLUDED.prompt_version, error = NULL,
				requested_by_user_id = EXCLUDED.requested_by_user_id, updated_by_user_id = EXCLUDED.updated_by_user_id,
				created_at = now(), updated_at = now(), completed_at = NULL
			RETURNING id`,
			orgID, projectID, mediaAssetID, text, model.Provider, model.ID, ai.BriefsPromptVersion, actorID, actorID).
			Scan(&id); err != nil {
			if strings.Contains(err.Error(), "briefs_media_asset_key") {
				return invalid("media_asset_id", "Another brief already uses this file.")
			}
			return fmt.Errorf("save brief: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM brief_rules WHERE brief_id = ?`, id); err != nil {
			return err
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindBriefRead, OrganisationID: orgID,
			MediaAssetID: mediaAssetID, CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 5}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "brief.submitted", "project", projectID,
			map[string]any{"media_asset_id": mediaAssetID, "provider": model.Provider, "model": model.ID})
	})
	if err != nil {
		return Brief{}, err
	}
	if s.onJob != nil {
		s.onJob()
	}
	return s.Get(ctx, actorID, orgID, projectID)
}

// Edit is a reviewed brief: the campaign message, guidance and the whole
// list of rules, which replaces the current one.
type Edit struct {
	CampaignMessage string
	Guidance        string
	Rules           []Rule
}

// Update saves a person's review of a read brief (project.update). Rules
// must be on the brief's video and within it.
func (s *Service) Update(ctx context.Context, actorID, orgID, projectID string, e Edit) (Brief, error) {
	e.CampaignMessage = strings.TrimSpace(e.CampaignMessage)
	e.Guidance = strings.TrimSpace(e.Guidance)
	if utf8.RuneCountInString(e.CampaignMessage) > ai.MaxCampaignRunes || !utf8.ValidString(e.CampaignMessage) {
		return Brief{}, invalid("campaign_message", fmt.Sprintf("Keep the campaign message under %d characters.", ai.MaxCampaignRunes))
	}
	if utf8.RuneCountInString(e.Guidance) > ai.MaxCampaignRunes || !utf8.ValidString(e.Guidance) {
		return Brief{}, invalid("guidance", fmt.Sprintf("Keep the guidance under %d characters.", ai.MaxCampaignRunes))
	}
	if len(e.Rules) > ai.MaxBriefRules {
		return Brief{}, invalid("rules", fmt.Sprintf("A brief can have at most %d rules.", ai.MaxBriefRules))
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := editable(ctx, tx, actorID, orgID, projectID); err != nil {
			return err
		}
		var id, status string
		var assetID sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT id, status, media_asset_id FROM briefs WHERE project_id = ? AND organisation_id = ? FOR UPDATE`,
			projectID, orgID).Scan(&id, &status, &assetID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load brief: %w", err)
		}
		if status != StatusReady {
			return ErrInProgress
		}
		var duration int64
		if len(e.Rules) > 0 {
			if !assetID.Valid {
				return invalid("rules", "The brief’s video was deleted. Submit the brief again with another file.")
			}
			if duration, err = video(ctx, tx, orgID, projectID, assetID.String); err != nil {
				return err
			}
		}
		for i := range e.Rules {
			r := &e.Rules[i]
			r.Note = strings.TrimSpace(r.Note)
			field := fmt.Sprintf("rules[%d]", i)
			switch {
			case r.MediaAssetID != assetID.String:
				return invalid(field+".media_asset_id", "Rules must be on the brief’s video.")
			case !ai.ValidRuleKind(r.Kind):
				return invalid(field+".kind", "Choose priority, exclude, blur, mute, caution or review.")
			case r.StartMS < 0 || r.StartMS >= duration:
				return invalid(field+".start_ms", "The rule must start within the video.")
			case r.EndMS < r.StartMS || r.EndMS > duration:
				return invalid(field+".end_ms", "The rule must end after it starts, within the video.")
			case r.Note == "" || !utf8.ValidString(r.Note) || utf8.RuneCountInString(r.Note) > ai.MaxRuleNoteRunes:
				return invalid(field+".note", fmt.Sprintf("Describe the rule in up to %d characters.", ai.MaxRuleNoteRunes))
			}
		}
		slices.SortStableFunc(e.Rules, func(a, b Rule) int { return cmp.Compare(a.StartMS, b.StartMS) })
		if _, err := tx.ExecContext(ctx, `
			UPDATE briefs SET campaign_message = ?, guidance = ?, updated_by_user_id = ?, updated_at = now() WHERE id = ?`,
			e.CampaignMessage, e.Guidance, actorID, id); err != nil {
			return fmt.Errorf("save brief: %w", err)
		}
		if err := saveRules(ctx, tx, orgID, id, assetID.String, toAI(e.Rules)); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "brief.updated", "project", projectID,
			map[string]any{"rules": len(e.Rules)})
	})
	if err != nil {
		return Brief{}, err
	}
	return s.Get(ctx, actorID, orgID, projectID)
}

func toAI(rs []Rule) []ai.BriefRule {
	out := make([]ai.BriefRule, len(rs))
	for i, r := range rs {
		out[i] = r.BriefRule
	}
	return out
}

func saveRules(ctx context.Context, tx bun.Tx, orgID, briefID, assetID string, rules []ai.BriefRule) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM brief_rules WHERE brief_id = ?`, briefID); err != nil {
		return fmt.Errorf("replace rules: %w", err)
	}
	for i, r := range rules {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO brief_rules (brief_id, position, organisation_id, media_asset_id, kind, start_ms, end_ms, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			briefID, i, orgID, assetID, r.Kind, r.StartMS, r.EndMS, r.Note); err != nil {
			return fmt.Errorf("save rule: %w", err)
		}
	}
	return nil
}

// Delete removes a project's brief and its rules (project.update).
func (s *Service) Delete(ctx context.Context, actorID, orgID, projectID string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := editable(ctx, tx, actorID, orgID, projectID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM briefs WHERE project_id = ? AND organisation_id = ?`, projectID, orgID)
		if err != nil {
			return fmt.Errorf("delete brief: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return account.Audit(ctx, tx, orgID, actorID, "brief.deleted", "project", projectID, nil)
	})
}

// Get returns a project's brief. Anyone who can see the project can read it.
func (s *Service) Get(ctx context.Context, actorID, orgID, projectID string) (Brief, error) {
	_, p, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Brief{}, err
	}
	var b Brief
	var assetID sql.NullString
	var completed sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT id, project_id, media_asset_id, source_text, campaign_message, guidance, status, provider, model,
			prompt_version, coalesce(error, ''), created_at, updated_at, completed_at
		FROM briefs WHERE project_id = ? AND organisation_id = ?`, projectID, orgID).
		Scan(&b.ID, &b.ProjectID, &assetID, &b.SourceText, &b.CampaignMessage, &b.Guidance, &b.Status, &b.Provider,
			&b.Model, &b.PromptVersion, &b.Error, &b.CreatedAt, &b.UpdatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return Brief{}, ErrNotFound
	}
	if err != nil {
		return Brief{}, fmt.Errorf("load brief: %w", err)
	}
	b.MediaAssetID = assetID.String
	b.CompletedAt = completed.Time
	if !assetID.Valid && (b.Status == StatusQueued || b.Status == StatusRunning) {
		// The video was deleted mid-read, taking its job with it.
		b.Status, b.Error = StatusFailed, msgVideoGone
	}
	b.CanEdit = slices.Contains(p.Permissions, authz.ProjectUpdate)
	b.ModelLabel = b.Model
	if m, ok := ai.Lookup(ai.TaskBriefs, b.Provider, b.Model); ok {
		b.ModelLabel = m.Label
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT media_asset_id, kind, start_ms, end_ms, note FROM brief_rules WHERE brief_id = ? ORDER BY position`, b.ID)
	if err != nil {
		return Brief{}, fmt.Errorf("load rules: %w", err)
	}
	defer rows.Close()
	b.Rules = []Rule{}
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.MediaAssetID, &r.Kind, &r.StartMS, &r.EndMS, &r.Note); err != nil {
			return Brief{}, err
		}
		b.Rules = append(b.Rules, r)
	}
	return b, rows.Err()
}

// Rules returns the rules on a video, in time order. Callers must already
// have authorised access to the video.
func Rules(ctx context.Context, db bun.IDB, orgID, assetID string) ([]ai.BriefRule, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.kind, r.start_ms, r.end_ms, r.note FROM brief_rules r JOIN briefs b ON b.id = r.brief_id
		WHERE r.media_asset_id = ? AND r.organisation_id = ? AND b.status = 'ready'
		ORDER BY r.start_ms, r.position`, assetID, orgID)
	if err != nil {
		return nil, fmt.Errorf("load brief rules: %w", err)
	}
	defer rows.Close()
	var out []ai.BriefRule
	for rows.Next() {
		var r ai.BriefRule
		if err := rows.Scan(&r.Kind, &r.StartMS, &r.EndMS, &r.Note); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FlagWindowMS widens a single-moment flag: briefs name the moment
// something appears, but a licence plate or logo usually stays on screen
// for a while around it.
const FlagWindowMS = 10_000

// Flags returns the rules a clip [startMS, endMS) touches, except
// priority rules, which aren't warnings. Single-moment rules count when the
// clip comes within FlagWindowMS of them; exclusions are hard limits on
// the times written and are checked exactly (see Excluded), but are still
// flagged when near.
func Flags(rules []ai.BriefRule, startMS, endMS int64) []ai.BriefRule {
	var out []ai.BriefRule
	for _, r := range rules {
		if r.Kind == ai.RulePriority {
			continue
		}
		w := r
		if r.StartMS == r.EndMS {
			w.StartMS, w.EndMS = max(0, r.StartMS-FlagWindowMS), r.EndMS+FlagWindowMS
		}
		if w.Overlaps(startMS, endMS) {
			out = append(out, r)
		}
	}
	return out
}

// Excluded returns the first exclude rule the clip touches, if any.
func Excluded(rules []ai.BriefRule, startMS, endMS int64) (ai.BriefRule, bool) {
	for _, r := range rules {
		if r.Kind == ai.RuleExclude && r.Overlaps(startMS, endMS) {
			return r, true
		}
	}
	return ai.BriefRule{}, false
}

// Clock formats milliseconds as M:SS (or H:MM:SS), as briefs write times.
func Clock(ms int64) string {
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// ExcludedError is the message shown when a clip includes excluded content.
func ExcludedError(field string, r ai.BriefRule) error {
	at := Clock(r.StartMS)
	if r.EndMS > r.StartMS {
		at += "–" + Clock(r.EndMS)
	}
	return invalid(field, fmt.Sprintf("This clip includes %s, which the brief says must not be used (%s).", at, r.Note))
}

// Reader handles brief.read jobs in the media worker.
type Reader struct {
	db     *bun.DB
	ai     ai.BriefReader
	logger *slog.Logger
}

// NewReader returns a brief reading job handler.
func NewReader(db *bun.DB, provider ai.BriefReader, logger *slog.Logger) *Reader {
	return &Reader{db: db, ai: provider, logger: logger}
}

var _ jobs.Handler = (*Reader)(nil)

// User-facing failure messages.
const (
	msgRejected  = "The AI provider couldn’t read this brief."
	msgGaveUp    = "Reading the brief kept failing, so we stopped trying. Check its timestamps are within the video, then submit it again."
	msgVideoGone = "The brief’s video was deleted or changed. Submit the brief again."
)

var errVideoGone = errors.New("brief: video is gone")

// Process implements jobs.Handler. It is idempotent: a rerun replaces the
// rules in one transaction.
func (r *Reader) Process(ctx context.Context, job jobs.Job) error {
	var id, projectID, text, provider, modelID string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, source_text, provider, model FROM briefs
		WHERE media_asset_id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		job.MediaAssetID, job.OrganisationID).Scan(&id, &projectID, &text, &provider, &modelID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted, or replaced; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load brief: %w", err)
	}
	// The model was fixed when the brief was submitted.
	model, ok := ai.Lookup(ai.TaskBriefs, provider, modelID)
	if !ok {
		return jobs.Permanent(fmt.Errorf("%w: model %s/%s is no longer supported", ai.ErrPermanent, provider, modelID))
	}
	duration, err := video(ctx, r.db, job.OrganisationID, projectID, job.MediaAssetID)
	var ve *account.ValidationError
	if errors.As(err, &ve) {
		return jobs.Permanent(errVideoGone)
	}
	if err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE briefs SET status = 'running', updated_at = now() WHERE id = ?`, id); err != nil {
		return err
	}

	b, usage, err := r.ai.ReadBrief(ctx, model, text, duration)
	r.recordUsage(ctx, job, model, usage, err == nil)
	if err != nil {
		if errors.Is(err, ai.ErrPermanent) {
			return jobs.Permanent(err)
		}
		return err // includes invalid output, which may pass on retry
	}

	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE briefs SET status = 'ready', campaign_message = ?, guidance = ?, error = NULL,
				completed_at = now(), updated_at = now()
			WHERE id = ? AND media_asset_id = ? AND status IN ('queued', 'running')`,
			b.CampaignMessage, b.Guidance, id, job.MediaAssetID)
		if err != nil {
			return fmt.Errorf("save brief: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // deleted or replaced meanwhile
		}
		return saveRules(ctx, tx, job.OrganisationID, id, job.MediaAssetID, b.Rules)
	})
}

func (r *Reader) recordUsage(ctx context.Context, job jobs.Job, model ai.Model, u ai.Usage, ok bool) {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO ai_usage (organisation_id, job_id, task, provider, model, input_tokens, output_tokens, succeeded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		job.OrganisationID, job.ID, ai.TaskBriefs, model.Provider, model.ID, u.InputTokens, u.OutputTokens, ok); err != nil {
		r.logger.WarnContext(ctx, "record ai usage failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
	}
}

// Failed implements jobs.Handler.
func (r *Reader) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgGaveUp
	switch {
	case errors.Is(err, errVideoGone):
		msg = msgVideoGone
	case errors.Is(err, ai.ErrPermanent):
		msg = msgRejected
	}
	_, dbErr := r.db.ExecContext(ctx, `
		UPDATE briefs SET status = 'failed', error = ?, updated_at = now()
		WHERE media_asset_id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		msg, job.MediaAssetID, job.OrganisationID)
	return dbErr
}

// Campaign returns what highlight detection needs from the brief on a
// video, or nil if the video has no ready brief. Callers must already have
// authorised access to the video.
func Campaign(ctx context.Context, db bun.IDB, orgID, assetID string) (*ai.Campaign, error) {
	var c ai.Campaign
	err := db.QueryRowContext(ctx, `
		SELECT campaign_message, guidance FROM briefs
		WHERE media_asset_id = ? AND organisation_id = ? AND status = 'ready'`, assetID, orgID).Scan(&c.Message, &c.Guidance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load brief: %w", err)
	}
	if c.Rules, err = Rules(ctx, db, orgID, assetID); err != nil {
		return nil, err
	}
	if c.Rules == nil {
		c.Rules = []ai.BriefRule{}
	}
	return &c, nil
}

// Priorities returns the priority rules a clip [startMS, endMS] features.
func Priorities(rules []ai.BriefRule, startMS, endMS int64) []ai.BriefRule {
	var out []ai.BriefRule
	for _, r := range rules {
		if r.Kind == ai.RulePriority && r.Overlaps(startMS, endMS+1) {
			out = append(out, r)
		}
	}
	return out
}

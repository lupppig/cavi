// Package plan proposes edit plans: montages an AI creative director
// suggests from a project's transcripts and creator brief, for people to
// review, change and render.
package plan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
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
	"github.com/lupppig/cavi/apps/api/internal/youtube"
)

// Statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusReady   = "ready"
	StatusFailed  = "failed"
)

// Length limits, in seconds.
const (
	MinSeconds = 10
	MaxSeconds = 180
)

// Errors returned to callers.
var (
	ErrNotFound   = account.ErrNotFound
	ErrForbidden  = account.ErrForbidden
	ErrInProgress = errors.New("plan: planning in progress")
)

// Request is what people ask the planner for.
type Request struct {
	Objective  string
	MinSeconds int
	MaxSeconds int
	// MediaAssetIDs are the files to plan from; empty means the project's
	// transcribed files (up to ai.MaxPlanFiles, newest first).
	MediaAssetIDs []string
	// Series asks for one story in consecutive parts.
	Series bool
}

// Segment is a stretch of a file in a plan.
type Segment struct {
	SourceAssetID string `json:"source_asset_id"`
	StartMS       int64  `json:"start_ms"`
	EndMS         int64  `json:"end_ms"`
	FirstSegment  int    `json:"first_segment"`
	LastSegment   int    `json:"last_segment"`
	// Flags are the brief's rules (other than priorities) it touches, from
	// the brief as it is now.
	Flags []ai.BriefRule `json:"-"`
}

// Plan is a proposed montage.
type Plan struct {
	Title           string
	Hook            string
	VoiceoverScript string
	Reason          string
	CampaignLink    string
	Segments        []Segment
	// Part (from 1) and SeriesTitle are set for the parts of a series.
	Part        int
	SeriesTitle string
}

// Run is a project's current plans as seen by a caller.
type Run struct {
	ID            string
	ProjectID     string
	Status        string
	Provider      string
	Model         string
	ModelLabel    string
	PromptVersion string
	Objective     string
	MinMS, MaxMS  int64
	// Series: the plans are the parts of one series.
	Series        bool
	MediaAssetIDs []string
	UsedBrief     bool
	Error         string
	CreatedAt     time.Time
	CompletedAt   time.Time
	Plans         []Plan
}

// snapshot is the brief as copied onto a run.
type snapshot struct {
	Campaign *ai.Campaign              `json:"campaign"`
	Rules    map[string][]ai.BriefRule `json:"rules"`
}

// Service manages plans. Every method authorises the actor itself.
type Service struct {
	db    *bun.DB
	onJob func()
}

// NewService returns a Service.
func NewService(db *bun.DB) *Service { return &Service{db: db} }

// OnJobQueued registers fn to run after a job is queued.
func (s *Service) OnJobQueued(fn func()) { s.onJob = fn }

func invalid(field, msg string) error { return &account.ValidationError{Field: field, Message: msg} }

// Request queues planning (editing.manage). The transcripts' text and the
// brief, not the media, are sent to the organisation's chosen model.
// Requesting again after a run is ready or failed replaces it.
func (s *Service) Request(ctx context.Context, actorID, orgID, projectID string, req Request) (Run, error) {
	req.Objective = strings.TrimSpace(req.Objective)
	if !utf8.ValidString(req.Objective) || utf8.RuneCountInString(req.Objective) > ai.MaxBriefRunes {
		return Run{}, invalid("objective", fmt.Sprintf("Keep the objective under %d characters.", ai.MaxBriefRunes))
	}
	if req.MinSeconds < MinSeconds || req.MaxSeconds > MaxSeconds || req.MaxSeconds < req.MinSeconds+5 {
		return Run{}, invalid("max_seconds", fmt.Sprintf("Choose lengths between %d seconds and %d minutes, the longest at least 5 seconds more than the shortest.", MinSeconds, MaxSeconds/60))
	}
	if len(req.MediaAssetIDs) > ai.MaxPlanFiles {
		return Run{}, invalid("media_asset_ids", fmt.Sprintf("Plan from at most %d files.", ai.MaxPlanFiles))
	}
	var runID string
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		if !authz.Can(acc.Role, authz.EditingManage) {
			return ErrForbidden
		}
		ids, err := transcribed(ctx, tx, orgID, projectID, req.MediaAssetIDs)
		if err != nil {
			return err
		}
		var status string
		err = tx.QueryRowContext(ctx, `SELECT status FROM plan_runs WHERE project_id = ? FOR UPDATE`, projectID).Scan(&status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load plans: %w", err)
		}
		if status == StatusQueued || status == StatusRunning {
			return ErrInProgress
		}
		model, err := ai.Resolve(ctx, tx, orgID, ai.TaskPlans)
		if err != nil {
			return err
		}
		snap, err := briefSnapshot(ctx, tx, orgID, projectID, ids)
		if err != nil {
			return err
		}
		var campaign any
		if snap != nil {
			b, err := json.Marshal(snap)
			if err != nil {
				return err
			}
			campaign = string(b)
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO plan_runs (organisation_id, project_id, status, provider, model, prompt_version, objective,
				min_ms, max_ms, media_asset_ids, campaign, requested_by_user_id, series)
			VALUES (?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?::uuid[], ?::jsonb, ?, ?)
			ON CONFLICT (project_id) DO UPDATE SET
				status = 'queued', provider = EXCLUDED.provider, model = EXCLUDED.model,
				prompt_version = EXCLUDED.prompt_version, objective = EXCLUDED.objective, min_ms = EXCLUDED.min_ms,
				max_ms = EXCLUDED.max_ms, media_asset_ids = EXCLUDED.media_asset_ids, campaign = EXCLUDED.campaign,
				requested_by_user_id = EXCLUDED.requested_by_user_id, series = EXCLUDED.series, error = NULL,
				created_at = now(), updated_at = now(), completed_at = NULL
			RETURNING id`,
			orgID, projectID, model.Provider, model.ID, ai.PlansPromptVersion, req.Objective,
			int64(req.MinSeconds)*1000, int64(req.MaxSeconds)*1000, "{"+strings.Join(ids, ",")+"}", campaign, actorID, req.Series).Scan(&runID); err != nil {
			return fmt.Errorf("queue plans: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM edit_plans WHERE plan_run_id = ?`, runID); err != nil {
			return err
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindPlansCreate, OrganisationID: orgID, SubjectID: runID,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 5}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "plans.requested", "project", projectID,
			map[string]any{"files": len(ids), "brief": snap != nil, "series": req.Series, "provider": model.Provider, "model": model.ID})
	})
	if err != nil {
		return Run{}, err
	}
	if s.onJob != nil {
		s.onJob()
	}
	return s.Get(ctx, actorID, orgID, projectID)
}

// transcribed checks the files to plan from are processed videos (what
// montages are made of) with ready transcripts with speech, or picks the
// project's newest such files.
func transcribed(ctx context.Context, tx bun.Tx, orgID, projectID string, ids []string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT m.id::text FROM media_assets m JOIN transcripts tr ON tr.media_asset_id = m.id
		WHERE m.project_id = ? AND m.organisation_id = ? AND m.status = 'uploaded' AND tr.status = 'ready'
			AND m.kind = 'video' AND m.processing_status = 'ready'
			AND EXISTS (SELECT 1 FROM transcript_segments s WHERE s.transcript_id = tr.id)
		ORDER BY m.created_at DESC`, projectID, orgID)
	if err != nil {
		return nil, fmt.Errorf("load transcribed files: %w", err)
	}
	defer rows.Close()
	ready := map[string]bool{}
	var newest []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ready[id] = true
		newest = append(newest, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		if len(newest) == 0 {
			return nil, invalid("media_asset_ids", "Transcribe a video in this project first. Plans are made from video transcripts.")
		}
		return newest[:min(len(newest), ai.MaxPlanFiles)], nil
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !ready[id] || seen[id] {
			return nil, invalid("media_asset_ids", "Choose different videos from this project that have finished transcripts with speech.")
		}
		seen[id] = true
	}
	return ids, nil
}

// briefSnapshot copies the project's ready brief, and the rules on each
// file, or returns nil when the project has no ready brief.
func briefSnapshot(ctx context.Context, db bun.IDB, orgID, projectID string, ids []string) (*snapshot, error) {
	c := &ai.Campaign{Rules: []ai.BriefRule{}}
	err := db.QueryRowContext(ctx, `
		SELECT campaign_message, guidance FROM briefs WHERE project_id = ? AND organisation_id = ? AND status = 'ready'`,
		projectID, orgID).Scan(&c.Message, &c.Guidance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load brief: %w", err)
	}
	snap := &snapshot{Campaign: c, Rules: map[string][]ai.BriefRule{}}
	for _, id := range ids {
		rules, err := brief.Rules(ctx, db, orgID, id)
		if err != nil {
			return nil, err
		}
		snap.Rules[id] = rules
	}
	return snap, nil
}

// Get returns the project's current plans. Anyone who can see the project
// can read them.
func (s *Service) Get(ctx context.Context, actorID, orgID, projectID string) (Run, error) {
	if _, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false); err != nil {
		return Run{}, err
	}
	var r Run
	var ids string
	var completed sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, status, provider, model, prompt_version, objective, min_ms, max_ms, media_asset_ids::text,
			campaign IS NOT NULL, coalesce(error, ''), created_at, completed_at, series
		FROM plan_runs WHERE project_id = ? AND organisation_id = ?`, projectID, orgID).
		Scan(&r.ID, &r.ProjectID, &r.Status, &r.Provider, &r.Model, &r.PromptVersion, &r.Objective, &r.MinMS, &r.MaxMS,
			&ids, &r.UsedBrief, &r.Error, &r.CreatedAt, &completed, &r.Series)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("load plans: %w", err)
	}
	r.CompletedAt = completed.Time
	r.MediaAssetIDs = strings.Split(strings.Trim(ids, "{}"), ",")
	r.ModelLabel = r.Model
	if m, ok := ai.Lookup(ai.TaskPlans, r.Provider, r.Model); ok {
		r.ModelLabel = m.Label
	}
	r.Plans = []Plan{}
	if r.Status != StatusReady {
		return r, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT title, hook, voiceover_script, reason, campaign_link, segments, coalesce(part, 0), series_title FROM edit_plans
		WHERE plan_run_id = ? ORDER BY position`, r.ID)
	if err != nil {
		return Run{}, fmt.Errorf("load plans: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p Plan
		var segs []byte
		if err := rows.Scan(&p.Title, &p.Hook, &p.VoiceoverScript, &p.Reason, &p.CampaignLink, &segs, &p.Part, &p.SeriesTitle); err != nil {
			return Run{}, err
		}
		if err := json.Unmarshal(segs, &p.Segments); err != nil {
			return Run{}, fmt.Errorf("plan segments: %w", err)
		}
		r.Plans = append(r.Plans, p)
	}
	if err := rows.Err(); err != nil {
		return Run{}, err
	}
	rules := map[string][]ai.BriefRule{}
	for i := range r.Plans {
		for j := range r.Plans[i].Segments {
			sg := &r.Plans[i].Segments[j]
			rr, ok := rules[sg.SourceAssetID]
			if !ok {
				if rr, err = brief.Rules(ctx, s.db, orgID, sg.SourceAssetID); err != nil {
					return Run{}, err
				}
				rules[sg.SourceAssetID] = rr
			}
			sg.Flags = brief.Flags(rr, sg.StartMS, sg.EndMS)
		}
	}
	return r, nil
}

// Planner handles plans.create jobs in the media worker.
type Planner struct {
	db     *bun.DB
	ai     ai.Planner
	logger *slog.Logger
}

// NewPlanner returns a planning job handler.
func NewPlanner(db *bun.DB, provider ai.Planner, logger *slog.Logger) *Planner {
	return &Planner{db: db, ai: provider, logger: logger}
}

var _ jobs.Handler = (*Planner)(nil)

// User-facing failure messages.
const (
	msgRejected = "The AI provider couldn’t plan from these transcripts."
	msgGaveUp   = "Planning kept failing, so we stopped trying. You can try again."
	msgGone     = "A file or transcript changed while we were planning. Plan again."
)

var errGone = errors.New("plan: a file or transcript is gone")

// Process implements jobs.Handler. It is idempotent: a rerun replaces the
// plans in one transaction.
func (p *Planner) Process(ctx context.Context, job jobs.Job) error {
	var projectID, provider, modelID, objective, ids string
	var series bool
	var minMS, maxMS int64
	var campaign []byte
	err := p.db.QueryRowContext(ctx, `
		SELECT project_id, provider, model, objective, min_ms, max_ms, media_asset_ids::text, campaign, series FROM plan_runs
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`, job.SubjectID, job.OrganisationID).
		Scan(&projectID, &provider, &modelID, &objective, &minMS, &maxMS, &ids, &campaign, &series)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted or replaced; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load plan run: %w", err)
	}
	model, ok := ai.Lookup(ai.TaskPlans, provider, modelID)
	if !ok {
		return jobs.Permanent(fmt.Errorf("%w: model %s/%s is no longer supported", ai.ErrPermanent, provider, modelID))
	}
	brf := ai.PlanBrief{Objective: objective, MinMS: minMS, MaxMS: maxMS, Series: series}
	if brf.Channel, err = channelResults(ctx, p.db, job.OrganisationID); err != nil {
		return err
	}
	if len(campaign) > 0 {
		var snap snapshot
		if err := json.Unmarshal(campaign, &snap); err != nil {
			return jobs.Permanent(fmt.Errorf("plan run %s brief: %w", job.SubjectID, err))
		}
		brf.Campaign, brf.Rules = snap.Campaign, snap.Rules
	}
	var files []ai.PlanFile
	for _, id := range strings.Split(strings.Trim(ids, "{}"), ",") {
		f, err := p.file(ctx, job.OrganisationID, projectID, id)
		if err != nil {
			return err
		}
		files = append(files, f)
	}
	if _, err := p.db.ExecContext(ctx, `UPDATE plan_runs SET status = 'running', updated_at = now() WHERE id = ?`, job.SubjectID); err != nil {
		return err
	}
	plans, usage, err := p.ai.Plans(ctx, model, files, brf)
	p.recordUsage(ctx, job, model, usage, err == nil)
	if err != nil {
		if errors.Is(err, ai.ErrPermanent) {
			return jobs.Permanent(err)
		}
		return err
	}
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE plan_runs SET status = 'ready', error = NULL, completed_at = now(), updated_at = now()
			WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`, job.SubjectID, job.OrganisationID)
		if err != nil {
			return fmt.Errorf("save plans: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM edit_plans WHERE plan_run_id = ?`, job.SubjectID); err != nil {
			return err
		}
		for i, pl := range plans {
			segs := make([]Segment, len(pl.Segments))
			for j, sg := range pl.Segments {
				segs[j] = Segment{SourceAssetID: files[sg.File].ID, StartMS: sg.StartMS, EndMS: sg.EndMS,
					FirstSegment: sg.FirstSegment, LastSegment: sg.LastSegment}
			}
			b, err := json.Marshal(segs)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO edit_plans (plan_run_id, position, title, hook, voiceover_script, reason, campaign_link, segments, part, series_title)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?::jsonb, NULLIF(?, 0), ?)`,
				job.SubjectID, i, pl.Title, pl.Hook, pl.VoiceoverScript, pl.Reason, pl.CampaignLink, string(b), pl.Part, pl.SeriesTitle); err != nil {
				return fmt.Errorf("save plan: %w", err)
			}
		}
		return nil
	})
}

// file loads one file's name and ready transcript, in the job's tenant.
func (p *Planner) file(ctx context.Context, orgID, projectID, id string) (ai.PlanFile, error) {
	f := ai.PlanFile{ID: id}
	err := p.db.QueryRowContext(ctx, `
		SELECT filename FROM media_assets WHERE id = ? AND project_id = ? AND organisation_id = ? AND status = 'uploaded'`,
		id, projectID, orgID).Scan(&f.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return f, jobs.Permanent(errGone)
	}
	if err != nil {
		return f, err
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT s.start_ms, s.end_ms, s.speaker, s.text
		FROM transcripts tr JOIN transcript_segments s ON s.transcript_id = tr.id
		WHERE tr.media_asset_id = ? AND tr.organisation_id = ? AND tr.status = 'ready'
		ORDER BY s.position`, id, orgID)
	if err != nil {
		return f, fmt.Errorf("load transcript: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sg ai.Segment
		if err := rows.Scan(&sg.StartMS, &sg.EndMS, &sg.Speaker, &sg.Text); err != nil {
			return f, err
		}
		f.Segments = append(f.Segments, sg)
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	if len(f.Segments) == 0 {
		return f, jobs.Permanent(errGone)
	}
	return f, nil
}

func (p *Planner) recordUsage(ctx context.Context, job jobs.Job, model ai.Model, u ai.Usage, ok bool) {
	if _, err := p.db.ExecContext(ctx, `
		INSERT INTO ai_usage (organisation_id, job_id, task, provider, model, input_tokens, output_tokens, succeeded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		job.OrganisationID, job.ID, ai.TaskPlans, model.Provider, model.ID, u.InputTokens, u.OutputTokens, ok); err != nil {
		p.logger.WarnContext(ctx, "record ai usage failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
	}
}

// Failed implements jobs.Handler.
func (p *Planner) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgGaveUp
	switch {
	case errors.Is(err, errGone):
		msg = msgGone
	case errors.Is(err, ai.ErrPermanent):
		msg = msgRejected
	}
	_, dbErr := p.db.ExecContext(ctx, `
		UPDATE plan_runs SET status = 'failed', error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`, msg, job.SubjectID, job.OrganisationID)
	return dbErr
}

// Channel results the planner learns from: uploads with at least
// minViews views, the best held first, at most maxChannelVideos.
const (
	minViews         = 50
	maxChannelVideos = 12
)

// channelResults is how the organisation's connected channel's uploads
// did, for the planner; nil without a channel or enough views.
func channelResults(ctx context.Context, db bun.IDB, orgID string) ([]ai.ChannelVideo, error) {
	stats, err := youtube.Stats(ctx, db, orgID)
	if err != nil {
		return nil, err
	}
	var out []ai.ChannelVideo
	for _, v := range stats {
		// Views are live; the share watched arrives a day or two later.
		if v.Views >= minViews && v.AvgViewPercentage > 0 {
			out = append(out, ai.ChannelVideo{Title: v.Title, DurationMS: v.DurationMS, Views: v.Views,
				AvgViewPercentage: v.AvgViewPercentage, SubscribersGained: v.SubscribersGained})
		}
	}
	if len(out) < ai.MinChannelVideos {
		return nil, nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AvgViewPercentage > out[j].AvgViewPercentage })
	if len(out) > maxChannelVideos {
		out = out[:maxChannelVideos]
	}
	return out, nil
}

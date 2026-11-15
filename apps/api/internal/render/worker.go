package render

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/brief"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/storage"
	"github.com/lupppig/cavi/apps/api/internal/voiceover"
)

// Clipper is what rendering needs from FFmpeg. *ffmpeg.Runner implements it.
type Clipper interface {
	Clip(ctx context.Context, input, output string, c ffmpeg.Clip, timeout time.Duration) error
	Montage(ctx context.Context, output string, m ffmpeg.Montage, timeout time.Duration) error
	ProbeFile(ctx context.Context, path string) (ffmpeg.Probe, error)
}

// Renderer handles render.clip jobs in the media worker.
type Renderer struct {
	db     *bun.DB
	store  storage.Store
	ff     Clipper
	logger *slog.Logger
	// TempDir holds work files; empty means the system default. Each job's
	// files are removed when it finishes, whatever the outcome.
	TempDir string
	// FontsDir holds the caption font (Inter). The worker image installs
	// it at DefaultFontsDir.
	FontsDir string
}

// DefaultFontsDir is where Alpine's font-inter package puts Inter.
const DefaultFontsDir = "/usr/share/fonts/inter"

// NewRenderer returns a render job handler.
func NewRenderer(db *bun.DB, store storage.Store, ff Clipper, logger *slog.Logger) *Renderer {
	return &Renderer{db: db, store: store, ff: ff, logger: logger, FontsDir: DefaultFontsDir}
}

var _ jobs.Handler = (*Renderer)(nil)

// Reasons a render fails for good.
var (
	errSourceGone    = errors.New("render: source is gone or no longer a processed video")
	errBadOutput     = errors.New("render: output failed checks")
	errExcluded      = errors.New("render: clip includes content the brief excludes")
	errVoiceoverGone = errors.New("render: voiceover was deleted")
)

// User-facing failure messages.
const (
	msgSourceGone    = "The source file was deleted or changed, so this clip can’t be made. Make it again from the file."
	msgUnreadable    = "The source file couldn’t be read, so this clip can’t be made."
	msgBadOutput     = "The rendered clip didn’t come out right, so we didn’t keep it. Try again."
	msgGaveUp        = "Rendering kept failing, so we stopped trying. Try again."
	msgExcluded      = "The creator brief now excludes part of this clip, so it wasn’t made. Choose another stretch."
	msgVoiceoverGone = "The voiceover was deleted, so this clip can’t be made. Make it again with another voiceover."
)

// timeoutFor bounds one FFmpeg run: generous for the clip's length, within
// the job's lease.
func timeoutFor(length time.Duration) time.Duration {
	return min(25*time.Minute, max(5*time.Minute, 3*length+2*time.Minute))
}

// Process implements jobs.Handler. It is idempotent: a rerun renders again
// to the same key and records the result once.
func (r *Renderer) Process(ctx context.Context, job jobs.Job) error {
	var projectID, sourceID, name string
	var spec []byte
	// The render must belong to the job's organisation; the job's foreign
	// key guarantees it, and this query re-checks rather than trusts.
	err := r.db.QueryRowContext(ctx, `
		SELECT project_id, coalesce(source_asset_id::text, ''), name, composition FROM renders
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		job.RenderID, job.OrganisationID).Scan(&projectID, &sourceID, &name, &spec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted since it was queued; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load render: %w", err)
	}
	var c Composition
	if err := json.Unmarshal(spec, &c); err != nil {
		return jobs.Permanent(fmt.Errorf("render %s composition: %w", job.RenderID, err))
	}
	if sourceID == "" || sourceID != c.SourceAssetID {
		return jobs.Permanent(errSourceGone)
	}
	if c.Version == MontageVersion {
		return r.montage(ctx, job, projectID, name, c)
	}
	src, key, err := source(ctx, r.db, job.OrganisationID, projectID, sourceID)
	if errors.Is(err, errBadSource) {
		return jobs.Permanent(errSourceGone)
	}
	if err != nil {
		return err
	}
	// The source may have been reprocessed since: check again.
	if err := c.Validate(src); err != nil {
		return jobs.Permanent(fmt.Errorf("%w: %v", errSourceGone, err))
	}
	// The brief may have changed since the clip was requested.
	rules, err := brief.Rules(ctx, r.db, job.OrganisationID, sourceID)
	if err != nil {
		return err
	}
	if rule, ok := brief.Excluded(rules, c.StartMS, c.EndMS); ok {
		return jobs.Permanent(fmt.Errorf("%w: %s", errExcluded, rule.Note))
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE renders SET status = 'running', updated_at = now() WHERE id = ?`, job.RenderID); err != nil {
		return err
	}

	plan := Plan(c, src)
	url, _, err := r.store.PresignDownload(ctx, key, timeoutFor(plan.Length)+10*time.Minute)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(r.TempDir, "render-")
	if err != nil {
		return fmt.Errorf("render work dir: %w", err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "clip.mp4")
	if c.Captions != nil {
		plan.Subtitles = filepath.Join(dir, "captions.ass")
		plan.FontsDir = r.FontsDir
		if err := os.WriteFile(plan.Subtitles, []byte(ASS(*c.Captions, c.Aspect)), 0o600); err != nil {
			return fmt.Errorf("write captions: %w", err)
		}
	}

	started := time.Now()
	if err := r.ff.Clip(ctx, url, out, plan, timeoutFor(plan.Length)); err != nil {
		if errors.Is(err, ffmpeg.ErrUnreadable) {
			return jobs.Permanent(err)
		}
		return err
	}
	took := time.Since(started)
	probe, err := r.ff.ProbeFile(ctx, out)
	if err != nil {
		return jobs.Permanent(fmt.Errorf("%w: %v", errBadOutput, err))
	}
	if err := check(probe, plan); err != nil {
		return jobs.Permanent(err)
	}
	return r.finish(ctx, job, projectID, name, out, probe, took, plan.Length)
}

// check verifies the output is what was asked for: a zero exit from
// FFmpeg doesn't guarantee a usable file.
func check(p ffmpeg.Probe, want ffmpeg.Clip) error {
	switch {
	case !p.HasVideo:
		return fmt.Errorf("%w: no video", errBadOutput)
	case p.Width != want.Width || p.Height != want.Height:
		return fmt.Errorf("%w: %dx%d, want %dx%d", errBadOutput, p.Width, p.Height, want.Width, want.Height)
	case p.HasAudio != want.Audio:
		return fmt.Errorf("%w: audio %v, want %v", errBadOutput, p.HasAudio, want.Audio)
	}
	if d := p.Duration - want.Length; d < -200*time.Millisecond || d > 200*time.Millisecond {
		return fmt.Errorf("%w: %v long, want %v", errBadOutput, p.Duration, want.Length)
	}
	return nil
}

// Failed implements jobs.Handler.
func (r *Renderer) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgGaveUp
	switch {
	case errors.Is(err, errSourceGone):
		msg = msgSourceGone
	case errors.Is(err, ffmpeg.ErrUnreadable):
		msg = msgUnreadable
	case errors.Is(err, errBadOutput):
		msg = msgBadOutput
	case errors.Is(err, errExcluded):
		msg = msgExcluded
	case errors.Is(err, errVoiceoverGone):
		msg = msgVoiceoverGone
	}
	_, dbErr := r.db.ExecContext(ctx, `
		UPDATE renders SET status = 'failed', error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		msg, job.RenderID, job.OrganisationID)
	return dbErr
}

// finish stores a checked output and records the render as ready.
func (r *Renderer) finish(ctx context.Context, job jobs.Job, projectID, name, out string, probe ffmpeg.Probe, took, length time.Duration) error {
	info, err := os.Stat(out)
	if err != nil {
		return err
	}

	outKey := outputKey(job.OrganisationID, projectID, job.RenderID)
	if err := r.store.PutFile(ctx, outKey, "video/mp4", Filename(name), out); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE renders SET status = 'ready', error = NULL, output_key = ?, output_size_bytes = ?,
			output_duration_ms = ?, output_width = ?, output_height = ?, render_ms = ?,
			completed_at = now(), updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		outKey, info.Size(), probe.Duration.Milliseconds(), probe.Width, probe.Height, took.Milliseconds(),
		job.RenderID, job.OrganisationID)
	if err != nil {
		return fmt.Errorf("save render: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Deleted while we worked: don't leave the file behind.
		_, _ = r.db.ExecContext(ctx, `INSERT INTO storage_deletions (storage_key) VALUES (?)`, outKey)
	}
	r.logger.InfoContext(ctx, "render finished", slog.String("render_id", job.RenderID),
		slog.Duration("render_time", took), slog.Duration("clip_length", length), slog.Int64("bytes", info.Size()))
	return nil
}

// montage renders a version-3 composition: every source is checked again
// (it may have been deleted or reprocessed), and so is its brief.
func (r *Renderer) montage(ctx context.Context, job jobs.Job, projectID, name string, c Composition) error {
	srcs := map[string]Source{}
	keys := map[string]string{}
	for _, id := range c.SourceIDs() {
		src, key, err := source(ctx, r.db, job.OrganisationID, projectID, id)
		if errors.Is(err, errBadSource) {
			return jobs.Permanent(errSourceGone)
		}
		if err != nil {
			return err
		}
		srcs[id], keys[id] = src, key
	}
	if err := c.ValidateMontage(srcs); err != nil {
		return jobs.Permanent(fmt.Errorf("%w: %v", errSourceGone, err))
	}
	for _, seg := range c.Segments {
		for _, s := range seg.Ranges() {
			rules, err := brief.Rules(ctx, r.db, job.OrganisationID, s.SourceAssetID)
			if err != nil {
				return err
			}
			if rule, ok := brief.Excluded(rules, s.StartMS, s.EndMS); ok {
				return jobs.Permanent(fmt.Errorf("%w: %s", errExcluded, rule.Note))
			}
		}
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE renders SET status = 'running', updated_at = now() WHERE id = ?`, job.RenderID); err != nil {
		return err
	}
	length := time.Duration(c.LengthMS()) * time.Millisecond
	inputs := map[string]string{}
	for id, key := range keys {
		url, _, err := r.store.PresignDownload(ctx, key, timeoutFor(length)+10*time.Minute)
		if err != nil {
			return err
		}
		inputs[id] = url
	}
	plan := PlanMontage(c, srcs, inputs)
	// voice adds a ready voiceover to the plan, at clip time at, for at
	// most length.
	voice := func(id string, at, length int64, duck float64) error {
		ready, err := voiceover.Load(ctx, r.db, job.OrganisationID, projectID, id)
		if errors.Is(err, voiceover.ErrNotFound) {
			return jobs.Permanent(errVoiceoverGone)
		}
		if err != nil {
			return err
		}
		url, _, err := r.store.PresignDownload(ctx, ready.StorageKey, timeoutFor(time.Duration(c.LengthMS())*time.Millisecond)+10*time.Minute)
		if err != nil {
			return err
		}
		plan.Voices = append(plan.Voices, ffmpeg.Voice{Input: url, Duck: duck,
			At: time.Duration(at) * time.Millisecond, Length: time.Duration(min(ready.DurationMS, length)) * time.Millisecond})
		return nil
	}
	if v := c.Voiceover; v != nil {
		if err := voice(v.VoiceoverID, v.StartMS, c.LengthMS()-v.StartMS, v.Duck); err != nil {
			return err
		}
	}
	if so := c.SignOff; so != nil && so.VoiceoverID != "" {
		from := c.SignOffStartMS()
		if err := voice(so.VoiceoverID, from, c.LengthMS()-from, signOffDuck); err != nil {
			return err
		}
	}
	dir, err := os.MkdirTemp(r.TempDir, "render-")
	if err != nil {
		return fmt.Errorf("render work dir: %w", err)
	}
	defer os.RemoveAll(dir)
	if ass := OverlayASS(c.Captions, c.Hook, c.SignOff, c.SignOffStartMS(), c.LengthMS(), c.Aspect); ass != "" {
		plan.Subtitles = filepath.Join(dir, "overlay.ass")
		plan.FontsDir = r.FontsDir
		if err := os.WriteFile(plan.Subtitles, []byte(ass), 0o600); err != nil {
			return fmt.Errorf("write overlay: %w", err)
		}
	}
	out := filepath.Join(dir, "clip.mp4")
	started := time.Now()
	if err := r.ff.Montage(ctx, out, plan, timeoutFor(length)); err != nil {
		if errors.Is(err, ffmpeg.ErrUnreadable) {
			return jobs.Permanent(err)
		}
		return err
	}
	took := time.Since(started)
	probe, err := r.ff.ProbeFile(ctx, out)
	if err != nil {
		return jobs.Permanent(fmt.Errorf("%w: %v", errBadOutput, err))
	}
	if err := check(probe, ffmpeg.Clip{Length: length, Width: plan.Width, Height: plan.Height, Audio: true}); err != nil {
		return jobs.Permanent(err)
	}
	return r.finish(ctx, job, projectID, name, out, probe, took, length)
}

package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Processing statuses.
const (
	ProcessingPending    = "pending"
	ProcessingProcessing = "processing"
	ProcessingReady      = "ready"
	ProcessingFailed     = "failed"
)

// Messages shown to users when processing gives up.
const (
	msgUnreadable = "We couldn’t read this file. It may be damaged or use a codec we don’t support yet."
	msgGaveUp     = "Processing kept failing, so we stopped trying. You can try again."
)

// thumbnailWidth is the largest thumbnail width in pixels.
const thumbnailWidth = 640

// Inspector is what processing needs from FFmpeg. *ffmpeg.Runner
// implements it.
type Inspector interface {
	Probe(ctx context.Context, input string) (ffmpeg.Probe, error)
	Frame(ctx context.Context, input string, offset time.Duration, maxWidth int) ([]byte, error)
}

// Processor handles media.process jobs: it probes a file and makes a
// thumbnail. It runs in the media worker.
type Processor struct {
	db     *bun.DB
	store  storage.Store
	ff     Inspector
	logger *slog.Logger
}

// NewProcessor returns a processor.
func NewProcessor(db *bun.DB, store storage.Store, ff Inspector, logger *slog.Logger) *Processor {
	return &Processor{db: db, store: store, ff: ff, logger: logger}
}

var _ jobs.Handler = (*Processor)(nil)

// Process implements jobs.Handler. It is idempotent: running it again
// re-derives the same results.
func (p *Processor) Process(ctx context.Context, job jobs.Job) error {
	var projectID, key, kind, status string
	// The asset must belong to the job's organisation; the job's own
	// foreign key guarantees it, and this query re-checks rather than trusts.
	err := p.db.QueryRowContext(ctx, `
		SELECT project_id, storage_key, coalesce(kind, ''), status FROM media_assets
		WHERE id = ? AND organisation_id = ?`, job.MediaAssetID, job.OrganisationID).
		Scan(&projectID, &key, &kind, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted since the job was queued; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load media: %w", err)
	}
	if status != "uploaded" {
		return jobs.Permanent(fmt.Errorf("media %s is not uploaded (status %s)", job.MediaAssetID, status))
	}
	if _, err := p.db.ExecContext(ctx, `
		UPDATE media_assets SET processing_status = 'processing', updated_at = now()
		WHERE id = ? AND processing_status <> 'ready'`, job.MediaAssetID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	url, _, err := p.store.PresignDownload(ctx, key, 30*time.Minute)
	if err != nil {
		return err
	}
	probe, err := p.ff.Probe(ctx, url)
	if err != nil {
		return classify(err)
	}

	var thumb []byte
	switch {
	case kind == KindImage:
		thumb, err = p.ff.Frame(ctx, url, 0, thumbnailWidth)
	case probe.HasVideo:
		thumb, err = p.ff.Frame(ctx, url, thumbnailOffset(probe.Duration), thumbnailWidth)
	}
	if err != nil {
		return classify(err)
	}

	thumbKey := ""
	if thumb != nil {
		// One key per job, so reprocessing never overwrites a thumbnail
		// that's still being served; the old one is queued for deletion.
		thumbKey = "organisations/" + job.OrganisationID + "/projects/" + projectID + "/assets/" + job.MediaAssetID + "/thumbnail-" + job.ID + ".jpg"
		if err := p.store.Put(ctx, thumbKey, "image/jpeg", thumb); err != nil {
			return err
		}
	}

	res, err := p.db.ExecContext(ctx, `
		UPDATE media_assets SET
			processing_status = 'ready', processing_error = NULL, processed_at = now(), updated_at = now(),
			container = NULLIF(?, ''), duration_ms = ?, bit_rate = ?,
			width = ?, height = ?, frame_rate = ?, video_codec = NULLIF(?, ''),
			audio_codec = NULLIF(?, ''), audio_channels = ?, sample_rate = ?,
			thumbnail_key = NULLIF(?, '')
		WHERE id = ? AND organisation_id = ?`,
		probe.Container, nullInt(probe.Duration.Milliseconds()), nullInt(probe.BitRate),
		nullInt(int64(probe.Width)), nullInt(int64(probe.Height)), nullFloat(probe.FrameRate), probe.VideoCodec,
		probe.AudioCodec, nullInt(int64(probe.AudioChannels)), nullInt(int64(probe.SampleRate)),
		thumbKey, job.MediaAssetID, job.OrganisationID)
	if err != nil {
		return fmt.Errorf("save processing results: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 && thumbKey != "" {
		// Deleted while we worked: don't leave the thumbnail behind.
		_, _ = p.db.ExecContext(ctx, `INSERT INTO storage_deletions (storage_key) VALUES (?)`, thumbKey)
	}
	return nil
}

// Failed implements jobs.Handler: it tells the user processing gave up.
func (p *Processor) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgGaveUp
	if errors.Is(err, ffmpeg.ErrUnreadable) {
		msg = msgUnreadable
	}
	_, dbErr := p.db.ExecContext(ctx, `
		UPDATE media_assets SET processing_status = 'failed', processing_error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ?`, msg, job.MediaAssetID, job.OrganisationID)
	return dbErr
}

func classify(err error) error {
	if errors.Is(err, ffmpeg.ErrUnreadable) {
		return jobs.Permanent(err)
	}
	return err
}

// thumbnailOffset picks a representative frame: a second in, or 10% of
// very short clips, avoiding black first frames.
func thumbnailOffset(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return min(time.Second, d/10)
}

func nullInt(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullFloat(v float64) any {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}

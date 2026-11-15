package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Framing statuses (media_assets.framing_status; empty until requested).
const (
	FramingPending    = "pending"
	FramingProcessing = "processing"
	FramingReady      = "ready"
	FramingFailed     = "failed"
)

// Messages shown when framing analysis gives up.
const (
	msgFramingGaveUp      = "Finding the people in this video kept failing. You can try again."
	msgFramingUnavailable = "Auto framing isn’t available on this server yet."
)

// ErrFramingInProgress means the file is already being analysed.
var ErrFramingInProgress = errors.New("media: framing analysis in progress")

// queueFraming marks a video for framing analysis and queues it, inside
// the caller's transaction.
func queueFraming(ctx context.Context, tx bun.IDB, orgID, assetID string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_assets SET framing_status = 'pending', framing_error = NULL, updated_at = now()
		WHERE id = ? AND organisation_id = ?`, assetID, orgID); err != nil {
		return fmt.Errorf("queue framing: %w", err)
	}
	_, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaFraming, OrganisationID: orgID,
		MediaAssetID: assetID, CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 3})
	return err
}

// RequestFraming (editing.manage) analyses a video for auto framing:
// files processed before analysis existed, or whose analysis failed.
// Calling it on an analysed file returns it unchanged.
func (s *Service) RequestFraming(ctx context.Context, actorID, orgID, projectID, assetID string) (Asset, error) {
	queued := false
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
		switch {
		case a.Kind != KindVideo || (a.ProcessingStatus == ProcessingReady && a.Width == 0):
			return invalid("media", "Only videos can be framed automatically.")
		case a.ProcessingStatus != ProcessingReady:
			return invalid("media", "Wait for the file to finish processing.")
		case a.FramingStatus == FramingReady:
			return nil
		case a.FramingStatus == FramingPending || a.FramingStatus == FramingProcessing:
			return ErrFramingInProgress
		}
		if err := queueFraming(ctx, tx, orgID, assetID); err != nil {
			return err
		}
		queued = true
		return account.Audit(ctx, tx, orgID, actorID, "media.framing_requested", "media_asset", assetID, nil)
	})
	if err != nil {
		return Asset{}, err
	}
	if queued {
		s.jobQueued()
	}
	return s.Get(ctx, actorID, orgID, projectID, assetID)
}

// FramingTrack returns a video's framing analysis, if it is ready and
// from the current analyser. The caller has already authorised access
// to the file.
func FramingTrack(ctx context.Context, db bun.IDB, orgID, assetID string) (framing.Track, bool, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `
		SELECT f.track FROM media_framing f JOIN media_assets m ON m.id = f.media_asset_id
		WHERE f.media_asset_id = ? AND f.organisation_id = ? AND m.framing_status = 'ready' AND f.analyser_version = ?`,
		assetID, orgID, framing.Version).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return framing.Track{}, false, nil
	}
	if err != nil {
		return framing.Track{}, false, fmt.Errorf("load framing: %w", err)
	}
	var t framing.Track
	if err := json.Unmarshal(raw, &t); err != nil {
		return framing.Track{}, false, fmt.Errorf("framing for %s: %w", assetID, err)
	}
	return t, true, nil
}

// FrameReader is what framing analysis needs from FFmpeg. *ffmpeg.Runner
// implements it.
type FrameReader interface {
	Frames(ctx context.Context, in string, s ffmpeg.Sampling, timeout time.Duration, fn func(i int, bgr []byte) error) error
}

// Detectors start a face detector for frames of w×h; stop it with the
// returned function.
type Detectors func(ctx context.Context, w, h int) (framing.Detector, func() error, error)

// HelperDetectors runs the cavi-faces helper (bin, with model).
func HelperDetectors(bin, model string) Detectors {
	return func(ctx context.Context, w, h int) (framing.Detector, func() error, error) {
		hp, err := framing.StartHelper(ctx, bin, model, w, h)
		if err != nil {
			return nil, nil, err
		}
		return hp, hp.Close, nil
	}
}

// errNoDetector means the face finder couldn't be started here.
var errNoDetector = errors.New("media: face finder unavailable")

// Framer handles media.framing jobs: it samples a video's frames, finds
// faces and shot changes, and stores the track. It runs in the media
// worker.
type Framer struct {
	db     *bun.DB
	store  storage.Store
	ff     FrameReader
	detect Detectors
	logger *slog.Logger
}

// NewFramer returns a Framer.
func NewFramer(db *bun.DB, store storage.Store, ff FrameReader, detect Detectors, logger *slog.Logger) *Framer {
	return &Framer{db: db, store: store, ff: ff, detect: detect, logger: logger}
}

var _ jobs.Handler = (*Framer)(nil)

// framingTimeout bounds one analysis: decoding runs several times faster
// than real time, so this is generous, within the job's lease.
func framingTimeout(d time.Duration) time.Duration {
	return min(110*time.Minute, max(10*time.Minute, d+5*time.Minute))
}

// Process implements jobs.Handler. Rerunning it redoes the analysis.
func (f *Framer) Process(ctx context.Context, job jobs.Job) error {
	var key, kind, status, processing, framingStatus string
	var w, h int
	var durationMS int64
	// The asset must belong to the job's organisation; re-check, don't trust.
	err := f.db.QueryRowContext(ctx, `
		SELECT storage_key, coalesce(kind, ''), status, processing_status, coalesce(framing_status, ''),
			coalesce(width, 0), coalesce(height, 0), coalesce(duration_ms, 0)
		FROM media_assets WHERE id = ? AND organisation_id = ?`, job.MediaAssetID, job.OrganisationID).
		Scan(&key, &kind, &status, &processing, &framingStatus, &w, &h, &durationMS)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted since the job was queued
	}
	if err != nil {
		return fmt.Errorf("load media: %w", err)
	}
	if framingStatus == FramingReady {
		return nil
	}
	if kind != KindVideo || status != "uploaded" || processing != ProcessingReady || w <= 0 || h <= 0 {
		return jobs.Permanent(fmt.Errorf("media %s is not a processed video", job.MediaAssetID))
	}
	if _, err := f.db.ExecContext(ctx, `
		UPDATE media_assets SET framing_status = 'processing', updated_at = now() WHERE id = ?`, job.MediaAssetID); err != nil {
		return fmt.Errorf("mark framing: %w", err)
	}

	length := time.Duration(durationMS) * time.Millisecond
	timeout := framingTimeout(length)
	url, _, err := f.store.PresignDownload(ctx, key, timeout+10*time.Minute)
	if err != nil {
		return err
	}
	fw, fh := framing.FrameSize(w, h)
	det, stop, err := f.detect(ctx, fw, fh)
	if err != nil {
		return jobs.Permanent(fmt.Errorf("%w: %v", errNoDetector, err))
	}
	a := framing.NewAnalyser(fw, fh, det)
	started := time.Now()
	err = f.ff.Frames(ctx, url, ffmpeg.Sampling{FPS: framing.SampleFPS, Width: fw, Height: fh}, timeout, a.Frame)
	if stopErr := stop(); err == nil && stopErr != nil {
		err = fmt.Errorf("framing: face finder: %w", stopErr)
	}
	if err != nil {
		return classify(err)
	}
	track := a.Track()
	if len(track.Faces) == 0 {
		return jobs.Permanent(fmt.Errorf("%w: no frames could be read", ffmpeg.ErrUnreadable))
	}
	raw, err := json.Marshal(track)
	if err != nil {
		return err
	}
	err = f.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE media_assets SET framing_status = 'ready', framing_error = NULL, updated_at = now()
			WHERE id = ? AND organisation_id = ?`, job.MediaAssetID, job.OrganisationID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // deleted while we worked
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO media_framing (media_asset_id, organisation_id, analyser_version, track) VALUES (?, ?, ?, ?)
			ON CONFLICT (media_asset_id) DO UPDATE SET analyser_version = EXCLUDED.analyser_version,
				track = EXCLUDED.track, created_at = now()`,
			job.MediaAssetID, job.OrganisationID, track.Version, string(raw))
		return err
	})
	if err != nil {
		return fmt.Errorf("save framing: %w", err)
	}
	withFaces := 0
	for _, fs := range track.Faces {
		if len(fs) > 0 {
			withFaces++
		}
	}
	f.logger.InfoContext(ctx, "framing analysed", slog.String("media_asset_id", job.MediaAssetID),
		slog.Duration("took", time.Since(started)), slog.Duration("video_length", length),
		slog.Int("samples", len(track.Faces)), slog.Int("samples_with_faces", withFaces), slog.Int("cuts", len(track.Cuts)))
	return nil
}

// Failed implements jobs.Handler.
func (f *Framer) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgFramingGaveUp
	switch {
	case errors.Is(err, ffmpeg.ErrUnreadable):
		msg = msgUnreadable
	case errors.Is(err, errNoDetector):
		msg = msgFramingUnavailable
	}
	_, dbErr := f.db.ExecContext(ctx, `
		UPDATE media_assets SET framing_status = 'failed', framing_error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ? AND framing_status IS DISTINCT FROM 'ready'`, msg, job.MediaAssetID, job.OrganisationID)
	return dbErr
}

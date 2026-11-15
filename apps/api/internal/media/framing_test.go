package media

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

// fakeFrames serves n grey frames; a face detector sees one face in each.
type fakeFrames struct {
	n        int
	sampling ffmpeg.Sampling
	err      error
}

func (f *fakeFrames) Frames(_ context.Context, _ string, s ffmpeg.Sampling, _ time.Duration, fn func(int, []byte) error) error {
	f.sampling = s
	pix := make([]byte, s.Width*s.Height*3)
	for i := range f.n {
		if err := fn(i, pix); err != nil {
			return err
		}
	}
	return f.err
}

type oneFace struct{}

func (oneFace) Detect([]byte) ([]framing.Box, error) {
	return []framing.Box{{X: 300, Y: 50, W: 60, H: 80, Score: 0.9}}, nil
}

func faces(context.Context, int, int) (framing.Detector, func() error, error) {
	return oneFace{}, func() error { return nil }, nil
}

func (w *world) framer(ff *fakeFrames, det Detectors) *Framer {
	return NewFramer(w.db, w.store, ff, det, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (w *world) framingJob(assetID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{MediaAssetID: assetID, Kind: jobs.KindMediaFraming}
	if err := w.db.QueryRowContext(w.ctx, `
		SELECT id, organisation_id FROM jobs WHERE media_asset_id = ? AND kind = 'media.framing' AND status = 'queued'
		ORDER BY created_at DESC LIMIT 1`, assetID).Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no framing job for %s: %v", assetID, err)
	}
	return j
}

func TestFramingAnalysis(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.upload(ana, org, proj, "a.mp4", mp4)
	// Processing a video queues its framing analysis.
	if err := w.processor(&fakeFF{probe: videoProbe}).Process(w.ctx, w.queuedJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	got, _ := w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if got.FramingStatus != FramingPending {
		t.Fatalf("framing status = %q", got.FramingStatus)
	}
	if _, ok, _ := FramingTrack(w.ctx, w.db, org, a.ID); ok {
		t.Fatal("track before analysis")
	}

	ff := &fakeFrames{n: 30}
	if err := w.framer(ff, faces).Process(w.ctx, w.framingJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	if ff.sampling != (ffmpeg.Sampling{FPS: framing.SampleFPS, Width: 480, Height: 270}) {
		t.Fatalf("sampling = %+v", ff.sampling)
	}
	got, _ = w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	tr, ok, err := FramingTrack(w.ctx, w.db, org, a.ID)
	if err != nil || !ok || got.FramingStatus != FramingReady {
		t.Fatalf("after analysis: status %q, track %v, %v", got.FramingStatus, ok, err)
	}
	// 30 frames at 10 a second: faces searched in frames 0, 4, ... 28.
	if len(tr.Faces) != 8 || len(tr.Faces[0]) != 1 || tr.Faces[0][0].X != 0.688 || tr.Version != framing.Version {
		t.Fatalf("track = %+v", tr)
	}
	// Another organisation can't load it.
	if _, ok, _ := FramingTrack(w.ctx, w.db, w.team(w.user("Eve"), nil), a.ID); ok {
		t.Fatal("track loaded across organisations")
	}
	// Asking again for an analysed file changes nothing.
	again, err := w.svc.RequestFraming(w.ctx, ana.id, org, proj, a.ID)
	if err != nil || again.FramingStatus != FramingReady {
		t.Fatalf("request again = %+v, %v", again.FramingStatus, err)
	}

	// Reprocessing redoes the analysis; meanwhile the old track is unused.
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET processing_status = 'failed' WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Reprocess(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.processor(&fakeFF{probe: videoProbe}).Process(w.ctx, w.queuedJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := FramingTrack(w.ctx, w.db, org, a.ID); ok {
		t.Fatal("stale track used while reanalysing")
	}
}

func TestFramingFailures(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	vic := w.user("Vic")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic})
	proj := w.project(ana, org)
	a := w.upload(ana, org, proj, "a.mp4", mp4)
	if err := w.processor(&fakeFF{probe: videoProbe}).Process(w.ctx, w.queuedJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	j := w.framingJob(a.ID)

	// No face finder on this server: fails for good, saying so.
	none := func(context.Context, int, int) (framing.Detector, func() error, error) {
		return nil, nil, errors.New("exec: cavi-faces: not found")
	}
	err := w.framer(&fakeFrames{n: 10}, none).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("missing helper: %v", err)
	}
	if err := w.framer(nil, none).Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	got, _ := w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if got.FramingStatus != FramingFailed || got.FramingError != msgFramingUnavailable {
		t.Fatalf("failed = %q %q", got.FramingStatus, got.FramingError)
	}
	// A failed decode is retried.
	if err := w.framer(&fakeFrames{n: 3, err: errors.New("ffmpeg: frames failed: connection reset")}, faces).Process(w.ctx, j); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("decode error: %v", err)
	}

	// Viewers can't ask for analysis; editors can, once.
	_, err = w.svc.RequestFraming(w.ctx, vic.id, org, proj, a.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer: %v", err)
	}
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET framing_status = 'failed' WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err = w.svc.RequestFraming(w.ctx, ana.id, org, proj, a.ID)
	if err != nil || got.FramingStatus != FramingPending {
		t.Fatalf("retry = %q, %v", got.FramingStatus, err)
	}
	if _, err := w.svc.RequestFraming(w.ctx, ana.id, org, proj, a.ID); !errors.Is(err, ErrFramingInProgress) {
		t.Fatalf("second request: %v", err)
	}
	// Audio files can't be framed.
	audio := w.upload(ana, org, proj, "a.mp3", append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), make([]byte, 50)...))
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET processing_status = 'ready' WHERE id = ?`, audio.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.RequestFraming(w.ctx, ana.id, org, proj, audio.ID); err == nil {
		t.Fatal("audio accepted for framing")
	}
}

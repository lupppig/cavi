package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

// fakeFF stands in for FFmpeg; the real runner has its own tests.
type fakeFF struct {
	probe    ffmpeg.Probe
	probeErr error
	frames   int
}

func (f *fakeFF) Probe(context.Context, string) (ffmpeg.Probe, error) { return f.probe, f.probeErr }
func (f *fakeFF) Frame(context.Context, string, time.Duration, int) ([]byte, error) {
	f.frames++
	return append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 200)...), nil
}

var videoProbe = ffmpeg.Probe{Container: "mov,mp4", Duration: 3 * time.Second, BitRate: 800000, HasVideo: true,
	VideoCodec: "h264", Width: 640, Height: 360, FrameRate: 25, HasAudio: true, AudioCodec: "aac", AudioChannels: 1, SampleRate: 44100}

// queuedJob returns the job queued for an asset.
func (w *world) queuedJob(assetID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{MediaAssetID: assetID, Kind: jobs.KindMediaProcess}
	if err := w.db.QueryRowContext(w.ctx, `
		SELECT id, organisation_id, coalesce(correlation_id, '') FROM jobs
		WHERE media_asset_id = ? AND status = 'queued' ORDER BY created_at DESC LIMIT 1`, assetID).
		Scan(&j.ID, &j.OrganisationID, &j.CorrelationID); err != nil {
		w.t.Fatalf("no queued job for %s: %v", assetID, err)
	}
	return j
}

func (w *world) processor(ff *fakeFF) *Processor {
	return NewProcessor(w.db, w.store, ff, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCompleteQueuesProcessing(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	woke := 0
	w.svc.OnJobQueued(func() { woke++ })

	a, _, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, "a.mp4", "video/mp4", int64(len(mp4)))
	if err != nil {
		t.Fatal(err)
	}
	w.store.put(storageKey(org, proj, a.ID), mp4)
	done, err := w.svc.Complete(jobs.WithCorrelationID(w.ctx, "req-42"), ana.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.ProcessingStatus != ProcessingPending || woke != 1 {
		t.Fatalf("after complete: status %q, woke %d", done.ProcessingStatus, woke)
	}
	j := w.queuedJob(a.ID)
	if j.OrganisationID != org || j.CorrelationID != "req-42" {
		t.Fatalf("job = %+v", j)
	}
	// Completing again doesn't queue a second job.
	if _, err := w.svc.Complete(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if n := w.count(`SELECT count(*) FROM jobs WHERE media_asset_id = ?`, a.ID); n != 1 {
		t.Fatalf("jobs = %d", n)
	}
}

func TestProcessVideo(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.upload(ana, org, proj, "a.mp4", mp4)
	ff := &fakeFF{probe: videoProbe}
	j := w.queuedJob(a.ID)

	if err := w.processor(ff).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProcessingStatus != ProcessingReady || got.DurationMS != 3000 || got.Width != 640 || got.Height != 360 ||
		got.FrameRate != 25 || got.VideoCodec != "h264" || got.AudioCodec != "aac" || got.SampleRate != 44100 ||
		got.ThumbnailURL == "" || ff.frames != 1 {
		t.Fatalf("processed = %+v (frames %d)", got, ff.frames)
	}
	thumbKey := storageKey(org, proj, a.ID)[:len(storageKey(org, proj, a.ID))-len("original")] + "thumbnail-" + j.ID + ".jpg"
	if !w.store.has(thumbKey) {
		t.Fatal("thumbnail not stored")
	}

	// Running the same job again is harmless.
	if err := w.processor(ff).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}

	// Deleting the media queues both the original and the thumbnail.
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if w.queued(thumbKey) != 1 || w.queued(storageKey(org, proj, a.ID)) != 1 {
		t.Fatal("deletion did not queue the original and thumbnail")
	}
}

func TestProcessAudioHasNoThumbnail(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	mp3 := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), make([]byte, 50)...)
	a, _, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, "vo.mp3", "audio/mpeg", int64(len(mp3)))
	if err != nil {
		t.Fatal(err)
	}
	w.store.put(storageKey(org, proj, a.ID), mp3)
	if _, err := w.svc.Complete(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	ff := &fakeFF{probe: ffmpeg.Probe{Container: "mp3", Duration: 90 * time.Second, HasAudio: true, AudioCodec: "mp3", AudioChannels: 2, SampleRate: 48000}}
	if err := w.processor(ff).Process(w.ctx, w.queuedJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	got, _ := w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if got.ProcessingStatus != ProcessingReady || got.DurationMS != 90000 || got.ThumbnailURL != "" || got.Width != 0 || ff.frames != 0 {
		t.Fatalf("audio = %+v", got)
	}
}

func TestUnreadableFileFailsWithReason(t *testing.T) {
	w := newWorld(t)
	ana, vic := w.user("Ana"), w.user("Vic")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic})
	proj := w.project(ana, org)
	a := w.upload(ana, org, proj, "broken.mp4", mp4)
	ff := &fakeFF{probeErr: fmt.Errorf("%w: moov atom not found", ffmpeg.ErrUnreadable)}
	p := w.processor(ff)
	j := w.queuedJob(a.ID)

	err := p.Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("err = %v, want permanent", err)
	}
	if err := p.Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	got, _ := w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if got.ProcessingStatus != ProcessingFailed || got.ProcessingError != msgUnreadable {
		t.Fatalf("failed = %+v", got)
	}

	// Transient failures are retried, not failed.
	ff.probeErr = errors.New("ffmpeg: ffprobe could not fetch the input: Server returned 5XX")
	if err := p.Process(w.ctx, j); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("transient err = %v", err)
	}

	// Retrying: viewers can't, uploaders can; not while already queued.
	_, err = w.svc.Reprocess(w.ctx, vic.id, org, proj, a.ID)
	wantErr(t, err, ErrForbidden)
	if _, err := w.db.ExecContext(w.ctx, `UPDATE jobs SET status = 'failed' WHERE media_asset_id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET processing_status = 'failed' WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	again, err := w.svc.Reprocess(w.ctx, ana.id, org, proj, a.ID)
	if err != nil || again.ProcessingStatus != ProcessingPending || again.ProcessingError != "" {
		t.Fatalf("reprocess = %+v, %v", again, err)
	}
	_, err = w.svc.Reprocess(w.ctx, ana.id, org, proj, a.ID)
	wantErr(t, err, ErrProcessingInProgress)

	// Now it decodes: processing completes.
	ff.probeErr, ff.probe = nil, videoProbe
	if err := p.Process(w.ctx, w.queuedJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	got, _ = w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if got.ProcessingStatus != ProcessingReady {
		t.Fatalf("after retry = %+v", got)
	}
}

func TestProcessorRespectsTenantAndDeletion(t *testing.T) {
	w := newWorld(t)
	ana, eve := w.user("Ana"), w.user("Eve")
	org := w.team(ana, nil)
	eveOrg := w.team(eve, nil)
	proj := w.project(ana, org)
	a := w.upload(ana, org, proj, "a.mp4", mp4)
	ff := &fakeFF{probe: videoProbe}
	j := w.queuedJob(a.ID)

	// A job claiming another organisation can't touch this media.
	forged := j
	forged.OrganisationID = eveOrg
	if err := w.processor(ff).Process(w.ctx, forged); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.svc.Get(w.ctx, ana.id, org, proj, a.ID); got.ProcessingStatus != ProcessingPending || ff.frames != 0 {
		t.Fatalf("forged job processed media: %+v", got)
	}

	// Media deleted before the job runs: nothing happens.
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.processor(ff).Process(w.ctx, j); err != nil || ff.frames != 0 {
		t.Fatalf("processed deleted media: %v", err)
	}
}

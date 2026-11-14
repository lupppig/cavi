package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

type fakeAudio struct {
	mu    sync.Mutex
	calls [][2]time.Duration
}

func (f *fakeAudio) Audio(_ context.Context, _ string, offset, length time.Duration) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, [2]time.Duration{offset, length})
	return []byte(fmt.Sprintf("audio@%d", offset.Milliseconds())), nil
}

// fakeAI returns one segment per chunk, or the configured error.
type fakeAI struct {
	mu     sync.Mutex
	models []string
	err    error
	// failOn makes the call at this offset (in ms) return invalid output
	// that many times before succeeding.
	failOn    int64
	failTimes int
}

func (f *fakeAI) Transcribe(_ context.Context, m ai.Model, audio []byte, _ string, durationMS int64) (ai.Transcript, ai.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.models = append(f.models, m.ID)
	if f.err != nil {
		return ai.Transcript{}, ai.Usage{InputTokens: 10}, f.err
	}
	if f.failTimes > 0 && string(audio) == fmt.Sprintf("audio@%d", f.failOn) {
		f.failTimes--
		return ai.Transcript{}, ai.Usage{InputTokens: 10}, fmt.Errorf("%w: segment ends after the audio", ai.ErrInvalidOutput)
	}
	return ai.Transcript{Language: "en", Segments: []ai.Segment{
		{StartMS: 1000, EndMS: min(4000, durationMS), Speaker: "Speaker 1", Text: "chunk " + string(audio)},
	}}, ai.Usage{InputTokens: 300, OutputTokens: 40}, nil
}

// processed uploads a file and marks it processed with the given duration
// and audio codec, as the media worker would.
func (w *world) processed(by person, org, proj, name string, durationMS int64, audioCodec string) Asset {
	w.t.Helper()
	a := w.upload(by, org, proj, name, mp4)
	if _, err := w.db.ExecContext(w.ctx, `
		UPDATE media_assets SET processing_status = 'ready', duration_ms = ?, audio_codec = NULLIF(?, ''), processed_at = now()
		WHERE id = ?`, durationMS, audioCodec, a.ID); err != nil {
		w.t.Fatal(err)
	}
	return a
}

func (w *world) transcriber(audio *fakeAudio, provider *fakeAI) *Transcriber {
	return NewTranscriber(w.db, w.store, audio, provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (w *world) transcribeJob(assetID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{MediaAssetID: assetID, Kind: jobs.KindMediaTranscribe}
	if err := w.db.QueryRowContext(w.ctx, `
		SELECT id, organisation_id FROM jobs WHERE media_asset_id = ? AND kind = 'media.transcribe'
		ORDER BY created_at DESC LIMIT 1`, assetID).Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no transcribe job: %v", err)
	}
	return j
}

func TestRequestTranscriptRules(t *testing.T) {
	w := newWorld(t)
	ana, ed, vic, cli := w.user("Ana"), w.user("Ed"), w.user("Vic"), w.user("Cli")
	org := w.team(ana, map[string]person{authz.RoleEditor: ed, authz.RoleViewer: vic, authz.RoleClient: cli})
	proj := w.project(ana, org)
	if err := w.projects.GrantAccess(w.ctx, ana.id, org, proj, cli.id); err != nil {
		t.Fatal(err)
	}

	pending := w.upload(ana, org, proj, "pending.mp4", mp4)
	_, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, pending.ID)
	wantField(t, err, "media")

	silent := w.processed(ana, org, proj, "silent.mp4", 5000, "")
	_, err = w.svc.RequestTranscript(w.ctx, ana.id, org, proj, silent.ID)
	wantField(t, err, "media")

	long := w.processed(ana, org, proj, "long.mp4", (MaxTranscriptionDuration + time.Minute).Milliseconds(), "aac")
	_, err = w.svc.RequestTranscript(w.ctx, ana.id, org, proj, long.ID)
	wantField(t, err, "media")

	a := w.processed(ana, org, proj, "talk.mp4", 60_000, "aac")
	_, err = w.svc.RequestTranscript(w.ctx, vic.id, org, proj, a.ID)
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.RequestTranscript(w.ctx, cli.id, org, proj, a.ID)
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Transcript(w.ctx, ana.id, org, proj, a.ID)
	wantErr(t, err, ErrNotFound)

	// The organisation's chosen model is fixed on the transcript.
	if err := ai.SetModel(w.ctx, w.db, ana.id, org, ai.TaskTranscription, ai.ProviderGemini, "gemini-3.5-flash-lite"); err != nil {
		t.Fatal(err)
	}
	woke := 0
	w.svc.OnJobQueued(func() { woke++ })
	tr, err := w.svc.RequestTranscript(w.ctx, ed.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TranscriptQueued || tr.Model != "gemini-3.5-flash-lite" || tr.Provider != ai.ProviderGemini ||
		tr.PromptVersion != ai.TranscriptionPromptVersion || woke != 1 {
		t.Fatalf("requested = %+v (woke %d)", tr, woke)
	}
	_, err = w.svc.RequestTranscript(w.ctx, ed.id, org, proj, a.ID)
	wantErr(t, err, ErrTranscriptionInProgress)
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id = ? AND action = 'media.transcription_requested' AND metadata->>'model' = 'gemini-3.5-flash-lite'`, a.ID); n != 1 {
		t.Errorf("audit events = %d", n)
	}
	// Viewers and clients with access can read it.
	if _, err := w.svc.Transcript(w.ctx, cli.id, org, proj, a.ID); err != nil {
		t.Fatalf("client read: %v", err)
	}
}

func TestTranscribeLongMediaInChunks(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.processed(ana, org, proj, "podcast.mp4", 25*60_000, "aac") // 25 minutes
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	audio, provider := &fakeAudio{}, &fakeAI{}
	j := w.transcribeJob(a.ID)
	if err := w.transcriber(audio, provider).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	want := [][2]time.Duration{{0, 10 * time.Minute}, {10 * time.Minute, 10 * time.Minute}, {20 * time.Minute, 5 * time.Minute}}
	if fmt.Sprint(audio.calls) != fmt.Sprint(want) {
		t.Fatalf("chunks = %v, want %v", audio.calls, want)
	}
	tr, err := w.svc.Transcript(w.ctx, ana.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TranscriptReady || tr.Language != "en" || len(tr.Segments) != 3 || tr.CompletedAt.IsZero() {
		t.Fatalf("transcript = %+v", tr)
	}
	for i, seg := range tr.Segments {
		offset := int64(i) * 600_000
		if seg.StartMS != offset+1000 || seg.Text != fmt.Sprintf("chunk audio@%d", offset) {
			t.Errorf("segment %d = %+v", i, seg)
		}
	}
	if n := w.count(`SELECT count(*) FROM ai_usage WHERE job_id = ? AND succeeded AND input_tokens = 300 AND model = 'gemini-3.5-flash'`, j.ID); n != 3 {
		t.Errorf("usage rows = %d, want 3", n)
	}
	if n := w.count(`SELECT coalesce(sum(audio_ms), 0) FROM ai_usage WHERE job_id = ?`, j.ID); n != 25*60_000 {
		t.Errorf("usage audio = %d ms", n)
	}

	// Transcribing again replaces the segments rather than adding to them.
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.transcriber(audio, provider).Process(w.ctx, w.transcribeJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	if tr, _ := w.svc.Transcript(w.ctx, ana.id, org, proj, a.ID); len(tr.Segments) != 3 {
		t.Fatalf("after retranscribe: %d segments", len(tr.Segments))
	}
}

func TestTranscriptionFailures(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.processed(ana, org, proj, "talk.mp4", 30_000, "aac")
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	j := w.transcribeJob(a.ID)

	// Invalid output is tried three times per section, then the attempt
	// fails (to be retried by the job system); every call is counted.
	provider := &fakeAI{err: fmt.Errorf("%w: segment 2 ends after the audio", ai.ErrInvalidOutput)}
	tx := w.transcriber(&fakeAudio{}, provider)
	if err := tx.Process(w.ctx, j); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("invalid output err = %v", err)
	}
	if n := w.count(`SELECT count(*) FROM ai_usage WHERE job_id = ? AND NOT succeeded`, j.ID); n != sectionTries {
		t.Errorf("failed usage rows = %d, want %d", n, sectionTries)
	}

	// A rejected request is permanent and explained.
	provider.err = fmt.Errorf("%w: input blocked", ai.ErrPermanent)
	err := tx.Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("rejected err = %v", err)
	}
	if err := tx.Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	tr, _ := w.svc.Transcript(w.ctx, ana.id, org, proj, a.ID)
	if tr.Status != TranscriptFailed || tr.Error != msgTranscriptionRejected {
		t.Fatalf("failed transcript = %+v", tr)
	}

	// A model removed from the catalogue after the request fails permanently
	// instead of silently switching models.
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `UPDATE transcripts SET model = 'gemini-retired' WHERE media_asset_id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	provider.err = nil
	if err := tx.Process(w.ctx, w.transcribeJob(a.ID)); !jobs.IsPermanent(err) || !errors.Is(err, ai.ErrPermanent) {
		t.Fatalf("retired model err = %v", err)
	}
	if len(provider.models) != sectionTries+1 { // the invalid tries and the rejected call, nothing more
		t.Fatalf("provider called %d times; the retired model must not be called", len(provider.models))
	}

	// Deleting the media cancels everything.
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	if n := w.count(`SELECT count(*) FROM transcripts WHERE media_asset_id = ?`, a.ID); n != 0 {
		t.Fatal("transcript outlived its media")
	}
	if err := tx.Process(w.ctx, j); err != nil {
		t.Fatalf("job for deleted media: %v", err)
	}
}

func TestInvalidSectionRetriedAlone(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.processed(ana, org, proj, "podcast.mp4", 25*60_000, "aac") // three sections
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	// The middle section comes back invalid twice, then fine.
	provider := &fakeAI{failOn: 600_000, failTimes: 2}
	j := w.transcribeJob(a.ID)
	if err := w.transcriber(&fakeAudio{}, provider).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	if len(provider.models) != 5 {
		t.Errorf("provider called %d times, want 5 (only the bad section repeated)", len(provider.models))
	}
	tr, _ := w.svc.Transcript(w.ctx, ana.id, org, proj, a.ID)
	if tr.Status != TranscriptReady || len(tr.Segments) != 3 {
		t.Fatalf("transcript = %s with %d segments", tr.Status, len(tr.Segments))
	}
	if n := w.count(`SELECT count(*) FROM ai_usage WHERE job_id = ? AND NOT succeeded`, j.ID); n != 2 {
		t.Errorf("failed usage rows = %d, want 2", n)
	}
}

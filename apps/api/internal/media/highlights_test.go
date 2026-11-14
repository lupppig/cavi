package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

// fakeHighlighter returns the configured clips (validated, as the real
// provider does), or the configured error.
type fakeHighlighter struct {
	mu     sync.Mutex
	calls  []ai.HighlightBrief
	models []string
	lines  []int
	clips  []ai.Highlight
	err    error
}

func (f *fakeHighlighter) Highlights(_ context.Context, m ai.Model, segs []ai.Segment, b ai.HighlightBrief) ([]ai.Highlight, ai.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, b)
	f.models = append(f.models, m.ID)
	f.lines = append(f.lines, len(segs))
	if f.err != nil {
		return nil, ai.Usage{InputTokens: 5}, f.err
	}
	hs, err := ai.ValidateHighlights(f.clips, segs, b)
	return hs, ai.Usage{InputTokens: 900, OutputTokens: 70}, err
}

// transcribed makes a processed file with a ready transcript of n
// ten-second lines.
func (w *world) transcribed(by person, org, proj, name string, n int) Asset {
	w.t.Helper()
	a := w.processed(by, org, proj, name, int64(n)*10_000, "aac")
	var id string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO transcripts (organisation_id, media_asset_id, status, provider, model, prompt_version, completed_at)
		VALUES (?, ?, 'ready', 'gemini', 'gemini-3.5-flash', 'transcription-v1', now()) RETURNING id`, org, a.ID).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	for i := range n {
		if _, err := w.db.ExecContext(w.ctx, `
			INSERT INTO transcript_segments (transcript_id, position, start_ms, end_ms, speaker, text) VALUES (?, ?, ?, ?, 'Speaker 1', ?)`,
			id, i, i*10_000, (i+1)*10_000, fmt.Sprintf("line %d", i)); err != nil {
			w.t.Fatal(err)
		}
	}
	return a
}

func (w *world) highlighter(p *fakeHighlighter) *Highlighter {
	return NewHighlighter(w.db, p, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (w *world) highlightsJob(assetID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{MediaAssetID: assetID, Kind: jobs.KindMediaHighlights}
	if err := w.db.QueryRowContext(w.ctx, `
		SELECT id, organisation_id FROM jobs WHERE media_asset_id = ? AND kind = 'media.highlights'
		ORDER BY created_at DESC LIMIT 1`, assetID).Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no highlights job: %v", err)
	}
	return j
}

var anyLength = HighlightRequest{MinSeconds: 15, MaxSeconds: 60}

func TestRequestHighlightsRules(t *testing.T) {
	w := newWorld(t)
	ana, ed, vic, cli := w.user("Ana"), w.user("Ed"), w.user("Vic"), w.user("Cli")
	org := w.team(ana, map[string]person{authz.RoleEditor: ed, authz.RoleViewer: vic, authz.RoleClient: cli})
	proj := w.project(ana, org)
	if err := w.projects.GrantAccess(w.ctx, ana.id, org, proj, cli.id); err != nil {
		t.Fatal(err)
	}

	// No transcript, a transcript in progress, or one with no speech.
	untranscribed := w.processed(ana, org, proj, "raw.mp4", 60_000, "aac")
	_, err := w.svc.RequestHighlights(w.ctx, ana.id, org, proj, untranscribed.ID, anyLength)
	wantField(t, err, "media")
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, untranscribed.ID); err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.RequestHighlights(w.ctx, ana.id, org, proj, untranscribed.ID, anyLength)
	wantField(t, err, "media")
	silent := w.transcribed(ana, org, proj, "silent.mp4", 0)
	_, err = w.svc.RequestHighlights(w.ctx, ana.id, org, proj, silent.ID, anyLength)
	wantField(t, err, "media")

	a := w.transcribed(ana, org, proj, "talk.mp4", 30) // 5 minutes
	for field, req := range map[string]HighlightRequest{
		"objective":   {Objective: strings.Repeat("a", ai.MaxBriefRunes+1), MinSeconds: 15, MaxSeconds: 60},
		"min_seconds": {MinSeconds: 2, MaxSeconds: 60},
		"max_seconds": {MinSeconds: 30, MaxSeconds: 32},
	} {
		_, err = w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID, req)
		wantField(t, err, field)
	}
	_, err = w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID, HighlightRequest{MinSeconds: 20, MaxSeconds: MaxClipSeconds + 1})
	wantField(t, err, "max_seconds")
	_, err = w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID, HighlightRequest{MinSeconds: 400, MaxSeconds: 500})
	wantField(t, err, "min_seconds") // longer than the file

	_, err = w.svc.RequestHighlights(w.ctx, vic.id, org, proj, a.ID, anyLength)
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.RequestHighlights(w.ctx, cli.id, org, proj, a.ID, anyLength)
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Highlights(w.ctx, ana.id, org, proj, a.ID)
	wantErr(t, err, ErrNotFound)

	// The organisation's chosen model and the brief are fixed on the run.
	if err := ai.SetModel(w.ctx, w.db, ana.id, org, ai.TaskHighlights, ai.ProviderGemini, "gemini-3.5-flash-lite"); err != nil {
		t.Fatal(err)
	}
	woke := 0
	w.svc.OnJobQueued(func() { woke++ })
	h, err := w.svc.RequestHighlights(w.ctx, ed.id, org, proj, a.ID,
		HighlightRequest{Objective: "  the funniest exchange  ", MinSeconds: 20, MaxSeconds: 45})
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != HighlightsQueued || h.Model != "gemini-3.5-flash-lite" || h.PromptVersion != ai.HighlightsPromptVersion ||
		h.Objective != "the funniest exchange" || h.MinMS != 20_000 || h.MaxMS != 45_000 || len(h.Clips) != 0 || woke != 1 {
		t.Fatalf("requested = %+v (woke %d)", h, woke)
	}
	_, err = w.svc.RequestHighlights(w.ctx, ed.id, org, proj, a.ID, anyLength)
	wantErr(t, err, ErrHighlightsInProgress)
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id = ? AND action = 'media.highlights_requested' AND metadata->>'model' = 'gemini-3.5-flash-lite'`, a.ID); n != 1 {
		t.Errorf("audit events = %d", n)
	}
	// Viewers and clients with access can read it.
	if _, err := w.svc.Highlights(w.ctx, cli.id, org, proj, a.ID); err != nil {
		t.Fatalf("client read: %v", err)
	}

	// Another organisation can't see or request them.
	eve := w.user("Eve")
	other := w.team(eve, nil)
	_, err = w.svc.Highlights(w.ctx, eve.id, other, proj, a.ID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.RequestHighlights(w.ctx, eve.id, org, proj, a.ID, anyLength)
	wantErr(t, err, ErrNotFound)
}

func TestFindHighlights(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.transcribed(ana, org, proj, "talk.mp4", 30)
	if _, err := w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID,
		HighlightRequest{Objective: "advice", MinSeconds: 20, MaxSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	p := &fakeHighlighter{clips: []ai.Highlight{
		{FirstSegment: 12, LastSegment: 15, Title: "Best", Reason: "Clear payoff."},
		{FirstSegment: 2, LastSegment: 3, Title: "Second", Reason: "Good setup."},
		{FirstSegment: 20, LastSegment: 20, Title: "Too short", Reason: "Dropped."},
	}}
	j := w.highlightsJob(a.ID)
	if err := w.highlighter(p).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 1 || p.calls[0] != (ai.HighlightBrief{Objective: "advice", MinMS: 20_000, MaxMS: 60_000}) ||
		p.lines[0] != 30 || p.models[0] != "gemini-3.5-flash" {
		t.Fatalf("provider got %+v, %v lines, %v", p.calls, p.lines, p.models)
	}
	h, err := w.svc.Highlights(w.ctx, ana.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != HighlightsReady || h.CompletedAt.IsZero() || len(h.Clips) != 2 {
		t.Fatalf("highlights = %+v", h)
	}
	if c := h.Clips[0]; c.Title != "Best" || c.StartMS != 120_000 || c.EndMS != 160_000 || c.FirstSegment != 12 || c.LastSegment != 15 {
		t.Errorf("best = %+v", c)
	}
	if h.Clips[1].Title != "Second" {
		t.Errorf("ranking lost: %+v", h.Clips)
	}
	if n := w.count(`SELECT count(*) FROM ai_usage WHERE job_id = ? AND task = 'highlights' AND succeeded AND input_tokens = 900`, j.ID); n != 1 {
		t.Errorf("usage rows = %d", n)
	}

	// Running again replaces the clips; a duplicate delivery of the old
	// job does nothing.
	if _, err := w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID, anyLength); err != nil {
		t.Fatal(err)
	}
	p.clips = p.clips[1:2]
	if err := w.highlighter(p).Process(w.ctx, w.highlightsJob(a.ID)); err != nil {
		t.Fatal(err)
	}
	if err := w.highlighter(p).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	if h, _ := w.svc.Highlights(w.ctx, ana.id, org, proj, a.ID); len(h.Clips) != 1 || h.Clips[0].Title != "Second" {
		t.Fatalf("after rerun: %+v", h.Clips)
	}
	if len(p.calls) != 2 {
		t.Errorf("provider called %d times, want 2", len(p.calls))
	}
}

func TestHighlightFailures(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.transcribed(ana, org, proj, "talk.mp4", 12)
	if _, err := w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID, anyLength); err != nil {
		t.Fatal(err)
	}
	j := w.highlightsJob(a.ID)

	// Invented line numbers are invalid output: retried, and counted.
	p := &fakeHighlighter{clips: []ai.Highlight{{FirstSegment: 40, LastSegment: 44, Title: "t", Reason: "r"}}}
	err := w.highlighter(p).Process(w.ctx, j)
	if err == nil || jobs.IsPermanent(err) {
		t.Fatalf("invalid output err = %v", err)
	}
	if n := w.count(`SELECT count(*) FROM ai_usage WHERE job_id = ? AND NOT succeeded`, j.ID); n != 1 {
		t.Errorf("failed usage rows = %d", n)
	}

	// A rejected request is permanent, and the user sees a reason.
	p = &fakeHighlighter{err: fmt.Errorf("%w: blocked", ai.ErrPermanent)}
	err = w.highlighter(p).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("permanent err = %v", err)
	}
	if err := w.highlighter(p).Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	h, _ := w.svc.Highlights(w.ctx, ana.id, org, proj, a.ID)
	if h.Status != HighlightsFailed || h.Error != msgHighlightsRejected {
		t.Fatalf("after failure: %+v", h)
	}

	// If the transcript is being redone when the job runs, it fails
	// permanently with a reason rather than using stale lines.
	if _, err := w.svc.RequestHighlights(w.ctx, ana.id, org, proj, a.ID, anyLength); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.RequestTranscript(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}
	j = w.highlightsJob(a.ID)
	err = w.highlighter(&fakeHighlighter{}).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("stale transcript err = %v", err)
	}
	_ = w.highlighter(p).Failed(w.ctx, j, err)
	if h, _ := w.svc.Highlights(w.ctx, ana.id, org, proj, a.ID); h.Error != msgHighlightsNoTranscript {
		t.Fatalf("stale transcript: %+v", h)
	}
}

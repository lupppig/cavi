package render

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

// memStore is an in-memory storage.Store.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	names   map[string]string
}

func (m *memStore) PresignUpload(context.Context, string, string, string, int64, time.Duration) (storage.Upload, error) {
	return storage.Upload{}, errors.New("not used")
}
func (m *memStore) PresignDownload(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return "https://storage.test/" + key + "?sig=x", time.Now().Add(ttl), nil
}
func (m *memStore) Stat(context.Context, string) (storage.ObjectInfo, error) {
	return storage.ObjectInfo{}, errors.New("not used")
}
func (m *memStore) ReadHead(context.Context, string, int64) ([]byte, error) {
	return nil, errors.New("not used")
}
func (m *memStore) Put(context.Context, string, string, []byte) error { return errors.New("not used") }

// Download writes the key as the file's contents (sources aren't stored).
func (m *memStore) Download(_ context.Context, key, path string) error {
	return os.WriteFile(path, []byte(key), 0o600)
}
func (m *memStore) PutFile(_ context.Context, key, _, filename, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key], m.names[key] = b, filename
	return nil
}
func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

// fakeFFmpeg writes a file for each clip and reports a probe matching the
// request unless told otherwise.
type fakeFFmpeg struct {
	mu       sync.Mutex
	ass      []string // the subtitle file's contents at render time
	clips    []ffmpeg.Clip
	montages []ffmpeg.Montage
	inputs   []string
	err      error
	probe    func(ffmpeg.Clip) ffmpeg.Probe
}

func (f *fakeFFmpeg) Clip(_ context.Context, input, output string, c ffmpeg.Clip, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clips = append(f.clips, c)
	f.inputs = append(f.inputs, input)
	if c.Subtitles != "" {
		b, _ := os.ReadFile(c.Subtitles)
		f.ass = append(f.ass, string(b))
	}
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(output, []byte("mp4 bytes"), 0o600)
}

// Montage records m and, so ProbeFile reports the montage, the clip it
// should come out as.
func (f *fakeFFmpeg) Montage(_ context.Context, output string, m ffmpeg.Montage, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.montages = append(f.montages, m)
	f.clips = append(f.clips, ffmpeg.Clip{Length: m.Length(), Width: m.Width, Height: m.Height, Audio: true})
	if m.Subtitles != "" {
		b, _ := os.ReadFile(m.Subtitles)
		f.ass = append(f.ass, string(b))
	}
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(output, []byte("mp4 bytes"), 0o600)
}

func (f *fakeFFmpeg) ProbeFile(context.Context, string) (ffmpeg.Probe, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.clips[len(f.clips)-1]
	if f.probe != nil {
		return f.probe(c), nil
	}
	return ffmpeg.Probe{HasVideo: true, Width: c.Width, Height: c.Height, HasAudio: c.Audio, Duration: c.Length + 20*time.Millisecond}, nil
}

type world struct {
	t        *testing.T
	ctx      context.Context
	db       *bun.DB
	accounts *account.Service
	projects *project.Service
	store    *memStore
	svc      *Service
}

func newWorld(t *testing.T) *world {
	db := testdb.Open(t)
	store := &memStore{objects: map[string][]byte{}, names: map[string]string{}}
	return &world{t: t, ctx: context.Background(), db: db, accounts: account.NewService(db), projects: project.NewService(db),
		store: store, svc: NewService(db, store, slog.New(slog.NewTextHandler(io.Discard, nil)))}
}

type person struct{ id, email string }

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_render_" + id, Email: "render-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person, members map[string]person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "rendertest team")
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() {
		_, _ = w.db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org.ID)
	})
	for role, p := range members {
		inv, err := w.accounts.CreateInvitation(w.ctx, owner.id, org.ID, p.email, role)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := w.accounts.AcceptInvitation(w.ctx, p.id, inv.ID); err != nil {
			w.t.Fatal(err)
		}
	}
	return org.ID
}

func (w *world) project(owner person, orgID string) string {
	w.t.Helper()
	p, err := w.projects.Create(w.ctx, owner.id, orgID, "", "Render test", "")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

// video adds a processed media file of the given kind and size.
func (w *world) video(orgID, projectID, kind string, width, height int, durationMS int64, audio bool) string {
	w.t.Helper()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	codec := ""
	if audio {
		codec = "aac"
	}
	var id string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO media_assets (organisation_id, project_id, storage_key, filename, content_type, size_bytes, status, kind,
			uploaded_at, processing_status, width, height, duration_ms, audio_codec)
		VALUES (?, ?, ?, 'talk.mp4', 'video/mp4', 100, 'uploaded', ?, now(), 'ready', NULLIF(?, 0), NULLIF(?, 0), ?, NULLIF(?, ''))
		RETURNING id`, orgID, projectID, "rendertest/"+hex.EncodeToString(b), kind, width, height, durationMS, codec).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *world) count(q string, args ...any) int {
	w.t.Helper()
	var n int
	if err := w.db.QueryRowContext(w.ctx, q, args...).Scan(&n); err != nil {
		w.t.Fatal(err)
	}
	return n
}

func (w *world) renderer(ff *fakeFFmpeg) *Renderer {
	r := NewRenderer(w.db, w.store, ff, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.TempDir = w.t.TempDir()
	return r
}

func (w *world) job(renderID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{RenderID: renderID, Kind: jobs.KindRenderClip}
	if err := w.db.QueryRowContext(w.ctx, `SELECT id, organisation_id FROM jobs WHERE render_id = ?`, renderID).
		Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no render job: %v", err)
	}
	return j
}

func wantErr(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	var v *account.ValidationError
	if !errors.As(err, &v) || v.Field != field {
		t.Fatalf("err = %v, want a validation error on %s", err, field)
	}
}

func vertical(source string) Composition {
	return Composition{Version: 1, SourceAssetID: source, StartMS: 87_000, EndMS: 150_000, Aspect: "9:16", Focus: 0.5}
}

func TestCreateRenderRules(t *testing.T) {
	w := newWorld(t)
	ana, ed, vic, cli := w.user("Ana"), w.user("Ed"), w.user("Vic"), w.user("Cli")
	org := w.team(ana, map[string]person{authz.RoleEditor: ed, authz.RoleViewer: vic, authz.RoleClient: cli})
	proj := w.project(ana, org)
	if err := w.projects.GrantAccess(w.ctx, ana.id, org, proj, cli.id); err != nil {
		t.Fatal(err)
	}
	src := w.video(org, proj, "video", 1920, 1080, 151_000, true)

	// Sources must be processed videos in this project.
	audio := w.video(org, proj, "audio", 0, 0, 151_000, true)
	otherProj := w.project(ana, org)
	elsewhere := w.video(org, otherProj, "video", 1920, 1080, 151_000, true)
	for _, id := range []string{audio, elsewhere, "not-a-uuid", "00000000-0000-4000-8000-000000000000"} {
		_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Clip", vertical(id), nil)
		wantField(t, err, "composition.source_asset_id")
	}
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "  ", vertical(src), nil)
	wantField(t, err, "name")
	c := vertical(src)
	c.EndMS = 160_000
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Clip", c, nil)
	wantField(t, err, "composition.end_ms")

	// Viewers and clients can't render.
	_, err = w.svc.Create(w.ctx, vic.id, org, proj, "Clip", vertical(src), nil)
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Create(w.ctx, cli.id, org, proj, "Clip", vertical(src), nil)
	wantErr(t, err, ErrForbidden)

	woke := 0
	w.svc.OnJobQueued(func() { woke++ })
	r, err := w.svc.Create(w.ctx, ed.id, org, proj, " Release \t advice ", vertical(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusQueued || r.Name != "Release advice" || !reflect.DeepEqual(r.Composition, vertical(src)) || r.SourceAssetID != src ||
		r.RequestedByID != ed.id || r.URL != "" || !r.CanDelete || woke != 1 {
		t.Fatalf("created = %+v (woke %d)", r, woke)
	}
	if n := w.count(`SELECT count(*) FROM jobs WHERE render_id = ? AND kind = 'render.clip' AND organisation_id = ?`, r.ID, org); n != 1 {
		t.Errorf("render jobs = %d", n)
	}
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id = ? AND action = 'render.requested'`, r.ID); n != 1 {
		t.Errorf("audit events = %d", n)
	}

	// Clients with access can see it but not delete it.
	got, err := w.svc.Get(w.ctx, cli.id, org, proj, r.ID)
	if err != nil || got.CanDelete {
		t.Fatalf("client get = %+v, %v", got, err)
	}
	wantErr(t, w.svc.Delete(w.ctx, cli.id, org, proj, r.ID), ErrForbidden)

	// Another organisation sees nothing, even with the right IDs.
	eve := w.user("Eve")
	evesOrg := w.team(eve, nil)
	_, err = w.svc.Get(w.ctx, eve.id, org, proj, r.ID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Get(w.ctx, eve.id, evesOrg, proj, r.ID)
	wantErr(t, err, ErrNotFound)
	_, err = w.svc.Create(w.ctx, eve.id, evesOrg, w.project(eve, evesOrg), "Steal", vertical(src), nil)
	wantField(t, err, "composition.source_asset_id")
	wantErr(t, w.svc.Delete(w.ctx, eve.id, org, proj, r.ID), ErrNotFound)
}

func TestListRenders(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	a := w.video(org, proj, "video", 1920, 1080, 151_000, true)
	b := w.video(org, proj, "video", 1080, 1920, 60_000, false)
	for i := range 3 {
		if _, err := w.svc.Create(w.ctx, ana.id, org, proj, fmt.Sprintf("A%d", i), vertical(a), nil); err != nil {
			t.Fatal(err)
		}
	}
	c := vertical(b)
	c.StartMS, c.EndMS = 0, 10_000
	if _, err := w.svc.Create(w.ctx, ana.id, org, proj, "B", c, nil); err != nil {
		t.Fatal(err)
	}
	page, err := w.svc.List(w.ctx, ana.id, org, proj, a, "", 2)
	if err != nil || len(page.Renders) != 2 || page.NextCursor == "" || page.Renders[0].Name != "A2" {
		t.Fatalf("page 1 = %+v, %v", page, err)
	}
	page, err = w.svc.List(w.ctx, ana.id, org, proj, a, page.NextCursor, 2)
	if err != nil || len(page.Renders) != 1 || page.NextCursor != "" || page.Renders[0].Name != "A0" {
		t.Fatalf("page 2 = %+v, %v", page, err)
	}
	all, err := w.svc.List(w.ctx, ana.id, org, proj, "", "", 0)
	if err != nil || len(all.Renders) != 4 {
		t.Fatalf("all = %d, %v", len(all.Renders), err)
	}
	_, err = w.svc.List(w.ctx, ana.id, org, proj, "", "garbage", 0)
	wantField(t, err, "cursor")
	_, err = w.svc.List(w.ctx, ana.id, org, proj, "", "", 500)
	wantField(t, err, "limit")
}

func TestRenderClip(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 151_000, true)
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Release: advice?", vertical(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	ff := &fakeFFmpeg{}
	j := w.job(r.ID)
	if err := w.renderer(ff).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	want := ffmpeg.Clip{Start: 87 * time.Second, Length: 63 * time.Second, CropX: 656, CropW: 608, CropH: 1080,
		Width: 1080, Height: 1920, Audio: true}
	if len(ff.clips) != 1 || !reflect.DeepEqual(ff.clips[0], want) {
		t.Fatalf("clips = %+v", ff.clips)
	}
	got, err := w.svc.Get(w.ctx, ana.id, org, proj, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	key := outputKey(org, proj, r.ID)
	if got.Status != StatusReady || got.URL == "" || got.SizeBytes != 9 || got.Width != 1080 || got.Height != 1920 ||
		got.DurationMS != 63_020 || got.CompletedAt.IsZero() {
		t.Fatalf("rendered = %+v", got)
	}
	if string(w.store.objects[key]) != "mp4 bytes" || w.store.names[key] != "Release- advice-.mp4" {
		t.Fatalf("stored %q as %q", w.store.objects[key], w.store.names[key])
	}
	if n := w.count(`SELECT count(*) FROM renders WHERE id = ? AND render_ms IS NOT NULL`, r.ID); n != 1 {
		t.Error("render time not recorded")
	}
	// A duplicate delivery does nothing.
	if err := w.renderer(ff).Process(w.ctx, j); err != nil || len(ff.clips) != 1 {
		t.Fatalf("rerun: %v, %d clips", err, len(ff.clips))
	}

	// Deleting the source keeps the clip; deleting the clip removes its file.
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM media_assets WHERE id = ?`, src); err != nil {
		t.Fatal(err)
	}
	got, err = w.svc.Get(w.ctx, ana.id, org, proj, r.ID)
	if err != nil || got.Status != StatusReady || got.SourceAssetID != "" {
		t.Fatalf("after source deleted = %+v, %v", got, err)
	}
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, r.ID); err != nil {
		t.Fatal(err)
	}
	if n := w.count(`SELECT count(*) FROM storage_deletions WHERE storage_key = ?`, key); n != 1 {
		t.Errorf("output not queued for deletion (%d)", n)
	}
	_, err = w.svc.Get(w.ctx, ana.id, org, proj, r.ID)
	wantErr(t, err, ErrNotFound)
}

func TestRenderFailures(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 151_000, false)
	status := func(id string) (string, string) {
		r, err := w.svc.Get(w.ctx, ana.id, org, proj, id)
		if err != nil {
			t.Fatal(err)
		}
		return r.Status, r.Error
	}

	// Output that isn't what was asked for is never kept.
	r, _ := w.svc.Create(w.ctx, ana.id, org, proj, "Wrong size", vertical(src), nil)
	ff := &fakeFFmpeg{probe: func(c ffmpeg.Clip) ffmpeg.Probe {
		return ffmpeg.Probe{HasVideo: true, Width: 640, Height: 360, Duration: c.Length}
	}}
	j := w.job(r.ID)
	err := w.renderer(ff).Process(w.ctx, j)
	if !jobs.IsPermanent(err) || !errors.Is(err, errBadOutput) {
		t.Fatalf("bad output err = %v", err)
	}
	_ = w.renderer(ff).Failed(w.ctx, j, err)
	if s, msg := status(r.ID); s != StatusFailed || msg != msgBadOutput || len(w.store.objects) != 0 {
		t.Fatalf("bad output: %s %q, %d stored", s, msg, len(w.store.objects))
	}
	// A clip that's too short is caught too; a silent source makes a silent clip.
	if err := check(ffmpeg.Probe{HasVideo: true, Width: 1080, Height: 1920, Duration: 50 * time.Second},
		Plan(vertical(src), Source{Width: 1920, Height: 1080, DurationMS: 151_000}, nil)); !errors.Is(err, errBadOutput) {
		t.Errorf("short output passed: %v", err)
	}
	if ff.clips[0].Audio {
		t.Error("asked for audio from a silent source")
	}

	// Storage hiccups retry.
	r, _ = w.svc.Create(w.ctx, ana.id, org, proj, "Flaky", vertical(src), nil)
	ff = &fakeFFmpeg{err: errors.New("ffmpeg: ffmpeg could not fetch the input: Server returned 503")}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("transient err = %v", err)
	}

	// Unreadable sources fail for good.
	r, _ = w.svc.Create(w.ctx, ana.id, org, proj, "Corrupt", vertical(src), nil)
	ff = &fakeFFmpeg{err: fmt.Errorf("%w: moov atom not found", ffmpeg.ErrUnreadable)}
	j = w.job(r.ID)
	err = w.renderer(ff).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("unreadable err = %v", err)
	}
	_ = w.renderer(ff).Failed(w.ctx, j, err)
	if s, msg := status(r.ID); s != StatusFailed || msg != msgUnreadable {
		t.Fatalf("unreadable: %s %q", s, msg)
	}

	// If the source goes before the job runs, it fails with a reason and
	// never calls FFmpeg.
	r, _ = w.svc.Create(w.ctx, ana.id, org, proj, "Orphan", vertical(src), nil)
	j = w.job(r.ID)
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM media_assets WHERE id = ?`, src); err != nil {
		t.Fatal(err)
	}
	ff = &fakeFFmpeg{}
	err = w.renderer(ff).Process(w.ctx, j)
	if !jobs.IsPermanent(err) || !errors.Is(err, errSourceGone) || len(ff.clips) != 0 {
		t.Fatalf("source gone err = %v, %d clips", err, len(ff.clips))
	}
	_ = w.renderer(ff).Failed(w.ctx, j, err)
	if s, msg := status(r.ID); s != StatusFailed || msg != msgSourceGone {
		t.Fatalf("source gone: %s %q", s, msg)
	}
}

// transcribe gives a file a ready transcript.
func (w *world) transcribe(orgID, assetID string, segs ...[3]any) {
	w.t.Helper()
	var id string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO transcripts (organisation_id, media_asset_id, status, provider, model, prompt_version, completed_at)
		VALUES (?, ?, 'ready', 'gemini', 'gemini-3.5-flash-lite', 'transcription-v2', now()) RETURNING id`, orgID, assetID).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	for i, sg := range segs {
		if _, err := w.db.ExecContext(w.ctx, `
			INSERT INTO transcript_segments (transcript_id, position, start_ms, end_ms, speaker, text) VALUES (?, ?, ?, ?, 'Speaker 1', ?)`,
			id, i, sg[0], sg[1], sg[2]); err != nil {
			w.t.Fatal(err)
		}
	}
}

func TestRenderWithCaptions(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 151_000, true)
	c := vertical(src) // 87 s to 150 s

	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Clip", c, &CaptionOptions{Style: CaptionsBold})
	wantField(t, err, "captions") // no transcript yet
	w.transcribe(org, src,
		[3]any{80_000, 90_000, "So the power goes out."},
		[3]any{90_000, 95_000, "I stood on a chair."},
		[3]any{160_000, 162_000, "After the clip."})
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Clip", c, &CaptionOptions{Style: "neon"})
	wantField(t, err, "captions.style")
	quiet := c
	quiet.StartMS, quiet.EndMS = 100_000, 140_000
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Clip", quiet, &CaptionOptions{Style: CaptionsBold})
	wantField(t, err, "captions") // no speech in that stretch

	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Captioned", c, &CaptionOptions{Style: CaptionsBoxed})
	if err != nil {
		t.Fatal(err)
	}
	cap := r.Composition.Captions
	// The clip starts at 87 s, inside "So the power goes out." (80-90 s):
	// only the words spoken inside the clip are captioned.
	if r.Composition.Version != CompositionVersion || cap == nil || cap.Style != CaptionsBoxed || len(cap.Cues) != 2 ||
		cap.Cues[0].Text != "out." || cap.Cues[0].StartMS < 500 || cap.Cues[0].EndMS != 3000 ||
		cap.Cues[1].StartMS != 3000 || cap.Cues[1].EndMS != 8000 || cap.Cues[1].Speaker != "Speaker 1" {
		t.Fatalf("stored composition = %+v, captions %+v", r.Composition, cap)
	}
	// Cues sent by a client are ignored: they only ever come from the transcript.
	forged := vertical(src)
	forged.Captions = &Captions{CaptionOptions: CaptionOptions{Style: CaptionsBold}, Cues: []Cue{{StartMS: 0, EndMS: 1000, Text: "Not what they said"}}}
	r2, err := w.svc.Create(w.ctx, ana.id, org, proj, "Plain", forged, nil)
	if err != nil || r2.Composition.Captions != nil {
		t.Fatalf("client cues kept: %+v, %v", r2.Composition.Captions, err)
	}

	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	if len(ff.clips) != 1 || ff.clips[0].FontsDir != DefaultFontsDir || len(ff.ass) != 1 ||
		!strings.Contains(ff.ass[0], ",0:00:03.00,Cap,,0,0,0,,{\\1c&H00FFFFFF&}out.\n") {
		t.Fatalf("clip %+v, ass %q", ff.clips, ff.ass)
	}
	if err := w.renderer(ff).Process(w.ctx, w.job(r2.ID)); err != nil || ff.clips[1].Subtitles != "" {
		t.Fatalf("plain render had subtitles: %+v, %v", ff.clips[1], err)
	}
}

// briefRules gives the project a ready brief with these rules on assetID.
func (w *world) briefRules(org, proj, assetID string, rules ...ai.BriefRule) string {
	w.t.Helper()
	var id string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO briefs (organisation_id, project_id, media_asset_id, source_text, status, provider, model, prompt_version, completed_at)
		VALUES (?, ?, ?, 'brief', 'ready', 'gemini', 'gemini-3.5-flash', 'briefs-v1', now()) RETURNING id`,
		org, proj, assetID).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	for i, r := range rules {
		w.addRule(org, id, assetID, i, r)
	}
	return id
}

func (w *world) addRule(org, briefID, assetID string, position int, r ai.BriefRule) {
	w.t.Helper()
	if _, err := w.db.ExecContext(w.ctx, `
		INSERT INTO brief_rules (brief_id, position, organisation_id, media_asset_id, kind, start_ms, end_ms, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, briefID, position, org, assetID, r.Kind, r.StartMS, r.EndMS, r.Note); err != nil {
		w.t.Fatal(err)
	}
}

func TestRendersFollowTheBrief(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 600_000, true)
	briefID := w.briefRules(org, proj, src,
		ai.BriefRule{Kind: ai.RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "Not to be used"},
		ai.BriefRule{Kind: ai.RuleMute, StartMS: 100_000, EndMS: 100_000, Note: "Copyright music"},
	)
	clip := func(start, end int64) Composition {
		return Composition{Version: 1, SourceAssetID: src, StartMS: start, EndMS: end, Aspect: "9:16", Focus: 0.5}
	}

	// Any overlap with an excluded range is refused, at either end.
	for _, c := range []Composition{clip(30_000, 48_000), clip(50_000, 70_000), clip(40_000, 60_000)} {
		_, err := w.svc.Create(w.ctx, ana.id, org, proj, "x", c, nil)
		wantField(t, err, "composition.start_ms")
		if !strings.Contains(err.Error(), "0:47–0:53") {
			t.Errorf("message %q doesn't say which part is excluded", err)
		}
	}
	// Right up to its edges is fine; touching a mute rule is flagged.
	before, err := w.svc.Create(w.ctx, ana.id, org, proj, "before", clip(30_000, 47_000), nil)
	if err != nil {
		t.Fatal(err)
	}
	music, err := w.svc.Create(w.ctx, ana.id, org, proj, "music", clip(53_000, 120_000), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Flags) != 0 || len(music.Flags) != 1 || music.Flags[0].Kind != ai.RuleMute {
		t.Fatalf("flags: before %+v, music %+v", before.Flags, music.Flags)
	}
	page, err := w.svc.List(w.ctx, ana.id, org, proj, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range page.Renders {
		if (r.ID == music.ID) != (len(r.Flags) == 1) {
			t.Errorf("listed %s flags = %+v", r.Name, r.Flags)
		}
	}

	// If the brief changes to exclude a queued clip, it isn't rendered.
	w.addRule(org, briefID, src, 2, ai.BriefRule{Kind: ai.RuleExclude, StartMS: 110_000, EndMS: 110_000, Note: "Alcohol"})
	ff := &fakeFFmpeg{}
	j := w.job(music.ID)
	err = w.renderer(ff).Process(w.ctx, j)
	if !jobs.IsPermanent(err) || len(ff.clips) != 0 {
		t.Fatalf("render over a newly excluded moment: err %v, clips %d", err, len(ff.clips))
	}
	if err := w.renderer(ff).Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	if r, _ := w.svc.Get(w.ctx, ana.id, org, proj, music.ID); r.Status != StatusFailed || r.Error != msgExcluded {
		t.Fatalf("render = %s %q", r.Status, r.Error)
	}
}

func TestCheckCatchesShortVideo(t *testing.T) {
	want := ffmpeg.Clip{Length: 60 * time.Second, Width: 1080, Height: 1920, Audio: true}
	ok := ffmpeg.Probe{HasVideo: true, HasAudio: true, Width: 1080, Height: 1920, Duration: 60 * time.Second, VideoDuration: 60 * time.Second}
	if err := check(ok, want); err != nil {
		t.Fatal(err)
	}
	short := ok
	short.VideoDuration = 34500 * time.Millisecond
	if err := check(short, want); !errors.Is(err, errBadOutput) {
		t.Fatalf("video stopping at 34.5 s under 60 s of audio passed: %v", err)
	}
}

// analysed stores a framing track for asset with one face at x (as a
// fraction of the width) throughout.
func (w *world) analysed(org, asset string, x float64, ms int64) {
	w.t.Helper()
	tr := framing.Track{Version: framing.Version, IntervalMS: framing.FaceIntervalMS, Cuts: []int64{}}
	for at := int64(0); at < ms; at += framing.FaceIntervalMS {
		tr.Faces = append(tr.Faces, []framing.Face{{X: x, Y: 0.4, S: 0.3, Q: 0.9}})
	}
	raw, _ := json.Marshal(tr)
	if _, err := w.db.ExecContext(w.ctx, `INSERT INTO media_framing (media_asset_id, organisation_id, analyser_version, track) VALUES (?, ?, ?, ?)`,
		asset, org, framing.Version, string(raw)); err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET framing_status = 'ready' WHERE id = ?`, asset); err != nil {
		w.t.Fatal(err)
	}
}

func TestRenderAutoFraming(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 151_000, true)
	c := vertical(src)
	c.Version, c.Framing = CompositionVersion, FramingAuto
	// Not analysed yet.
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Auto", c, nil)
	wantField(t, err, "composition.framing")
	bad := vertical(src)
	bad.Framing = "wobbly"
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Bad", bad, nil)
	wantField(t, err, "composition.framing")

	w.analysed(org, src, 0.8, 151_000)
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Auto", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Composition.Framing != FramingAuto {
		t.Fatalf("framing not kept: %+v", r.Composition)
	}
	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	// The crop (608 wide in 1920) sits on the face at 0.8: its left edge
	// at 0.8×1920 − 304 = 1232, held still.
	p := ff.clips[0].Pan
	if len(p) == 0 || ff.clips[0].PanVertical || p[0].At != 0 {
		t.Fatalf("pan = %+v", ff.clips[0])
	}
	for _, k := range p {
		if k.Pos < 1228 || k.Pos > 1236 {
			t.Fatalf("pan = %+v, want about 1232", p)
		}
	}

	// A montage follows faces in every segment.
	m := Composition{Version: MontageVersion, Aspect: "9:16", Framing: FramingAuto, Segments: []Segment{
		{SourceAssetID: src, StartMS: 1000, EndMS: 5000, Focus: 0.5},
		{SourceAssetID: src, StartMS: 9000, EndMS: 12000, Focus: 0.5},
	}}
	mr, err := w.svc.Create(w.ctx, ana.id, org, proj, "Auto montage", m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.renderer(ff).Process(w.ctx, w.job(mr.ID)); err != nil {
		t.Fatal(err)
	}
	for i, part := range ff.montages[0].Parts {
		if len(part.Pan) == 0 || part.Pan[0].Pos < 1228 || part.Pan[0].Pos > 1236 {
			t.Fatalf("part %d pan = %+v", i, part.Pan)
		}
	}

	// Reprocessing the source since the clip was requested loses its
	// analysis: the render fails for good with a reason.
	again, err := w.svc.Create(w.ctx, ana.id, org, proj, "Auto again", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET framing_status = 'pending' WHERE id = ?`, src); err != nil {
		t.Fatal(err)
	}
	err = w.renderer(ff).Process(w.ctx, w.job(again.ID))
	if !jobs.IsPermanent(err) || !errors.Is(err, errFramingGone) {
		t.Fatalf("err = %v", err)
	}
	if err := w.renderer(ff).Failed(w.ctx, w.job(again.ID), err); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.svc.Get(w.ctx, ana.id, org, proj, again.ID); got.Error != msgFramingGone {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestPanFollowsTheShape(t *testing.T) {
	keys := []framing.Key{{AtMS: 0, Focus: 0}, {AtMS: 1000, Focus: 1}}
	// 16:9 made 9:16 pans across.
	p, vert := pan(keys, Shape{Aspect: "9:16", Width: 1080, Height: 1920}, Source{Width: 1920, Height: 1080})
	if vert || p[0].Pos != 0 || p[1].Pos != 1312 || p[1].At != time.Second {
		t.Fatalf("horizontal pan = %+v, %v", p, vert)
	}
	// 9:16 made 16:9 pans up and down.
	p, vert = pan(keys, Shape{Aspect: "16:9", Width: 1920, Height: 1080}, Source{Width: 1080, Height: 1920})
	if !vert || p[1].Pos != 1920-608 {
		t.Fatalf("vertical pan = %+v, %v", p, vert)
	}
	// Auto framing needs version 2 or later.
	c := Composition{Version: 1, SourceAssetID: "x", EndMS: 2000, Aspect: "9:16", Focus: 0.5, Framing: FramingAuto}
	if err := c.Validate(Source{Width: 1920, Height: 1080, DurationMS: 5000}); err == nil {
		t.Fatal("version 1 accepted auto framing")
	}
}

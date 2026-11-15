package media

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

func TestImportSource(t *testing.T) {
	good := map[string][2]string{
		"https://www.youtube.com/watch?v=abc123&t=42s":     {"https://www.youtube.com/watch?v=abc123&t=42s", "YouTube"},
		"youtu.be/abc123":                                  {"https://youtu.be/abc123", "YouTube"},
		" https://YouTube.com/shorts/abc123 ":              {"https://youtube.com/shorts/abc123", "YouTube"},
		"http://m.youtube.com/watch?v=abc#frag":            {"https://m.youtube.com/watch?v=abc", "YouTube"},
		"https://www.twitch.tv/videos/123456789":           {"https://www.twitch.tv/videos/123456789", "Twitch"},
		"https://clips.twitch.tv/FunnyClipName":            {"https://clips.twitch.tv/FunnyClipName", "Twitch"},
		"https://kick.com/somestreamer/videos/1234-abcd":   {"https://kick.com/somestreamer/videos/1234-abcd", "Kick"},
		"https://kick.com/somestreamer?clip=clip_01ABCDEF": {"https://kick.com/somestreamer?clip=clip_01ABCDEF", "Kick"},
	}
	for in, want := range good {
		got, platform, err := ImportSource(in)
		if err != nil || got != want[0] || platform != want[1] {
			t.Errorf("ImportSource(%q) = %q, %q, %v; want %q, %q", in, got, platform, err, want[0], want[1])
		}
	}
	for _, bad := range []string{
		"", "not a link", "https://vimeo.com/123", "https://youtube.com.evil.example/watch?v=x",
		"https://user:pw@youtube.com/watch?v=x", "https://youtube.com:8443/watch?v=x", "ftp://youtube.com/watch?v=x",
		"file:///etc/passwd", "https://youtube.com/", "https://youtu.be/", "https://www.youtube.com/playlist?list=PL123",
		"https://localhost/watch", "https://169.254.169.254/latest", "https://youtube.com/" + strings.Repeat("a", 2100),
	} {
		if _, _, err := ImportSource(bad); err == nil {
			t.Errorf("ImportSource(%q) accepted", bad)
		}
	}
}

func TestImportFilename(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"Marlon in Botswana: the week", ".mp4"}: "Marlon in Botswana: the week.mp4",
		{"a/b\\c", ".webm"}:                      "c.webm",
		{"  ", ".mp4"}:                           "Imported video.mp4",
		{"Title", ""}:                            "Title.mp4",
		{strings.Repeat("é", 300), ".mkv"}:       strings.Repeat("é", 251) + ".mkv",
	} {
		if got := importFilename(in[0], in[1]); got != want {
			t.Errorf("importFilename(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// fakeFetcher "downloads" body as source.mp4, or fails.
type fakeFetcher struct {
	body  []byte
	err   error
	calls int
	got   string
	after func()
}

func (f *fakeFetcher) Fetch(_ context.Context, link, dir string) (Fetched, error) {
	f.calls++
	f.got = link
	if f.after != nil {
		defer f.after()
	}
	if f.err != nil {
		return Fetched{}, f.err
	}
	p := filepath.Join(dir, "source.mp4")
	if err := os.WriteFile(p, f.body, 0o600); err != nil {
		return Fetched{}, err
	}
	return Fetched{Path: p, Title: "Marlon’s week in Botswana"}, nil
}

func (w *world) importer(f Fetcher) *Importer {
	return NewImporter(w.db, w.store, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (w *world) importJob(assetID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{MediaAssetID: assetID, Kind: jobs.KindMediaImport}
	if err := w.db.QueryRowContext(w.ctx, `
		SELECT id, organisation_id FROM jobs WHERE media_asset_id = ? AND kind = 'media.import' AND status = 'queued'`, assetID).
		Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no import job for %s: %v", assetID, err)
	}
	return j
}

func TestImportFromLink(t *testing.T) {
	w := newWorld(t)
	ana, ed, vic := w.user("Ana"), w.user("Ed"), w.user("Vic")
	org := w.team(ana, map[string]person{authz.RoleEditor: ed, authz.RoleViewer: vic})
	proj := w.project(ana, org)
	link := "https://www.youtube.com/watch?v=abc123"

	_, err := w.svc.CreateImport(w.ctx, ana.id, org, proj, "https://vimeo.com/1", true)
	var verr *account.ValidationError
	if !errors.As(err, &verr) || verr.Field != "url" {
		t.Fatalf("unsupported link: %v", err)
	}
	if _, err := w.svc.CreateImport(w.ctx, ana.id, org, proj, link, false); !errors.As(err, &verr) || verr.Field != "rights_confirmed" {
		t.Fatalf("rights not confirmed: %v", err)
	}
	if _, err := w.svc.CreateImport(w.ctx, vic.id, org, proj, link, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer: %v", err)
	}
	if _, err := w.svc.CreateImport(w.ctx, w.user("Eve").id, org, proj, link, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}

	a, err := w.svc.CreateImport(w.ctx, ana.id, org, proj, link, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "importing" || a.SourceURL != link || a.Filename != "YouTube video" || a.URL != "" {
		t.Fatalf("import = %+v", a)
	}
	if n := w.count(`SELECT count(*) FROM audit_events WHERE action = 'media.import_requested' AND target_id = ? AND metadata->>'rights_confirmed' = 'true'`, a.ID); n != 1 {
		t.Fatalf("rights confirmation not audited (%d)", n)
	}
	// Everyone on the project sees it importing; it can't be "completed".
	page, err := w.svc.List(w.ctx, vic.id, org, proj, "", 0)
	if err != nil || len(page.Assets) != 1 || page.Assets[0].Status != "importing" {
		t.Fatalf("list = %+v, %v", page.Assets, err)
	}
	if _, err := w.svc.Complete(w.ctx, ana.id, org, proj, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("complete an import: %v", err)
	}

	f := &fakeFetcher{body: mp4}
	j := w.importJob(a.ID)
	if err := w.importer(f).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.Get(w.ctx, ed.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if f.got != link || got.Status != "uploaded" || got.Kind != KindVideo || got.SizeBytes != int64(len(mp4)) ||
		got.Filename != "Marlon’s week in Botswana.mp4" || got.URL == "" || got.ProcessingStatus != ProcessingPending {
		t.Fatalf("imported = %+v", got)
	}
	if !w.store.has(storageKey(org, proj, a.ID)) {
		t.Fatal("file not stored")
	}
	if n := w.count(`SELECT count(*) FROM jobs WHERE media_asset_id = ? AND kind = 'media.process'`, a.ID); n != 1 {
		t.Fatalf("processing jobs = %d", n)
	}
	// A duplicate delivery does nothing.
	if err := w.importer(f).Process(w.ctx, j); err != nil || f.calls != 1 {
		t.Fatalf("rerun: %v, %d downloads", err, f.calls)
	}
}

func TestImportFailures(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	start := func() Asset {
		a, err := w.svc.CreateImport(w.ctx, ana.id, org, proj, "https://kick.com/someone/videos/abc", true)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	// A refusal fails for good, with its reason.
	a := start()
	j := w.importJob(a.ID)
	err := w.importer(&fakeFetcher{err: &ImportError{Reason: "That video is private, so it can’t be imported."}}).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("refusal: %v", err)
	}
	if err := w.importer(nil).Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	got, _ := w.svc.Get(w.ctx, ana.id, org, proj, a.ID)
	if got.Status != "importing" || got.ImportError != "That video is private, so it can’t be imported." {
		t.Fatalf("after refusal = %+v", got)
	}
	// It can be deleted.
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, a.ID); err != nil {
		t.Fatal(err)
	}

	// A network failure is retried.
	b := start()
	if err := w.importer(&fakeFetcher{err: errors.New("yt-dlp failed: connection reset")}).Process(w.ctx, w.importJob(b.ID)); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("network failure: %v", err)
	}
	// Something that isn't a video is refused.
	if err := w.importer(&fakeFetcher{body: []byte("<html>nope</html>")}).Process(w.ctx, w.importJob(b.ID)); !jobs.IsPermanent(err) {
		t.Fatalf("not a video: %v", err)
	}

	// Deleted while downloading: the stored file is queued for deletion.
	c := start()
	f := &fakeFetcher{body: mp4, after: func() {
		if err := w.svc.Delete(w.ctx, ana.id, org, proj, c.ID); err != nil {
			t.Fatal(err)
		}
	}}
	if err := w.importer(f).Process(w.ctx, w.importJob(c.ID)); err != nil {
		t.Fatal(err)
	}
	if w.queued(storageKey(org, proj, c.ID)) != 2 {
		t.Fatalf("object queued %d times, want 2 (on delete, and after storing)", w.queued(storageKey(org, proj, c.ID)))
	}
}

// TestYTDLP runs Fetch against a stand-in yt-dlp script.
func TestYTDLP(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
case "$*" in
  *private*) echo "ERROR: [youtube] x: Private video. Sign in if you've been granted access" >&2; exit 1 ;;
  *flaky*) echo "ERROR: unable to download: connection reset" >&2; exit 1 ;;
  *--dump-single-json*live*) echo '{"_type":"video","title":"Live","live_status":"is_live"}' ;;
  *--dump-single-json*long*) echo '{"_type":"video","title":"Long","duration":20000}' ;;
  *--dump-single-json*list*) echo '{"_type":"playlist","title":"List"}' ;;
  *--dump-single-json*) echo '{"_type":"video","title":"A video","duration":61.5}' ;;
  *--load-info-json*) out="$PWD/source.mp4"; printf 'video' > "$out"; echo "[info] downloading"; echo "$out" ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	y := YTDLP{Bin: bin}
	work := t.TempDir()
	got, err := y.Fetch(context.Background(), "https://youtu.be/ok", work)
	if err != nil || got.Title != "A video" || got.Path != filepath.Join(work, "source.mp4") {
		t.Fatalf("fetch = %+v, %v", got, err)
	}
	for link, want := range map[string]string{
		"https://youtu.be/private": "private",
		"https://youtu.be/live":    "live stream",
		"https://youtu.be/long":    "4 hours",
		"https://youtu.be/list":    "playlist",
	} {
		_, err := y.Fetch(context.Background(), link, t.TempDir())
		var refused *ImportError
		if !errors.As(err, &refused) || !strings.Contains(refused.Reason, want) {
			t.Errorf("%s: err = %v, want a refusal mentioning %q", link, err, want)
		}
	}
	_, err = y.Fetch(context.Background(), "https://youtu.be/flaky", t.TempDir())
	var refused *ImportError
	if err == nil || errors.As(err, &refused) {
		t.Fatalf("flaky: %v", err)
	}
	if _, err := (YTDLP{Bin: filepath.Join(dir, "missing")}).Fetch(context.Background(), "https://youtu.be/ok", t.TempDir()); !errors.As(err, &refused) {
		t.Fatalf("missing yt-dlp: %v", err)
	}
}

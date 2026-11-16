package youtube

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

// fakeGoogle answers like Google's OAuth, Data and Analytics endpoints.
type fakeGoogle struct {
	mu        sync.Mutex
	revoked   bool
	channel   string
	scope     string
	refreshes int
	revokes   int
}

func (f *fakeGoogle) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		if r.Form.Get("client_secret") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "good-code" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access-1", "refresh_token": "refresh-SECRET", "scope": f.scope})
		case "refresh_token":
			f.refreshes++
			if f.revoked || r.Form.Get("refresh_token") != "refresh-SECRET" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access-2"})
		}
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.revokes++
		f.mu.Unlock()
	})
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer access-") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/data/channels", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		id := f.channel
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"items":[{"id":"` + id + `","snippet":{"title":"IshowClipse"},"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
	}))
	mux.HandleFunc("/data/playlistItems", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"nextPageToken":"p2","items":[{"contentDetails":{"videoId":"v1"}},{"contentDetails":{"videoId":"v2"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"v3"}}]}`))
	}))
	mux.HandleFunc("/data/videos", auth(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
			{"id":"v1","snippet":{"title":"Marlon's First Day in Botswana 🇧🇼 Didn't Expect This! 👀","publishedAt":"2026-11-15T10:00:00Z"},"contentDetails":{"duration":"PT30S"}},
			{"id":"v2","snippet":{"title":"Cartman Being an Absolute MENACE","publishedAt":"2026-10-23T10:00:00Z"},"contentDetails":{"duration":"PT13S"}},
			{"id":"v3","snippet":{"title":"Old upload","publishedAt":"2026-01-01T10:00:00Z"},"contentDetails":{"duration":"PT1M13.5S"}}]}`))
	}))
	mux.HandleFunc("/analytics/reports", auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("ids") != "channel==MINE" || q.Get("dimensions") != "video" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"columnHeaders":[{"name":"video"},{"name":"views"},{"name":"estimatedMinutesWatched"},{"name":"averageViewDuration"},
			{"name":"averageViewPercentage"},{"name":"subscribersGained"},{"name":"likes"},{"name":"shares"}],
			"rows":[["v1",1188,159.8,8,27.0,0,40,3],["v2",578,45.5,4.7,36.2,1,12,0]]}`))
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeGoogle) client(srv *httptest.Server) *Client {
	c := NewClient("client-id", "secret", "http://api.example/api/v1/youtube/callback")
	c.AuthURL, c.TokenURL, c.DataURL, c.AnalyticsURL = srv.URL+"/auth", srv.URL+"/token", srv.URL+"/data", srv.URL+"/analytics"
	return c
}

func TestISODuration(t *testing.T) {
	for in, want := range map[string]int64{"PT30S": 30_000, "PT1M13.5S": 73_500, "PT1H2M": 3_720_000, "P1DT1S": 86_401_000, "": 0, "nonsense": 0} {
		if got := isoDuration(in); got != want {
			t.Errorf("isoDuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSeal(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	sealed, err := seal(key, "org-a", "refresh-SECRET")
	if err != nil || bytes.Contains(sealed, []byte("SECRET")) {
		t.Fatalf("sealed = %q, %v", sealed, err)
	}
	if got, err := open(key, "org-a", sealed); err != nil || got != "refresh-SECRET" {
		t.Fatalf("open = %q, %v", got, err)
	}
	if _, err := open(key, "org-b", sealed); err == nil {
		t.Fatal("token opened for another organisation")
	}
	if _, err := open(bytes.Repeat([]byte{8}, 32), "org-a", sealed); err == nil {
		t.Fatal("token opened with another key")
	}
}

func TestMatch(t *testing.T) {
	pub := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	videos := []Video{
		{ID: "v1", Title: "Marlon's First Day in Botswana 🇧🇼 Didn't Expect This! 👀", PublishedAt: pub, DurationMS: 30_000},
		{ID: "v2", Title: "Something else entirely", PublishedAt: pub, DurationMS: 30_000},
		{ID: "v3", Title: "How Diamonds Built Modern Botswana (Part 2)", PublishedAt: pub, DurationMS: 31_000},
	}
	clips := []Clip{
		{ID: "c1", Name: "marlon’s first day in botswana", CreatedAt: pub.Add(-time.Hour), DurationMS: 30_020},
		{ID: "late", Name: "Something else entirely", CreatedAt: pub.Add(48 * time.Hour), DurationMS: 30_000}, // made after
		{ID: "p1", Name: "How Diamonds Built Modern Botswana (Part 1)", CreatedAt: pub.Add(-time.Hour), DurationMS: 27_400},
		{ID: "p2", Name: "How Diamonds Built Modern Botswana (Part 2)", CreatedAt: pub.Add(-time.Hour), DurationMS: 30_900},
	}
	got := Match(videos, clips)
	if len(got) != 2 || got["v1"] != "c1" || got["v3"] != "p2" {
		t.Fatalf("matches = %v", got)
	}
}

type world struct {
	t        *testing.T
	ctx      context.Context
	db       *bun.DB
	accounts *account.Service
	google   *fakeGoogle
	svc      *Service
	syncer   *Syncer
}

type person struct{ id, email string }

func newWorld(t *testing.T) *world {
	db := testdb.Open(t)
	g := &fakeGoogle{channel: "UC1", scope: strings.Join(Scopes, " ")}
	c := g.client(g.server(t))
	key := bytes.Repeat([]byte{3}, 32)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &world{t: t, ctx: context.Background(), db: db, accounts: account.NewService(db), google: g,
		svc: NewService(db, c, key, log), syncer: NewSyncer(db, c, key, log)}
}

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_yt_" + id, Email: "yt-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person, members map[string]person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "yttest team")
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org.ID) })
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

func (w *world) syncJob(orgID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{Kind: jobs.KindYouTubeSync, OrganisationID: orgID}
	if err := w.db.QueryRowContext(w.ctx, `SELECT id FROM jobs WHERE organisation_id = ? AND kind = 'youtube.sync' ORDER BY created_at DESC LIMIT 1`, orgID).
		Scan(&j.ID); err != nil {
		w.t.Fatal(err)
	}
	return j
}

// state pulls the state out of a consent URL.
func state(t *testing.T, consent string) string {
	u, err := url.Parse(consent)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("access_type") != "offline" || !strings.Contains(q.Get("scope"), "yt-analytics.readonly") || q.Get("state") == "" {
		t.Fatalf("consent URL = %s", consent)
	}
	return q.Get("state")
}

func TestConnectSyncDisconnect(t *testing.T) {
	w := newWorld(t)
	ana, ed, eve := w.user("Ana"), w.user("Ed"), w.user("Eve")
	org := w.team(ana, map[string]person{authz.RoleEditor: ed})

	if _, err := w.svc.Status(w.ctx, ana.id, org); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("status before connecting: %v", err)
	}
	if _, err := w.svc.Connect(w.ctx, ed.id, org); !errors.Is(err, ErrForbidden) {
		t.Fatalf("editor connecting: %v", err)
	}
	if _, err := w.svc.Connect(w.ctx, eve.id, org); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider connecting: %v", err)
	}
	consent, err := w.svc.Connect(w.ctx, ana.id, org)
	if err != nil {
		t.Fatal(err)
	}
	st := state(t, consent)
	// Someone else can't finish Ana's request, and a state works once.
	if _, err := w.svc.Callback(w.ctx, eve.id, st, "good-code", ""); !errors.Is(err, ErrConsentFailed) {
		t.Fatalf("someone else's state: %v", err)
	}
	consent, _ = w.svc.Connect(w.ctx, ana.id, org)
	st = state(t, consent)
	got, err := w.svc.Callback(w.ctx, ana.id, st, "good-code", "")
	if err != nil || got != org {
		t.Fatalf("callback = %q, %v", got, err)
	}
	if _, err := w.svc.Callback(w.ctx, ana.id, st, "good-code", ""); !errors.Is(err, ErrConsentFailed) {
		t.Fatalf("state reused: %v", err)
	}
	var stored []byte
	if err := w.db.QueryRowContext(w.ctx, `SELECT refresh_token FROM youtube_connections WHERE organisation_id = ?`, org).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("SECRET")) {
		t.Fatal("refresh token stored in the clear")
	}
	c, err := w.svc.Status(w.ctx, ed.id, org)
	if err != nil || c.ChannelTitle != "IshowClipse" || c.SyncStatus != SyncQueued || c.ConnectedByName != "Ana" {
		t.Fatalf("status = %+v, %v", c, err)
	}

	// A clip made in CAVI that became the first upload.
	var projectID, assetID, renderID string
	proj, err := project.NewService(w.db).Create(w.ctx, ana.id, org, "", "P", "")
	if err != nil {
		t.Fatal(err)
	}
	projectID = proj.ID
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO media_assets (organisation_id, project_id, storage_key, filename, kind, content_type, size_bytes, status, uploaded_at)
		VALUES (?, ?, 'k-'||gen_random_uuid(), 'a.mp4', 'video', 'video/mp4', 1, 'uploaded', now()) RETURNING id`, org, projectID).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO renders (organisation_id, project_id, source_asset_id, name, composition, composition_version, status, output_duration_ms, created_at, output_key)
		VALUES (?, ?, ?, 'Marlon''s First Day in Botswana', '{}', 3, 'ready', 30000, '2026-11-14T10:00:00Z', 'k-'||gen_random_uuid()) RETURNING id`,
		org, projectID, assetID).Scan(&renderID); err != nil {
		t.Fatal(err)
	}

	if err := w.syncer.Process(w.ctx, w.syncJob(org)); err != nil {
		t.Fatal(err)
	}
	vs, err := w.svc.Videos(w.ctx, ed.id, org)
	if err != nil || len(vs) != 3 {
		t.Fatalf("videos = %+v, %v", vs, err)
	}
	v1 := vs[0]
	if v1.VideoID != "v1" || v1.Views != 1188 || v1.AvgViewPercentage != 27 || v1.DurationMS != 30_000 || v1.RenderID != renderID || v1.ProjectID != projectID {
		t.Fatalf("v1 = %+v", v1)
	}
	if vs[2].VideoID != "v3" || vs[2].Views != 0 || vs[2].DurationMS != 73_500 {
		t.Fatalf("an upload without numbers = %+v", vs[2])
	}
	if c, _ := w.svc.Status(w.ctx, ana.id, org); c.SyncStatus != SyncIdle || c.SyncedAt.IsZero() || c.Videos != 3 {
		t.Fatalf("after sync = %+v", c)
	}
	if _, err := w.svc.Videos(w.ctx, eve.id, org); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider videos: %v", err)
	}

	// Syncing again on request; not twice at once.
	if _, err := w.svc.RequestSync(w.ctx, ed.id, org); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.RequestSync(w.ctx, ed.id, org); !errors.Is(err, ErrSyncing) {
		t.Fatalf("second sync: %v", err)
	}
	// Revoked access fails for good, telling people to reconnect.
	w.google.mu.Lock()
	w.google.revoked = true
	w.google.mu.Unlock()
	j := w.syncJob(org)
	err = w.syncer.Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("revoked: %v", err)
	}
	if err := w.syncer.Failed(w.ctx, j, err); err != nil {
		t.Fatal(err)
	}
	if c, _ := w.svc.Status(w.ctx, ana.id, org); c.SyncStatus != SyncFailed || c.SyncError != msgRevoked {
		t.Fatalf("after revoke = %+v", c)
	}

	if err := w.svc.Disconnect(w.ctx, ed.id, org); !errors.Is(err, ErrForbidden) {
		t.Fatalf("editor disconnecting: %v", err)
	}
	if err := w.svc.Disconnect(w.ctx, ana.id, org); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Status(w.ctx, ana.id, org); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after disconnect: %v", err)
	}
	if vs, _ := w.svc.Videos(w.ctx, ana.id, org); len(vs) != 0 || w.google.revokes != 1 {
		t.Fatalf("after disconnect: %d videos, %d revokes", len(vs), w.google.revokes)
	}
}

func TestCallbackNeedsBothScopes(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	w.google.scope = Scopes[0] // analytics unticked
	consent, err := w.svc.Connect(w.ctx, ana.id, org)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Callback(w.ctx, ana.id, state(t, consent), "good-code", ""); !errors.Is(err, ErrConsentFailed) {
		t.Fatalf("one scope: %v", err)
	}
	consent, _ = w.svc.Connect(w.ctx, ana.id, org)
	if _, err := w.svc.Callback(w.ctx, ana.id, state(t, consent), "", "access_denied"); !errors.Is(err, ErrConsentFailed) {
		t.Fatalf("declined: %v", err)
	}
}

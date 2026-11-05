package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

// memStore is an in-memory storage.Store. The real S3 implementation is
// covered by package storage's integration test.
type memStore struct {
	mu         sync.Mutex
	objects    map[string][]byte
	failDelete bool
}

func newMemStore() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) PresignUpload(_ context.Context, key, contentType, filename string, size int64, ttl time.Duration) (storage.Upload, error) {
	return storage.Upload{URL: "https://storage.test/" + key, Method: "PUT",
		Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: time.Now().Add(ttl)}, nil
}
func (m *memStore) PresignDownload(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return "https://storage.test/" + key + "?signed", time.Now().Add(ttl), nil
}
func (m *memStore) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	if !ok {
		return storage.ObjectInfo{}, storage.ErrNotFound
	}
	return storage.ObjectInfo{Size: int64(len(b))}, nil
}
func (m *memStore) ReadHead(_ context.Context, key string, n int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return b[:min(int64(len(b)), n)], nil
}
func (m *memStore) Put(_ context.Context, key, _ string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = body
	return nil
}
func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failDelete {
		return errors.New("storage unavailable")
	}
	delete(m.objects, key)
	return nil
}
func (m *memStore) put(key string, b []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = b
}
func (m *memStore) has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok
}

var mp4 = append([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2"), make([]byte, 100)...)

type world struct {
	t        *testing.T
	db       *bun.DB
	ctx      context.Context
	accounts *account.Service
	projects *project.Service
	store    *memStore
	svc      *Service
}

func newWorld(t *testing.T) *world {
	db := testdb.Open(t)
	store := newMemStore()
	return &world{t: t, db: db, ctx: context.Background(), accounts: account.NewService(db),
		projects: project.NewService(db), store: store,
		svc: NewService(db, store, slog.New(slog.NewTextHandler(io.Discard, nil)))}
}

type person struct{ id, email string }

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_media_" + id, Email: "media-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person, members map[string]person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "mediatest team")
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
	p, err := w.projects.Create(w.ctx, owner.id, orgID, "", "Media test", "")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

// upload runs the whole flow and returns the uploaded asset.
func (w *world) upload(by person, orgID, projectID, name string, body []byte) Asset {
	w.t.Helper()
	a, _, err := w.svc.CreateUpload(w.ctx, by.id, orgID, projectID, name, "video/mp4", int64(len(body)))
	if err != nil {
		w.t.Fatal(err)
	}
	w.store.put(storageKey(orgID, projectID, a.ID), body)
	done, err := w.svc.Complete(w.ctx, by.id, orgID, projectID, a.ID)
	if err != nil {
		w.t.Fatal(err)
	}
	return done
}

func (w *world) count(q string, args ...any) int {
	w.t.Helper()
	var n int
	if err := w.db.QueryRowContext(w.ctx, q, args...).Scan(&n); err != nil {
		w.t.Fatal(err)
	}
	return n
}

func (w *world) queued(key string) int {
	return w.count(`SELECT count(*) FROM storage_deletions WHERE storage_key = ?`, key)
}

func wantErr(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	var v *account.ValidationError
	if !errors.As(err, &v) || v.Field != field {
		t.Fatalf("err = %v, want validation error on %q", err, field)
	}
}

func wantRejected(t *testing.T, err error) {
	t.Helper()
	var r *RejectedError
	if !errors.As(err, &r) {
		t.Fatalf("err = %v, want RejectedError", err)
	}
}

func TestUploadFlow(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)

	a, up, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, `C:\Users\ana\Desktop\Interview take 2.MP4`, "Video/MP4", int64(len(mp4)))
	if err != nil {
		t.Fatal(err)
	}
	key := storageKey(org, proj, a.ID)
	if a.Status != "uploading" || a.Filename != "Interview take 2.MP4" || a.URL != "" ||
		up.URL != "https://storage.test/"+key || !strings.HasPrefix(key, "organisations/"+org+"/projects/"+proj+"/assets/") {
		t.Fatalf("asset = %+v, upload = %+v", a, up)
	}

	// Pending uploads aren't listed.
	page, err := w.svc.List(w.ctx, ana.id, org, proj, "", 0)
	if err != nil || len(page.Assets) != 0 {
		t.Fatalf("list before completion = %+v, %v", page, err)
	}

	// Completing before the bytes arrive says so and keeps the upload open.
	_, err = w.svc.Complete(w.ctx, ana.id, org, proj, a.ID)
	wantErr(t, err, ErrNotReceived)

	w.store.put(key, mp4)
	done, err := w.svc.Complete(w.ctx, ana.id, org, proj, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "uploaded" || done.Kind != KindVideo || done.ContentType != "video/mp4" || done.URL == "" || !done.CanDelete {
		t.Fatalf("completed = %+v", done)
	}
	again, err := w.svc.Complete(w.ctx, ana.id, org, proj, a.ID)
	if err != nil || again.Status != "uploaded" {
		t.Fatalf("second complete = %+v, %v", again, err)
	}
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id = ? AND action = 'media.uploaded'`, a.ID); n != 1 {
		t.Errorf("audit events = %d, want 1", n)
	}

	page, err = w.svc.List(w.ctx, ana.id, org, proj, "", 0)
	if err != nil || len(page.Assets) != 1 || page.Assets[0].URL == "" || page.Assets[0].UploadedByName != "Ana" {
		t.Fatalf("list = %+v, %v", page, err)
	}
}

func TestUploadValidation(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)

	_, _, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, "notes.html", "text/html", 10)
	wantField(t, err, "content_type")
	_, _, err = w.svc.CreateUpload(w.ctx, ana.id, org, proj, "x.mp4", "video/mp4", 0)
	wantField(t, err, "size_bytes")
	_, _, err = w.svc.CreateUpload(w.ctx, ana.id, org, proj, "x.mp4", "video/mp4", MaxUploadBytes+1)
	wantField(t, err, "size_bytes")
	for _, name := range []string{"", "   ", "../", strings.Repeat("a", 256)} {
		_, _, err = w.svc.CreateUpload(w.ctx, ana.id, org, proj, name, "video/mp4", 10)
		wantField(t, err, "filename")
	}
	a, _, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, "../../etc/pass\x00wd", "", 10)
	if err != nil || a.Filename != "passwd" || a.ContentType != "application/octet-stream" {
		t.Fatalf("cleaned = %+v, %v", a, err)
	}
}

func TestRejectedUploads(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)

	try := func(declared int64, body []byte) (string, error) {
		a, _, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, "f.mp4", "video/mp4", declared)
		if err != nil {
			t.Fatal(err)
		}
		key := storageKey(org, proj, a.ID)
		w.store.put(key, body)
		_, err = w.svc.Complete(w.ctx, ana.id, org, proj, a.ID)
		if n := w.count(`SELECT count(*) FROM media_assets WHERE id = ?`, a.ID); n != 0 {
			t.Errorf("rejected asset still in database")
		}
		return key, err
	}

	// Size differs from what was declared.
	key, err := try(int64(len(mp4))+5, mp4)
	wantRejected(t, err)
	if w.queued(key) != 1 {
		t.Error("rejected object not queued for deletion")
	}
	// An HTML page renamed to .mp4.
	html := []byte("<!doctype html><script>alert(document.cookie)</script>")
	key, err = try(int64(len(html)), html)
	wantRejected(t, err)
	if w.queued(key) != 1 {
		t.Error("rejected object not queued for deletion")
	}
}

func TestMediaPermissions(t *testing.T) {
	w := newWorld(t)
	ana, ed, vic, cli, eve := w.user("Ana"), w.user("Ed"), w.user("Vic"), w.user("Cli"), w.user("Eve")
	org := w.team(ana, map[string]person{authz.RoleEditor: ed, authz.RoleViewer: vic, authz.RoleClient: cli})
	proj := w.project(ana, org)
	anas := w.upload(ana, org, proj, "a.mp4", mp4)
	eds := w.upload(ed, org, proj, "e.mp4", mp4)

	// Outsiders can't see or touch anything.
	_, err := w.svc.List(w.ctx, eve.id, org, proj, "", 0)
	wantErr(t, err, ErrNotFound)
	_, _, err = w.svc.CreateUpload(w.ctx, eve.id, org, proj, "x.mp4", "video/mp4", 10)
	wantErr(t, err, ErrNotFound)
	wantErr(t, w.svc.Delete(w.ctx, eve.id, org, proj, anas.ID), ErrNotFound)
	_, err = w.svc.Get(w.ctx, eve.id, org, proj, anas.ID)
	wantErr(t, err, ErrNotFound)

	// An asset ID can't be read through a different project.
	other := w.project(ana, org)
	_, err = w.svc.Get(w.ctx, ana.id, org, other, anas.ID)
	wantErr(t, err, ErrNotFound)

	// Viewers see but can't upload or delete.
	if page, err := w.svc.List(w.ctx, vic.id, org, proj, "", 0); err != nil || len(page.Assets) != 2 || page.Assets[0].CanDelete {
		t.Fatalf("viewer list = %+v, %v", page, err)
	}
	_, _, err = w.svc.CreateUpload(w.ctx, vic.id, org, proj, "x.mp4", "video/mp4", 10)
	wantErr(t, err, ErrForbidden)
	wantErr(t, w.svc.Delete(w.ctx, vic.id, org, proj, anas.ID), ErrForbidden)

	// Clients see media only in projects shared with them.
	_, err = w.svc.List(w.ctx, cli.id, org, proj, "", 0)
	wantErr(t, err, ErrNotFound)
	if err := w.projects.GrantAccess(w.ctx, ana.id, org, proj, cli.id); err != nil {
		t.Fatal(err)
	}
	if page, err := w.svc.List(w.ctx, cli.id, org, proj, "", 0); err != nil || len(page.Assets) != 2 {
		t.Fatalf("client list = %+v, %v", page, err)
	}
	_, _, err = w.svc.CreateUpload(w.ctx, cli.id, org, proj, "x.mp4", "video/mp4", 10)
	wantErr(t, err, ErrForbidden)

	// Someone else can't complete your upload.
	pending, _, err := w.svc.CreateUpload(w.ctx, ed.id, org, proj, "p.mp4", "video/mp4", int64(len(mp4)))
	if err != nil {
		t.Fatal(err)
	}
	w.store.put(storageKey(org, proj, pending.ID), mp4)
	_, err = w.svc.Complete(w.ctx, ana.id, org, proj, pending.ID)
	wantErr(t, err, ErrForbidden)
	_, err = w.svc.Get(w.ctx, vic.id, org, proj, pending.ID)
	wantErr(t, err, ErrNotFound)

	// Editors delete their own uploads but not other people's; owners delete any.
	wantErr(t, w.svc.Delete(w.ctx, ed.id, org, proj, anas.ID), ErrForbidden)
	if err := w.svc.Delete(w.ctx, ed.id, org, proj, eds.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, anas.ID); err != nil {
		t.Fatal(err)
	}
	if n := w.count(`SELECT count(*) FROM audit_events WHERE target_id IN (?, ?) AND action = 'media.deleted'`, anas.ID, eds.ID); n != 2 {
		t.Errorf("delete audit events = %d", n)
	}
}

func TestSweepRemovesObjects(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)

	kept := w.upload(ana, org, proj, "kept.mp4", mp4)
	gone := w.upload(ana, org, proj, "gone.mp4", mp4)
	keptKey, goneKey := storageKey(org, proj, kept.ID), storageKey(org, proj, gone.ID)

	// An abandoned upload, started more than a day ago.
	stale, _, err := w.svc.CreateUpload(w.ctx, ana.id, org, proj, "stale.mp4", "video/mp4", int64(len(mp4)))
	if err != nil {
		t.Fatal(err)
	}
	staleKey := storageKey(org, proj, stale.ID)
	w.store.put(staleKey, mp4)
	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET created_at = now() - interval '25 hours' WHERE id = ?`, stale.ID); err != nil {
		t.Fatal(err)
	}

	if err := w.svc.Delete(w.ctx, ana.id, org, proj, gone.ID); err != nil {
		t.Fatal(err)
	}

	// Storage is down: nothing is lost, the deletion is retried later.
	w.store.failDelete = true
	if _, _, err := w.svc.Sweep(w.ctx); err != nil {
		t.Fatal(err)
	}
	if !w.store.has(goneKey) || w.count(`SELECT count(*) FROM storage_deletions WHERE storage_key = ? AND attempts = 1 AND next_try_at > now()`, goneKey) != 1 {
		t.Fatal("failed deletion not scheduled for retry")
	}
	if w.count(`SELECT count(*) FROM media_assets WHERE id = ?`, stale.ID) != 0 {
		t.Error("abandoned upload not removed")
	}

	// Storage is back.
	w.store.failDelete = false
	if _, err := w.db.ExecContext(w.ctx, `UPDATE storage_deletions SET next_try_at = now() WHERE storage_key IN (?, ?)`, goneKey, staleKey); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.svc.Sweep(w.ctx); err != nil {
		t.Fatal(err)
	}
	if w.store.has(goneKey) || w.store.has(staleKey) || w.queued(goneKey)+w.queued(staleKey) != 0 {
		t.Fatal("objects not deleted after recovery")
	}
	if !w.store.has(keptKey) {
		t.Fatal("kept object deleted")
	}

	// Deleting the project removes its media and, via the sweep, the objects.
	if err := w.projects.Delete(w.ctx, ana.id, org, proj); err != nil {
		t.Fatal(err)
	}
	if w.queued(keptKey) != 1 {
		t.Fatal("project deletion did not queue its media")
	}
	if _, _, err := w.svc.Sweep(w.ctx); err != nil {
		t.Fatal(err)
	}
	if w.store.has(keptKey) {
		t.Fatal("object outlived its project")
	}
}

func TestListPaging(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	for i := 0; i < 3; i++ {
		w.upload(ana, org, proj, "f.mp4", mp4)
	}
	p1, err := w.svc.List(w.ctx, ana.id, org, proj, "", 2)
	if err != nil || len(p1.Assets) != 2 || p1.NextCursor == "" {
		t.Fatalf("page 1 = %+v, %v", p1, err)
	}
	p2, err := w.svc.List(w.ctx, ana.id, org, proj, p1.NextCursor, 2)
	if err != nil || len(p2.Assets) != 1 || p2.NextCursor != "" || p2.Assets[0].ID == p1.Assets[1].ID {
		t.Fatalf("page 2 = %+v, %v", p2, err)
	}
	_, err = w.svc.List(w.ctx, ana.id, org, proj, "nope!", 2)
	wantField(t, err, "cursor")
	_, err = w.svc.List(w.ctx, ana.id, org, proj, "", 500)
	wantField(t, err, "limit")
}

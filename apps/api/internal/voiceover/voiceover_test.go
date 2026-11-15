package voiceover

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

type memStore struct {
	storage.Store
	mu      sync.Mutex
	objects map[string][]byte
}

func (m *memStore) Put(_ context.Context, key, _ string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = body
	return nil
}

func (m *memStore) PresignDownload(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return "https://storage.test/" + key + "?sig=x", time.Now().Add(ttl), nil
}

// fakeNarrator speaks every script as a tone of a plausible length.
type fakeNarrator struct {
	err   error
	calls int
}

func (f *fakeNarrator) Speak(_ context.Context, _ ai.Model, script, voice string) (ai.Speech, ai.Usage, error) {
	f.calls++
	if f.err != nil {
		return ai.Speech{}, ai.Usage{}, f.err
	}
	n := 24000 * 3
	pcm := make([]byte, 2*n)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(pcm[2*i:], uint16(int16(8000*math.Sin(float64(i)/10))))
	}
	s := ai.Speech{PCM: pcm, SampleRate: 24000}
	return s, ai.Usage{InputTokens: 30, OutputTokens: 900}, ai.ValidateSpeech(s, script)
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
	store := &memStore{objects: map[string][]byte{}}
	return &world{t: t, ctx: context.Background(), db: db, accounts: account.NewService(db), projects: project.NewService(db),
		store: store, svc: NewService(db, store)}
}

type person struct{ id, email string }

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_vo_" + id, Email: "vo-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person, members map[string]person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "votest team")
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
	p, err := w.projects.Create(w.ctx, owner.id, orgID, "", "Voiceover test", "")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

func (w *world) job(id string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{SubjectID: id, Kind: jobs.KindVoiceoverSpeak}
	if err := w.db.QueryRowContext(w.ctx, `SELECT id, organisation_id FROM jobs WHERE subject_id = ?`, id).Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no voiceover job: %v", err)
	}
	return j
}

func (w *world) narrator(f *fakeNarrator) *Narrator {
	return NewNarrator(w.db, w.store, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

const script = "In Botswana, natural diamonds helped pay for schools and roads."

func TestVoiceovers(t *testing.T) {
	w := newWorld(t)
	ana, vic, eve := w.user("Ana"), w.user("Vic"), w.user("Eve")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic})
	proj := w.project(ana, org)

	if _, err := w.svc.Create(w.ctx, vic.id, org, proj, script, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer create: %v", err)
	}
	if _, err := w.svc.Create(w.ctx, eve.id, org, proj, script, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider create: %v", err)
	}
	for field, args := range map[string][2]string{"script": {" ", ""}, "voice": {script, "Robot"}} {
		_, err := w.svc.Create(w.ctx, ana.id, org, proj, args[0], args[1])
		var v *account.ValidationError
		if !errors.As(err, &v) || v.Field != field {
			t.Errorf("%s: err = %v", field, err)
		}
	}

	v, err := w.svc.Create(w.ctx, ana.id, org, proj, "  "+script+"  ", "Puck")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusQueued || v.Script != script || v.Voice != "Puck" || v.Model != "gemini-3.8-flash-tts" || v.URL != "" {
		t.Fatalf("created = %+v", v)
	}
	if _, err := Load(w.ctx, w.db, org, proj, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("a queued voiceover loaded for rendering")
	}
	f := &fakeNarrator{}
	if err := w.narrator(f).Process(w.ctx, w.job(v.ID)); err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.Get(w.ctx, vic.id, org, proj, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusReady || got.DurationMS != 3000 || got.URL == "" || got.CanDelete {
		t.Fatalf("ready (viewer) = %+v", got)
	}
	key := audioKey(org, proj, v.ID)
	if wav := w.store.objects[key]; len(wav) != 44+2*24000*3 || string(wav[:4]) != "RIFF" {
		t.Fatalf("stored audio %d bytes", len(wav))
	}
	if r, err := Load(w.ctx, w.db, org, proj, v.ID); err != nil || r.StorageKey != key || r.DurationMS != 3000 {
		t.Fatalf("Load = %+v, %v", r, err)
	}
	// A redelivered job does nothing.
	if err := w.narrator(f).Process(w.ctx, w.job(v.ID)); err != nil || f.calls != 1 {
		t.Fatalf("rerun: %v, %d calls", err, f.calls)
	}
	// Another tenant's job naming it does nothing.
	j := w.job(v.ID)
	j.OrganisationID = w.team(eve, nil)
	if err := w.narrator(f).Process(w.ctx, j); err != nil || f.calls != 1 {
		t.Fatalf("cross-tenant: %v, %d calls", err, f.calls)
	}
	if list, _ := w.svc.List(w.ctx, ana.id, org, proj); len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}

	// Failures: blocked scripts fail for good with a message.
	bad, _ := w.svc.Create(w.ctx, ana.id, org, proj, script, "")
	err = w.narrator(&fakeNarrator{err: fmt.Errorf("%w: blocked", ai.ErrPermanent)}).Process(w.ctx, w.job(bad.ID))
	if !jobs.IsPermanent(err) {
		t.Fatalf("blocked: %v", err)
	}
	_ = w.narrator(f).Failed(w.ctx, w.job(bad.ID), err)
	if b, _ := w.svc.Get(w.ctx, ana.id, org, proj, bad.ID); b.Status != StatusFailed || b.Error != msgRejected {
		t.Fatalf("failed = %+v", b)
	}

	// Deleting queues the audio for removal.
	if err := w.svc.Delete(w.ctx, vic.id, org, proj, v.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer delete: %v", err)
	}
	if err := w.svc.Delete(w.ctx, ana.id, org, proj, v.ID); err != nil {
		t.Fatal(err)
	}
	var queued int
	_ = w.db.QueryRowContext(w.ctx, `SELECT count(*) FROM storage_deletions WHERE storage_key = ?`, key).Scan(&queued)
	if queued != 1 {
		t.Fatalf("audio deletion queued %d times", queued)
	}
}

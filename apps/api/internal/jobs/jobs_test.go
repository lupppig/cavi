package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

// world is real Postgres plus a real NATS server with a stream of its own.
type world struct {
	t   *testing.T
	ctx context.Context
	db  *bun.DB
	bus *Bus
	org string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	url := os.Getenv("CAVI_TEST_NATS_URL")
	if url == "" {
		t.Skip("CAVI_TEST_NATS_URL not set")
	}
	db := testdb.Open(t)
	ctx := context.Background()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)
	bus, err := Connect(ctx, BusConfig{URL: url, ClientName: "jobs-test", Stream: "TEST_JOBS_" + suffix, SubjectPrefix: "test" + suffix + ".jobs"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = bus.js.DeleteStream(context.Background(), "TEST_JOBS_"+suffix)
		bus.Close()
	})
	var org string
	if err := db.QueryRowContext(ctx, `INSERT INTO organisations (name) VALUES ('jobstest') RETURNING id`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org) })
	return &world{t: t, ctx: ctx, db: db, bus: bus, org: org}
}

// asset creates a media asset to hang a job off.
func (w *world) asset() string {
	w.t.Helper()
	var ws, proj, id string
	if err := w.db.QueryRowContext(w.ctx, `INSERT INTO workspaces (organisation_id, name) VALUES (?, 'Main') RETURNING id`, w.org).Scan(&ws); err != nil {
		w.t.Fatal(err)
	}
	if err := w.db.QueryRowContext(w.ctx, `INSERT INTO projects (organisation_id, workspace_id, name) VALUES (?, ?, 'p') RETURNING id`, w.org, ws).Scan(&proj); err != nil {
		w.t.Fatal(err)
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO media_assets (organisation_id, project_id, storage_key, filename, content_type, size_bytes)
		VALUES (?, ?, ?, 'f.mp4', 'video/mp4', 1) RETURNING id`, w.org, proj, "jobstest/"+hex.EncodeToString(b)).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *world) enqueue(maxAttempts int) (jobID, assetID string) {
	w.t.Helper()
	assetID = w.asset()
	id, err := Enqueue(w.ctx, w.db, New{Kind: KindMediaProcess, OrganisationID: w.org, MediaAssetID: assetID, CorrelationID: "req-1", MaxAttempts: maxAttempts})
	if err != nil {
		w.t.Fatal(err)
	}
	return id, assetID
}

type jobState struct {
	status    string
	attempts  int
	lastError string
	published bool
}

func (w *world) state(id string) jobState {
	w.t.Helper()
	var s jobState
	if err := w.db.QueryRowContext(w.ctx, `SELECT status, attempts, coalesce(last_error, ''), published_at IS NOT NULL FROM jobs WHERE id = ?`, id).
		Scan(&s.status, &s.attempts, &s.lastError, &s.published); err != nil {
		w.t.Fatal(err)
	}
	return s
}

// fakeHandler fails the first `fail` attempts of each job.
type fakeHandler struct {
	mu        sync.Mutex
	calls     map[string]int
	fail      int
	permanent bool
	failed    map[string]error
	delay     time.Duration
}

func newHandler() *fakeHandler {
	return &fakeHandler{calls: map[string]int{}, failed: map[string]error{}}
}

func (h *fakeHandler) Process(ctx context.Context, j Job) error {
	h.mu.Lock()
	h.calls[j.ID]++
	n := h.calls[j.ID]
	h.mu.Unlock()
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	if n <= h.fail {
		if h.permanent {
			return Permanent(errors.New("file is corrupt"))
		}
		return errors.New("storage timed out")
	}
	return nil
}

func (h *fakeHandler) Failed(_ context.Context, j Job, err error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failed[j.ID] = err
	return nil
}

func (h *fakeHandler) count(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[id]
}

func (w *world) run(h Handler) (context.CancelFunc, *Relay) {
	ctx, cancel := context.WithCancel(w.ctx)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	relay := NewRelay(w.db, w.bus, logger)
	relay.Interval = 100 * time.Millisecond
	worker := NewWorker(w.db, w.bus, KindMediaProcess, h, logger, WorkerOptions{
		Concurrency: 2, Lease: 10 * time.Second,
		RetryDelay: func(int) time.Duration { return 100 * time.Millisecond },
	})
	done := make(chan struct{})
	go relay.Run(ctx)
	go func() { _ = worker.Run(ctx); close(done) }()
	w.t.Cleanup(func() { cancel(); <-done })
	return cancel, relay
}

func (w *world) waitFor(id, status string) jobState {
	w.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if s := w.state(id); s.status == status {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	w.t.Fatalf("job %s never reached %s: %+v", id, status, w.state(id))
	return jobState{}
}

func TestJobRunsOnce(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	id, _ := w.enqueue(5)
	if s := w.state(id); s.status != "queued" || s.published {
		t.Fatalf("new job = %+v", s)
	}
	w.run(h)
	s := w.waitFor(id, "succeeded")
	if s.attempts != 1 || !s.published || h.count(id) != 1 {
		t.Fatalf("state = %+v, calls = %d", s, h.count(id))
	}
}

func TestTransientFailuresRetry(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	h.fail = 2
	id, _ := w.enqueue(5)
	w.run(h)
	s := w.waitFor(id, "succeeded")
	if s.attempts != 3 || h.count(id) != 3 || s.lastError != "" {
		t.Fatalf("state = %+v, calls = %d", s, h.count(id))
	}
}

func TestRetriesStopAtMaxAttempts(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	h.fail = 100
	id, _ := w.enqueue(3)
	w.run(h)
	s := w.waitFor(id, "failed")
	time.Sleep(500 * time.Millisecond) // no further attempts arrive
	if s.attempts != 3 || h.count(id) != 3 || s.lastError != "storage timed out" || h.failed[id] == nil {
		t.Fatalf("state = %+v, calls = %d, failed = %v", s, h.count(id), h.failed[id])
	}
}

func TestPermanentFailureIsNotRetried(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	h.fail, h.permanent = 1, true
	id, _ := w.enqueue(5)
	w.run(h)
	s := w.waitFor(id, "failed")
	time.Sleep(500 * time.Millisecond)
	if s.attempts != 1 || h.count(id) != 1 || !IsPermanent(h.failed[id]) {
		t.Fatalf("state = %+v, calls = %d", s, h.count(id))
	}
}

func TestDuplicateDeliveriesRunOnce(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	h.delay = 300 * time.Millisecond
	id, asset := w.enqueue(5)
	// The same job published several times (here deduplicated by JetStream,
	// plus the relay's own publish) must run once.
	m := Message{Version: MessageVersion, JobID: id, Kind: KindMediaProcess, OrganisationID: w.org, MediaAssetID: asset}
	for i := 0; i < 3; i++ {
		if err := w.bus.Publish(w.ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	w.run(h)
	w.waitFor(id, "succeeded")
	time.Sleep(500 * time.Millisecond)
	if h.count(id) != 1 {
		t.Fatalf("handler ran %d times", h.count(id))
	}
}

func TestMismatchedMessageIsDiscarded(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	id, asset := w.enqueue(5)
	// Mark it published so the relay leaves it alone, then send a message
	// claiming a different organisation.
	if _, err := w.db.ExecContext(w.ctx, `UPDATE jobs SET published_at = now() WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	forged := Message{Version: MessageVersion, JobID: id, Kind: KindMediaProcess,
		OrganisationID: "00000000-0000-4000-8000-000000000000", MediaAssetID: asset}
	if err := w.bus.Publish(w.ctx, forged); err != nil {
		t.Fatal(err)
	}
	w.run(h)
	time.Sleep(time.Second)
	if h.count(id) != 0 || w.state(id).status != "queued" || w.state(id).attempts != 0 {
		t.Fatalf("forged message ran the job: calls %d, state %+v", h.count(id), w.state(id))
	}
}

func TestDeletedResourceCancelsJob(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	id, asset := w.enqueue(5)
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM media_assets WHERE id = ?`, asset); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = w.db.QueryRowContext(w.ctx, `SELECT count(*) FROM jobs WHERE id = ?`, id).Scan(&n)
	if n != 0 {
		t.Fatal("job outlived its media")
	}
	// A message already in flight is acknowledged without running anything.
	if err := w.bus.Publish(w.ctx, Message{Version: MessageVersion, JobID: id, Kind: KindMediaProcess, OrganisationID: w.org, MediaAssetID: asset}); err != nil {
		t.Fatal(err)
	}
	w.run(h)
	time.Sleep(time.Second)
	if h.count(id) != 0 {
		t.Fatal("handler ran for a deleted job")
	}
}

func TestExpiredLeaseIsTakenOver(t *testing.T) {
	w := newWorld(t)
	h := newHandler()
	id, _ := w.enqueue(5)
	// Simulate a worker that claimed the job and died.
	if _, err := w.db.ExecContext(w.ctx, `
		UPDATE jobs SET status = 'running', attempts = 1, lease_expires_at = now() - interval '1 second', published_at = now()
		WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	w.run(h)
	// The relay won't republish a running job; deliver it as JetStream
	// would after the dead worker's ack wait.
	var org, asset string
	_ = w.db.QueryRowContext(w.ctx, `SELECT organisation_id, media_asset_id FROM jobs WHERE id = ?`, id).Scan(&org, &asset)
	if err := w.bus.Publish(w.ctx, Message{Version: MessageVersion, JobID: id, Kind: KindMediaProcess, OrganisationID: org, MediaAssetID: asset}); err != nil {
		t.Fatal(err)
	}
	s := w.waitFor(id, "succeeded")
	if s.attempts != 2 || h.count(id) != 1 {
		t.Fatalf("state = %+v, calls = %d", s, h.count(id))
	}
}

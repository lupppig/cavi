package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/uptrace/bun"
)

// Handler runs one kind of job.
type Handler interface {
	// Process does the work. Return Permanent(err) for failures that
	// retrying cannot fix. It must be safe to run more than once for the
	// same job (a worker may crash after finishing but before recording it).
	Process(ctx context.Context, job Job) error
	// Failed is called once when the job gives up, so the handler can
	// record the failure on the resource the user sees.
	Failed(ctx context.Context, job Job, err error) error
}

// WorkerOptions tune a worker. Zero values use sensible defaults.
type WorkerOptions struct {
	// Concurrency is how many jobs run at once (default 2).
	Concurrency int
	// Lease is how long one attempt may run before another worker may take
	// the job over (default 15 minutes). Attempts are cancelled at the lease.
	Lease time.Duration
	// RetryDelay returns the delay before attempt n+1 (default 15s doubling,
	// capped at 5 minutes).
	RetryDelay func(attempt int) time.Duration
}

// Worker consumes one kind of job.
type Worker struct {
	db      *bun.DB
	bus     *Bus
	kind    string
	handler Handler
	logger  *slog.Logger
	opts    WorkerOptions
}

// NewWorker returns a worker for kind.
func NewWorker(db *bun.DB, bus *Bus, kind string, h Handler, logger *slog.Logger, opts WorkerOptions) *Worker {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 2
	}
	if opts.Lease <= 0 {
		opts.Lease = 15 * time.Minute
	}
	if opts.RetryDelay == nil {
		opts.RetryDelay = func(n int) time.Duration { return min(15*time.Second<<min(n-1, 6), 5*time.Minute) }
	}
	return &Worker{db: db, bus: bus, kind: kind, handler: h, logger: logger.With(slog.String("job_kind", kind)), opts: opts}
}

// Run consumes jobs until ctx ends, then waits for running jobs to finish
// recording their outcome.
func (w *Worker) Run(ctx context.Context) error {
	// AckWait outlasts the lease so JetStream doesn't redeliver a job that
	// is still legitimately running; InProgress extends it as we go.
	cons, err := w.bus.consumer(ctx, w.kind, w.opts.Lease+time.Minute)
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < w.opts.Concurrency; i++ {
		// Each runner pulls one message at a time, so a message is never
		// held in a local buffer while its ack wait runs down.
		iter, err := cons.Messages(jetstream.PullMaxMessages(1))
		if err != nil {
			return fmt.Errorf("subscribe to jobs: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				msg, err := iter.Next()
				if err != nil {
					if errors.Is(err, jetstream.ErrMsgIteratorClosed) {
						return
					}
					w.logger.Warn("receive job failed", slog.String("error", err.Error()))
					continue
				}
				w.handle(context.WithoutCancel(ctx), msg)
			}
		}()
		go func() {
			<-ctx.Done()
			// Stop pulling; a job already running finishes and records its
			// outcome first.
			iter.Drain()
		}()
	}
	wg.Wait()
	return nil
}

// handle processes one delivery. It uses a context detached from shutdown
// so an attempt in progress can record its outcome.
func (w *Worker) handle(ctx context.Context, msg jetstream.Msg) {
	var m Message
	if err := json.Unmarshal(msg.Data(), &m); err != nil || m.Version != MessageVersion || m.Kind != w.kind ||
		!uuidPattern.MatchString(m.JobID) || !uuidPattern.MatchString(m.OrganisationID) ||
		(m.MediaAssetID != "" && !uuidPattern.MatchString(m.MediaAssetID)) {
		w.logger.Error("discarding malformed job message", slog.String("subject", msg.Subject()))
		_ = msg.Term()
		return
	}
	log := w.logger.With(slog.String("job_id", m.JobID), slog.String("organisation_id", m.OrganisationID),
		slog.String("correlation_id", m.CorrelationID))

	job, ok, err := w.claim(ctx, m)
	if err != nil {
		log.Warn("claim job failed", slog.String("error", err.Error()))
		_ = msg.NakWithDelay(5 * time.Second)
		return
	}
	if !ok {
		if w.mismatched(ctx, m) {
			// The message names a different tenant or resource than the job
			// record: a forged or corrupted message. Never run it.
			log.Error("job message does not match the job record; discarding it")
			_ = msg.Term()
			return
		}
		// Already finished, cancelled (deleted), or running elsewhere under
		// a live lease. That attempt owns the outcome.
		_ = msg.Ack()
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, w.opts.Lease)
	stop := keepAlive(runCtx, msg)
	start := time.Now()
	err = w.handler.Process(runCtx, job)
	stop()
	cancel()
	log = log.With(slog.Int("attempt", job.Attempt), slog.Duration("duration", time.Since(start)))

	switch {
	case err == nil:
		if err := w.finish(ctx, job.ID); err != nil {
			log.Error("record job success failed", slog.String("error", err.Error()))
			_ = msg.NakWithDelay(5 * time.Second)
			return
		}
		log.Info("job succeeded")
		_ = msg.Ack()
	case IsPermanent(err) || job.Attempt >= job.MaxAttempts:
		log.Warn("job failed", slog.Bool("permanent", IsPermanent(err)), slog.String("error", err.Error()))
		if ferr := w.handler.Failed(ctx, job, err); ferr != nil {
			log.Error("record failure on resource failed", slog.String("error", ferr.Error()))
		}
		if err := w.fail(ctx, job.ID, err.Error()); err != nil {
			log.Error("record job failure failed", slog.String("error", err.Error()))
			_ = msg.NakWithDelay(5 * time.Second)
			return
		}
		_ = msg.Term()
	default:
		delay := w.opts.RetryDelay(job.Attempt)
		log.Warn("job attempt failed; will retry", slog.Duration("retry_in", delay), slog.String("error", err.Error()))
		if err := w.release(ctx, job.ID, err.Error(), delay); err != nil {
			log.Error("record job retry failed", slog.String("error", err.Error()))
		}
		_ = msg.NakWithDelay(delay)
	}
}

// keepAlive tells JetStream the job is still being worked on.
func keepAlive(ctx context.Context, msg jetstream.Msg) func() {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				_ = msg.InProgress()
			}
		}
	}()
	return func() { close(done) }
}

// claim takes the job for one attempt if it is queued, or running under
// an expired lease (its worker died), and only if the message's tenant and
// resource match the job record.
func (w *Worker) claim(ctx context.Context, m Message) (Job, bool, error) {
	j := Job{ID: m.JobID}
	err := w.db.QueryRowContext(ctx, `
		UPDATE jobs SET status = 'running', attempts = attempts + 1, lease_expires_at = ?,
			started_at = coalesce(started_at, now()), updated_at = now()
		WHERE id = ? AND kind = ? AND organisation_id::text = ? AND coalesce(media_asset_id::text, '') = ?
			AND (status = 'queued' OR (status = 'running' AND lease_expires_at < now()))
		RETURNING kind, organisation_id, coalesce(media_asset_id::text, ''), coalesce(correlation_id, ''), attempts, max_attempts`,
		time.Now().Add(w.opts.Lease), m.JobID, w.kind, m.OrganisationID, m.MediaAssetID).
		Scan(&j.Kind, &j.OrganisationID, &j.MediaAssetID, &j.CorrelationID, &j.Attempt, &j.MaxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// mismatched reports whether the job exists but belongs to a different
// tenant or resource than the message claims.
func (w *Worker) mismatched(ctx context.Context, m Message) bool {
	var org, asset string
	err := w.db.QueryRowContext(ctx, `
		SELECT organisation_id::text, coalesce(media_asset_id::text, '') FROM jobs WHERE id::text = ?`, m.JobID).Scan(&org, &asset)
	return err == nil && (org != m.OrganisationID || asset != m.MediaAssetID)
}

func (w *Worker) finish(ctx context.Context, id string) error {
	_, err := w.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'succeeded', last_error = NULL, lease_expires_at = NULL, finished_at = now(), updated_at = now()
		WHERE id = ?`, id)
	return err
}

func (w *Worker) fail(ctx context.Context, id, msg string) error {
	_, err := w.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'failed', last_error = ?, lease_expires_at = NULL, finished_at = now(), updated_at = now()
		WHERE id = ?`, truncate(msg), id)
	return err
}

// release puts the job back in the queue for a retry. published_at is set
// so the relay doesn't republish it while the delayed redelivery is pending.
func (w *Worker) release(ctx context.Context, id, msg string, delay time.Duration) error {
	_, err := w.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'queued', last_error = ?, lease_expires_at = NULL, published_at = ?, updated_at = now()
		WHERE id = ?`, truncate(msg), time.Now().Add(delay), id)
	return err
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func truncate(s string) string {
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

// Package jobs runs background work reliably.
//
// Postgres is the source of truth: every job is a row in the jobs table
// with its status, attempts and last error, so users see the right state
// even after leaving a page. NATS JetStream only delivers job IDs to
// workers. The design:
//
//   - Enqueue writes a job row inside the transaction that needs the work
//     (an outbox), so a job is never lost or created for a change that
//     rolled back.
//   - Relay publishes queued rows to JetStream. It republishes rows that
//     stay queued for a long time, in case a message was lost.
//   - Worker claims a job in Postgres with a lease before running it, so
//     duplicate deliveries run the job at most once at a time, and a
//     crashed worker's job is picked up again when its lease expires.
//   - Failures are retried with backoff up to the job's max attempts;
//     errors marked Permanent are not retried.
//
// Messages carry identifiers only, never file contents.
package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// Kinds of job.
const (
	KindMediaProcess = "media.process"
)

// Message is what travels over NATS. It is versioned so workers can
// reject messages they don't understand.
type Message struct {
	Version        int    `json:"version"`
	JobID          string `json:"job_id"`
	Kind           string `json:"kind"`
	OrganisationID string `json:"organisation_id"`
	MediaAssetID   string `json:"media_asset_id,omitempty"`
	CorrelationID  string `json:"correlation_id,omitempty"`
}

// MessageVersion is the current message schema version.
const MessageVersion = 1

// Job is a claimed job handed to a handler.
type Job struct {
	ID             string
	Kind           string
	OrganisationID string
	MediaAssetID   string
	CorrelationID  string
	// Attempt is 1 for the first run.
	Attempt     int
	MaxAttempts int
}

// New describes a job to enqueue.
type New struct {
	Kind           string
	OrganisationID string
	MediaAssetID   string
	CorrelationID  string
	MaxAttempts    int
}

// Enqueue records a queued job. Call it inside the transaction that makes
// the job necessary; the relay publishes it after commit.
func Enqueue(ctx context.Context, db bun.IDB, n New) (string, error) {
	if n.Kind == "" || n.OrganisationID == "" {
		return "", errors.New("jobs: kind and organisation are required")
	}
	if n.MaxAttempts == 0 {
		n.MaxAttempts = 5
	}
	var id string
	err := db.QueryRowContext(ctx, `
		INSERT INTO jobs (kind, organisation_id, media_asset_id, correlation_id, max_attempts)
		VALUES (?, ?, NULLIF(?, '')::uuid, NULLIF(?, ''), ?) RETURNING id`,
		n.Kind, n.OrganisationID, n.MediaAssetID, n.CorrelationID, n.MaxAttempts).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	return id, nil
}

// permanent marks an error that retrying cannot fix.
type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Permanent wraps err so the job fails without further attempts.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

// IsPermanent reports whether err was wrapped with Permanent.
func IsPermanent(err error) bool {
	var p permanent
	return errors.As(err, &p)
}

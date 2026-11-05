package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"
)

// Publisher sends job messages. *Bus implements it.
type Publisher interface {
	Publish(ctx context.Context, m Message) error
}

// Relay publishes queued jobs from Postgres to the bus.
type Relay struct {
	db     *bun.DB
	bus    Publisher
	logger *slog.Logger
	wake   chan struct{}
	// Republish is how long a published job may stay queued before it is
	// published again in case the message was lost.
	Republish time.Duration
	// Interval is how often the relay checks for work without a Wake.
	Interval time.Duration
}

// NewRelay returns a relay.
func NewRelay(db *bun.DB, bus Publisher, logger *slog.Logger) *Relay {
	return &Relay{db: db, bus: bus, logger: logger, wake: make(chan struct{}, 1),
		Republish: 10 * time.Minute, Interval: 5 * time.Second}
}

// Wake asks the relay to publish now, typically right after a commit that
// enqueued a job. It never blocks.
func (r *Relay) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run publishes until ctx ends.
func (r *Relay) Run(ctx context.Context) {
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	for {
		if _, err := r.PublishPending(ctx); err != nil && ctx.Err() == nil {
			r.logger.WarnContext(ctx, "job relay failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.wake:
		}
	}
}

// PublishPending publishes up to 100 queued jobs that are unpublished or
// overdue and returns how many it sent. Safe to run from several processes.
func (r *Relay) PublishPending(ctx context.Context) (int, error) {
	sent := 0
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, kind, organisation_id, coalesce(media_asset_id::text, ''), coalesce(correlation_id, '')
			FROM jobs
			WHERE status = 'queued' AND (published_at IS NULL OR published_at < ?)
			ORDER BY created_at LIMIT 100
			FOR UPDATE SKIP LOCKED`, time.Now().Add(-r.Republish))
		if err != nil {
			return fmt.Errorf("load queued jobs: %w", err)
		}
		var msgs []Message
		for rows.Next() {
			m := Message{Version: MessageVersion}
			if err := rows.Scan(&m.JobID, &m.Kind, &m.OrganisationID, &m.MediaAssetID, &m.CorrelationID); err != nil {
				rows.Close()
				return err
			}
			msgs = append(msgs, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, m := range msgs {
			if err := r.bus.Publish(ctx, m); err != nil {
				// Leave it unpublished; the next run retries.
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE jobs SET published_at = now() WHERE id = ?`, m.JobID); err != nil {
				return err
			}
			sent++
		}
		return nil
	})
	return sent, err
}

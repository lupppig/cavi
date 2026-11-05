package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// BusConfig names the JetStream stream that carries jobs.
type BusConfig struct {
	URL string
	// ClientName identifies this process in NATS monitoring.
	ClientName string
	// Stream and SubjectPrefix default to CAVI_JOBS and "cavi.jobs". Tests
	// use their own.
	Stream        string
	SubjectPrefix string
}

// Bus is CAVI's connection to NATS JetStream for jobs. Nothing outside
// this package imports the NATS client.
type Bus struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	stream jetstream.Stream
	prefix string
}

// Connect connects to NATS and makes sure the jobs stream exists. The
// stream is a work queue: a message is removed once a worker acknowledges
// it, and duplicate publishes of the same job within two minutes are
// dropped.
func Connect(ctx context.Context, cfg BusConfig) (*Bus, error) {
	if cfg.Stream == "" {
		cfg.Stream = "CAVI_JOBS"
	}
	if cfg.SubjectPrefix == "" {
		cfg.SubjectPrefix = "cavi.jobs"
	}
	nc, err := nats.Connect(cfg.URL,
		nats.Name(cfg.ClientName),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       cfg.Stream,
		Subjects:   []string{cfg.SubjectPrefix + ".>"},
		Retention:  jetstream.WorkQueuePolicy,
		Storage:    jetstream.FileStorage,
		Duplicates: 2 * time.Minute,
		MaxAge:     7 * 24 * time.Hour,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jobs stream: %w", err)
	}
	return &Bus{nc: nc, js: js, stream: stream, prefix: cfg.SubjectPrefix}, nil
}

func (b *Bus) subject(kind string) string { return b.prefix + "." + kind }

// Publish sends a job message. The job ID is the deduplication key.
func (b *Bus) Publish(ctx context.Context, m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = b.js.Publish(ctx, b.subject(m.Kind), data, jetstream.WithMsgID(m.JobID))
	if err != nil {
		return fmt.Errorf("publish job: %w", err)
	}
	return nil
}

// consumer returns the durable consumer workers share for kind.
func (b *Bus) consumer(ctx context.Context, kind string, ackWait time.Duration) (jetstream.Consumer, error) {
	name := "worker-" + sanitize(kind)
	return b.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       name,
		FilterSubject: b.subject(kind),
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		// Attempts are counted in Postgres; JetStream keeps redelivering
		// until the worker acknowledges or terminates the message.
		MaxDeliver: -1,
	})
}

// Ping reports whether the connection is up.
func (b *Bus) Ping() error {
	if b.nc.Status() != nats.CONNECTED {
		return errors.New("nats not connected")
	}
	return nil
}

// Close drains and closes the connection.
func (b *Bus) Close() {
	_ = b.nc.Drain()
}

func sanitize(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r == '.' || r == '*' || r == '>' || r == ' ' {
			out[i] = '-'
		}
	}
	return string(out)
}

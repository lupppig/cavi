// Command media-worker processes uploaded media: it probes each file with
// FFmpeg and makes thumbnails. It is deployed separately from the API (it
// needs FFmpeg and more CPU) and scales by running more copies.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lupppig/cavi/apps/api/internal/config"
	"github.com/lupppig/cavi/apps/api/internal/database"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/media"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("media worker exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadWorker(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := newLogger(cfg.Env).With(slog.String("service", "media-worker"))
	slog.SetDefault(logger)

	ff := ffmpeg.New()
	if err := ff.Available(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db := database.Open(cfg.DatabaseURL)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}

	bus, err := jobs.Connect(ctx, jobs.BusConfig{URL: cfg.NATSURL, ClientName: "cavi-media-worker"})
	if err != nil {
		return err
	}
	defer bus.Close()

	st := cfg.Storage
	store := storage.NewS3(storage.S3Config{
		// Workers fetch from storage directly, never via the browser endpoint.
		Endpoint: st.Endpoint, Region: st.Region, Bucket: st.Bucket,
		AccessKeyID: st.AccessKeyID, SecretAccessKey: st.SecretAccessKey, PathStyle: st.PathStyle,
	})
	processor := media.NewProcessor(db, store, ff, logger)
	worker := jobs.NewWorker(db, bus, jobs.KindMediaProcess, processor, logger,
		jobs.WorkerOptions{Concurrency: cfg.Concurrency})

	logger.Info("media worker started", slog.Int("concurrency", cfg.Concurrency))
	err = worker.Run(ctx)
	logger.Info("media worker stopped")
	return err
}

func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env == config.EnvDevelopment {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

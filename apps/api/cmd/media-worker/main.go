// Command media-worker processes uploaded media: it probes each file with
// FFmpeg and makes thumbnails, and transcribes speech with the
// organisation's AI model when GEMINI_API_KEY is set. It is deployed
// separately from the API (it needs FFmpeg, more CPU and the AI key) and
// scales by running more copies.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/ai"
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
	opts := jobs.WorkerOptions{Concurrency: cfg.Concurrency}
	workers := []*jobs.Worker{
		jobs.NewWorker(db, bus, jobs.KindMediaProcess, media.NewProcessor(db, store, ff, logger), logger, opts),
	}
	if cfg.GeminiAPIKey != "" {
		gemini, err := ai.NewGemini(ctx, cfg.GeminiAPIKey)
		if err != nil {
			return err
		}
		// Transcription calls are slow and rate-limited; run fewer at once.
		workers = append(workers, jobs.NewWorker(db, bus, jobs.KindMediaTranscribe,
			media.NewTranscriber(db, store, ff, gemini, logger), logger,
			jobs.WorkerOptions{Concurrency: max(1, cfg.Concurrency/2), Lease: 45 * time.Minute}))
	} else {
		logger.Warn("transcription disabled: GEMINI_API_KEY is not set; transcription jobs will wait")
	}

	logger.Info("media worker started", slog.Int("concurrency", cfg.Concurrency), slog.Bool("transcription", cfg.GeminiAPIKey != ""))
	var wg sync.WaitGroup
	errs := make(chan error, len(workers))
	for _, w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Run(ctx); err != nil {
				errs <- err
				stop()
			}
		}()
	}
	wg.Wait()
	close(errs)
	logger.Info("media worker stopped")
	return <-errs
}

func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env == config.EnvDevelopment {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

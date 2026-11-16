// Command media-worker processes uploaded media: it probes each file with
// FFmpeg, imports videos from links, makes thumbnails, finds the people in
// videos for auto framing
// and renders clips, and, when GEMINI_API_KEY is set, transcribes
// speech and finds highlights with the organisation's AI model. It is deployed
// separately from the API (it needs FFmpeg, more CPU and the AI key) and
// scales by running more copies.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/brief"
	"github.com/lupppig/cavi/apps/api/internal/config"
	"github.com/lupppig/cavi/apps/api/internal/database"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/media"
	"github.com/lupppig/cavi/apps/api/internal/plan"
	"github.com/lupppig/cavi/apps/api/internal/render"
	"github.com/lupppig/cavi/apps/api/internal/storage"
	"github.com/lupppig/cavi/apps/api/internal/voiceover"
	"github.com/lupppig/cavi/apps/api/internal/youtube"
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
		// Rendering is CPU-heavy and slow: fewer at once, long leases, and
		// FFmpeg's own timeout (at most 25 minutes) stays inside the lease.
		jobs.NewWorker(db, bus, jobs.KindRenderClip, render.NewRenderer(db, store, ff, logger), logger,
			jobs.WorkerOptions{Concurrency: max(1, cfg.Concurrency/2), Lease: 30 * time.Minute}),
		// Framing analysis decodes whole videos: one at a time, with a lease
		// longer than its own timeout (at most 110 minutes).
		jobs.NewWorker(db, bus, jobs.KindMediaFraming,
			media.NewFramer(db, store, ff, media.HelperDetectors(framing.DefaultHelper, framing.DefaultModel), logger), logger,
			jobs.WorkerOptions{Concurrency: 1, Lease: 2 * time.Hour}),
		// Imports download from YouTube, Twitch or Kick for up to an hour.
		jobs.NewWorker(db, bus, jobs.KindMediaImport,
			media.NewImporter(db, store, media.YTDLP{Bin: "yt-dlp"}, logger), logger,
			jobs.WorkerOptions{Concurrency: 2, Lease: 75 * time.Minute}),
	}
	if cfg.YouTube.Enabled() {
		// Reading a channel's numbers is a handful of API calls.
		workers = append(workers, jobs.NewWorker(db, bus, jobs.KindYouTubeSync,
			youtube.NewSyncer(db, youtube.NewClient(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret, ""), cfg.YouTube.TokenKey, logger), logger,
			jobs.WorkerOptions{Concurrency: 1, Lease: 5 * time.Minute}))
	}
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		logger.Warn("imports from links disabled: yt-dlp isn't installed; imports will fail (it is in the media worker image)")
	}
	if _, err := exec.LookPath(framing.DefaultHelper); err != nil {
		logger.Warn("auto framing disabled: the cavi-faces helper isn't installed; framing analysis will fail (it is in the media worker image)")
	}
	if cfg.GeminiAPIKey != "" {
		gemini, err := ai.NewGemini(ctx, cfg.GeminiAPIKey)
		if err != nil {
			return err
		}
		// Voiceovers go to the provider of the organisation's model.
		narrators := ai.Narrators{ai.ProviderGemini: gemini}
		if cfg.ElevenLabsAPIKey != "" {
			eleven, err := ai.NewElevenLabs(cfg.ElevenLabsAPIKey, "")
			if err != nil {
				return err
			}
			narrators[ai.ProviderElevenLabs] = eleven
		} else {
			logger.Warn("ElevenLabs disabled: ELEVENLABS_API_KEY is not set; ElevenLabs voiceovers will fail")
		}
		// AI calls are slow and rate-limited; run fewer at once.
		workers = append(workers, jobs.NewWorker(db, bus, jobs.KindMediaTranscribe,
			media.NewTranscriber(db, store, ff, gemini, logger), logger,
			jobs.WorkerOptions{
				Concurrency: max(1, cfg.Concurrency/2),
				Lease:       45 * time.Minute,
				RetryDelay:  providerRetry,
			}),
			jobs.NewWorker(db, bus, jobs.KindMediaHighlights,
				media.NewHighlighter(db, gemini, logger), logger,
				jobs.WorkerOptions{Concurrency: max(1, cfg.Concurrency/2), Lease: 10 * time.Minute, RetryDelay: providerRetry}),
			jobs.NewWorker(db, bus, jobs.KindBriefRead,
				brief.NewReader(db, gemini, logger), logger,
				jobs.WorkerOptions{Concurrency: 1, Lease: 5 * time.Minute, RetryDelay: providerRetry}),
			jobs.NewWorker(db, bus, jobs.KindPlansCreate,
				plan.NewPlanner(db, gemini, logger), logger,
				jobs.WorkerOptions{Concurrency: 1, Lease: 10 * time.Minute, RetryDelay: providerRetry}),
			jobs.NewWorker(db, bus, jobs.KindVoiceRecording,
				voiceover.NewRecorder(db, store, ff, gemini, logger), logger,
				jobs.WorkerOptions{Concurrency: 1, Lease: 5 * time.Minute, RetryDelay: providerRetry}),
			jobs.NewWorker(db, bus, jobs.KindVoiceoverSpeak,
				voiceover.NewNarrator(db, store, narrators, gemini, logger), logger,
				jobs.WorkerOptions{Concurrency: 1, Lease: 5 * time.Minute, RetryDelay: providerRetry}))
	} else {
		logger.Warn("AI disabled: GEMINI_API_KEY is not set; transcription, highlight and brief jobs will wait")
	}

	logger.Info("media worker started", slog.Int("concurrency", cfg.Concurrency), slog.Bool("ai", cfg.GeminiAPIKey != ""))
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

// providerRetry backs off longer than media processing: provider overload
// ("high demand") usually clears in minutes.
func providerRetry(n int) time.Duration { return min(30*time.Second<<min(n-1, 6), 5*time.Minute) }

func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env == config.EnvDevelopment {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

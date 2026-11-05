package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Transcription limits.
const (
	// MaxTranscriptionDuration bounds cost per request.
	MaxTranscriptionDuration = 3 * time.Hour
	// transcriptionChunk is how much audio goes to the model per call, so
	// output stays well within the model's limits on long media.
	transcriptionChunk = 10 * time.Minute
)

// Transcript statuses.
const (
	TranscriptQueued  = "queued"
	TranscriptRunning = "running"
	TranscriptReady   = "ready"
	TranscriptFailed  = "failed"
)

// ErrTranscriptionInProgress means a transcript is already queued or running.
var ErrTranscriptionInProgress = errors.New("media: transcription in progress")

// Transcript is a file's transcript as seen by a caller.
type Transcript struct {
	ID            string
	MediaAssetID  string
	Status        string
	Provider      string
	Model         string
	ModelLabel    string
	PromptVersion string
	Language      string
	Error         string
	CreatedAt     time.Time
	CompletedAt   time.Time
	Segments      []ai.Segment
}

// RequestTranscript queues transcription of a file's speech (editing.manage).
// The audio is sent to the organisation's chosen AI provider, which the
// audit log records. Requesting again after a transcript is ready or
// failed replaces it.
func (s *Service) RequestTranscript(ctx context.Context, actorID, orgID, projectID, assetID string) (Transcript, error) {
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		a, err := s.load(ctx, tx, acc, actorID, orgID, projectID, assetID, true)
		if err != nil {
			return err
		}
		if a.Status != "uploaded" {
			return ErrNotFound
		}
		if !authz.Can(acc.Role, authz.EditingManage) {
			return ErrForbidden
		}
		switch {
		case a.ProcessingStatus == ProcessingPending || a.ProcessingStatus == ProcessingProcessing:
			return invalid("media", "Wait for the file to finish processing, then transcribe it.")
		case a.ProcessingStatus == ProcessingFailed:
			return invalid("media", "This file couldn’t be processed, so it can’t be transcribed.")
		case a.AudioCodec == "":
			return invalid("media", "This file has no audio to transcribe.")
		case time.Duration(a.DurationMS)*time.Millisecond > MaxTranscriptionDuration:
			return invalid("media", "Files longer than 3 hours can’t be transcribed yet.")
		}
		var status string
		err = tx.QueryRowContext(ctx, `SELECT status FROM transcripts WHERE media_asset_id = ? FOR UPDATE`, assetID).Scan(&status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load transcript: %w", err)
		}
		if status == TranscriptQueued || status == TranscriptRunning {
			return ErrTranscriptionInProgress
		}
		model, err := ai.Resolve(ctx, tx, orgID, ai.TaskTranscription)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transcripts (organisation_id, media_asset_id, status, provider, model, prompt_version, requested_by_user_id)
			VALUES (?, ?, 'queued', ?, ?, ?, ?)
			ON CONFLICT (media_asset_id) DO UPDATE SET
				status = 'queued', provider = EXCLUDED.provider, model = EXCLUDED.model,
				prompt_version = EXCLUDED.prompt_version, requested_by_user_id = EXCLUDED.requested_by_user_id,
				error = NULL, updated_at = now(), created_at = now(), completed_at = NULL`,
			orgID, assetID, model.Provider, model.ID, ai.TranscriptionPromptVersion, actorID); err != nil {
			return fmt.Errorf("queue transcript: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaTranscribe, OrganisationID: orgID,
			MediaAssetID: assetID, CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 6}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "media.transcription_requested", "media_asset", assetID,
			map[string]any{"provider": model.Provider, "model": model.ID})
	})
	if err != nil {
		return Transcript{}, err
	}
	s.jobQueued()
	return s.Transcript(ctx, actorID, orgID, projectID, assetID)
}

// Transcript returns a file's transcript, with segments once ready. Anyone
// who can see the project can read it.
func (s *Service) Transcript(ctx context.Context, actorID, orgID, projectID, assetID string) (Transcript, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Transcript{}, err
	}
	a, err := s.load(ctx, s.db, acc, actorID, orgID, projectID, assetID, false)
	if err != nil {
		return Transcript{}, err
	}
	if a.Status != "uploaded" {
		return Transcript{}, ErrNotFound
	}
	var t Transcript
	var completed sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT id, media_asset_id, status, provider, model, prompt_version, coalesce(language, ''), coalesce(error, ''), created_at, completed_at
		FROM transcripts WHERE media_asset_id = ? AND organisation_id = ?`, assetID, orgID).
		Scan(&t.ID, &t.MediaAssetID, &t.Status, &t.Provider, &t.Model, &t.PromptVersion, &t.Language, &t.Error, &t.CreatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return Transcript{}, ErrNotFound
	}
	if err != nil {
		return Transcript{}, fmt.Errorf("load transcript: %w", err)
	}
	t.CompletedAt = completed.Time
	if m, ok := ai.Lookup(ai.TaskTranscription, t.Provider, t.Model); ok {
		t.ModelLabel = m.Label
	} else {
		t.ModelLabel = t.Model
	}
	t.Segments = []ai.Segment{}
	if t.Status != TranscriptReady {
		return t, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT start_ms, end_ms, speaker, text FROM transcript_segments WHERE transcript_id = ? ORDER BY position`, t.ID)
	if err != nil {
		return Transcript{}, fmt.Errorf("load segments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var seg ai.Segment
		if err := rows.Scan(&seg.StartMS, &seg.EndMS, &seg.Speaker, &seg.Text); err != nil {
			return Transcript{}, err
		}
		t.Segments = append(t.Segments, seg)
	}
	return t, rows.Err()
}

// AudioSource extracts audio for transcription. *ffmpeg.Runner implements it.
type AudioSource interface {
	Audio(ctx context.Context, input string, offset, length time.Duration) ([]byte, error)
}

// Transcriber handles media.transcribe jobs in the media worker.
type Transcriber struct {
	db     *bun.DB
	store  storage.Store
	audio  AudioSource
	ai     ai.Transcriber
	logger *slog.Logger
}

// NewTranscriber returns a transcription job handler.
func NewTranscriber(db *bun.DB, store storage.Store, audio AudioSource, provider ai.Transcriber, logger *slog.Logger) *Transcriber {
	return &Transcriber{db: db, store: store, audio: audio, ai: provider, logger: logger}
}

var _ jobs.Handler = (*Transcriber)(nil)

// User-facing failure messages.
const (
	msgTranscriptionRejected = "The AI provider couldn’t transcribe this file."
	msgTranscriptionGaveUp   = "Transcription kept failing, so we stopped trying. You can try again."
)

// Process implements jobs.Handler. It is idempotent: a rerun replaces the
// segments in one transaction.
func (t *Transcriber) Process(ctx context.Context, job jobs.Job) error {
	var transcriptID, provider, modelID, key string
	var durationMS int64
	err := t.db.QueryRowContext(ctx, `
		SELECT tr.id, tr.provider, tr.model, m.storage_key, coalesce(m.duration_ms, 0)
		FROM transcripts tr JOIN media_assets m ON m.id = tr.media_asset_id AND m.organisation_id = tr.organisation_id
		WHERE tr.media_asset_id = ? AND tr.organisation_id = ? AND tr.status IN ('queued', 'running')`,
		job.MediaAssetID, job.OrganisationID).Scan(&transcriptID, &provider, &modelID, &key, &durationMS)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted, or superseded; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load transcript: %w", err)
	}
	// The model was fixed when transcription was requested.
	model, ok := ai.Lookup(ai.TaskTranscription, provider, modelID)
	if !ok {
		return jobs.Permanent(fmt.Errorf("%w: model %s/%s is no longer supported", ai.ErrPermanent, provider, modelID))
	}
	if durationMS <= 0 {
		return jobs.Permanent(fmt.Errorf("media %s has no duration", job.MediaAssetID))
	}
	if _, err := t.db.ExecContext(ctx, `UPDATE transcripts SET status = 'running', updated_at = now() WHERE id = ?`, transcriptID); err != nil {
		return err
	}
	url, _, err := t.store.PresignDownload(ctx, key, time.Hour)
	if err != nil {
		return err
	}

	total := time.Duration(durationMS) * time.Millisecond
	var segments []ai.Segment
	language := ""
	for offset := time.Duration(0); offset < total; offset += transcriptionChunk {
		length := min(transcriptionChunk, total-offset)
		audio, err := t.audio.Audio(ctx, url, offset, length)
		if err != nil {
			if errors.Is(err, ffmpeg.ErrUnreadable) {
				return jobs.Permanent(err)
			}
			return err
		}
		part, usage, err := t.ai.Transcribe(ctx, model, audio, "audio/mpeg", length.Milliseconds())
		t.recordUsage(ctx, job, model, usage, length, err == nil)
		if err != nil {
			if errors.Is(err, ai.ErrPermanent) {
				return jobs.Permanent(err)
			}
			return err // includes invalid output, which may pass on retry
		}
		if language == "" {
			language = part.Language
		}
		for _, seg := range part.Segments {
			seg.StartMS += offset.Milliseconds()
			seg.EndMS += offset.Milliseconds()
			segments = append(segments, seg)
		}
	}

	return t.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE transcripts SET status = 'ready', language = NULLIF(?, ''), error = NULL, completed_at = now(), updated_at = now()
			WHERE id = ? AND status IN ('queued', 'running')`, language, transcriptID)
		if err != nil {
			return fmt.Errorf("save transcript: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // deleted or replaced meanwhile
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transcript_segments WHERE transcript_id = ?`, transcriptID); err != nil {
			return err
		}
		for i, seg := range segments {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transcript_segments (transcript_id, position, start_ms, end_ms, speaker, text) VALUES (?, ?, ?, ?, ?, ?)`,
				transcriptID, i, seg.StartMS, seg.EndMS, seg.Speaker, seg.Text); err != nil {
				return fmt.Errorf("save segment: %w", err)
			}
		}
		return nil
	})
}

func (t *Transcriber) recordUsage(ctx context.Context, job jobs.Job, model ai.Model, u ai.Usage, audio time.Duration, ok bool) {
	if _, err := t.db.ExecContext(ctx, `
		INSERT INTO ai_usage (organisation_id, job_id, task, provider, model, input_tokens, output_tokens, audio_ms, succeeded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.OrganisationID, job.ID, ai.TaskTranscription, model.Provider, model.ID, u.InputTokens, u.OutputTokens, audio.Milliseconds(), ok); err != nil {
		t.logger.WarnContext(ctx, "record ai usage failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
	}
}

// Failed implements jobs.Handler.
func (t *Transcriber) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgTranscriptionGaveUp
	switch {
	case errors.Is(err, ffmpeg.ErrUnreadable):
		msg = msgUnreadable
	case errors.Is(err, ai.ErrPermanent):
		msg = msgTranscriptionRejected
	}
	_, dbErr := t.db.ExecContext(ctx, `
		UPDATE transcripts SET status = 'failed', error = ?, updated_at = now()
		WHERE media_asset_id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		msg, job.MediaAssetID, job.OrganisationID)
	return dbErr
}

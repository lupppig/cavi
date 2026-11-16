package voiceover

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Recordings: the creator's own voice, recorded in the browser.
const (
	// ProviderRecording marks voiceovers that are recordings.
	ProviderRecording = "recording"
	recordingModel    = "own-voice"
	recordingVersion  = "recording-v1"
	// MaxRecordingBytes is the largest upload (a minute of browser audio
	// is well under 1 MB).
	MaxRecordingBytes = 10 << 20
	// Recordings (once trimmed) must last between these.
	minRecordingMS = 400
	maxRecordingMS = 60_000
	recordingRate  = 24000
	uploadURLTTL   = 15 * time.Minute
)

// StatusUploading is a recording the browser is still sending.
const StatusUploading = "uploading"

// recordingTypes are the audio formats browsers record in.
var recordingTypes = map[string]bool{
	"audio/webm": true, "video/webm": true, "audio/ogg": true, "audio/mp4": true, "video/mp4": true,
	"audio/mpeg": true, "audio/wav": true, "audio/x-wav": true, "audio/aac": true, "audio/x-m4a": true,
}

// ErrNotReceived means the browser hasn't finished uploading.
var ErrNotReceived = errors.New("voiceover: recording not received")

func recordingKey(orgID, projectID, id string) string {
	return "organisations/" + orgID + "/projects/" + projectID + "/voiceovers/" + id + "-recording"
}

// CreateRecording starts a recording of script in the creator's own voice
// (editing.manage): it returns the voiceover (uploading) and the signed
// instruction the browser follows to send the audio.
func (s *Service) CreateRecording(ctx context.Context, actorID, orgID, projectID, script, contentType string, size int64) (Voiceover, storage.Upload, error) {
	script, err := ai.CleanScript(script)
	if err != nil {
		return Voiceover{}, storage.Upload{}, invalid("script", fmt.Sprintf("Write what you'll say, up to %d characters.", ai.MaxScriptRunes))
	}
	contentType = strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if !recordingTypes[contentType] {
		return Voiceover{}, storage.Upload{}, invalid("content_type", "Record in a format browsers use: WebM, Ogg, MP4, MP3 or WAV audio.")
	}
	if size < 1 || size > MaxRecordingBytes {
		return Voiceover{}, storage.Upload{}, invalid("size_bytes", "Recordings must be under 10 MB.")
	}
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Voiceover{}, storage.Upload{}, err
	}
	if !authz.Can(acc.Role, authz.EditingManage) {
		return Voiceover{}, storage.Upload{}, ErrForbidden
	}
	var id string
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, true); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO voiceovers (organisation_id, project_id, script, voice, style, status, provider, model, prompt_version, requested_by_user_id)
			VALUES (?, ?, ?, 'own', 'natural', 'uploading', ?, ?, ?, ?) RETURNING id`,
			orgID, projectID, script, ProviderRecording, recordingModel, recordingVersion, actorID).Scan(&id); err != nil {
			return fmt.Errorf("insert recording: %w", err)
		}
		_, err := tx.ExecContext(ctx, `UPDATE voiceovers SET recording_key = ? WHERE id = ?`, recordingKey(orgID, projectID, id), id)
		return err
	})
	if err != nil {
		return Voiceover{}, storage.Upload{}, err
	}
	up, err := s.store.PresignUpload(ctx, recordingKey(orgID, projectID, id), contentType, "recording", size, uploadURLTTL)
	if err != nil {
		return Voiceover{}, storage.Upload{}, err
	}
	v, err := s.Get(ctx, actorID, orgID, projectID, id)
	return v, up, err
}

// CompleteRecording checks the uploaded recording arrived and queues it
// for processing. Only the person recording can complete it; completing a
// processed recording returns it unchanged.
func (s *Service) CompleteRecording(ctx context.Context, actorID, orgID, projectID, id string) (Voiceover, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Voiceover{}, err
	}
	if !account.ValidUUID(id) {
		return Voiceover{}, ErrNotFound
	}
	var status, provider, by, key string
	err = s.db.QueryRowContext(ctx, `
		SELECT status, provider, coalesce(requested_by_user_id::text, ''), coalesce(recording_key, '') FROM voiceovers
		WHERE id = ? AND project_id = ? AND organisation_id = ?`, id, projectID, orgID).Scan(&status, &provider, &by, &key)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && provider != ProviderRecording) {
		return Voiceover{}, ErrNotFound
	}
	if err != nil {
		return Voiceover{}, err
	}
	if status != StatusUploading {
		return s.Get(ctx, actorID, orgID, projectID, id)
	}
	if by != actorID || !authz.Can(acc.Role, authz.EditingManage) {
		return Voiceover{}, ErrForbidden
	}
	if _, err := s.store.Stat(ctx, key); errors.Is(err, storage.ErrNotFound) {
		return Voiceover{}, ErrNotReceived
	} else if err != nil {
		return Voiceover{}, err
	}
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE voiceovers SET status = 'queued', updated_at = now() WHERE id = ? AND status = 'uploading'`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindVoiceRecording, OrganisationID: orgID, SubjectID: id,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 3}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "voiceover.recorded", "voiceover", id, map[string]any{"project_id": projectID})
	})
	if err != nil {
		return Voiceover{}, err
	}
	if s.onJob != nil {
		s.onJob()
	}
	return s.Get(ctx, actorID, orgID, projectID, id)
}

// VoiceDecoder is what processing a recording needs from FFmpeg.
// *ffmpeg.Runner implements it.
type VoiceDecoder interface {
	Voice(ctx context.Context, in string, rate int, timeout time.Duration) ([]byte, error)
}

// Recorder handles voiceover.recording jobs: it decodes a recording, trims
// its silence, checks it, times its words for captions and stores it as a
// ready voiceover. It runs in the media worker.
type Recorder struct {
	db     *bun.DB
	store  storage.Store
	ff     VoiceDecoder
	timer  ai.Transcriber
	logger *slog.Logger
}

// NewRecorder returns a Recorder. timer times the words for captions; nil
// spreads them by length.
func NewRecorder(db *bun.DB, store storage.Store, ff VoiceDecoder, timer ai.Transcriber, logger *slog.Logger) *Recorder {
	return &Recorder{db: db, store: store, ff: ff, timer: timer, logger: logger}
}

var _ jobs.Handler = (*Recorder)(nil)

// RecordingError is why a recording can't be used, for the person who
// made it.
type RecordingError struct{ Reason string }

func (e *RecordingError) Error() string { return "voiceover: recording refused: " + e.Reason }

// Process implements jobs.Handler. It is idempotent.
func (r *Recorder) Process(ctx context.Context, job jobs.Job) error {
	var projectID, script, key string
	err := r.db.QueryRowContext(ctx, `
		SELECT project_id, script, coalesce(recording_key, '') FROM voiceovers
		WHERE id = ? AND organisation_id = ? AND provider = ? AND status IN ('queued', 'running')`,
		job.SubjectID, job.OrganisationID, ProviderRecording).Scan(&projectID, &script, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load recording: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE voiceovers SET status = 'running', updated_at = now() WHERE id = ?`, job.SubjectID); err != nil {
		return err
	}
	url, _, err := r.store.PresignDownload(ctx, key, 10*time.Minute)
	if err != nil {
		return err
	}
	pcm, err := r.ff.Voice(ctx, url, recordingRate, 3*time.Minute)
	if err != nil {
		return jobs.Permanent(&RecordingError{Reason: "The recording couldn’t be read. Record it again."})
	}
	speech := ai.Speech{PCM: pcm, SampleRate: recordingRate}
	switch d := speech.DurationMS(); {
	case d < minRecordingMS || peak(pcm) < 0.02:
		return jobs.Permanent(&RecordingError{Reason: "Nothing was heard in the recording. Check your microphone and record it again."})
	case d > maxRecordingMS:
		return jobs.Permanent(&RecordingError{Reason: "Keep each recording under a minute."})
	}
	words, usage := TimeWords(ctx, r.timer, script, speech)
	if r.timer != nil {
		recordUsage(ctx, r.db, r.logger, job, ai.TaskTranscription, ai.Default(ai.TaskTranscription), usage, true)
	}
	wj := make([]wordJSON, len(words))
	for i, w := range words {
		wj[i] = wordJSON{StartMS: w.StartMS, EndMS: w.EndMS, Text: w.Text}
	}
	wordsJSON, err := json.Marshal(wj)
	if err != nil {
		return err
	}
	wav := speech.WAV()
	out := audioKey(job.OrganisationID, projectID, job.SubjectID)
	if err := r.store.Put(ctx, out, "audio/wav", wav); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE voiceovers SET status = 'ready', storage_key = ?, duration_ms = ?, size_bytes = ?, words = ?::jsonb,
			error = NULL, completed_at = now(), updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		out, speech.DurationMS(), len(wav), string(wordsJSON), job.SubjectID, job.OrganisationID)
	if err != nil {
		return fmt.Errorf("save recording: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, _ = r.db.ExecContext(ctx, `INSERT INTO storage_deletions (storage_key) VALUES (?)`, out)
	}
	return nil
}

// Failed implements jobs.Handler.
func (r *Recorder) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := "Processing the recording kept failing. Record it again."
	var refused *RecordingError
	if errors.As(err, &refused) {
		msg = refused.Reason
	}
	_, dbErr := r.db.ExecContext(ctx, `
		UPDATE voiceovers SET status = 'failed', error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`, msg, job.SubjectID, job.OrganisationID)
	return dbErr
}

// peak is the loudest sample of 16-bit PCM, from 0 to 1.
func peak(pcm []byte) float64 {
	m := 0.0
	for i := 0; i+1 < len(pcm); i += 2 {
		m = math.Max(m, math.Abs(float64(int16(binary.LittleEndian.Uint16(pcm[i:])))/32768))
	}
	return m
}

func recordUsage(ctx context.Context, db *bun.DB, logger *slog.Logger, job jobs.Job, task string, model ai.Model, u ai.Usage, ok bool) {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO ai_usage (organisation_id, job_id, task, provider, model, input_tokens, output_tokens, succeeded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		job.OrganisationID, job.ID, task, model.Provider, model.ID, u.InputTokens, u.OutputTokens, ok); err != nil {
		logger.WarnContext(ctx, "record ai usage failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
	}
}

// Sweep removes recordings never finished uploading after a day (their
// objects are queued for deletion by a trigger).
func (s *Service) Sweep(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM voiceovers WHERE status = 'uploading' AND created_at < now() - interval '1 day'`)
	if err != nil {
		return 0, fmt.Errorf("sweep recordings: %w", err)
	}
	return res.RowsAffected()
}

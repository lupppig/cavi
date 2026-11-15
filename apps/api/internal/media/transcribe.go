package media

import (
	"context"
	"database/sql"
	"encoding/json"
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
	// minTranscriptionChunk is the shortest stretch a section is split
	// into when the model's transcript of it is too long for one reply.
	minTranscriptionChunk = 150 * time.Second
	// sectionTries is how many times one section is sent before invalid
	// output fails the job attempt.
	sectionTries = 3
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
	Speakers      []Speaker
}

// Speaker is a transcript's speaker label and how their voice sounds.
type Speaker struct {
	Label string
	Voice string
	// VoiceSetBy is "ai" (suggested) or "person" (corrected by someone).
	VoiceSetBy string
}

// storedWord is a word as kept in transcript_segments.words.
type storedWord struct {
	S int64  `json:"s"`
	E int64  `json:"e"`
	T string `json:"t"`
}

func encodeWords(ws []ai.Word) any {
	if len(ws) == 0 {
		return nil
	}
	out := make([]storedWord, len(ws))
	for i, w := range ws {
		out[i] = storedWord{S: w.StartMS, E: w.EndMS, T: w.Text}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// DecodeWords reads transcript_segments.words (nil for none).
func DecodeWords(raw []byte) []ai.Word {
	if len(raw) == 0 {
		return nil
	}
	var in []storedWord
	if json.Unmarshal(raw, &in) != nil {
		return nil
	}
	out := make([]ai.Word, len(in))
	for i, w := range in {
		out[i] = ai.Word{StartMS: w.S, EndMS: w.E, Text: w.T}
	}
	return out
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
	if err := rows.Err(); err != nil {
		return Transcript{}, err
	}
	t.Speakers, err = loadSpeakers(ctx, s.db, t.ID)
	return t, err
}

func loadSpeakers(ctx context.Context, db bun.IDB, transcriptID string) ([]Speaker, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT label, voice, voice_set_by FROM transcript_speakers WHERE transcript_id = ? ORDER BY position`, transcriptID)
	if err != nil {
		return nil, fmt.Errorf("load speakers: %w", err)
	}
	defer rows.Close()
	out := []Speaker{}
	for rows.Next() {
		var sp Speaker
		if err := rows.Scan(&sp.Label, &sp.Voice, &sp.VoiceSetBy); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// SetSpeakerVoice corrects how a speaker's voice is labelled
// (editing.manage). The change is audited.
func (s *Service) SetSpeakerVoice(ctx context.Context, actorID, orgID, projectID, assetID, label, voice string) (Transcript, error) {
	if !ai.ValidVoice(voice) {
		return Transcript{}, invalid("voice", "Choose woman, man or unclear.")
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		if _, err := s.load(ctx, tx, acc, actorID, orgID, projectID, assetID, false); err != nil {
			return err
		}
		if !authz.Can(acc.Role, authz.EditingManage) {
			return ErrForbidden
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE transcript_speakers sp SET voice = ?, voice_set_by = 'person'
			FROM transcripts tr
			WHERE sp.transcript_id = tr.id AND tr.media_asset_id = ? AND tr.organisation_id = ? AND tr.status = 'ready' AND sp.label = ?`,
			voice, assetID, orgID, label)
		if err != nil {
			return fmt.Errorf("set speaker voice: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return account.Audit(ctx, tx, orgID, actorID, "media.speaker_voice_set", "media_asset", assetID,
			map[string]any{"speaker": label, "voice": voice})
	})
	if err != nil {
		return Transcript{}, err
	}
	return s.Transcript(ctx, actorID, orgID, projectID, assetID)
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
	voices := map[string]string{}
	var order []string
	language := ""
	// section transcribes one stretch of audio. Invalid output is often a
	// one-off, so it retries the stretch alone a couple of times before
	// failing the attempt (which would re-send every section). Speech too
	// dense for one reply is split in half instead, down to
	// minTranscriptionChunk.
	type part struct {
		offset time.Duration
		ai.Transcript
	}
	var section func(offset, length time.Duration) ([]part, error)
	section = func(offset, length time.Duration) ([]part, error) {
		audio, err := t.audio.Audio(ctx, url, offset, length)
		if err != nil {
			if errors.Is(err, ffmpeg.ErrUnreadable) {
				return nil, jobs.Permanent(err)
			}
			return nil, err
		}
		for try := 1; ; try++ {
			tr, usage, err := t.ai.Transcribe(ctx, model, audio, "audio/mpeg", length.Milliseconds())
			t.recordUsage(ctx, job, model, usage, length, err == nil)
			switch {
			case err == nil:
				return []part{{offset, tr}}, nil
			case errors.Is(err, ai.ErrOutputTooLong) && length/2 >= minTranscriptionChunk:
				t.logger.InfoContext(ctx, "transcription section too dense for one reply; splitting it",
					slog.String("job_id", job.ID), slog.Duration("offset", offset), slog.Duration("length", length))
				first, err := section(offset, length/2)
				if err != nil {
					return nil, err
				}
				rest, err := section(offset+length/2, length-length/2)
				return append(first, rest...), err
			case !errors.Is(err, ai.ErrInvalidOutput) || try == sectionTries:
				if errors.Is(err, ai.ErrPermanent) {
					return nil, jobs.Permanent(err)
				}
				return nil, err // includes invalid output, which may pass on retry
			}
			t.logger.WarnContext(ctx, "transcription section output invalid; retrying the section",
				slog.String("job_id", job.ID), slog.Duration("offset", offset), slog.Int("try", try), slog.String("error", err.Error()))
		}
	}
	var parts []part
	for offset := time.Duration(0); offset < total; offset += transcriptionChunk {
		ps, err := section(offset, min(transcriptionChunk, total-offset))
		if err != nil {
			return err
		}
		parts = append(parts, ps...)
	}
	for _, p := range parts {
		offset, part := p.offset, p.Transcript
		if language == "" {
			language = part.Language
		}
		for _, seg := range part.Segments {
			seg.StartMS += offset.Milliseconds()
			seg.EndMS += offset.Milliseconds()
			for i := range seg.Words {
				seg.Words[i].StartMS += offset.Milliseconds()
				seg.Words[i].EndMS += offset.Milliseconds()
			}
			segments = append(segments, seg)
		}
		// Labels are per section; where sections disagree about a voice,
		// call it unclear rather than pick one.
		for _, sp := range part.Speakers {
			if v, ok := voices[sp.Label]; ok && v != sp.Voice {
				voices[sp.Label] = ai.VoiceUnclear
			} else if !ok {
				voices[sp.Label] = sp.Voice
				order = append(order, sp.Label)
			}
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
				INSERT INTO transcript_segments (transcript_id, position, start_ms, end_ms, speaker, text, words)
				VALUES (?, ?, ?, ?, ?, ?, ?::jsonb)`,
				transcriptID, i, seg.StartMS, seg.EndMS, seg.Speaker, seg.Text, encodeWords(seg.Words)); err != nil {
				return fmt.Errorf("save segment: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transcript_speakers WHERE transcript_id = ?`, transcriptID); err != nil {
			return err
		}
		used := map[string]bool{}
		for _, seg := range segments {
			used[seg.Speaker] = true
		}
		pos := 0
		for _, label := range order {
			if !used[label] {
				continue
			}
			delete(used, label)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transcript_speakers (transcript_id, position, label, voice) VALUES (?, ?, ?, ?)`,
				transcriptID, pos, label, voices[label]); err != nil {
				return fmt.Errorf("save speaker: %w", err)
			}
			pos++
		}
		// Labels the model didn't describe.
		for _, seg := range segments {
			if !used[seg.Speaker] {
				continue
			}
			delete(used, seg.Speaker)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transcript_speakers (transcript_id, position, label, voice) VALUES (?, ?, ?, 'unclear')`,
				transcriptID, pos, seg.Speaker); err != nil {
				return fmt.Errorf("save speaker: %w", err)
			}
			pos++
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

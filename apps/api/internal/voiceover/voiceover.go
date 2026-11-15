// Package voiceover makes voiceovers: a script spoken by an AI voice and
// stored as audio, for mixing into montages.
//
// People write (or approve) the script; a background job has the
// organisation's text-to-speech model speak it, checks the audio, and
// stores it. Renders reference a ready voiceover by ID.
package voiceover

import (
	"context"
	"database/sql"
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

// Statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusReady   = "ready"
	StatusFailed  = "failed"
)

const (
	maxList        = 100
	downloadURLTTL = time.Hour
)

// Errors returned to callers.
var (
	ErrNotFound  = account.ErrNotFound
	ErrForbidden = account.ErrForbidden
)

// Voiceover is a voiceover as seen by a caller.
type Voiceover struct {
	ID            string
	ProjectID     string
	Script        string
	Voice         string
	Status        string
	Provider      string
	Model         string
	ModelLabel    string
	PromptVersion string
	DurationMS    int64
	Error         string
	CreatedAt     time.Time
	CompletedAt   time.Time
	CanDelete     bool
	// URL plays the audio, once ready.
	URL          string
	URLExpiresAt time.Time
}

// Service manages voiceovers. Every method authorises the actor itself.
type Service struct {
	db    *bun.DB
	store storage.Store
	onJob func()
}

// NewService returns a Service.
func NewService(db *bun.DB, store storage.Store) *Service { return &Service{db: db, store: store} }

// OnJobQueued registers fn to run after a job is queued.
func (s *Service) OnJobQueued(fn func()) { s.onJob = fn }

func invalid(field, msg string) error { return &account.ValidationError{Field: field, Message: msg} }

func audioKey(orgID, projectID, id string) string {
	return "organisations/" + orgID + "/projects/" + projectID + "/voiceovers/" + id + ".wav"
}

// Create queues a voiceover (editing.manage). The script is sent to the
// organisation's text-to-speech model; the request is audited.
func (s *Service) Create(ctx context.Context, actorID, orgID, projectID, script, voice string) (Voiceover, error) {
	script, err := ai.CleanScript(script)
	if err != nil {
		return Voiceover{}, invalid("script", fmt.Sprintf("Write a script of up to %d characters.", ai.MaxScriptRunes))
	}
	if voice == "" {
		voice = ai.Voices[0].ID
	}
	if !ai.ValidVoiceID(voice) {
		return Voiceover{}, invalid("voice", "Choose one of the offered voices.")
	}
	var id string
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		if !authz.Can(acc.Role, authz.EditingManage) {
			return ErrForbidden
		}
		model, err := ai.Resolve(ctx, tx, orgID, ai.TaskVoiceover)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO voiceovers (organisation_id, project_id, script, voice, provider, model, prompt_version, requested_by_user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			orgID, projectID, script, voice, model.Provider, model.ID, ai.VoiceoverPromptVersion, actorID).Scan(&id); err != nil {
			return fmt.Errorf("insert voiceover: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindVoiceoverSpeak, OrganisationID: orgID, SubjectID: id,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 4}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "voiceover.requested", "voiceover", id,
			map[string]any{"project_id": projectID, "voice": voice, "provider": model.Provider, "model": model.ID})
	})
	if err != nil {
		return Voiceover{}, err
	}
	if s.onJob != nil {
		s.onJob()
	}
	return s.Get(ctx, actorID, orgID, projectID, id)
}

const columns = ` v.id, v.project_id, v.script, v.voice, v.status, v.provider, v.model, v.prompt_version,
	coalesce(v.duration_ms, 0), coalesce(v.error, ''), v.created_at, v.completed_at, coalesce(v.storage_key, '')`

func (s *Service) scan(ctx context.Context, row interface{ Scan(...any) error }, acc account.Access) (Voiceover, error) {
	var v Voiceover
	var completed sql.NullTime
	var key string
	if err := row.Scan(&v.ID, &v.ProjectID, &v.Script, &v.Voice, &v.Status, &v.Provider, &v.Model, &v.PromptVersion,
		&v.DurationMS, &v.Error, &v.CreatedAt, &completed, &key); err != nil {
		return Voiceover{}, err
	}
	v.CompletedAt = completed.Time
	v.CanDelete = authz.Can(acc.Role, authz.EditingManage)
	v.ModelLabel = v.Model
	if m, ok := ai.Lookup(ai.TaskVoiceover, v.Provider, v.Model); ok {
		v.ModelLabel = m.Label
	}
	if v.Status == StatusReady && key != "" {
		url, exp, err := s.store.PresignDownload(ctx, key, downloadURLTTL)
		if err != nil {
			return Voiceover{}, err
		}
		v.URL, v.URLExpiresAt = url, exp
	}
	return v, nil
}

// Get returns one voiceover. Anyone who can see the project can read it.
func (s *Service) Get(ctx context.Context, actorID, orgID, projectID, id string) (Voiceover, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Voiceover{}, err
	}
	if !account.ValidUUID(id) {
		return Voiceover{}, ErrNotFound
	}
	v, err := s.scan(ctx, s.db.QueryRowContext(ctx, `SELECT`+columns+` FROM voiceovers v
		WHERE v.id = ? AND v.project_id = ? AND v.organisation_id = ?`, id, projectID, orgID), acc)
	if errors.Is(err, sql.ErrNoRows) {
		return Voiceover{}, ErrNotFound
	}
	return v, err
}

// List returns a project's most recent voiceovers, newest first.
func (s *Service) List(ctx context.Context, actorID, orgID, projectID string) ([]Voiceover, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT`+columns+` FROM voiceovers v
		WHERE v.project_id = ? AND v.organisation_id = ? ORDER BY v.created_at DESC, v.id DESC LIMIT ?`, projectID, orgID, maxList)
	if err != nil {
		return nil, fmt.Errorf("list voiceovers: %w", err)
	}
	defer rows.Close()
	out := []Voiceover{}
	for rows.Next() {
		v, err := s.scan(ctx, rows, acc)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Delete removes a voiceover and (via a trigger) its audio
// (editing.manage). Renders already made keep their mixed sound.
func (s *Service) Delete(ctx context.Context, actorID, orgID, projectID, id string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		if !account.ValidUUID(id) {
			return ErrNotFound
		}
		if !authz.Can(acc.Role, authz.EditingManage) {
			return ErrForbidden
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM voiceovers WHERE id = ? AND project_id = ? AND organisation_id = ?`, id, projectID, orgID)
		if err != nil {
			return fmt.Errorf("delete voiceover: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return account.Audit(ctx, tx, orgID, actorID, "voiceover.deleted", "voiceover", id, map[string]any{"project_id": projectID})
	})
}

// Ready is a ready voiceover as rendering needs it.
type Ready struct {
	StorageKey string
	DurationMS int64
	Script     string
	// Words are the script's words timed against the audio, for captions.
	Words []ai.Word
}

type wordJSON struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// Load returns a ready voiceover in the project. Callers must already have
// authorised access to the project. Anything else is ErrNotFound.
func Load(ctx context.Context, db bun.IDB, orgID, projectID, id string) (Ready, error) {
	if !account.ValidUUID(id) {
		return Ready{}, ErrNotFound
	}
	var r Ready
	var words []byte
	err := db.QueryRowContext(ctx, `
		SELECT storage_key, duration_ms, script, words FROM voiceovers
		WHERE id = ? AND project_id = ? AND organisation_id = ? AND status = 'ready'`, id, projectID, orgID).
		Scan(&r.StorageKey, &r.DurationMS, &r.Script, &words)
	if errors.Is(err, sql.ErrNoRows) {
		return Ready{}, ErrNotFound
	}
	if err != nil {
		return Ready{}, err
	}
	var ws []wordJSON
	if err := json.Unmarshal(words, &ws); err != nil {
		return Ready{}, fmt.Errorf("voiceover words: %w", err)
	}
	for _, w := range ws {
		r.Words = append(r.Words, ai.Word{StartMS: w.StartMS, EndMS: w.EndMS, Text: w.Text})
	}
	return r, nil
}

// TimeWords times the script's words against its speech. The audio is
// transcribed (by timer, with the transcription model) for word start
// times, which are mapped onto the script's own words, so captions show
// exactly what was written. When the transcript doesn't line up with the
// script, times are spread across the speech by word length instead.
func TimeWords(ctx context.Context, timer ai.Transcriber, script string, s ai.Speech) ([]ai.Word, ai.Usage) {
	words := strings.Fields(script)
	dur := s.DurationMS()
	var usage ai.Usage
	var heard []ai.Word
	if timer != nil {
		tr, u, err := timer.Transcribe(ctx, ai.Default(ai.TaskTranscription), s.WAV(), "audio/wav", dur)
		usage = u
		if err == nil {
			for _, seg := range tr.Segments {
				heard = append(heard, seg.Words...)
			}
		}
	}
	out := make([]ai.Word, len(words))
	n, m := len(words), len(heard)
	if m > 0 && math.Abs(float64(m-n)) <= math.Max(2, float64(n)/5) {
		for i, w := range words {
			j := 0
			if n > 1 {
				j = int(math.Round(float64(i) * float64(m-1) / float64(n-1)))
			}
			out[i] = ai.Word{StartMS: heard[j].StartMS, Text: w}
		}
	} else {
		total := 0
		for _, w := range words {
			total += len(w) + 1
		}
		at := 0
		for i, w := range words {
			out[i] = ai.Word{StartMS: int64(at) * dur / int64(total), Text: w}
			at += len(w) + 1
		}
	}
	for i := range out {
		if i > 0 && out[i].StartMS < out[i-1].StartMS {
			out[i].StartMS = out[i-1].StartMS
		}
		out[i].StartMS = min(out[i].StartMS, dur)
	}
	for i := range out {
		out[i].EndMS = dur
		if i+1 < len(out) {
			out[i].EndMS = max(out[i].StartMS, out[i+1].StartMS)
		}
	}
	return out, usage
}

// Narrator handles voiceover.speak jobs in the media worker.
type Narrator struct {
	db     *bun.DB
	store  storage.Store
	ai     ai.Narrator
	timer  ai.Transcriber
	logger *slog.Logger
}

// NewNarrator returns a voiceover job handler. timer times the spoken
// words for captions; nil spreads them by length.
func NewNarrator(db *bun.DB, store storage.Store, provider ai.Narrator, timer ai.Transcriber, logger *slog.Logger) *Narrator {
	return &Narrator{db: db, store: store, ai: provider, timer: timer, logger: logger}
}

var _ jobs.Handler = (*Narrator)(nil)

// User-facing failure messages.
const (
	msgRejected = "The AI provider wouldn’t speak this script. Try rewording it."
	msgGaveUp   = "Making the voiceover kept failing, so we stopped trying. Try again."
)

// Process implements jobs.Handler. It is idempotent: a rerun speaks again
// to the same key and records the result once.
func (n *Narrator) Process(ctx context.Context, job jobs.Job) error {
	var projectID, script, voice, provider, modelID string
	err := n.db.QueryRowContext(ctx, `
		SELECT project_id, script, voice, provider, model FROM voiceovers
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		job.SubjectID, job.OrganisationID).Scan(&projectID, &script, &voice, &provider, &modelID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted, or done; nothing to do
	}
	if err != nil {
		return fmt.Errorf("load voiceover: %w", err)
	}
	model, ok := ai.Lookup(ai.TaskVoiceover, provider, modelID)
	if !ok {
		return jobs.Permanent(fmt.Errorf("%w: model %s/%s is no longer supported", ai.ErrPermanent, provider, modelID))
	}
	if _, err := n.db.ExecContext(ctx, `UPDATE voiceovers SET status = 'running', updated_at = now() WHERE id = ?`, job.SubjectID); err != nil {
		return err
	}
	speech, usage, err := n.ai.Speak(ctx, model, script, voice)
	n.recordUsage(ctx, job, model, usage, err == nil)
	if err != nil {
		if errors.Is(err, ai.ErrPermanent) {
			return jobs.Permanent(err)
		}
		return err
	}
	words, timing := TimeWords(ctx, n.timer, script, speech)
	if n.timer != nil {
		tm := ai.Default(ai.TaskTranscription)
		n.recordUsageFor(ctx, job, ai.TaskTranscription, tm, timing, true)
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
	key := audioKey(job.OrganisationID, projectID, job.SubjectID)
	if err := n.store.Put(ctx, key, "audio/wav", wav); err != nil {
		return err
	}
	res, err := n.db.ExecContext(ctx, `
		UPDATE voiceovers SET status = 'ready', storage_key = ?, duration_ms = ?, size_bytes = ?, words = ?::jsonb,
			error = NULL, completed_at = now(), updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`,
		key, speech.DurationMS(), len(wav), string(wordsJSON), job.SubjectID, job.OrganisationID)
	if err != nil {
		return fmt.Errorf("save voiceover: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		// Deleted while we worked: don't leave the audio behind.
		_, _ = n.db.ExecContext(ctx, `INSERT INTO storage_deletions (storage_key) VALUES (?)`, key)
	}
	return nil
}

func (n *Narrator) recordUsage(ctx context.Context, job jobs.Job, model ai.Model, u ai.Usage, ok bool) {
	n.recordUsageFor(ctx, job, ai.TaskVoiceover, model, u, ok)
}

func (n *Narrator) recordUsageFor(ctx context.Context, job jobs.Job, task string, model ai.Model, u ai.Usage, ok bool) {
	if _, err := n.db.ExecContext(ctx, `
		INSERT INTO ai_usage (organisation_id, job_id, task, provider, model, input_tokens, output_tokens, succeeded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		job.OrganisationID, job.ID, task, model.Provider, model.ID, u.InputTokens, u.OutputTokens, ok); err != nil {
		n.logger.WarnContext(ctx, "record ai usage failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
	}
}

// Failed implements jobs.Handler.
func (n *Narrator) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgGaveUp
	if errors.Is(err, ai.ErrPermanent) {
		msg = msgRejected
	}
	_, dbErr := n.db.ExecContext(ctx, `
		UPDATE voiceovers SET status = 'failed', error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status IN ('queued', 'running')`, msg, job.SubjectID, job.OrganisationID)
	return dbErr
}

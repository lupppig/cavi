package render

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/brief"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/media"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Render statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusReady   = "ready"
	StatusFailed  = "failed"
)

// Limits.
const (
	MaxNameRunes    = 120
	DefaultPageSize = 50
	MaxPageSize     = 100
	downloadURLTTL  = time.Hour
)

// Errors returned to callers.
var (
	ErrNotFound  = account.ErrNotFound
	ErrForbidden = account.ErrForbidden
)

// Render is a clip as seen by a caller.
type Render struct {
	ID              string
	ProjectID       string
	SourceAssetID   string // empty once the source is deleted
	Name            string
	Composition     Composition
	Status          string
	Error           string
	RequestedByID   string
	RequestedByName string
	CreatedAt       time.Time
	CompletedAt     time.Time
	CanDelete       bool
	// Flags are the creator brief's rules (other than priorities) the clip
	// touches, from the brief as it is now.
	Flags []ai.BriefRule
	// Output, once ready.
	URL          string
	URLExpiresAt time.Time
	SizeBytes    int64
	DurationMS   int64
	Width        int
	Height       int
}

// Page is one page of renders, newest first.
type Page struct {
	Renders    []Render
	NextCursor string
}

// Service manages renders. Every method authorises the actor itself.
type Service struct {
	db     *bun.DB
	store  storage.Store
	logger *slog.Logger
	onJob  func()
}

// NewService returns a Service.
func NewService(db *bun.DB, store storage.Store, logger *slog.Logger) *Service {
	return &Service{db: db, store: store, logger: logger}
}

// OnJobQueued registers fn to run after a render job is queued (typically
// jobs.Relay.Wake).
func (s *Service) OnJobQueued(fn func()) { s.onJob = fn }

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// outputKey makes tenant ownership explicit in the object path.
func outputKey(orgID, projectID, renderID string) string {
	return "organisations/" + orgID + "/projects/" + projectID + "/renders/" + renderID + "/clip.mp4"
}

// cleanName trims a clip name and drops control characters.
func cleanName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	return strings.Join(strings.Fields(name), " ")
}

// Filename is the download name for a render: its name, made safe for
// file systems, as MP4.
func Filename(name string) string {
	f := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
	if f = strings.Trim(f, " ."); f == "" {
		f = "clip"
	}
	return f + ".mp4"
}

// Create queues a render of composition (render.execute). The source must
// be a processed video in the same project. A non-empty captionStyle
// burns in captions made from the source's transcript, copied into the
// composition now so the render can be repeated exactly. captions is nil
// for none.
func (s *Service) Create(ctx context.Context, actorID, orgID, projectID, name string, c Composition, captions *CaptionOptions) (Render, error) {
	name = cleanName(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxNameRunes {
		return Render{}, invalid("name", fmt.Sprintf("Give the clip a name of up to %d characters.", MaxNameRunes))
	}
	id := newID()
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, true)
		if err != nil {
			return err
		}
		if !authz.Can(acc.Role, authz.RenderExecute) {
			return ErrForbidden
		}
		src, _, err := source(ctx, tx, orgID, projectID, c.SourceAssetID)
		if err != nil {
			return err
		}
		c.Captions = nil // cues only ever come from the transcript, below
		if captions != nil {
			opts, err := captions.Normalize()
			if err != nil {
				return err
			}
			segs, err := transcriptSegments(ctx, tx, orgID, c.SourceAssetID)
			if err != nil {
				return err
			}
			if segs == nil {
				return invalid("captions", "Transcribe this file first to add captions.")
			}
			c.Version = CompositionVersion
			c.Captions = &Captions{CaptionOptions: opts, Cues: BuildCues(segs, c.StartMS, c.EndMS, c.Aspect, opts.Mode)}
		}
		if err := c.Validate(src); err != nil {
			return err
		}
		rules, err := brief.Rules(ctx, tx, orgID, c.SourceAssetID)
		if err != nil {
			return err
		}
		if r, ok := brief.Excluded(rules, c.StartMS, c.EndMS); ok {
			return brief.ExcludedError("composition.start_ms", r)
		}
		spec, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO renders (id, organisation_id, project_id, source_asset_id, name, composition, composition_version, requested_by_user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, orgID, projectID, c.SourceAssetID, name, string(spec), c.Version, actorID); err != nil {
			return fmt.Errorf("insert render: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindRenderClip, OrganisationID: orgID, RenderID: id,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 3}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "render.requested", "render", id,
			map[string]any{"source_asset_id": c.SourceAssetID, "aspect": c.Aspect, "duration_ms": c.EndMS - c.StartMS, "captions": captions != nil})
	})
	if err != nil {
		return Render{}, err
	}
	if s.onJob != nil {
		s.onJob()
	}
	return s.Get(ctx, actorID, orgID, projectID, id)
}

// errBadSource means the source isn't (or is no longer) a processed video
// in the project.
var errBadSource = invalid("composition.source_asset_id", "Choose a processed video from this project.")

// source loads a render source and its storage key, locking the row
// against deletion for the transaction. Anything but a processed video in
// the project is errBadSource, so IDs from other projects or tenants
// reveal nothing.
func source(ctx context.Context, db bun.IDB, orgID, projectID, assetID string) (Source, string, error) {
	if !account.ValidUUID(assetID) {
		return Source{}, "", errBadSource
	}
	var src Source
	var kind, status, processing, audio, key string
	err := db.QueryRowContext(ctx, `
		SELECT coalesce(kind, ''), status, processing_status, coalesce(width, 0), coalesce(height, 0),
			coalesce(duration_ms, 0), coalesce(audio_codec, ''), storage_key
		FROM media_assets WHERE id = ? AND project_id = ? AND organisation_id = ? FOR SHARE`, assetID, projectID, orgID).
		Scan(&kind, &status, &processing, &src.Width, &src.Height, &src.DurationMS, &audio, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, "", errBadSource
	}
	if err != nil {
		return Source{}, "", fmt.Errorf("load render source: %w", err)
	}
	if kind != "video" || status != "uploaded" || processing != "ready" {
		return Source{}, "", errBadSource
	}
	src.HasAudio = audio != ""
	return src, key, nil
}

// transcriptSegments returns the file's ready transcript in order (empty
// if no speech was found), or nil if it has no ready transcript.
func transcriptSegments(ctx context.Context, db bun.IDB, orgID, assetID string) ([]ai.Segment, error) {
	var ready bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM transcripts WHERE media_asset_id = ? AND organisation_id = ? AND status = 'ready')`,
		assetID, orgID).Scan(&ready); err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	if !ready {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT s.start_ms, s.end_ms, s.speaker, s.text, s.words
		FROM transcripts tr JOIN transcript_segments s ON s.transcript_id = tr.id
		WHERE tr.media_asset_id = ? AND tr.organisation_id = ? AND tr.status = 'ready'
		ORDER BY s.position`, assetID, orgID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	defer rows.Close()
	segs := []ai.Segment{}
	for rows.Next() {
		var sg ai.Segment
		var words []byte
		if err := rows.Scan(&sg.StartMS, &sg.EndMS, &sg.Speaker, &sg.Text, &words); err != nil {
			return nil, err
		}
		sg.Words = media.DecodeWords(words)
		segs = append(segs, sg)
	}
	return segs, rows.Err()
}

const renderColumns = `
	r.id, r.project_id, coalesce(r.source_asset_id::text, ''), r.name, r.composition, r.status, coalesce(r.error, ''),
	coalesce(r.requested_by_user_id::text, ''),
	coalesce(nullif(trim(coalesce(u.first_name, '') || ' ' || coalesce(u.last_name, '')), ''), u.email, ''),
	r.created_at, r.completed_at, coalesce(r.output_key, ''), coalesce(r.output_size_bytes, 0),
	coalesce(r.output_duration_ms, 0), coalesce(r.output_width, 0), coalesce(r.output_height, 0)`

const renderFrom = ` FROM renders r LEFT JOIN users u ON u.id = r.requested_by_user_id`

type scanner interface{ Scan(...any) error }

func (s *Service) scan(ctx context.Context, row scanner, acc account.Access) (Render, error) {
	var r Render
	var spec []byte
	var completed sql.NullTime
	var key string
	if err := row.Scan(&r.ID, &r.ProjectID, &r.SourceAssetID, &r.Name, &spec, &r.Status, &r.Error,
		&r.RequestedByID, &r.RequestedByName, &r.CreatedAt, &completed, &key, &r.SizeBytes,
		&r.DurationMS, &r.Width, &r.Height); err != nil {
		return Render{}, err
	}
	if err := json.Unmarshal(spec, &r.Composition); err != nil {
		return Render{}, fmt.Errorf("render %s composition: %w", r.ID, err)
	}
	r.CompletedAt = completed.Time
	r.CanDelete = authz.Can(acc.Role, authz.RenderExecute)
	if r.Status == StatusReady && key != "" {
		url, exp, err := s.store.PresignDownload(ctx, key, downloadURLTTL)
		if err != nil {
			return Render{}, err
		}
		r.URL, r.URLExpiresAt = url, exp
	}
	return r, nil
}

// Get returns one render. Anyone who can see the project can read it.
func (s *Service) Get(ctx context.Context, actorID, orgID, projectID, renderID string) (Render, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Render{}, err
	}
	if !account.ValidUUID(renderID) {
		return Render{}, ErrNotFound
	}
	r, err := s.scan(ctx, s.db.QueryRowContext(ctx, `SELECT`+renderColumns+renderFrom+`
		WHERE r.id = ? AND r.project_id = ? AND r.organisation_id = ?`, renderID, projectID, orgID), acc)
	if errors.Is(err, sql.ErrNoRows) {
		return Render{}, ErrNotFound
	}
	if err != nil {
		return Render{}, err
	}
	err = s.flag(ctx, orgID, []*Render{&r})
	return r, err
}

// flag sets each render's Flags from its source's brief rules.
func (s *Service) flag(ctx context.Context, orgID string, rs []*Render) error {
	rules := map[string][]ai.BriefRule{}
	for _, r := range rs {
		if r.SourceAssetID == "" {
			continue
		}
		rr, ok := rules[r.SourceAssetID]
		if !ok {
			var err error
			if rr, err = brief.Rules(ctx, s.db, orgID, r.SourceAssetID); err != nil {
				return err
			}
			rules[r.SourceAssetID] = rr
		}
		r.Flags = brief.Flags(rr, r.Composition.StartMS, r.Composition.EndMS)
	}
	return nil
}

// List returns a project's renders, newest first, optionally only those
// made from one source file.
func (s *Service) List(ctx context.Context, actorID, orgID, projectID, sourceAssetID, cursor string, limit int) (Page, error) {
	switch {
	case limit == 0:
		limit = DefaultPageSize
	case limit < 1 || limit > MaxPageSize:
		return Page{}, invalid("limit", fmt.Sprintf("Limit must be between 1 and %d.", MaxPageSize))
	}
	if sourceAssetID != "" && !account.ValidUUID(sourceAssetID) {
		return Page{}, invalid("source_asset_id", "That isn’t a media file ID.")
	}
	var after time.Time
	var afterID string
	if cursor != "" {
		var err error
		if after, afterID, err = decodeCursor(cursor); err != nil {
			return Page{}, invalid("cursor", "The page cursor is invalid. Start again from the first page.")
		}
	}
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT`+renderColumns+renderFrom+`
		WHERE r.project_id = ? AND r.organisation_id = ?
			AND (? = '' OR r.source_asset_id::text = ?)
			AND (? = '' OR (r.created_at, r.id) < (?, ?::uuid))
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT ?`, projectID, orgID, sourceAssetID, sourceAssetID, afterID, after, nullUUID(afterID), limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list renders: %w", err)
	}
	defer rows.Close()
	page := Page{Renders: []Render{}}
	for rows.Next() {
		r, err := s.scan(ctx, rows, acc)
		if err != nil {
			return Page{}, fmt.Errorf("scan render: %w", err)
		}
		page.Renders = append(page.Renders, r)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Renders) > limit {
		page.Renders = page.Renders[:limit]
		last := page.Renders[limit-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	ptrs := make([]*Render, len(page.Renders))
	for i := range page.Renders {
		ptrs[i] = &page.Renders[i]
	}
	if err := s.flag(ctx, orgID, ptrs); err != nil {
		return Page{}, err
	}
	return page, nil
}

func nullUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(c string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok || !account.ValidUUID(id) {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	return t, id, err
}

// Delete removes a render and its output (render.execute). A render in
// progress is cancelled: its job goes with it.
func (s *Service) Delete(ctx context.Context, actorID, orgID, projectID, renderID string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		if !account.ValidUUID(renderID) {
			return ErrNotFound
		}
		var name string
		err = tx.QueryRowContext(ctx, `
			SELECT name FROM renders WHERE id = ? AND project_id = ? AND organisation_id = ? FOR UPDATE`,
			renderID, projectID, orgID).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !authz.Can(acc.Role, authz.RenderExecute) {
			return ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM renders WHERE id = ?`, renderID); err != nil {
			return fmt.Errorf("delete render: %w", err)
		}
		return account.Audit(ctx, tx, orgID, actorID, "render.deleted", "render", renderID, map[string]any{"name": name})
	})
}

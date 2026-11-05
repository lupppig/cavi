// Package media owns media assets: files uploaded to a project.
//
// Upload flow (bytes never pass through the API):
//  1. CreateUpload checks permissions, records an 'uploading' asset and
//     returns a short-lived signed URL.
//  2. The browser PUTs the file straight to object storage.
//  3. Complete checks the stored object: its size must match and its first
//     bytes must be a supported video, audio or image format. Passing
//     files become 'uploaded'; failing ones are deleted.
//
// Anyone who can see a project can see and download its media. Uploading
// needs media.upload; deleting needs media.delete, except that people may
// delete their own uploads.
//
// Objects are never deleted inline. Deleting a media row (directly or via
// its project or organisation) queues the object in storage_deletions, and
// Sweep removes it, retrying on failure.
package media

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Errors shared with package account so the HTTP layer maps them once.
var (
	ErrNotFound  = account.ErrNotFound
	ErrForbidden = account.ErrForbidden
	// ErrNotReceived means the browser hasn't finished uploading the file.
	ErrNotReceived = errors.New("media: file not received")
)

// RejectedError means an uploaded file failed checks and was discarded.
type RejectedError struct{ Reason string }

func (e *RejectedError) Error() string { return "media: upload rejected: " + e.Reason }

// Limits and lifetimes.
const (
	// MaxUploadBytes is the largest single upload (the S3 single-PUT limit).
	MaxUploadBytes   int64 = 5 << 30
	MaxFilenameRunes       = 255
	DefaultPageSize        = 50
	MaxPageSize            = 100
	uploadURLTTL           = 15 * time.Minute
	downloadURLTTL         = time.Hour
	// abandonAfter is when an upload that was never completed is removed.
	abandonAfter = 24 * time.Hour
)

// acceptedTypes are the content types browsers may declare. The real type
// is decided by sniffing the stored bytes, not by this.
var acceptedTypes = map[string]bool{
	"video/mp4": true, "video/quicktime": true, "video/webm": true, "video/x-matroska": true, "video/x-msvideo": true,
	"audio/mpeg": true, "audio/mp4": true, "audio/x-m4a": true, "audio/wav": true, "audio/x-wav": true, "audio/wave": true,
	"audio/aac": true, "audio/flac": true, "audio/x-flac": true, "audio/ogg": true,
	"image/png": true, "image/jpeg": true, "image/webp": true,
	// Some browsers don't recognise .mkv and similar.
	"application/octet-stream": true,
}

// Asset is a media file as seen by one caller.
type Asset struct {
	ID             string
	OrganisationID string
	ProjectID      string
	Filename       string
	Kind           string
	ContentType    string
	SizeBytes      int64
	Status         string
	UploadedByID   string
	UploadedByName string
	CreatedAt      time.Time
	UploadedAt     time.Time
	CanDelete      bool
	// URL is a signed, short-lived link to the file (uploaded assets only).
	URL          string
	URLExpiresAt time.Time

	// Processing state and what it learned (zero until ready).
	ProcessingStatus string
	ProcessingError  string
	Container        string
	DurationMS       int64
	Width, Height    int
	FrameRate        float64
	VideoCodec       string
	AudioCodec       string
	AudioChannels    int
	SampleRate       int
	// ThumbnailURL is a signed link to a JPEG preview, when there is one.
	ThumbnailURL string

	thumbnailKey string
}

// Page is one page of assets, newest first.
type Page struct {
	Assets     []Asset
	NextCursor string
}

// Service manages media assets.
type Service struct {
	db     *bun.DB
	store  storage.Store
	logger *slog.Logger
	// onJob is told when a job was queued, to publish it promptly.
	onJob func()
}

// OnJobQueued registers fn to run after a transaction queues a job
// (typically jobs.Relay.Wake). Without it, the relay's own polling picks
// jobs up.
func (s *Service) OnJobQueued(fn func()) { s.onJob = fn }

func (s *Service) jobQueued() {
	if s.onJob != nil {
		s.onJob()
	}
}

// NewService returns a media service.
func NewService(db *bun.DB, store storage.Store, logger *slog.Logger) *Service {
	return &Service{db: db, store: store, logger: logger}
}

func invalid(field, msg string) error { return &account.ValidationError{Field: field, Message: msg} }

// cleanFilename keeps only the base name, drops control characters and
// trims it to a sensible length. It is used for display and downloads only;
// storage keys never contain it.
func cleanFilename(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == "/" || name == ".." {
		name = ""
	}
	if n := len([]rune(name)); n < 1 || n > MaxFilenameRunes {
		return "", invalid("filename", fmt.Sprintf("File names must be between 1 and %d characters.", MaxFilenameRunes))
	}
	return name, nil
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// storageKey makes tenant ownership explicit in the object path. It is
// not an authorisation mechanism: access always goes through this package.
func storageKey(orgID, projectID, assetID string) string {
	return "organisations/" + orgID + "/projects/" + projectID + "/assets/" + assetID + "/original"
}

func canUpload(a account.Access) bool { return authz.Can(a.Role, authz.MediaUpload) }

func canDelete(a account.Access, actorID, uploadedBy string) bool {
	return authz.Can(a.Role, authz.MediaDelete) || (canUpload(a) && uploadedBy == actorID)
}

// CreateUpload starts an upload and returns the asset and the signed
// instruction the browser follows to send the bytes.
func (s *Service) CreateUpload(ctx context.Context, actorID, orgID, projectID, filename, contentType string, size int64) (Asset, storage.Upload, error) {
	filename, err := cleanFilename(filename)
	if err != nil {
		return Asset{}, storage.Upload{}, err
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !acceptedTypes[contentType] {
		return Asset{}, storage.Upload{}, invalid("content_type", "CAVI accepts video, audio, PNG, JPEG and WebP files.")
	}
	if size < 1 || size > MaxUploadBytes {
		return Asset{}, storage.Upload{}, invalid("size_bytes", "Files must be between 1 byte and 5 GB.")
	}
	a, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Asset{}, storage.Upload{}, err
	}
	if !canUpload(a) {
		return Asset{}, storage.Upload{}, ErrForbidden
	}

	id := newID()
	key := storageKey(orgID, projectID, id)
	up, err := s.store.PresignUpload(ctx, key, contentType, filename, size, uploadURLTTL)
	if err != nil {
		return Asset{}, storage.Upload{}, err
	}
	var asset Asset
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Re-check inside the transaction and lock the project so it can't be
		// deleted between the check and the insert.
		if _, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, true); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_assets (id, organisation_id, project_id, storage_key, filename, content_type, size_bytes, uploaded_by_user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, orgID, projectID, key, filename, contentType, size, actorID); err != nil {
			return fmt.Errorf("insert media asset: %w", err)
		}
		asset, err = s.load(ctx, tx, a, actorID, orgID, projectID, id, false)
		return err
	})
	if err != nil {
		return Asset{}, storage.Upload{}, err
	}
	return asset, up, nil
}

const assetColumns = `
	m.id, m.organisation_id, m.project_id, m.filename, coalesce(m.kind, ''), m.content_type, m.size_bytes, m.status,
	coalesce(m.uploaded_by_user_id::text, ''),
	coalesce(nullif(trim(coalesce(u.first_name, '') || ' ' || coalesce(u.last_name, '')), ''), u.email, ''),
	m.created_at, coalesce(m.uploaded_at, m.created_at), m.storage_key,
	m.processing_status, coalesce(m.processing_error, ''), coalesce(m.container, ''), coalesce(m.duration_ms, 0),
	coalesce(m.width, 0), coalesce(m.height, 0), coalesce(m.frame_rate, 0)::float8, coalesce(m.video_codec, ''),
	coalesce(m.audio_codec, ''), coalesce(m.audio_channels, 0), coalesce(m.sample_rate, 0), coalesce(m.thumbnail_key, '')`

const assetFrom = `
	FROM media_assets m LEFT JOIN users u ON u.id = m.uploaded_by_user_id`

type scanner interface{ Scan(...any) error }

func scanAsset(row scanner) (Asset, string, error) {
	var a Asset
	var key string
	err := row.Scan(&a.ID, &a.OrganisationID, &a.ProjectID, &a.Filename, &a.Kind, &a.ContentType, &a.SizeBytes,
		&a.Status, &a.UploadedByID, &a.UploadedByName, &a.CreatedAt, &a.UploadedAt, &key,
		&a.ProcessingStatus, &a.ProcessingError, &a.Container, &a.DurationMS,
		&a.Width, &a.Height, &a.FrameRate, &a.VideoCodec,
		&a.AudioCodec, &a.AudioChannels, &a.SampleRate, &a.thumbnailKey)
	return a, key, err
}

// load returns an asset in a project the caller can already see.
func (s *Service) load(ctx context.Context, db bun.IDB, acc account.Access, actorID, orgID, projectID, assetID string, forUpdate bool) (Asset, error) {
	a, _, err := s.loadWithKey(ctx, db, acc, actorID, orgID, projectID, assetID, forUpdate)
	return a, err
}

func (s *Service) loadWithKey(ctx context.Context, db bun.IDB, acc account.Access, actorID, orgID, projectID, assetID string, forUpdate bool) (Asset, string, error) {
	if !account.ValidUUID(assetID) {
		return Asset{}, "", ErrNotFound
	}
	q := `SELECT` + assetColumns + assetFrom + ` WHERE m.id = ? AND m.project_id = ? AND m.organisation_id = ?`
	if forUpdate {
		q += " FOR UPDATE OF m"
	}
	a, key, err := scanAsset(db.QueryRowContext(ctx, q, assetID, projectID, orgID))
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, "", ErrNotFound
	}
	if err != nil {
		return Asset{}, "", fmt.Errorf("load media asset: %w", err)
	}
	a.CanDelete = canDelete(acc, actorID, a.UploadedByID)
	return a, key, nil
}

func (s *Service) sign(ctx context.Context, a *Asset, key string) error {
	if a.Status != "uploaded" {
		return nil
	}
	url, exp, err := s.store.PresignDownload(ctx, key, downloadURLTTL)
	if err != nil {
		return err
	}
	a.URL, a.URLExpiresAt = url, exp
	if a.thumbnailKey != "" {
		if a.ThumbnailURL, _, err = s.store.PresignDownload(ctx, a.thumbnailKey, downloadURLTTL); err != nil {
			return err
		}
	}
	return nil
}

// Complete checks an upload the caller started and makes it available.
// Calling it again on an uploaded asset returns it unchanged.
func (s *Service) Complete(ctx context.Context, actorID, orgID, projectID, assetID string) (Asset, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Asset{}, err
	}
	asset, key, err := s.loadWithKey(ctx, s.db, acc, actorID, orgID, projectID, assetID, false)
	if err != nil {
		return Asset{}, err
	}
	if asset.Status == "uploaded" {
		return asset, s.sign(ctx, &asset, key)
	}
	// Only the person uploading can finish their own upload.
	if asset.UploadedByID != actorID || !canUpload(acc) {
		return Asset{}, ErrForbidden
	}

	info, err := s.store.Stat(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return Asset{}, ErrNotReceived
	}
	if err != nil {
		return Asset{}, err
	}
	if info.Size != asset.SizeBytes {
		return Asset{}, s.reject(ctx, asset, "The file that arrived isn’t the size we expected. Please upload it again.")
	}
	head, err := s.store.ReadHead(ctx, key, sniffBytes)
	if err != nil {
		return Asset{}, err
	}
	kind, ok := sniff(head)
	if !ok {
		return Asset{}, s.reject(ctx, asset, "This file isn’t a supported video, audio or image format.")
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE media_assets SET status = 'uploaded', kind = ?, content_type = ?, uploaded_at = now(), updated_at = now()
			WHERE id = ? AND status = 'uploading'`, kind.Kind, kind.ContentType, assetID)
		if err != nil {
			return fmt.Errorf("mark media uploaded: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Completed concurrently, or deleted in the meantime.
			_, err := s.load(ctx, tx, acc, actorID, orgID, projectID, assetID, false)
			return err
		}
		// Processing is queued in the same transaction, so an uploaded file
		// always gets processed.
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaProcess, OrganisationID: orgID,
			MediaAssetID: assetID, CorrelationID: jobs.CorrelationID(ctx)}); err != nil {
			return err
		}
		return account.Audit(ctx, tx, orgID, actorID, "media.uploaded", "media_asset", assetID,
			map[string]any{"project_id": projectID, "kind": kind.Kind, "size_bytes": asset.SizeBytes})
	})
	if err != nil {
		return Asset{}, err
	}
	s.jobQueued()
	asset, key, err = s.loadWithKey(ctx, s.db, acc, actorID, orgID, projectID, assetID, false)
	if err != nil {
		return Asset{}, err
	}
	return asset, s.sign(ctx, &asset, key)
}

// reject discards a failed upload; the trigger queues the object for removal.
func (s *Service) reject(ctx context.Context, a Asset, reason string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM media_assets WHERE id = ? AND status = 'uploading'`, a.ID); err != nil {
		return fmt.Errorf("discard rejected upload: %w", err)
	}
	return &RejectedError{Reason: reason}
}

func encodeCursor(a Asset) string {
	return base64.RawURLEncoding.EncodeToString([]byte(a.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + a.ID))
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

// List returns a project's uploaded media, newest first, each with a
// signed URL.
func (s *Service) List(ctx context.Context, actorID, orgID, projectID, cursor string, limit int) (Page, error) {
	switch {
	case limit == 0:
		limit = DefaultPageSize
	case limit < 1 || limit > MaxPageSize:
		return Page{}, invalid("limit", fmt.Sprintf("Limit must be between 1 and %d.", MaxPageSize))
	}
	var after any
	var afterID any
	if cursor != "" {
		t, id, err := decodeCursor(cursor)
		if err != nil {
			return Page{}, invalid("cursor", "The page cursor is invalid. Start again from the first page.")
		}
		after, afterID = t, id
	}
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT`+assetColumns+assetFrom+`
		WHERE m.project_id = ? AND m.organisation_id = ? AND m.status = 'uploaded'
		AND (?::uuid IS NULL OR (m.created_at, m.id) < (?::timestamptz, ?::uuid))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT ?`, projectID, orgID, afterID, after, afterID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list media: %w", err)
	}
	defer rows.Close()
	page := Page{Assets: []Asset{}}
	var keys []string
	for rows.Next() {
		a, key, err := scanAsset(rows)
		if err != nil {
			return Page{}, fmt.Errorf("scan media: %w", err)
		}
		a.CanDelete = canDelete(acc, actorID, a.UploadedByID)
		page.Assets = append(page.Assets, a)
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Assets) > limit {
		page.Assets, keys = page.Assets[:limit], keys[:limit]
		page.NextCursor = encodeCursor(page.Assets[limit-1])
	}
	for i := range page.Assets {
		if err := s.sign(ctx, &page.Assets[i], keys[i]); err != nil {
			return Page{}, err
		}
	}
	return page, nil
}

// Get returns one uploaded asset with a fresh signed URL.
func (s *Service) Get(ctx context.Context, actorID, orgID, projectID, assetID string) (Asset, error) {
	acc, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Asset{}, err
	}
	a, key, err := s.loadWithKey(ctx, s.db, acc, actorID, orgID, projectID, assetID, false)
	if err != nil {
		return Asset{}, err
	}
	if a.Status != "uploaded" && a.UploadedByID != actorID {
		return Asset{}, ErrNotFound
	}
	return a, s.sign(ctx, &a, key)
}

// ErrProcessingInProgress means the file is already being processed.
var ErrProcessingInProgress = errors.New("media: processing in progress")

// Reprocess queues processing again for a file whose processing failed
// (media.upload). Calling it on a ready file returns it unchanged.
func (s *Service) Reprocess(ctx context.Context, actorID, orgID, projectID, assetID string) (Asset, error) {
	queued := false
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
		if !canUpload(acc) {
			return ErrForbidden
		}
		switch a.ProcessingStatus {
		case ProcessingReady:
			return nil
		case ProcessingPending, ProcessingProcessing:
			return ErrProcessingInProgress
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE media_assets SET processing_status = 'pending', processing_error = NULL, updated_at = now()
			WHERE id = ?`, assetID); err != nil {
			return fmt.Errorf("reset processing: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaProcess, OrganisationID: orgID,
			MediaAssetID: assetID, CorrelationID: jobs.CorrelationID(ctx)}); err != nil {
			return err
		}
		queued = true
		return account.Audit(ctx, tx, orgID, actorID, "media.reprocess_requested", "media_asset", assetID, nil)
	})
	if err != nil {
		return Asset{}, err
	}
	if queued {
		s.jobQueued()
	}
	return s.Get(ctx, actorID, orgID, projectID, assetID)
}

// Delete removes an asset (media.delete, or the uploader's own). The
// stored object is removed in the background.
func (s *Service) Delete(ctx context.Context, actorID, orgID, projectID, assetID string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		acc, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, false)
		if err != nil {
			return err
		}
		a, err := s.load(ctx, tx, acc, actorID, orgID, projectID, assetID, true)
		if err != nil {
			return err
		}
		if a.Status != "uploaded" && a.UploadedByID != actorID {
			return ErrNotFound
		}
		if !a.CanDelete {
			return ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_assets WHERE id = ?`, assetID); err != nil {
			return fmt.Errorf("delete media asset: %w", err)
		}
		return account.Audit(ctx, tx, orgID, actorID, "media.deleted", "media_asset", assetID,
			map[string]any{"project_id": projectID, "filename": a.Filename})
	})
}

// Sweep removes abandoned uploads and deletes queued objects from storage.
// It is safe to run concurrently from several API instances.
func (s *Service) Sweep(ctx context.Context) (abandoned, deleted int, err error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM media_assets WHERE status = 'uploading' AND created_at < ?`, time.Now().Add(-abandonAfter))
	if err != nil {
		return 0, 0, fmt.Errorf("remove abandoned uploads: %w", err)
	}
	n, _ := res.RowsAffected()
	abandoned = int(n)

	for {
		done, err := s.deleteBatch(ctx)
		deleted += done
		if err != nil || done == 0 {
			return abandoned, deleted, err
		}
	}
}

// deleteBatch deletes up to 50 queued objects. Failures are retried later
// with backoff (1 minute doubling, capped at 6 hours).
func (s *Service) deleteBatch(ctx context.Context) (int, error) {
	done := 0
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, storage_key, attempts FROM storage_deletions
			WHERE next_try_at <= now() ORDER BY id LIMIT 50 FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return fmt.Errorf("load storage deletions: %w", err)
		}
		type item struct {
			id       int64
			key      string
			attempts int
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.id, &it.key, &it.attempts); err != nil {
				rows.Close()
				return err
			}
			items = append(items, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, it := range items {
			if delErr := s.store.Delete(ctx, it.key); delErr != nil {
				backoff := min(time.Minute<<min(it.attempts, 9), 6*time.Hour)
				s.logger.WarnContext(ctx, "storage deletion failed", slog.Int64("deletion_id", it.id),
					slog.Int("attempts", it.attempts+1), slog.String("error", delErr.Error()))
				if _, err := tx.ExecContext(ctx, `
					UPDATE storage_deletions SET attempts = attempts + 1, last_error = ?, next_try_at = ?
					WHERE id = ?`, delErr.Error(), time.Now().Add(backoff), it.id); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM storage_deletions WHERE id = ?`, it.id); err != nil {
				return err
			}
			done++
		}
		return nil
	})
	return done, err
}

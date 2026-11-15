package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// Import limits.
const (
	MaxImportURLBytes = 2048
	// MaxImportDuration is the longest video that can be imported.
	MaxImportDuration = 4 * time.Hour
	importTimeout     = time.Hour
)

// Platforms videos can be imported from, by host. Only these hosts are
// accepted, and the downloader is limited to these sites' extractors, so a
// link can't make the worker fetch anything else.
var importHosts = map[string]string{
	"youtube.com": "YouTube", "www.youtube.com": "YouTube", "m.youtube.com": "YouTube", "youtu.be": "YouTube",
	"twitch.tv": "Twitch", "www.twitch.tv": "Twitch", "m.twitch.tv": "Twitch", "clips.twitch.tv": "Twitch",
	"kick.com": "Kick", "www.kick.com": "Kick",
}

// ImportSource checks a pasted link and returns it cleaned up, with its
// platform's name. Only single videos on supported platforms pass.
func ImportSource(raw string) (string, string, error) {
	bad := invalid("url", "Paste a link to a YouTube, Twitch or Kick video.")
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxImportURLBytes || !utf8.ValidString(raw) {
		return "", "", bad
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", "", bad
	}
	host := strings.ToLower(u.Hostname())
	platform, ok := importHosts[host]
	if !ok {
		return "", "", bad
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" || (host == "youtu.be" && strings.Count(path, "/") != 1) {
		return "", "", bad
	}
	if platform == "YouTube" && host != "youtu.be" && path == "/playlist" {
		return "", "", invalid("url", "That’s a playlist. Paste a link to one video.")
	}
	u.Scheme, u.Host, u.Fragment = "https", host, ""
	return u.String(), platform, nil
}

// CreateImport (media.upload) starts importing a video from a link. The
// caller must confirm they have the rights to use it; the confirmation is
// recorded in the audit log. The file appears as 'importing' until the
// worker has downloaded it, then is processed like an upload.
func (s *Service) CreateImport(ctx context.Context, actorID, orgID, projectID, rawURL string, rightsConfirmed bool) (Asset, error) {
	src, platform, err := ImportSource(rawURL)
	if err != nil {
		return Asset{}, err
	}
	if !rightsConfirmed {
		return Asset{}, invalid("rights_confirmed", "Confirm you own this video or have the creator’s permission to clip it.")
	}
	a, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false)
	if err != nil {
		return Asset{}, err
	}
	if !canUpload(a) {
		return Asset{}, ErrForbidden
	}
	id := newID()
	var asset Asset
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, _, err := project.Resolve(ctx, tx, actorID, orgID, projectID, true); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_assets (id, organisation_id, project_id, storage_key, filename, content_type, size_bytes,
				status, source_url, uploaded_by_user_id)
			VALUES (?, ?, ?, ?, ?, 'video/mp4', 0, 'importing', ?, ?)`,
			id, orgID, projectID, storageKey(orgID, projectID, id), platform+" video", src, actorID); err != nil {
			return fmt.Errorf("insert import: %w", err)
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaImport, OrganisationID: orgID, MediaAssetID: id,
			CorrelationID: jobs.CorrelationID(ctx), MaxAttempts: 3}); err != nil {
			return err
		}
		if err := account.Audit(ctx, tx, orgID, actorID, "media.import_requested", "media_asset", id,
			map[string]any{"project_id": projectID, "platform": platform, "url": src, "rights_confirmed": true}); err != nil {
			return err
		}
		asset, err = s.load(ctx, tx, a, actorID, orgID, projectID, id, false)
		return err
	})
	if err != nil {
		return Asset{}, err
	}
	s.jobQueued()
	return asset, nil
}

// Fetched is a video a Fetcher downloaded.
type Fetched struct {
	// Path is the downloaded file, inside the directory given to Fetch.
	Path  string
	Title string
}

// ImportError is a reason a link can't be imported that retrying won't
// change, in words for the user.
type ImportError struct{ Reason string }

func (e *ImportError) Error() string { return "media: import refused: " + e.Reason }

// Fetcher downloads one video from a supported link into dir.
type Fetcher interface {
	Fetch(ctx context.Context, link, dir string) (Fetched, error)
}

// Importer handles media.import jobs: it downloads the linked video,
// checks it like an upload, stores it and queues processing. It runs in
// the media worker.
type Importer struct {
	db     *bun.DB
	store  storage.Store
	fetch  Fetcher
	logger *slog.Logger
	// TempDir is where downloads go (default: the system's).
	TempDir string
}

// NewImporter returns an Importer.
func NewImporter(db *bun.DB, store storage.Store, fetch Fetcher, logger *slog.Logger) *Importer {
	return &Importer{db: db, store: store, fetch: fetch, logger: logger}
}

var _ jobs.Handler = (*Importer)(nil)

// Messages shown when an import gives up.
const (
	msgImportGaveUp   = "Importing kept failing, so we stopped trying. Check the link works, then paste it again."
	msgImportNotVideo = "What came back from that link isn’t a video we can use."
	msgImportTooBig   = "That video is bigger than 5 GB, so it can’t be imported."
)

// Process implements jobs.Handler. Rerunning it downloads again.
func (im *Importer) Process(ctx context.Context, job jobs.Job) error {
	var projectID, key, link, actorID string
	// The asset must belong to the job's organisation; re-check, don't trust.
	err := im.db.QueryRowContext(ctx, `
		SELECT project_id, storage_key, source_url, coalesce(uploaded_by_user_id::text, '') FROM media_assets
		WHERE id = ? AND organisation_id = ? AND status = 'importing'`, job.MediaAssetID, job.OrganisationID).
		Scan(&projectID, &key, &link, &actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted, or already imported
	}
	if err != nil {
		return fmt.Errorf("load import: %w", err)
	}
	// Only supported links are ever stored, but check again.
	if _, _, err := ImportSource(link); err != nil {
		return jobs.Permanent(&ImportError{Reason: "That link isn’t a YouTube, Twitch or Kick video."})
	}
	dir, err := os.MkdirTemp(im.TempDir, "import-")
	if err != nil {
		return fmt.Errorf("import work dir: %w", err)
	}
	defer os.RemoveAll(dir)

	started := time.Now()
	got, err := im.fetch.Fetch(ctx, link, dir)
	var refused *ImportError
	if errors.As(err, &refused) {
		return jobs.Permanent(err)
	}
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(dir, got.Path); err != nil || strings.HasPrefix(rel, "..") {
		return jobs.Permanent(fmt.Errorf("media: download landed outside its directory"))
	}
	info, err := os.Stat(got.Path)
	if err != nil {
		return fmt.Errorf("downloaded file: %w", err)
	}
	if info.Size() > MaxUploadBytes {
		return jobs.Permanent(&ImportError{Reason: msgImportTooBig})
	}
	head, err := readHead(got.Path)
	if err != nil {
		return err
	}
	kind, ok := sniff(head)
	if !ok || kind.Kind != KindVideo || info.Size() == 0 {
		return jobs.Permanent(&ImportError{Reason: msgImportNotVideo})
	}
	filename := importFilename(got.Title, filepath.Ext(got.Path))
	if err := im.store.PutFile(ctx, key, kind.ContentType, filename, got.Path); err != nil {
		return err
	}
	queued := false
	err = im.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE media_assets SET status = 'uploaded', kind = ?, content_type = ?, size_bytes = ?, filename = ?,
				uploaded_at = now(), import_error = NULL, updated_at = now()
			WHERE id = ? AND organisation_id = ? AND status = 'importing'`,
			kind.Kind, kind.ContentType, info.Size(), filename, job.MediaAssetID, job.OrganisationID)
		if err != nil {
			return fmt.Errorf("save import: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Deleted while we downloaded: its object was queued for deletion
			// before we stored the file, so queue it again.
			_, err = tx.ExecContext(ctx, `INSERT INTO storage_deletions (storage_key) VALUES (?)`, key)
			return err
		}
		if _, err := jobs.Enqueue(ctx, tx, jobs.New{Kind: jobs.KindMediaProcess, OrganisationID: job.OrganisationID,
			MediaAssetID: job.MediaAssetID, CorrelationID: job.CorrelationID}); err != nil {
			return err
		}
		queued = true
		return account.Audit(ctx, tx, job.OrganisationID, actorID, "media.imported", "media_asset", job.MediaAssetID,
			map[string]any{"project_id": projectID, "size_bytes": info.Size()})
	})
	if err != nil {
		return err
	}
	if queued {
		im.logger.InfoContext(ctx, "media imported", slog.String("media_asset_id", job.MediaAssetID),
			slog.Int64("bytes", info.Size()), slog.Duration("took", time.Since(started)))
	}
	return nil
}

// Failed implements jobs.Handler: it tells the user why importing stopped.
func (im *Importer) Failed(ctx context.Context, job jobs.Job, err error) error {
	msg := msgImportGaveUp
	var refused *ImportError
	if errors.As(err, &refused) {
		msg = refused.Reason
	}
	_, dbErr := im.db.ExecContext(ctx, `
		UPDATE media_assets SET import_error = ?, updated_at = now()
		WHERE id = ? AND organisation_id = ? AND status = 'importing'`, msg, job.MediaAssetID, job.OrganisationID)
	return dbErr
}

func readHead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := make([]byte, sniffBytes)
	n, err := io.ReadFull(f, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:n], nil
}

// importFilename names an imported file after the video's title.
func importFilename(title, ext string) string {
	title = strings.Join(strings.Fields(strings.ReplaceAll(title, "/", "-")), " ")
	if ext == "" || len(ext) > 6 {
		ext = ".mp4"
	}
	if r := []rune(title); len(r) > MaxFilenameRunes-len(ext) {
		title = strings.TrimSpace(string(r[:MaxFilenameRunes-len(ext)]))
	}
	name, err := cleanFilename(title + ext)
	if err != nil || strings.TrimSuffix(name, ext) == "" {
		return "Imported video" + ext
	}
	return name
}

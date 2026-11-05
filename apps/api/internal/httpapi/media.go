package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/media"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// MediaService is the media behaviour the handlers need. Every method
// authorises the actor itself.
type MediaService interface {
	CreateUpload(ctx context.Context, actorID, orgID, projectID, filename, contentType string, size int64) (media.Asset, storage.Upload, error)
	Complete(ctx context.Context, actorID, orgID, projectID, assetID string) (media.Asset, error)
	List(ctx context.Context, actorID, orgID, projectID, cursor string, limit int) (media.Page, error)
	Get(ctx context.Context, actorID, orgID, projectID, assetID string) (media.Asset, error)
	Delete(ctx context.Context, actorID, orgID, projectID, assetID string) error
}

type mediaHandler struct {
	svc MediaService
	orgHandler
}

type assetResponse struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"project_id"`
	Filename     string   `json:"filename"`
	Kind         *string  `json:"kind"`
	ContentType  string   `json:"content_type"`
	SizeBytes    int64    `json:"size_bytes"`
	Status       string   `json:"status"`
	UploadedBy   *userRef `json:"uploaded_by"`
	CreatedAt    string   `json:"created_at"`
	UploadedAt   *string  `json:"uploaded_at"`
	CanDelete    bool     `json:"can_delete"`
	URL          *string  `json:"url"`
	URLExpiresAt *string  `json:"url_expires_at"`
}

type uploadInstruction struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt string            `json:"expires_at"`
}

type uploadResponse struct {
	Asset  assetResponse     `json:"asset"`
	Upload uploadInstruction `json:"upload"`
}

type assetPage struct {
	Data       []assetResponse `json:"data"`
	NextCursor *string         `json:"next_cursor"`
}

func toAssetResponse(a media.Asset) assetResponse {
	r := assetResponse{
		ID: a.ID, ProjectID: a.ProjectID, Filename: a.Filename, ContentType: a.ContentType,
		SizeBytes: a.SizeBytes, Status: a.Status, CreatedAt: ts(a.CreatedAt), CanDelete: a.CanDelete,
	}
	if a.Kind != "" {
		r.Kind = &a.Kind
	}
	if a.UploadedByID != "" {
		r.UploadedBy = &userRef{ID: a.UploadedByID, Name: a.UploadedByName}
	}
	if a.Status == "uploaded" {
		u := ts(a.UploadedAt)
		r.UploadedAt = &u
	}
	if a.URL != "" {
		u, e := a.URL, ts(a.URLExpiresAt)
		r.URL, r.URLExpiresAt = &u, &e
	}
	return r
}

func (h mediaHandler) fail(c *gin.Context, err error) {
	var rejected *media.RejectedError
	switch {
	case errors.As(err, &rejected):
		writeError(c, http.StatusUnprocessableEntity, CodeUploadRejected, rejected.Reason)
	case errors.Is(err, media.ErrNotReceived):
		writeError(c, http.StatusConflict, CodeUploadIncomplete, "We haven’t received the file yet. Finish uploading it, then try again.")
	default:
		h.orgHandler.fail(c, err)
	}
}

type createUploadRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

func (h mediaHandler) createUpload(c *gin.Context) {
	var req createUploadRequest
	if !decodeJSON(c, &req) {
		return
	}
	a, up, err := h.svc.CreateUpload(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"),
		req.Filename, req.ContentType, req.SizeBytes)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, uploadResponse{
		Asset:  toAssetResponse(a),
		Upload: uploadInstruction{URL: up.URL, Method: up.Method, Headers: up.Headers, ExpiresAt: ts(up.ExpiresAt)},
	})
}

func (h mediaHandler) complete(c *gin.Context) {
	a, err := h.svc.Complete(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toAssetResponse(a))
}

func (h mediaHandler) list(c *gin.Context) {
	limit := 0
	if l := c.Query("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			n = -1 // rejected by the service with a field error
		}
		limit = n
	}
	page, err := h.svc.List(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Query("cursor"), limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out := assetPage{Data: make([]assetResponse, len(page.Assets))}
	for i, a := range page.Assets {
		out.Data[i] = toAssetResponse(a)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, out)
}

func (h mediaHandler) get(c *gin.Context) {
	a, err := h.svc.Get(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toAssetResponse(a))
}

func (h mediaHandler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/render"
)

// RenderService is the render behaviour the handlers need. Every method
// authorises the actor itself.
type RenderService interface {
	Create(ctx context.Context, actorID, orgID, projectID, name string, c render.Composition) (render.Render, error)
	List(ctx context.Context, actorID, orgID, projectID, sourceAssetID, cursor string, limit int) (render.Page, error)
	Get(ctx context.Context, actorID, orgID, projectID, renderID string) (render.Render, error)
	Delete(ctx context.Context, actorID, orgID, projectID, renderID string) error
}

type renderHandler struct {
	svc RenderService
	orgHandler
}

type compositionBody struct {
	Version       int     `json:"version"`
	SourceAssetID string  `json:"source_asset_id"`
	StartMS       int64   `json:"start_ms"`
	EndMS         int64   `json:"end_ms"`
	Aspect        string  `json:"aspect"`
	Focus         float64 `json:"focus"`
}

type renderOutput struct {
	URL          string `json:"url"`
	URLExpiresAt string `json:"url_expires_at"`
	SizeBytes    int64  `json:"size_bytes"`
	DurationMS   int64  `json:"duration_ms"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

type renderResponse struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	SourceAssetID *string         `json:"source_asset_id"`
	Name          string          `json:"name"`
	Composition   compositionBody `json:"composition"`
	Status        string          `json:"status"`
	Error         *string         `json:"error"`
	RequestedBy   *userRef        `json:"requested_by"`
	CreatedAt     string          `json:"created_at"`
	CompletedAt   *string         `json:"completed_at"`
	CanDelete     bool            `json:"can_delete"`
	Output        *renderOutput   `json:"output"`
}

type renderPage struct {
	Data       []renderResponse `json:"data"`
	NextCursor *string          `json:"next_cursor"`
}

func toRenderResponse(r render.Render) renderResponse {
	c := r.Composition
	out := renderResponse{
		ID: r.ID, ProjectID: r.ProjectID, SourceAssetID: ptr(r.SourceAssetID), Name: r.Name,
		Composition: compositionBody{Version: c.Version, SourceAssetID: c.SourceAssetID, StartMS: c.StartMS,
			EndMS: c.EndMS, Aspect: c.Aspect, Focus: c.Focus},
		Status: r.Status, Error: ptr(r.Error), CreatedAt: ts(r.CreatedAt), CanDelete: r.CanDelete,
	}
	if r.RequestedByID != "" {
		out.RequestedBy = &userRef{ID: r.RequestedByID, Name: r.RequestedByName}
	}
	if r.Status == render.StatusReady && !r.CompletedAt.IsZero() {
		done := ts(r.CompletedAt)
		out.CompletedAt = &done
	}
	if r.URL != "" {
		out.Output = &renderOutput{URL: r.URL, URLExpiresAt: ts(r.URLExpiresAt), SizeBytes: r.SizeBytes,
			DurationMS: r.DurationMS, Width: r.Width, Height: r.Height}
	}
	return out
}

type createRenderRequest struct {
	Name        string          `json:"name"`
	Composition compositionBody `json:"composition"`
}

func (h renderHandler) create(c *gin.Context) {
	var req createRenderRequest
	if !decodeJSON(c, &req) {
		return
	}
	b := req.Composition
	r, err := h.svc.Create(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), req.Name,
		render.Composition{Version: b.Version, SourceAssetID: b.SourceAssetID, StartMS: b.StartMS, EndMS: b.EndMS,
			Aspect: b.Aspect, Focus: b.Focus})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, toRenderResponse(r))
}

func (h renderHandler) list(c *gin.Context) {
	limit := 0
	if l := c.Query("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			n = -1 // rejected by the service with a field error
		}
		limit = n
	}
	page, err := h.svc.List(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"),
		c.Query("source_asset_id"), c.Query("cursor"), limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out := renderPage{Data: make([]renderResponse, len(page.Renders))}
	for i, r := range page.Renders {
		out.Data[i] = toRenderResponse(r)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, out)
}

func (h renderHandler) get(c *gin.Context) {
	r, err := h.svc.Get(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("renderID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toRenderResponse(r))
}

func (h renderHandler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("renderID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

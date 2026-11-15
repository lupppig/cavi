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
	Create(ctx context.Context, actorID, orgID, projectID, name string, c render.Composition, captions *render.CaptionOptions) (render.Render, error)
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
	// Montages (version 3). The composition's segment and hook shapes are
	// the API's: they are plain, validated data.
	Segments  []render.Segment     `json:"segments,omitempty"`
	Hook      *render.Hook         `json:"hook,omitempty"`
	Voiceover *render.VoiceoverMix `json:"voiceover,omitempty"`
}

// captionOptions are the caption choices, in requests and responses.
type captionOptions struct {
	Style     string `json:"style"`
	Mode      string `json:"mode,omitempty"`
	Position  string `json:"position,omitempty"`
	Uppercase bool   `json:"uppercase,omitempty"`
	Colors    *struct {
		Text      string `json:"text"`
		Highlight string `json:"highlight"`
		Stroke    string `json:"stroke"`
	} `json:"colors,omitempty"`
	SpeakerColors map[string]string `json:"speaker_colors,omitempty"`
}

func (o captionOptions) toRender() *render.CaptionOptions {
	out := &render.CaptionOptions{Style: o.Style, Mode: o.Mode, Position: o.Position, Uppercase: o.Uppercase, SpeakerColors: o.SpeakerColors}
	if o.Colors != nil {
		out.Colors = render.CaptionColors{Text: o.Colors.Text, Highlight: o.Colors.Highlight, Stroke: o.Colors.Stroke}
	}
	return out
}

// captionsSummary is what responses say about captions: the choices and
// how many cues were made. The cues themselves stay server-side.
type captionsSummary struct {
	captionOptions
	CueCount int `json:"cue_count"`
}

func summarise(c *render.Captions) *captionsSummary {
	o := captionOptions{Style: c.Style, Mode: c.Mode, Position: c.Position, Uppercase: c.Uppercase, SpeakerColors: c.SpeakerColors}
	o.Colors = &struct {
		Text      string `json:"text"`
		Highlight string `json:"highlight"`
		Stroke    string `json:"stroke"`
	}{c.Colors.Text, c.Colors.Highlight, c.Colors.Stroke}
	return &captionsSummary{captionOptions: o, CueCount: len(c.Cues)}
}

type compositionResponse struct {
	compositionBody
	Captions *captionsSummary `json:"captions"`
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
	ID            string              `json:"id"`
	ProjectID     string              `json:"project_id"`
	SourceAssetID *string             `json:"source_asset_id"`
	Name          string              `json:"name"`
	Composition   compositionResponse `json:"composition"`
	Status        string              `json:"status"`
	Error         *string             `json:"error"`
	RequestedBy   *userRef            `json:"requested_by"`
	CreatedAt     string              `json:"created_at"`
	CompletedAt   *string             `json:"completed_at"`
	CanDelete     bool                `json:"can_delete"`
	Output        *renderOutput       `json:"output"`
	Flags         []briefFlag         `json:"flags"`
}

type renderPage struct {
	Data       []renderResponse `json:"data"`
	NextCursor *string          `json:"next_cursor"`
}

func toRenderResponse(r render.Render) renderResponse {
	c := r.Composition
	out := renderResponse{
		ID: r.ID, ProjectID: r.ProjectID, SourceAssetID: ptr(r.SourceAssetID), Name: r.Name,
		Composition: compositionResponse{compositionBody: compositionBody{Version: c.Version, SourceAssetID: c.SourceAssetID,
			StartMS: c.StartMS, EndMS: c.EndMS, Aspect: c.Aspect, Focus: c.Focus, Segments: c.Segments, Hook: c.Hook, Voiceover: c.Voiceover}},
		Status: r.Status, Error: ptr(r.Error), CreatedAt: ts(r.CreatedAt), CanDelete: r.CanDelete,
		Flags: toFlags(r.Flags),
	}
	if c.Captions != nil {
		out.Composition.Captions = summarise(c.Captions)
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
	Captions    *captionOptions `json:"captions"`
}

func (h renderHandler) create(c *gin.Context) {
	var req createRenderRequest
	if !decodeJSON(c, &req) {
		return
	}
	b := req.Composition
	var captions *render.CaptionOptions
	if req.Captions != nil {
		captions = req.Captions.toRender()
	}
	r, err := h.svc.Create(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), req.Name,
		render.Composition{Version: b.Version, SourceAssetID: b.SourceAssetID, StartMS: b.StartMS, EndMS: b.EndMS,
			Aspect: b.Aspect, Focus: b.Focus, Segments: b.Segments, Hook: b.Hook, Voiceover: b.Voiceover}, captions)
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

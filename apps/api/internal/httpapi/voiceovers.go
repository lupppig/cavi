package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/voiceover"
)

// VoiceoverService is the voiceover behaviour the handlers need. Every
// method authorises the actor itself.
type VoiceoverService interface {
	Create(ctx context.Context, actorID, orgID, projectID, script, voice, style string) (voiceover.Voiceover, error)
	List(ctx context.Context, actorID, orgID, projectID string) ([]voiceover.Voiceover, error)
	Get(ctx context.Context, actorID, orgID, projectID, id string) (voiceover.Voiceover, error)
	Delete(ctx context.Context, actorID, orgID, projectID, id string) error
}

type voiceoverHandler struct {
	svc VoiceoverService
	orgHandler
}

type voiceoverResponse struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"project_id"`
	Script        string  `json:"script"`
	Voice         string  `json:"voice"`
	Style         string  `json:"style"`
	Status        string  `json:"status"`
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	ModelLabel    string  `json:"model_label"`
	PromptVersion string  `json:"prompt_version"`
	DurationMS    *int64  `json:"duration_ms"`
	Error         *string `json:"error"`
	CreatedAt     string  `json:"created_at"`
	CompletedAt   *string `json:"completed_at"`
	CanDelete     bool    `json:"can_delete"`
	URL           *string `json:"url"`
	URLExpiresAt  *string `json:"url_expires_at"`
}

func toVoiceoverResponse(v voiceover.Voiceover) voiceoverResponse {
	r := voiceoverResponse{ID: v.ID, ProjectID: v.ProjectID, Script: v.Script, Voice: v.Voice, Style: v.Style, Status: v.Status,
		Provider: v.Provider, Model: v.Model, ModelLabel: v.ModelLabel, PromptVersion: v.PromptVersion,
		Error: ptr(v.Error), CreatedAt: ts(v.CreatedAt), CanDelete: v.CanDelete, URL: ptr(v.URL)}
	if v.Status == voiceover.StatusReady {
		d, c, e := v.DurationMS, ts(v.CompletedAt), ts(v.URLExpiresAt)
		r.DurationMS, r.CompletedAt, r.URLExpiresAt = &d, &c, &e
	}
	return r
}

type createVoiceoverRequest struct {
	Script string `json:"script"`
	Voice  string `json:"voice"`
	Style  string `json:"style"`
}

func (h voiceoverHandler) create(c *gin.Context) {
	var req createVoiceoverRequest
	if !decodeJSON(c, &req) {
		return
	}
	v, err := h.svc.Create(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), req.Script, req.Voice, req.Style)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, toVoiceoverResponse(v))
}

func (h voiceoverHandler) list(c *gin.Context) {
	vs, err := h.svc.List(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]voiceoverResponse, len(vs))
	for i, v := range vs {
		out[i] = toVoiceoverResponse(v)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, listResponse[voiceoverResponse]{Data: out})
}

func (h voiceoverHandler) get(c *gin.Context) {
	v, err := h.svc.Get(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("voiceoverID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toVoiceoverResponse(v))
}

func (h voiceoverHandler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("voiceoverID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type voiceResponse struct {
	Provider string   `json:"provider"`
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Styles   []string `json:"styles"`
}

// voices lists the voices voiceovers can use, per provider (each
// provider's first is its default), with the delivery styles each can
// perform.
func voices(c *gin.Context) {
	out := make([]voiceResponse, len(ai.Voices))
	for i, v := range ai.Voices {
		out[i] = voiceResponse{Provider: v.Provider, ID: v.ID, Label: v.Label, Styles: ai.StylesFor(v.Provider)}
	}
	c.JSON(http.StatusOK, listResponse[voiceResponse]{Data: out})
}

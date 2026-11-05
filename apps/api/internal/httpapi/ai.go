package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/ai"
)

// AISettingsService is the organisation AI model settings behaviour the
// handlers need. It authorises the actor itself.
type AISettingsService interface {
	Settings(ctx context.Context, actorID, orgID string) ([]ai.TaskSetting, error)
	SetModel(ctx context.Context, actorID, orgID, task, provider, modelID string) error
}

type aiHandler struct {
	svc AISettingsService
	orgHandler
}

type aiModelResponse struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Label    string `json:"label"`
	Default  bool   `json:"default"`
}

type aiTaskSettingResponse struct {
	Task    string            `json:"task"`
	Model   aiModelResponse   `json:"model"`
	Custom  bool              `json:"custom"`
	Options []aiModelResponse `json:"options"`
}

func toModel(m ai.Model) aiModelResponse {
	return aiModelResponse{Provider: m.Provider, ID: m.ID, Label: m.Label, Default: m.Default}
}

func (h aiHandler) settings(c *gin.Context) {
	ss, err := h.svc.Settings(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]aiTaskSettingResponse, len(ss))
	for i, s := range ss {
		opts := make([]aiModelResponse, len(s.Options))
		for j, m := range s.Options {
			opts[j] = toModel(m)
		}
		out[i] = aiTaskSettingResponse{Task: s.Task, Model: toModel(s.Model), Custom: s.Custom, Options: opts}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, listResponse[aiTaskSettingResponse]{Data: out})
}

type setModelRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (h aiHandler) setModel(c *gin.Context) {
	var req setModelRequest
	if !decodeJSON(c, &req) {
		return
	}
	if err := h.svc.SetModel(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("task"), req.Provider, req.Model); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

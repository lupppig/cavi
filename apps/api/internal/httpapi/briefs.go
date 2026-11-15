package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/brief"
)

// BriefService is the creator brief behaviour the handlers need. Every
// method authorises the actor itself.
type BriefService interface {
	Get(ctx context.Context, actorID, orgID, projectID string) (brief.Brief, error)
	Submit(ctx context.Context, actorID, orgID, projectID, text, mediaAssetID string) (brief.Brief, error)
	Update(ctx context.Context, actorID, orgID, projectID string, e brief.Edit) (brief.Brief, error)
	Delete(ctx context.Context, actorID, orgID, projectID string) error
}

type briefHandler struct {
	svc BriefService
	orgHandler
}

// briefRule is a rule in requests and responses.
type briefRule struct {
	MediaAssetID string `json:"media_asset_id"`
	Kind         string `json:"kind"`
	StartMS      int64  `json:"start_ms"`
	EndMS        int64  `json:"end_ms"`
	Note         string `json:"note"`
}

type briefResponse struct {
	ID              string      `json:"id"`
	ProjectID       string      `json:"project_id"`
	MediaAssetID    *string     `json:"media_asset_id"`
	Text            string      `json:"text"`
	CampaignMessage string      `json:"campaign_message"`
	Guidance        string      `json:"guidance"`
	Status          string      `json:"status"`
	Provider        string      `json:"provider"`
	Model           string      `json:"model"`
	ModelLabel      string      `json:"model_label"`
	PromptVersion   string      `json:"prompt_version"`
	Error           *string     `json:"error"`
	RequestedAt     string      `json:"requested_at"`
	UpdatedAt       string      `json:"updated_at"`
	CompletedAt     *string     `json:"completed_at"`
	Rules           []briefRule `json:"rules"`
	CanEdit         bool        `json:"can_edit"`
}

func toBriefResponse(b brief.Brief) briefResponse {
	r := briefResponse{
		ID: b.ID, ProjectID: b.ProjectID, MediaAssetID: ptr(b.MediaAssetID), Text: b.SourceText,
		CampaignMessage: b.CampaignMessage, Guidance: b.Guidance, Status: b.Status, Provider: b.Provider,
		Model: b.Model, ModelLabel: b.ModelLabel, PromptVersion: b.PromptVersion, Error: ptr(b.Error),
		RequestedAt: ts(b.CreatedAt), UpdatedAt: ts(b.UpdatedAt), Rules: make([]briefRule, len(b.Rules)),
		CanEdit: b.CanEdit,
	}
	if !b.CompletedAt.IsZero() && b.Status == brief.StatusReady {
		c := ts(b.CompletedAt)
		r.CompletedAt = &c
	}
	for i, x := range b.Rules {
		r.Rules[i] = briefRule{MediaAssetID: x.MediaAssetID, Kind: x.Kind, StartMS: x.StartMS, EndMS: x.EndMS, Note: x.Note}
	}
	return r
}

func (h briefHandler) fail(c *gin.Context, err error) {
	if errors.Is(err, brief.ErrInProgress) {
		writeError(c, http.StatusConflict, CodeBriefInProgress, "The brief is still being read. Wait for it to finish.")
		return
	}
	h.orgHandler.fail(c, err)
}

func (h briefHandler) reply(c *gin.Context, status int, b brief.Brief, err error) {
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(status, toBriefResponse(b))
}

func (h briefHandler) get(c *gin.Context) {
	b, err := h.svc.Get(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"))
	h.reply(c, http.StatusOK, b, err)
}

type submitBriefRequest struct {
	Text         string `json:"text"`
	MediaAssetID string `json:"media_asset_id"`
}

func (h briefHandler) submit(c *gin.Context) {
	var req submitBriefRequest
	if !decodeJSON(c, &req) {
		return
	}
	b, err := h.svc.Submit(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), req.Text, req.MediaAssetID)
	h.reply(c, http.StatusAccepted, b, err)
}

type updateBriefRequest struct {
	CampaignMessage string      `json:"campaign_message"`
	Guidance        string      `json:"guidance"`
	Rules           []briefRule `json:"rules"`
}

func (h briefHandler) update(c *gin.Context) {
	var req updateBriefRequest
	if !decodeJSON(c, &req) {
		return
	}
	e := brief.Edit{CampaignMessage: req.CampaignMessage, Guidance: req.Guidance, Rules: make([]brief.Rule, len(req.Rules))}
	for i, r := range req.Rules {
		e.Rules[i] = brief.Rule{MediaAssetID: r.MediaAssetID,
			BriefRule: ai.BriefRule{Kind: r.Kind, StartMS: r.StartMS, EndMS: r.EndMS, Note: r.Note}}
	}
	b, err := h.svc.Update(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), e)
	h.reply(c, http.StatusOK, b, err)
}

func (h briefHandler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// briefFlag is a brief rule a clip touches.
type briefFlag struct {
	Kind    string `json:"kind"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Note    string `json:"note"`
}

func toFlags(rs []ai.BriefRule) []briefFlag {
	out := make([]briefFlag, len(rs))
	for i, r := range rs {
		out[i] = briefFlag{Kind: r.Kind, StartMS: r.StartMS, EndMS: r.EndMS, Note: r.Note}
	}
	return out
}

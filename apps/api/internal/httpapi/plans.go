package httpapi

import (
	"context"
	"errors"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/plan"
)

// PlanService is the edit plan behaviour the handlers need. Every method
// authorises the actor itself.
type PlanService interface {
	Request(ctx context.Context, actorID, orgID, projectID string, req plan.Request) (plan.Run, error)
	Get(ctx context.Context, actorID, orgID, projectID string) (plan.Run, error)
}

type planHandler struct {
	svc PlanService
	orgHandler
}

type planSegmentResponse struct {
	SourceAssetID string      `json:"source_asset_id"`
	StartMS       int64       `json:"start_ms"`
	EndMS         int64       `json:"end_ms"`
	Flags         []briefFlag `json:"flags"`
}

type planResponse struct {
	Title           string                `json:"title"`
	Hook            string                `json:"hook"`
	VoiceoverScript string                `json:"voiceover_script"`
	Reason          string                `json:"reason"`
	CampaignLink    string                `json:"campaign_link"`
	DurationMS      int64                 `json:"duration_ms"`
	Segments        []planSegmentResponse `json:"segments"`
	// Commentary is narration through the plan, anchored to its segments.
	Commentary []ai.CommentaryLine `json:"commentary"`
	// Part (from 1) and SeriesTitle: the plan is a part of a series.
	Part        *int    `json:"part"`
	SeriesTitle *string `json:"series_title"`
}

type planRunResponse struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	ModelLabel    string         `json:"model_label"`
	PromptVersion string         `json:"prompt_version"`
	Objective     string         `json:"objective"`
	MinSeconds    int64          `json:"min_seconds"`
	MaxSeconds    int64          `json:"max_seconds"`
	MediaAssetIDs []string       `json:"media_asset_ids"`
	UsedBrief     bool           `json:"used_brief"`
	Series        bool           `json:"series"`
	Error         *string        `json:"error"`
	RequestedAt   string         `json:"requested_at"`
	CompletedAt   *string        `json:"completed_at"`
	Plans         []planResponse `json:"plans"`
}

func toPlanRunResponse(r plan.Run) planRunResponse {
	out := planRunResponse{ID: r.ID, Status: r.Status, Provider: r.Provider, Model: r.Model, ModelLabel: r.ModelLabel,
		PromptVersion: r.PromptVersion, Objective: r.Objective, MinSeconds: r.MinMS / 1000, MaxSeconds: r.MaxMS / 1000,
		MediaAssetIDs: r.MediaAssetIDs, UsedBrief: r.UsedBrief, Series: r.Series, Error: ptr(r.Error), RequestedAt: ts(r.CreatedAt),
		Plans: make([]planResponse, len(r.Plans))}
	if r.Status == plan.StatusReady && !r.CompletedAt.IsZero() {
		c := ts(r.CompletedAt)
		out.CompletedAt = &c
	}
	for i, p := range r.Plans {
		pr := planResponse{Title: p.Title, Hook: p.Hook, VoiceoverScript: p.VoiceoverScript, Reason: p.Reason,
			CampaignLink: p.CampaignLink, Segments: make([]planSegmentResponse, len(p.Segments)),
			Part: ptr(p.Part), SeriesTitle: ptr(p.SeriesTitle), Commentary: p.Commentary}
		if pr.Commentary == nil {
			pr.Commentary = []ai.CommentaryLine{}
		}
		for j, s := range p.Segments {
			pr.Segments[j] = planSegmentResponse{SourceAssetID: s.SourceAssetID, StartMS: s.StartMS, EndMS: s.EndMS, Flags: toFlags(s.Flags)}
			pr.DurationMS += s.EndMS - s.StartMS
		}
		out.Plans[i] = pr
	}
	return out
}

func (h planHandler) fail(c *gin.Context, err error) {
	if errors.Is(err, plan.ErrInProgress) {
		writeError(c, http.StatusConflict, CodePlansInProgress, "Plans are already being made for this project.")
		return
	}
	h.orgHandler.fail(c, err)
}

type requestPlansRequest struct {
	Objective     string   `json:"objective"`
	MinSeconds    int      `json:"min_seconds"`
	MaxSeconds    int      `json:"max_seconds"`
	MediaAssetIDs []string `json:"media_asset_ids"`
	Series        bool     `json:"series"`
}

func (h planHandler) request(c *gin.Context) {
	var req requestPlansRequest
	if !decodeJSON(c, &req) {
		return
	}
	r, err := h.svc.Request(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"),
		plan.Request{Objective: req.Objective, MinSeconds: req.MinSeconds, MaxSeconds: req.MaxSeconds, MediaAssetIDs: req.MediaAssetIDs, Series: req.Series})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, toPlanRunResponse(r))
}

func (h planHandler) get(c *gin.Context) {
	r, err := h.svc.Get(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toPlanRunResponse(r))
}

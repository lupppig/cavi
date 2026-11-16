package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/youtube"
)

// YouTubeService connects channels and reads their synced analytics.
// Every method authorises the actor itself.
type YouTubeService interface {
	Status(ctx context.Context, actorID, orgID string) (youtube.Connection, error)
	Connect(ctx context.Context, actorID, orgID string) (string, error)
	Callback(ctx context.Context, actorID, state, code, googleError string) (string, error)
	Disconnect(ctx context.Context, actorID, orgID string) error
	RequestSync(ctx context.Context, actorID, orgID string) (youtube.Connection, error)
	Videos(ctx context.Context, actorID, orgID string) ([]youtube.VideoStats, error)
}

type youtubeHandler struct {
	svc YouTubeService
	// webURL is where people go back to after connecting.
	webURL string
	orgHandler
}

type youtubeStatusResponse struct {
	// Available: this server can connect channels.
	Available    bool    `json:"available"`
	Connected    bool    `json:"connected"`
	ChannelID    *string `json:"channel_id"`
	ChannelTitle *string `json:"channel_title"`
	ConnectedAt  *string `json:"connected_at"`
	ConnectedBy  *string `json:"connected_by"`
	SyncStatus   *string `json:"sync_status"`
	SyncError    *string `json:"sync_error"`
	SyncedAt     *string `json:"synced_at"`
	Videos       int     `json:"videos"`
}

func toYouTubeStatus(c youtube.Connection) youtubeStatusResponse {
	r := youtubeStatusResponse{Available: true, Connected: true, ChannelID: ptr(c.ChannelID), ChannelTitle: ptr(c.ChannelTitle),
		ConnectedBy: ptr(c.ConnectedByName), SyncStatus: ptr(c.SyncStatus), SyncError: ptr(c.SyncError), Videos: c.Videos}
	at := ts(c.ConnectedAt)
	r.ConnectedAt = &at
	if !c.SyncedAt.IsZero() {
		s := ts(c.SyncedAt)
		r.SyncedAt = &s
	}
	return r
}

func (h youtubeHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, youtube.ErrNotConnected):
		writeError(c, http.StatusNotFound, CodeNotFound, "No YouTube channel is connected.")
	case errors.Is(err, youtube.ErrSyncing):
		writeError(c, http.StatusConflict, CodeSyncInProgress, "The channel is already being synced.")
	default:
		h.orgHandler.fail(c, err)
	}
}

func (h youtubeHandler) status(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusOK, youtubeStatusResponse{})
		return
	}
	conn, err := h.svc.Status(c.Request.Context(), actor(c), c.Param("orgID"))
	if errors.Is(err, youtube.ErrNotConnected) {
		c.JSON(http.StatusOK, youtubeStatusResponse{Available: true})
		return
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toYouTubeStatus(conn))
}

func (h youtubeHandler) connect(c *gin.Context) {
	consent, err := h.svc.Connect(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"url": consent})
}

// callback is where Google sends the browser back: it finishes
// connecting and returns the person to their organisation's page.
func (h youtubeHandler) callback(c *gin.Context) {
	orgID, err := h.svc.Callback(c.Request.Context(), actor(c), c.Query("state"), c.Query("code"), c.Query("error"))
	back := h.webURL + "/account"
	if orgID != "" {
		back = h.webURL + "/account/organisations/" + url.PathEscape(orgID)
	}
	q := url.Values{"youtube": {"connected"}}
	if err != nil {
		q = url.Values{"youtube": {"failed"}}
	}
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, back+"?"+q.Encode())
}

func (h youtubeHandler) disconnect(c *gin.Context) {
	if err := h.svc.Disconnect(c.Request.Context(), actor(c), c.Param("orgID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h youtubeHandler) sync(c *gin.Context) {
	conn, err := h.svc.RequestSync(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, toYouTubeStatus(conn))
}

type youtubeVideoResponse struct {
	VideoID           string  `json:"video_id"`
	Title             string  `json:"title"`
	PublishedAt       *string `json:"published_at"`
	DurationMS        int64   `json:"duration_ms"`
	Views             int64   `json:"views"`
	WatchMinutes      float64 `json:"watch_minutes"`
	AvgViewSeconds    float64 `json:"avg_view_seconds"`
	AvgViewPercentage float64 `json:"avg_view_percentage"`
	SubscribersGained int64   `json:"subscribers_gained"`
	Likes             int64   `json:"likes"`
	Shares            int64   `json:"shares"`
	Clip              *struct {
		RenderID  string `json:"render_id"`
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
	} `json:"clip"`
}

func (h youtubeHandler) videos(c *gin.Context) {
	vs, err := h.svc.Videos(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	out := listResponse[youtubeVideoResponse]{Data: make([]youtubeVideoResponse, len(vs))}
	for i, v := range vs {
		r := youtubeVideoResponse{VideoID: v.VideoID, Title: v.Title, DurationMS: v.DurationMS, Views: v.Views, WatchMinutes: v.WatchMinutes,
			AvgViewSeconds: v.AvgViewSeconds, AvgViewPercentage: v.AvgViewPercentage, SubscribersGained: v.SubscribersGained,
			Likes: v.Likes, Shares: v.Shares}
		if !v.PublishedAt.IsZero() {
			p := ts(v.PublishedAt)
			r.PublishedAt = &p
		}
		if v.RenderID != "" {
			r.Clip = &struct {
				RenderID  string `json:"render_id"`
				ProjectID string `json:"project_id"`
				Name      string `json:"name"`
			}{v.RenderID, v.ProjectID, v.RenderName}
		}
		out.Data[i] = r
	}
	c.JSON(http.StatusOK, out)
}

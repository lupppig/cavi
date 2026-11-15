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
	Reprocess(ctx context.Context, actorID, orgID, projectID, assetID string) (media.Asset, error)
	RequestTranscript(ctx context.Context, actorID, orgID, projectID, assetID string) (media.Transcript, error)
	Transcript(ctx context.Context, actorID, orgID, projectID, assetID string) (media.Transcript, error)
	SetSpeakerVoice(ctx context.Context, actorID, orgID, projectID, assetID, label, voice string) (media.Transcript, error)
	RequestHighlights(ctx context.Context, actorID, orgID, projectID, assetID string, req media.HighlightRequest) (media.Highlights, error)
	Highlights(ctx context.Context, actorID, orgID, projectID, assetID string) (media.Highlights, error)
}

type mediaHandler struct {
	svc MediaService
	orgHandler
}

type assetResponse struct {
	ID           string             `json:"id"`
	ProjectID    string             `json:"project_id"`
	Filename     string             `json:"filename"`
	Kind         *string            `json:"kind"`
	ContentType  string             `json:"content_type"`
	SizeBytes    int64              `json:"size_bytes"`
	Status       string             `json:"status"`
	UploadedBy   *userRef           `json:"uploaded_by"`
	CreatedAt    string             `json:"created_at"`
	UploadedAt   *string            `json:"uploaded_at"`
	CanDelete    bool               `json:"can_delete"`
	URL          *string            `json:"url"`
	URLExpiresAt *string            `json:"url_expires_at"`
	ThumbnailURL *string            `json:"thumbnail_url"`
	Processing   processingResponse `json:"processing"`
	Metadata     *metadataResponse  `json:"metadata"`
}

type processingResponse struct {
	Status string  `json:"status"`
	Error  *string `json:"error"`
}

// metadataResponse is what processing learned; fields that don't apply
// (dimensions of audio, frame rate of a still) are null.
type metadataResponse struct {
	Container     *string  `json:"container"`
	DurationMS    *int64   `json:"duration_ms"`
	Width         *int     `json:"width"`
	Height        *int     `json:"height"`
	FrameRate     *float64 `json:"frame_rate"`
	VideoCodec    *string  `json:"video_codec"`
	AudioCodec    *string  `json:"audio_codec"`
	AudioChannels *int     `json:"audio_channels"`
	SampleRate    *int     `json:"sample_rate"`
}

// ptr returns nil for zero values, so absent metadata is null in JSON.
func ptr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
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
	r.ThumbnailURL = ptr(a.ThumbnailURL)
	r.Processing = processingResponse{Status: a.ProcessingStatus, Error: ptr(a.ProcessingError)}
	if a.ProcessingStatus == media.ProcessingReady {
		r.Metadata = &metadataResponse{
			Container: ptr(a.Container), DurationMS: ptr(a.DurationMS), Width: ptr(a.Width), Height: ptr(a.Height),
			FrameRate: ptr(a.FrameRate), VideoCodec: ptr(a.VideoCodec), AudioCodec: ptr(a.AudioCodec),
			AudioChannels: ptr(a.AudioChannels), SampleRate: ptr(a.SampleRate),
		}
	}
	return r
}

func (h mediaHandler) fail(c *gin.Context, err error) {
	var rejected *media.RejectedError
	switch {
	case errors.As(err, &rejected):
		writeError(c, http.StatusUnprocessableEntity, CodeUploadRejected, rejected.Reason)
	case errors.Is(err, media.ErrTranscriptionInProgress):
		writeError(c, http.StatusConflict, CodeTranscriptionInProgress, "This file is already being transcribed.")
	case errors.Is(err, media.ErrHighlightsInProgress):
		writeError(c, http.StatusConflict, CodeHighlightsInProgress, "Highlights are already being found for this file.")
	case errors.Is(err, media.ErrProcessingInProgress):
		writeError(c, http.StatusConflict, CodeProcessingInProgress, "This file is already being processed.")
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

func (h mediaHandler) reprocess(c *gin.Context) {
	a, err := h.svc.Reprocess(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, toAssetResponse(a))
}

type segmentResponse struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

type transcriptResponse struct {
	ID            string            `json:"id"`
	MediaAssetID  string            `json:"media_asset_id"`
	Status        string            `json:"status"`
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	ModelLabel    string            `json:"model_label"`
	PromptVersion string            `json:"prompt_version"`
	Language      *string           `json:"language"`
	Error         *string           `json:"error"`
	RequestedAt   string            `json:"requested_at"`
	CompletedAt   *string           `json:"completed_at"`
	Segments      []segmentResponse `json:"segments"`
	Speakers      []speakerResponse `json:"speakers"`
}

type speakerResponse struct {
	Label      string `json:"label"`
	Voice      string `json:"voice"`
	VoiceSetBy string `json:"voice_set_by"`
}

func toTranscriptResponse(t media.Transcript) transcriptResponse {
	r := transcriptResponse{
		ID: t.ID, MediaAssetID: t.MediaAssetID, Status: t.Status, Provider: t.Provider, Model: t.Model,
		ModelLabel: t.ModelLabel, PromptVersion: t.PromptVersion, Language: ptr(t.Language), Error: ptr(t.Error),
		RequestedAt: ts(t.CreatedAt), Segments: make([]segmentResponse, len(t.Segments)),
		Speakers: make([]speakerResponse, len(t.Speakers)),
	}
	for i, sp := range t.Speakers {
		r.Speakers[i] = speakerResponse{Label: sp.Label, Voice: sp.Voice, VoiceSetBy: sp.VoiceSetBy}
	}
	if !t.CompletedAt.IsZero() && t.Status == media.TranscriptReady {
		c := ts(t.CompletedAt)
		r.CompletedAt = &c
	}
	for i, s := range t.Segments {
		r.Segments[i] = segmentResponse{StartMS: s.StartMS, EndMS: s.EndMS, Speaker: s.Speaker, Text: s.Text}
	}
	return r
}

func (h mediaHandler) requestTranscript(c *gin.Context) {
	t, err := h.svc.RequestTranscript(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, toTranscriptResponse(t))
}

func (h mediaHandler) transcript(c *gin.Context) {
	t, err := h.svc.Transcript(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toTranscriptResponse(t))
}

type highlightResponse struct {
	StartMS      int64       `json:"start_ms"`
	EndMS        int64       `json:"end_ms"`
	FirstSegment int         `json:"first_segment"`
	LastSegment  int         `json:"last_segment"`
	Title        string      `json:"title"`
	Reason       string      `json:"reason"`
	Flags        []briefFlag `json:"flags"`
}

type highlightsResponse struct {
	ID            string              `json:"id"`
	MediaAssetID  string              `json:"media_asset_id"`
	Status        string              `json:"status"`
	Provider      string              `json:"provider"`
	Model         string              `json:"model"`
	ModelLabel    string              `json:"model_label"`
	PromptVersion string              `json:"prompt_version"`
	Objective     string              `json:"objective"`
	MinSeconds    int64               `json:"min_seconds"`
	MaxSeconds    int64               `json:"max_seconds"`
	Error         *string             `json:"error"`
	RequestedAt   string              `json:"requested_at"`
	CompletedAt   *string             `json:"completed_at"`
	Clips         []highlightResponse `json:"clips"`
}

func toHighlightsResponse(h media.Highlights) highlightsResponse {
	r := highlightsResponse{
		ID: h.ID, MediaAssetID: h.MediaAssetID, Status: h.Status, Provider: h.Provider, Model: h.Model,
		ModelLabel: h.ModelLabel, PromptVersion: h.PromptVersion, Objective: h.Objective,
		MinSeconds: h.MinMS / 1000, MaxSeconds: h.MaxMS / 1000, Error: ptr(h.Error),
		RequestedAt: ts(h.CreatedAt), Clips: make([]highlightResponse, len(h.Clips)),
	}
	if !h.CompletedAt.IsZero() && h.Status == media.HighlightsReady {
		c := ts(h.CompletedAt)
		r.CompletedAt = &c
	}
	for i, c := range h.Clips {
		r.Clips[i] = highlightResponse{StartMS: c.StartMS, EndMS: c.EndMS, FirstSegment: c.FirstSegment,
			LastSegment: c.LastSegment, Title: c.Title, Reason: c.Reason, Flags: toFlags(c.Flags)}
	}
	return r
}

type requestHighlightsRequest struct {
	Objective  string `json:"objective"`
	MinSeconds int    `json:"min_seconds"`
	MaxSeconds int    `json:"max_seconds"`
}

func (h mediaHandler) requestHighlights(c *gin.Context) {
	var req requestHighlightsRequest
	if !decodeJSON(c, &req) {
		return
	}
	out, err := h.svc.RequestHighlights(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"),
		media.HighlightRequest{Objective: req.Objective, MinSeconds: req.MinSeconds, MaxSeconds: req.MaxSeconds})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, toHighlightsResponse(out))
}

func (h mediaHandler) highlights(c *gin.Context) {
	out, err := h.svc.Highlights(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toHighlightsResponse(out))
}

type setSpeakerVoiceRequest struct {
	Voice string `json:"voice"`
}

func (h mediaHandler) setSpeakerVoice(c *gin.Context) {
	var req setSpeakerVoiceRequest
	if !decodeJSON(c, &req) {
		return
	}
	t, err := h.svc.SetSpeakerVoice(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("assetID"),
		c.Param("label"), req.Voice)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toTranscriptResponse(t))
}

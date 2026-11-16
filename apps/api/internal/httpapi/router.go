// Package httpapi is the Gin HTTP transport for the CAVI API. It owns routing,
// middleware, and request/response shapes; business logic lives elsewhere.
package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Options are the dependencies needed to build the HTTP router.
type Options struct {
	Logger        *slog.Logger
	DB            Pinger
	Waitlist      WaitlistService
	Auth          *AuthOptions
	Organisations OrganisationService
	Projects      ProjectService
	Media         MediaService
	Renders       RenderService
	Briefs        BriefService
	Voiceovers    VoiceoverService
	Plans         PlanService
	AISettings    AISettingsService
	// YouTube connects channels; nil when not configured.
	YouTube YouTubeService
	// WebURL is the frontend, where people return after connecting YouTube.
	WebURL         string
	AllowedOrigins []string
	// TrustedProxies lists proxy IPs or CIDRs whose X-Forwarded-For header
	// is trusted when determining the client IP. Empty trusts none.
	TrustedProxies []string
	// Debug enables Gin debug mode. Keep false outside local development.
	Debug bool
}

// Waitlist rate limit: per client IP, per API instance.
const (
	waitlistRateLimit  = 10
	waitlistRateWindow = time.Hour
)

func requireService(ok bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ok {
			writeError(c, http.StatusServiceUnavailable, CodeUnavailable, "This service is not configured.")
			return
		}
		c.Next()
	}
}

// NewRouter builds the API's HTTP handler.
func NewRouter(opts Options) (*gin.Engine, error) {
	if opts.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Gin trusts every proxy by default, which lets any client spoof its IP
	// through X-Forwarded-For. Trust only configured proxies.
	if err := r.SetTrustedProxies(opts.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}
	r.HandleMethodNotAllowed = true
	r.Use(
		requestID(),
		requestLogger(opts.Logger),
		recovery(opts.Logger),
		cors(opts.AllowedOrigins),
	)

	r.NoRoute(func(c *gin.Context) {
		writeError(c, http.StatusNotFound, CodeNotFound, "The requested resource does not exist.")
	})
	r.NoMethod(func(c *gin.Context) {
		writeError(c, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "This method is not allowed for the requested resource.")
	})

	health := healthHandler{db: opts.DB, logger: opts.Logger}

	v1 := r.Group("/api/v1")
	v1.GET("/health", health.live)
	v1.GET("/health/ready", health.ready)

	waitlistH := waitlistHandler{svc: opts.Waitlist, logger: opts.Logger}
	v1.POST("/waitlist",
		rateLimit(newFixedWindowLimiter(waitlistRateLimit, waitlistRateWindow)),
		waitlistH.join,
	)

	auth := authHandler{opts: opts.Auth, logger: opts.Logger}
	v1.GET("/auth/login", auth.login)
	v1.GET("/auth/callback", auth.callback)
	v1.POST("/auth/logout", requireAllowedOrigin(opts.AllowedOrigins), auth.logout)
	v1.GET("/me", auth.requireSession, auth.me)

	// Signed-in routes. Each service call authorises the actor against the
	// organisation itself; writes must come from an allowed origin.
	orgs := orgHandler{svc: opts.Organisations, logger: opts.Logger}
	signedIn := v1.Group("", auth.requireSession, requireSameOriginWrites(opts.AllowedOrigins), requireService(opts.Organisations != nil))
	signedIn.POST("/organisations", orgs.create)
	signedIn.GET("/organisations/:orgID", orgs.get)
	signedIn.PATCH("/organisations/:orgID", orgs.rename)
	signedIn.GET("/organisations/:orgID/members", orgs.members)
	signedIn.PATCH("/organisations/:orgID/members/:userID", orgs.changeRole)
	signedIn.DELETE("/organisations/:orgID/members/:userID", orgs.removeMember)
	signedIn.GET("/organisations/:orgID/invitations", orgs.invitations)
	signedIn.POST("/organisations/:orgID/invitations", orgs.invite)
	signedIn.DELETE("/organisations/:orgID/invitations/:invitationID", orgs.revoke)
	signedIn.GET("/invitations", orgs.myInvitations)
	signedIn.POST("/invitations/:invitationID/accept", orgs.accept)
	signedIn.POST("/invitations/:invitationID/decline", orgs.decline)

	projects := projectHandler{svc: opts.Projects, orgHandler: orgs}
	proj := signedIn.Group("/organisations/:orgID/projects", requireService(opts.Projects != nil))
	proj.GET("", projects.list)
	proj.POST("", projects.create)
	proj.GET("/:projectID", projects.get)
	proj.PATCH("/:projectID", projects.update)
	proj.DELETE("/:projectID", projects.delete)
	proj.GET("/:projectID/access", projects.access)
	proj.POST("/:projectID/access", projects.grant)
	proj.DELETE("/:projectID/access/:userID", projects.revoke)

	mediaH := mediaHandler{svc: opts.Media, orgHandler: orgs}
	med := proj.Group("/:projectID/media", requireService(opts.Media != nil))
	med.GET("", mediaH.list)
	med.POST("/uploads", mediaH.createUpload)
	med.POST("/imports", mediaH.createImport)
	med.GET("/:assetID", mediaH.get)
	med.DELETE("/:assetID", mediaH.delete)
	med.POST("/:assetID/complete", mediaH.complete)
	med.POST("/:assetID/process", mediaH.reprocess)
	med.GET("/:assetID/transcript", mediaH.transcript)
	med.POST("/:assetID/transcript", mediaH.requestTranscript)
	med.PATCH("/:assetID/transcript/speakers/:label", mediaH.setSpeakerVoice)
	med.GET("/:assetID/highlights", mediaH.highlights)
	med.POST("/:assetID/highlights", mediaH.requestHighlights)
	med.POST("/:assetID/framing", mediaH.requestFraming)

	briefs := briefHandler{svc: opts.Briefs, orgHandler: orgs}
	br := proj.Group("/:projectID/brief", requireService(opts.Briefs != nil))
	br.GET("", briefs.get)
	br.PUT("", briefs.submit)
	br.PATCH("", briefs.update)
	br.DELETE("", briefs.delete)

	plans := planHandler{svc: opts.Plans, orgHandler: orgs}
	pl := proj.Group("/:projectID/plans", requireService(opts.Plans != nil))
	pl.GET("", plans.get)
	pl.POST("", plans.request)

	vo := voiceoverHandler{svc: opts.Voiceovers, orgHandler: orgs}
	vog := proj.Group("/:projectID/voiceovers", requireService(opts.Voiceovers != nil))
	vog.GET("", vo.list)
	vog.POST("", vo.create)
	vog.POST("/recordings", vo.createRecording)
	vog.POST("/:voiceoverID/complete", vo.completeRecording)
	vog.GET("/:voiceoverID", vo.get)
	vog.DELETE("/:voiceoverID", vo.delete)
	signedIn.GET("/voices", voices)

	renders := renderHandler{svc: opts.Renders, orgHandler: orgs}
	rend := proj.Group("/:projectID/renders", requireService(opts.Renders != nil))
	rend.GET("", renders.list)
	rend.POST("", renders.create)
	rend.GET("/:renderID", renders.get)
	rend.DELETE("/:renderID", renders.delete)
	// The camera path is computed by the render service.
	med.GET("/:assetID/framing/path", requireService(opts.Renders != nil), renders.framingPath)

	yt := youtubeHandler{svc: opts.YouTube, webURL: opts.WebURL, orgHandler: orgs}
	signedIn.GET("/organisations/:orgID/youtube", yt.status)
	ytGroup := signedIn.Group("/organisations/:orgID/youtube", requireService(opts.YouTube != nil))
	ytGroup.POST("/connect", yt.connect)
	ytGroup.DELETE("", yt.disconnect)
	ytGroup.POST("/sync", yt.sync)
	ytGroup.GET("/videos", yt.videos)
	signedIn.GET("/youtube/callback", requireService(opts.YouTube != nil), yt.callback)

	lookGroup := signedIn.Group("/organisations/:orgID/clip-look", requireService(opts.Renders != nil))
	lookGroup.GET("", renders.look)
	lookGroup.PUT("", renders.setLook)

	aiH := aiHandler{svc: opts.AISettings, orgHandler: orgs}
	aiGroup := signedIn.Group("/organisations/:orgID/ai-models", requireService(opts.AISettings != nil))
	aiGroup.GET("", aiH.settings)
	aiGroup.PUT("/:task", aiH.setModel)

	return r, nil
}

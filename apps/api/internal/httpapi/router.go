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
	Logger         *slog.Logger
	DB             Pinger
	Waitlist       WaitlistService
	Auth           *AuthOptions
	Organisations  OrganisationService
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

	return r, nil
}

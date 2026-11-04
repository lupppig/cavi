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

	return r, nil
}

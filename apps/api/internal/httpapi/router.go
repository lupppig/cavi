// Package httpapi is the Gin HTTP transport for the CAVI API. It owns routing,
// middleware, and request/response shapes; business logic lives elsewhere.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Options are the dependencies needed to build the HTTP router.
type Options struct {
	Logger         *slog.Logger
	DB             Pinger
	AllowedOrigins []string
	// Debug enables Gin debug mode. Keep false outside local development.
	Debug bool
}

// NewRouter builds the API's HTTP handler.
func NewRouter(opts Options) *gin.Engine {
	if opts.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
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

	return r
}

package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger reports whether a dependency is reachable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

type healthHandler struct {
	db     Pinger
	logger *slog.Logger
}

type statusResponse struct {
	Status string `json:"status"`
}

// live reports that the process is running and serving HTTP.
func (h healthHandler) live(c *gin.Context) {
	c.JSON(http.StatusOK, statusResponse{Status: "ok"})
}

// ready reports whether the API can serve traffic that needs its dependencies.
func (h healthHandler) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		h.logger.WarnContext(ctx, "readiness check failed",
			slog.String("request_id", requestIDFrom(c)),
			slog.String("dependency", "postgres"),
			slog.String("error", err.Error()),
		)
		writeError(c, http.StatusServiceUnavailable, CodeUnavailable, "A required dependency is unavailable.")
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "ready"})
}

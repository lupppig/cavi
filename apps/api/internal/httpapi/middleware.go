package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/jobs"
)

// HeaderRequestID carries the request identifier in requests and responses.
const HeaderRequestID = "X-Request-ID"

const requestIDKey = "request_id"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// requestID accepts a well-formed incoming X-Request-ID (so callers can
// correlate across services) or generates a new one.
func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header(HeaderRequestID, id)
		// Background jobs started by this request carry its ID.
		c.Request = c.Request.WithContext(jobs.WithCorrelationID(c.Request.Context(), id))
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return hex.EncodeToString(b)
}

func requestIDFrom(c *gin.Context) string {
	return c.GetString(requestIDKey)
}

// requestLogger logs one structured line per request. It logs the path
// without the query string so tokens or signed parameters never reach logs.
func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		logger.LogAttrs(c.Request.Context(), level, "http request",
			slog.String("request_id", requestIDFrom(c)),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("route", c.FullPath()),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
		)
	}
}

// recovery converts panics into a consistent 500 response and logs them.
func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.ErrorContext(c.Request.Context(), "panic recovered",
					slog.String("request_id", requestIDFrom(c)),
					slog.Any("panic", rec),
				)
				writeError(c, http.StatusInternalServerError, CodeInternal, "An unexpected error occurred.")
			}
		}()
		c.Next()
	}
}

// cors allows credentialed requests from an explicit origin allowlist only.
func cors(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		c.Writer.Header().Add("Vary", "Origin")
		if !slices.Contains(allowedOrigins, origin) {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Expose-Headers", HeaderRequestID)

		if c.Request.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, "+HeaderRequestID)
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// fixedWindowLimiter allows up to limit requests per key per window.
//
// State is held in process memory, so limits apply per API instance and
// reset on restart. That is acceptable for low-risk public endpoints such
// as the waitlist; move to a shared store (Redis) when the API runs as
// multiple instances or protects costlier operations.
type fixedWindowLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	windows   map[string]*windowCount
	lastSweep time.Time
}

type windowCount struct {
	start time.Time
	count int
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		limit:   limit,
		window:  window,
		now:     time.Now,
		windows: map[string]*windowCount{},
	}
}

// allow records a request for key and reports whether it is permitted.
// When denied, retryAfter is the time until the key's window resets.
func (l *fixedWindowLimiter) allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastSweep) >= l.window {
		for k, w := range l.windows {
			if now.Sub(w.start) >= l.window {
				delete(l.windows, k)
			}
		}
		l.lastSweep = now
	}

	w, exists := l.windows[key]
	if !exists || now.Sub(w.start) >= l.window {
		l.windows[key] = &windowCount{start: now, count: 1}
		return true, 0
	}
	if w.count >= l.limit {
		return false, w.start.Add(l.window).Sub(now)
	}
	w.count++
	return true, 0
}

// rateLimit rejects requests over the limiter's budget, keyed by client IP.
func rateLimit(l *fixedWindowLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, retryAfter := l.allow(c.ClientIP())
		if !ok {
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
			writeError(c, http.StatusTooManyRequests, CodeRateLimited, "Too many requests. Please try again later.")
			return
		}
		c.Next()
	}
}

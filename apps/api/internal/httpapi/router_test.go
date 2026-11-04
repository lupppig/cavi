package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

func newTestRouter(db Pinger) *gin.Engine {
	return NewRouter(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:             db,
		AllowedOrigins: []string{"http://localhost:3000"},
	})
}

func do(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorDetail {
	t.Helper()
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error
}

func TestHealthLive(t *testing.T) {
	rec := do(t, newTestRouter(fakePinger{}), httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestHealthReady(t *testing.T) {
	rec := do(t, newTestRouter(fakePinger{}), httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"ready"}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestHealthReadyDatabaseDown(t *testing.T) {
	rec := do(t, newTestRouter(fakePinger{err: errors.New("connection refused")}),
		httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	detail := decodeError(t, rec)
	if detail.Code != CodeUnavailable {
		t.Errorf("code = %q", detail.Code)
	}
	if detail.RequestID == "" || detail.RequestID != rec.Header().Get(HeaderRequestID) {
		t.Errorf("request id %q does not match header %q", detail.RequestID, rec.Header().Get(HeaderRequestID))
	}
}

func TestNotFoundUsesErrorEnvelope(t *testing.T) {
	rec := do(t, newTestRouter(fakePinger{}), httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != CodeNotFound {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, newTestRouter(fakePinger{}), httptest.NewRequest(http.MethodPost, "/api/v1/health", nil))
	if rec.Code != http.StatusMethodNotAllowed || decodeError(t, rec).Code != CodeMethodNotAllowed {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRequestIDPropagation(t *testing.T) {
	router := newTestRouter(fakePinger{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set(HeaderRequestID, "client-abc_123")
	if got := do(t, router, req).Header().Get(HeaderRequestID); got != "client-abc_123" {
		t.Errorf("valid incoming id not kept: %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set(HeaderRequestID, "bad id\nwith newline")
	got := do(t, router, req).Header().Get(HeaderRequestID)
	if got == "" || got == "bad id\nwith newline" || !validRequestID.MatchString(got) {
		t.Errorf("invalid incoming id not replaced: %q", got)
	}
}

func TestCORS(t *testing.T) {
	router := newTestRouter(fakePinger{})

	t.Run("allowed origin preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rec := do(t, router, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" ||
			rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("missing CORS headers: %v", rec.Header())
		}
	})

	t.Run("disallowed origin preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := do(t, router, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("disallowed origin received Allow-Origin")
		}
	})

	t.Run("disallowed origin simple request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := do(t, router, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("disallowed origin received Allow-Origin")
		}
	})
}

func TestRecoveryReturnsErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestID(), recovery(slog.New(slog.NewTextHandler(io.Discard, nil))))
	r.GET("/panic", func(*gin.Context) { panic("boom") })

	rec := do(t, r, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	detail := decodeError(t, rec)
	if detail.Code != CodeInternal || detail.RequestID == "" {
		t.Errorf("unexpected detail: %+v", detail)
	}
}

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/waitlist"
)

type fakeWaitlist struct {
	calls []string
	err   error
}

func (f *fakeWaitlist) Join(_ context.Context, email, discipline string) (waitlist.JoinResult, error) {
	f.calls = append(f.calls, email+"|"+discipline)
	if f.err != nil {
		return waitlist.JoinResult{}, f.err
	}
	return waitlist.JoinResult{Created: true}, nil
}

func postWaitlist(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return do(t, h, req)
}

func TestWaitlistJoin(t *testing.T) {
	svc := &fakeWaitlist{}
	rec := postWaitlist(t, newTestRouterWith(Options{Waitlist: svc}),
		`{"email":"ada@example.com","discipline":"musician"}`)
	if rec.Code != http.StatusAccepted || rec.Body.String() != `{"status":"joined"}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if len(svc.calls) != 1 || svc.calls[0] != "ada@example.com|musician" {
		t.Errorf("calls = %v", svc.calls)
	}
}

func TestWaitlistValidationError(t *testing.T) {
	svc := &fakeWaitlist{err: &waitlist.ValidationError{Field: "email", Err: waitlist.ErrInvalidEmail}}
	rec := postWaitlist(t, newTestRouterWith(Options{Waitlist: svc}), `{"email":"nope"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", rec.Code)
	}
	d := decodeError(t, rec)
	if d.Code != CodeValidation || len(d.Fields) != 1 || d.Fields[0].Field != "email" || d.Fields[0].Message == "" {
		t.Errorf("detail = %+v", d)
	}
}

func TestWaitlistStoreFailureIsInternal(t *testing.T) {
	svc := &fakeWaitlist{err: errors.New("connection refused")}
	rec := postWaitlist(t, newTestRouterWith(Options{Waitlist: svc}), `{"email":"ada@example.com"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Error("internal error detail leaked to client")
	}
}

func TestWaitlistRejectsMalformedBodies(t *testing.T) {
	cases := map[string]struct {
		body   string
		status int
		code   string
	}{
		"not json":       {`email=ada@example.com`, http.StatusBadRequest, CodeInvalidRequest},
		"unknown field":  {`{"email":"a@b.co","admin":true}`, http.StatusBadRequest, CodeInvalidRequest},
		"two objects":    {`{"email":"a@b.co"}{"email":"c@d.co"}`, http.StatusBadRequest, CodeInvalidRequest},
		"wrong type":     {`{"email":42}`, http.StatusBadRequest, CodeInvalidRequest},
		"oversized body": {`{"email":"` + strings.Repeat("a", 5000) + `"}`, http.StatusRequestEntityTooLarge, CodePayloadTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeWaitlist{}
			rec := postWaitlist(t, newTestRouterWith(Options{Waitlist: svc}), tc.body)
			if rec.Code != tc.status || decodeError(t, rec).Code != tc.code {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
			if len(svc.calls) != 0 {
				t.Error("service called for malformed body")
			}
		})
	}
}

func TestWaitlistHoneypot(t *testing.T) {
	svc := &fakeWaitlist{}
	rec := postWaitlist(t, newTestRouterWith(Options{Waitlist: svc}),
		`{"email":"bot@example.com","company_website":"https://spam.example"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(svc.calls) != 0 {
		t.Error("honeypot submission reached the service")
	}
}

func TestWaitlistRateLimitedPerIP(t *testing.T) {
	router := newTestRouterWith(Options{Waitlist: &fakeWaitlist{}})
	send := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(`{"email":"a@b.co"}`))
		req.RemoteAddr = ip + ":1234"
		return do(t, router, req)
	}
	for i := range waitlistRateLimit {
		if rec := send("203.0.113.7"); rec.Code != http.StatusAccepted {
			t.Fatalf("request %d: status %d", i+1, rec.Code)
		}
	}
	rec := send("203.0.113.7")
	if rec.Code != http.StatusTooManyRequests || decodeError(t, rec).Code != CodeRateLimited {
		t.Fatalf("over limit: got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After")
	}
	if rec := send("198.51.100.9"); rec.Code != http.StatusAccepted {
		t.Errorf("other IP limited: %d", rec.Code)
	}
}

func TestWaitlistIgnoresSpoofedForwardedFor(t *testing.T) {
	router := newTestRouterWith(Options{Waitlist: &fakeWaitlist{}})
	for i := 0; i <= waitlistRateLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(`{"email":"a@b.co"}`))
		req.RemoteAddr = "203.0.113.7:1234"
		req.Header.Set("X-Forwarded-For", "10.0.0."+string(rune('0'+i%10)))
		rec := do(t, router, req)
		if i == waitlistRateLimit && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("spoofed X-Forwarded-For bypassed the limit: %d", rec.Code)
		}
	}
}

func TestFixedWindowLimiterResets(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newFixedWindowLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	for i := range 2 {
		if ok, _ := l.allow("k"); !ok {
			t.Fatalf("request %d denied", i+1)
		}
	}
	ok, retry := l.allow("k")
	if ok || retry != time.Minute {
		t.Fatalf("third request: ok=%v retry=%v", ok, retry)
	}

	now = now.Add(time.Minute)
	if ok, _ := l.allow("k"); !ok {
		t.Fatal("not reset after window")
	}
	if len(l.windows) != 1 {
		t.Errorf("expired windows not swept: %d", len(l.windows))
	}
}

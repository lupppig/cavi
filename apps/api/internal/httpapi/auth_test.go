package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/session"
)

const webURL = "http://localhost:3001"

type fakeProvider struct {
	lastState string
	authErr   error
	profile   identity.Profile
}

func (f *fakeProvider) AuthorizationURL(state string) (string, error) {
	f.lastState = state
	return "https://auth.example.com/authorize?state=" + url.QueryEscape(state), nil
}

func (f *fakeProvider) Authenticate(_ context.Context, code, _, _ string) (identity.Profile, error) {
	if f.authErr != nil {
		return identity.Profile{}, f.authErr
	}
	if code != "good-code" {
		return identity.Profile{}, identity.ErrAuthenticationFailed
	}
	return f.profile, nil
}

func (f *fakeProvider) LogoutURL(sid string) (string, error) {
	return "https://auth.example.com/logout?session_id=" + sid, nil
}

type fakeAccounts struct {
	users map[string]account.User // by provider user id
}

func (f *fakeAccounts) SignIn(_ context.Context, p identity.Profile) (account.User, error) {
	if f.users == nil {
		f.users = map[string]account.User{}
	}
	u, ok := f.users[p.ProviderUserID]
	if !ok {
		u = account.User{ID: "u-" + p.ProviderUserID, Email: p.Email, FirstName: p.FirstName}
		f.users[p.ProviderUserID] = u
	}
	return u, nil
}

func (f *fakeAccounts) Me(_ context.Context, userID string) (account.User, []account.Membership, error) {
	for _, u := range f.users {
		if u.ID == userID {
			return u, []account.Membership{{
				OrganisationID: "org-" + u.ID, OrganisationName: u.FirstName + "’s studio", Personal: true, Role: "owner",
				Workspaces: []account.Workspace{{ID: "ws-1", Name: "Main"}},
			}}, nil
		}
	}
	return account.User{}, nil, account.ErrUserNotFound
}

type fakeSessions struct {
	byToken map[string]session.Session
}

func (f *fakeSessions) Create(_ context.Context, userID, sid string) (string, time.Time, error) {
	if f.byToken == nil {
		f.byToken = map[string]session.Session{}
	}
	tok := "tok-" + userID
	f.byToken[tok] = session.Session{ID: "s1", UserID: userID, ProviderSessionID: sid}
	return tok, time.Now().Add(time.Hour), nil
}

func (f *fakeSessions) Lookup(_ context.Context, token string) (session.Session, error) {
	s, ok := f.byToken[token]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) Delete(_ context.Context, token string) (session.Session, error) {
	s, ok := f.byToken[token]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	delete(f.byToken, token)
	return s, nil
}

type authFixture struct {
	router   *gin.Engine
	provider *fakeProvider
	accounts *fakeAccounts
	sessions *fakeSessions
}

func newAuthFixture() *authFixture {
	f := &authFixture{
		provider: &fakeProvider{profile: identity.Profile{
			ProviderUserID: "user_ada", Email: "ada@example.com", FirstName: "Ada", ProviderSessionID: "session_workos",
		}},
		accounts: &fakeAccounts{},
		sessions: &fakeSessions{},
	}
	f.router = newTestRouterWith(Options{
		AllowedOrigins: []string{webURL},
		Auth: &AuthOptions{
			Provider: f.provider, Accounts: f.accounts, Sessions: f.sessions,
			WebURL: webURL, SecureCookies: true,
		},
	})
	return f
}

func cookieFrom(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// signIn runs login + callback and returns the session cookie.
func (f *authFixture) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	rec := do(t, f.router, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	state := cookieFrom(rec, stateCookie)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=good-code&state="+url.QueryEscape(state.Value), nil)
	req.AddCookie(state)
	rec = do(t, f.router, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != webURL+"/account" {
		t.Fatalf("callback: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	c := cookieFrom(rec, sessionCookie)
	if c == nil {
		t.Fatal("no session cookie")
	}
	return c
}

func TestLoginRedirectsWithStateCookie(t *testing.T) {
	f := newAuthFixture()
	rec := do(t, f.router, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	c := cookieFrom(rec, stateCookie)
	if c == nil || c.Value == "" || c.Value != f.provider.lastState {
		t.Fatalf("state cookie = %+v, provider state %q", c, f.provider.lastState)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/api/v1/auth" {
		t.Errorf("state cookie flags = %+v", c)
	}
	if !strings.HasPrefix(rec.Header().Get("Location"), "https://auth.example.com/authorize") {
		t.Errorf("location = %s", rec.Header().Get("Location"))
	}
}

func TestCallbackSuccessSetsSessionCookie(t *testing.T) {
	f := newAuthFixture()
	c := f.signIn(t)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge <= 0 {
		t.Errorf("session cookie flags = %+v", c)
	}
	if _, ok := f.accounts.users["user_ada"]; !ok {
		t.Error("account not created")
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	cases := map[string]func(*http.Request){
		"no state cookie": func(r *http.Request) {},
		"mismatched state": func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: stateCookie, Value: "other"})
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=good-code&state=attacker", nil)
			setup(req)
			rec := do(t, f.router, req)
			if loc := rec.Header().Get("Location"); loc != webURL+"/account?auth_error=invalid_state" {
				t.Errorf("location = %s", loc)
			}
			if cookieFrom(rec, sessionCookie) != nil {
				t.Error("session created despite bad state")
			}
			if len(f.accounts.users) != 0 {
				t.Error("account touched despite bad state")
			}
		})
	}
}

func TestCallbackProviderFailures(t *testing.T) {
	cases := map[string]struct {
		query   string
		authErr error
		want    string
	}{
		"user cancelled":   {"error=access_denied", nil, "cancelled"},
		"bad code":         {"code=bad-code", nil, "sign_in_failed"},
		"provider outage":  {"code=good-code", errors.New("timeout"), "sign_in_failed"},
		"missing the code": {"", nil, "sign_in_failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture()
			f.provider.authErr = tc.authErr
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?state=s&"+tc.query, nil)
			req.AddCookie(&http.Cookie{Name: stateCookie, Value: "s"})
			rec := do(t, f.router, req)
			if loc := rec.Header().Get("Location"); loc != webURL+"/account?auth_error="+tc.want {
				t.Errorf("location = %s", loc)
			}
			if cookieFrom(rec, sessionCookie) != nil {
				t.Error("session created on failure")
			}
		})
	}
}

func TestMeRequiresSession(t *testing.T) {
	f := newAuthFixture()
	rec := do(t, f.router, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized || decodeError(t, rec).Code != CodeUnauthenticated {
		t.Fatalf("no cookie: %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "forged"})
	rec = do(t, f.router, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie: %d", rec.Code)
	}
	if c := cookieFrom(rec, sessionCookie); c == nil || c.MaxAge >= 0 {
		t.Error("invalid session cookie not cleared")
	}
}

func TestMeReturnsAccount(t *testing.T) {
	f := newAuthFixture()
	c := f.signIn(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(c)
	rec := do(t, f.router, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	var body meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.User.Email != "ada@example.com" || len(body.Organisations) != 1 ||
		body.Organisations[0].Role != "owner" || len(body.Organisations[0].Workspaces) != 1 {
		t.Errorf("body = %+v", body)
	}
}

func TestLogout(t *testing.T) {
	f := newAuthFixture()
	c := f.signIn(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", webURL)
	req.AddCookie(c)
	rec := do(t, f.router, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body logoutResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.RedirectURL != "https://auth.example.com/logout?session_id=session_workos" {
		t.Errorf("redirect = %q", body.RedirectURL)
	}
	if cl := cookieFrom(rec, sessionCookie); cl == nil || cl.MaxAge >= 0 {
		t.Error("session cookie not cleared")
	}

	// The old cookie no longer works.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(c)
	if rec := do(t, f.router, req); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout: %d", rec.Code)
	}

	// Logging out without a session still succeeds and goes home.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", webURL)
	rec = do(t, f.router, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.RedirectURL != webURL+"/" {
		t.Errorf("anonymous logout: %d %q", rec.Code, body.RedirectURL)
	}
}

func TestLogoutRejectsCrossSiteRequests(t *testing.T) {
	f := newAuthFixture()
	c := f.signIn(t)
	for _, origin := range []string{"", "https://evil.example"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.AddCookie(c)
		rec := do(t, f.router, req)
		if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != CodeForbidden {
			t.Errorf("origin %q: %d", origin, rec.Code)
		}
	}
	if len(f.sessions.byToken) != 1 {
		t.Error("cross-site request ended the session")
	}
}

func TestAuthNotConfigured(t *testing.T) {
	r := newTestRouter(fakePinger{})
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/me"} {
		rec := do(t, r, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

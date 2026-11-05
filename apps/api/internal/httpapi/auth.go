package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/session"
)

const (
	sessionCookie = "cavi_session"
	stateCookie   = "cavi_oauth_state"
	stateTTL      = 10 * time.Minute
	userIDKey     = "user_id"
)

// AccountService is the account behaviour the auth handlers need.
type AccountService interface {
	SignIn(ctx context.Context, p identity.Profile) (account.User, error)
	Me(ctx context.Context, userID string) (account.User, []account.Membership, error)
}

// SessionStore is the session behaviour the auth handlers need.
type SessionStore interface {
	Create(ctx context.Context, userID, providerSessionID string) (string, time.Time, error)
	Lookup(ctx context.Context, token string) (session.Session, error)
	Delete(ctx context.Context, token string) (session.Session, error)
}

// AuthOptions configures sign-in. When nil, auth endpoints report that
// sign-in is not configured.
type AuthOptions struct {
	Provider identity.Provider
	Accounts AccountService
	Sessions SessionStore
	// WebURL is the frontend's base URL, e.g. https://app.example.com.
	WebURL string
	// SecureCookies sets the Secure flag; disable only for local http.
	SecureCookies bool
}

type authHandler struct {
	opts   *AuthOptions
	logger *slog.Logger
}

func (h authHandler) configured(c *gin.Context) bool {
	if h.opts == nil {
		writeError(c, http.StatusServiceUnavailable, CodeUnavailable, "Sign-in is not configured.")
		return false
	}
	return true
}

func (h authHandler) setCookie(c *gin.Context, name, value, path string, expires time.Time, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h authHandler) clearCookie(c *gin.Context, name, path string) {
	h.setCookie(c, name, "", path, time.Unix(0, 0), -1)
}

// webRedirect sends the browser to a frontend path, optionally with an
// auth_error code the frontend can explain.
func (h authHandler) webRedirect(c *gin.Context, path, authError string) {
	target := h.opts.WebURL + path
	if authError != "" {
		target += "?" + url.Values{"auth_error": {authError}}.Encode()
	}
	c.Redirect(http.StatusFound, target)
}

// login starts sign-in: it sets a one-time state cookie and redirects to
// Google (via the identity provider). Sign-in and sign-up are one flow.
func (h authHandler) login(c *gin.Context) {
	if !h.configured(c) {
		return
	}
	state, err := randomToken()
	if err != nil {
		writeError(c, http.StatusInternalServerError, CodeInternal, "Could not start sign-in.")
		return
	}
	target, err := h.opts.Provider.AuthorizationURL(state)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "authorization url failed",
			slog.String("request_id", requestIDFrom(c)), slog.String("error", err.Error()))
		writeError(c, http.StatusInternalServerError, CodeInternal, "Could not start sign-in.")
		return
	}
	h.setCookie(c, stateCookie, state, "/api/v1/auth", time.Now().Add(stateTTL), int(stateTTL.Seconds()))
	c.Redirect(http.StatusFound, target)
}

// callback completes sign-in and redirects the browser back to the app.
// Failures redirect too, with an error code, because this is a browser
// navigation rather than an API call.
func (h authHandler) callback(c *gin.Context) {
	if !h.configured(c) {
		return
	}
	ctx := c.Request.Context()
	log := func(msg string, err error) {
		h.logger.WarnContext(ctx, msg, slog.String("request_id", requestIDFrom(c)), slog.String("error", err.Error()))
	}

	expected, _ := c.Cookie(stateCookie)
	h.clearCookie(c, stateCookie, "/api/v1/auth")

	if c.Query("error") != "" {
		h.webRedirect(c, "/account", "cancelled")
		return
	}
	got := c.Query("state")
	if expected == "" || got == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(got)) != 1 {
		h.webRedirect(c, "/account", "invalid_state")
		return
	}
	code := c.Query("code")
	if code == "" {
		h.webRedirect(c, "/account", "sign_in_failed")
		return
	}

	profile, err := h.opts.Provider.Authenticate(ctx, code, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		log("authenticate failed", err)
		h.webRedirect(c, "/account", "sign_in_failed")
		return
	}
	user, err := h.opts.Accounts.SignIn(ctx, profile)
	if err != nil {
		log("account sign-in failed", err)
		h.webRedirect(c, "/account", "sign_in_failed")
		return
	}
	token, expires, err := h.opts.Sessions.Create(ctx, user.ID, profile.ProviderSessionID)
	if err != nil {
		log("create session failed", err)
		h.webRedirect(c, "/account", "sign_in_failed")
		return
	}
	h.setCookie(c, sessionCookie, token, "/", expires, int(time.Until(expires).Seconds()))
	h.logger.InfoContext(ctx, "signed in", slog.String("request_id", requestIDFrom(c)), slog.String("user_id", user.ID))
	h.webRedirect(c, "/account", "")
}

type logoutResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// logout ends the CAVI session and returns where to send the browser so the
// identity provider's session ends too.
func (h authHandler) logout(c *gin.Context) {
	if !h.configured(c) {
		return
	}
	redirect := h.opts.WebURL + "/"
	if token, err := c.Cookie(sessionCookie); err == nil {
		s, err := h.opts.Sessions.Delete(c.Request.Context(), token)
		switch {
		case err == nil && s.ProviderSessionID != "":
			if u, err := h.opts.Provider.LogoutURL(s.ProviderSessionID); err == nil {
				redirect = u
			}
		case err != nil && !errors.Is(err, session.ErrNotFound):
			h.logger.ErrorContext(c.Request.Context(), "delete session failed",
				slog.String("request_id", requestIDFrom(c)), slog.String("error", err.Error()))
			writeError(c, http.StatusInternalServerError, CodeInternal, "Could not sign out. Please try again.")
			return
		}
	}
	h.clearCookie(c, sessionCookie, "/")
	c.JSON(http.StatusOK, logoutResponse{RedirectURL: redirect})
}

type meResponse struct {
	User          meUser           `json:"user"`
	Organisations []meOrganisation `json:"organisations"`
}

type meUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url"`
}

type meOrganisation struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Personal   bool          `json:"personal"`
	Role       string        `json:"role"`
	Workspaces []meWorkspace `json:"workspaces"`
}

type meWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// me returns the signed-in user and their organisations.
func (h authHandler) me(c *gin.Context) {
	u, ms, err := h.opts.Accounts.Me(c.Request.Context(), c.GetString(userIDKey))
	if errors.Is(err, account.ErrUserNotFound) {
		h.clearCookie(c, sessionCookie, "/")
		writeError(c, http.StatusUnauthorized, CodeUnauthenticated, "You are not signed in.")
		return
	}
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "load account failed",
			slog.String("request_id", requestIDFrom(c)), slog.String("error", err.Error()))
		writeError(c, http.StatusInternalServerError, CodeInternal, "Could not load your account.")
		return
	}
	res := meResponse{
		User:          meUser{ID: u.ID, Email: u.Email, FirstName: u.FirstName, LastName: u.LastName, AvatarURL: u.AvatarURL},
		Organisations: make([]meOrganisation, 0, len(ms)),
	}
	for _, m := range ms {
		org := meOrganisation{ID: m.OrganisationID, Name: m.OrganisationName, Personal: m.Personal, Role: m.Role, Workspaces: make([]meWorkspace, 0, len(m.Workspaces))}
		for _, w := range m.Workspaces {
			org.Workspaces = append(org.Workspaces, meWorkspace{ID: w.ID, Name: w.Name})
		}
		res.Organisations = append(res.Organisations, org)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, res)
}

// requireSession resolves the session cookie to a user, or responds 401.
func (h authHandler) requireSession(c *gin.Context) {
	if !h.configured(c) {
		return
	}
	token, err := c.Cookie(sessionCookie)
	if err != nil || token == "" {
		writeError(c, http.StatusUnauthorized, CodeUnauthenticated, "You are not signed in.")
		return
	}
	s, err := h.opts.Sessions.Lookup(c.Request.Context(), token)
	if errors.Is(err, session.ErrNotFound) {
		h.clearCookie(c, sessionCookie, "/")
		writeError(c, http.StatusUnauthorized, CodeUnauthenticated, "Your session has expired. Please sign in again.")
		return
	}
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "session lookup failed",
			slog.String("request_id", requestIDFrom(c)), slog.String("error", err.Error()))
		writeError(c, http.StatusInternalServerError, CodeInternal, "Could not verify your session.")
		return
	}
	c.Set(userIDKey, s.UserID)
	c.Next()
}

// requireAllowedOrigin protects cookie-authenticated state changes from
// cross-site requests: the browser's Origin must be on the allowlist.
func requireAllowedOrigin(allowed []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !slices.Contains(allowed, c.GetHeader("Origin")) {
			writeError(c, http.StatusForbidden, CodeForbidden, "This request must come from the CAVI app.")
			return
		}
		c.Next()
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

package identity

import (
	"encoding/base64"
	"errors"
	"net/url"
	"testing"

	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

func jwt(payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"RS256"}`)) + "." + enc([]byte(payload)) + ".sig"
}

func TestSessionIDFromAccessToken(t *testing.T) {
	cases := map[string]struct {
		token string
		want  string
	}{
		"valid":         {jwt(`{"sid":"session_123","sub":"user_1"}`), "session_123"},
		"missing claim": {jwt(`{"sub":"user_1"}`), ""},
		"not a jwt":     {"abc", ""},
		"bad base64":    {"a.%%%.c", ""},
		"bad json":      {jwt(`not json`), ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := sessionIDFromAccessToken(tc.token); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthorizationURL(t *testing.T) {
	w := NewWorkOS("sk_test_x", "client_abc", "http://localhost:8080/api/v1/auth/callback", "http://localhost:3001/")
	raw, err := w.AuthorizationURL("state-xyz")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"client_id":     "client_abc",
		"redirect_uri":  "http://localhost:8080/api/v1/auth/callback",
		"provider":      "GoogleOAuth",
		"state":         "state-xyz",
		"response_type": "code",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if q.Get("client_secret") != "" || q.Get("api_key") != "" {
		t.Error("secret leaked into authorization URL")
	}
}

func TestProfileFromAcceptsOnlyGoogle(t *testing.T) {
	user := usermanagement.User{ID: "user_1", Email: "a@example.com", EmailVerified: true, FirstName: "Ana"}
	p, err := profileFrom(usermanagement.AuthenticateResponse{User: user, AuthenticationMethod: usermanagement.GoogleOAuth})
	if err != nil || p.ProviderUserID != "user_1" || !p.EmailVerified || p.FirstName != "Ana" {
		t.Fatalf("google: %+v, %v", p, err)
	}
	for _, m := range []usermanagement.AuthenticationMethod{
		usermanagement.Password, usermanagement.MagicAuthAuthenticationMethod, usermanagement.GitHubOAuth,
		usermanagement.SSO, usermanagement.Impersonation, "",
	} {
		if _, err := profileFrom(usermanagement.AuthenticateResponse{User: user, AuthenticationMethod: m}); !errors.Is(err, ErrAuthenticationFailed) {
			t.Errorf("method %q: err = %v, want ErrAuthenticationFailed", m, err)
		}
	}
	if _, err := profileFrom(usermanagement.AuthenticateResponse{AuthenticationMethod: usermanagement.GoogleOAuth}); !errors.Is(err, ErrAuthenticationFailed) {
		t.Errorf("missing user: err = %v", err)
	}
}

func TestLogoutURL(t *testing.T) {
	w := NewWorkOS("sk_test_x", "client_abc", "http://cb", "http://localhost:3001/")
	raw, err := w.LogoutURL("session_123")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	if u.Query().Get("session_id") != "session_123" || u.Query().Get("return_to") != "http://localhost:3001/" {
		t.Errorf("logout url = %s", raw)
	}
	if _, err := w.LogoutURL(""); err == nil {
		t.Error("expected error for empty session id")
	}
}

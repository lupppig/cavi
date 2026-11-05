package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/workos/workos-go/v6/pkg/usermanagement"
	"github.com/workos/workos-go/v6/pkg/workos_errors"
)

// WorkOS implements Provider with WorkOS AuthKit.
type WorkOS struct {
	client      *usermanagement.Client
	clientID    string
	redirectURI string
	returnTo    string
}

// NewWorkOS returns a WorkOS-backed Provider.
//
// redirectURI is CAVI's callback (registered in the WorkOS dashboard) and
// returnTo is where users land after signing out (also registered there).
func NewWorkOS(apiKey, clientID, redirectURI, returnTo string) *WorkOS {
	return &WorkOS{
		client:      usermanagement.NewClient(apiKey),
		clientID:    clientID,
		redirectURI: redirectURI,
		returnTo:    returnTo,
	}
}

// CAVI accepts Google sign-in only. Sign-in and sign-up are the same flow:
// the first Google sign-in creates the account.
const signInMethod = usermanagement.GoogleOAuth

// AuthorizationURL implements Provider. It sends the browser straight to
// Google (through WorkOS) rather than to the AuthKit hosted page.
func (w *WorkOS) AuthorizationURL(state string) (string, error) {
	u, err := w.client.GetAuthorizationURL(usermanagement.GetAuthorizationURLOpts{
		ClientID:    w.clientID,
		RedirectURI: w.redirectURI,
		Provider:    string(signInMethod),
		State:       state,
	})
	if err != nil {
		return "", fmt.Errorf("workos authorization url: %w", err)
	}
	return u.String(), nil
}

// Authenticate implements Provider.
func (w *WorkOS) Authenticate(ctx context.Context, code, ipAddress, userAgent string) (Profile, error) {
	res, err := w.client.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID:  w.clientID,
		Code:      code,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	})
	if err != nil {
		var httpErr workos_errors.HTTPError
		if errors.As(err, &httpErr) && httpErr.Code >= http.StatusBadRequest && httpErr.Code < http.StatusInternalServerError {
			return Profile{}, fmt.Errorf("%w: %s", ErrAuthenticationFailed, httpErr.Message)
		}
		return Profile{}, fmt.Errorf("workos authenticate: %w", err)
	}
	return profileFrom(res)
}

// profileFrom validates an authenticate response. The AuthKit hosted page
// can be reached without going through CAVI's login URL, so the method is
// checked here too: anything other than Google is refused.
func profileFrom(res usermanagement.AuthenticateResponse) (Profile, error) {
	if res.AuthenticationMethod != signInMethod {
		return Profile{}, fmt.Errorf("%w: unsupported sign-in method %q", ErrAuthenticationFailed, res.AuthenticationMethod)
	}
	if res.User.ID == "" || res.User.Email == "" {
		return Profile{}, fmt.Errorf("%w: response missing user", ErrAuthenticationFailed)
	}
	return Profile{
		ProviderUserID:    res.User.ID,
		Email:             res.User.Email,
		EmailVerified:     res.User.EmailVerified,
		FirstName:         res.User.FirstName,
		LastName:          res.User.LastName,
		AvatarURL:         res.User.ProfilePictureURL,
		ProviderSessionID: sessionIDFromAccessToken(res.AccessToken),
	}, nil
}

// LogoutURL implements Provider.
func (w *WorkOS) LogoutURL(providerSessionID string) (string, error) {
	u, err := w.client.GetLogoutURL(usermanagement.GetLogoutURLOpts{
		SessionID: providerSessionID,
		ReturnTo:  w.returnTo,
	})
	if err != nil {
		return "", fmt.Errorf("workos logout url: %w", err)
	}
	return u.String(), nil
}

// sessionIDFromAccessToken reads the `sid` claim from a WorkOS access token.
//
// The token comes straight from WorkOS's authenticate endpoint over TLS in
// a server-to-server call, and the claim is only used to build the WorkOS
// logout URL, so the signature is not verified here. Never use this to make
// authorisation decisions. Returns "" if the claim cannot be read.
func sessionIDFromAccessToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.SID
}

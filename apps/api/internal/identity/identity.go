// Package identity is CAVI's boundary with its identity provider (WorkOS).
//
// The provider answers one question: who is this person? Everything about
// what they may access lives in CAVI's own database (see package account).
// Nothing outside this package should import the WorkOS SDK.
package identity

import (
	"context"
	"errors"
)

// Screen hints for the hosted sign-in page.
const (
	ScreenSignIn = "sign-in"
	ScreenSignUp = "sign-up"
)

// ErrAuthenticationFailed means the provider rejected the authorization
// code (expired, reused, or forged).
var ErrAuthenticationFailed = errors.New("identity: authentication failed")

// Profile is a verified identity returned after sign-in.
type Profile struct {
	ProviderUserID    string
	Email             string
	FirstName         string
	LastName          string
	AvatarURL         string
	ProviderSessionID string
}

// Provider is the identity provider used for sign-in and sign-out.
type Provider interface {
	// AuthorizationURL returns the hosted sign-in URL. state is echoed back
	// to the callback and must be verified there.
	AuthorizationURL(state, screenHint string) (string, error)
	// Authenticate exchanges an authorization code for a verified profile.
	Authenticate(ctx context.Context, code, ipAddress, userAgent string) (Profile, error)
	// LogoutURL ends the provider's session and then returns the user to CAVI.
	LogoutURL(providerSessionID string) (string, error)
}

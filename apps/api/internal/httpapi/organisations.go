package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
)

// OrganisationService is the organisation behaviour the handlers need.
// Every method authorises the actor itself.
type OrganisationService interface {
	CreateOrganisation(ctx context.Context, actorID, name string) (account.Organisation, error)
	Organisation(ctx context.Context, actorID, orgID string) (account.Organisation, error)
	RenameOrganisation(ctx context.Context, actorID, orgID, name string) (account.Organisation, error)
	Members(ctx context.Context, actorID, orgID string) ([]account.Member, error)
	ChangeRole(ctx context.Context, actorID, orgID, targetID, role string) error
	RemoveMember(ctx context.Context, actorID, orgID, targetID string) error
	CreateInvitation(ctx context.Context, actorID, orgID, email, role string) (account.Invitation, error)
	Invitations(ctx context.Context, actorID, orgID string) ([]account.Invitation, error)
	RevokeInvitation(ctx context.Context, actorID, orgID, invitationID string) error
	PendingInvitations(ctx context.Context, userID string) ([]account.Invitation, error)
	AcceptInvitation(ctx context.Context, userID, invitationID string) (account.Organisation, error)
	DeclineInvitation(ctx context.Context, userID, invitationID string) error
}

type orgHandler struct {
	svc    OrganisationService
	logger *slog.Logger
}

// ---- Response shapes ----

type organisationResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Personal    bool     `json:"personal"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"created_at"`
}

type memberResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
	JoinedAt  string `json:"joined_at"`
}

type invitationResponse struct {
	ID               string `json:"id"`
	OrganisationID   string `json:"organisation_id"`
	OrganisationName string `json:"organisation_name"`
	Email            string `json:"email"`
	Role             string `json:"role"`
	InvitedByName    string `json:"invited_by_name"`
	CreatedAt        string `json:"created_at"`
	ExpiresAt        string `json:"expires_at"`
}

type listResponse[T any] struct {
	Data []T `json:"data"`
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func toOrgResponse(o account.Organisation) organisationResponse {
	perms := make([]string, len(o.Permissions))
	for i, p := range o.Permissions {
		perms[i] = string(p)
	}
	return organisationResponse{ID: o.ID, Name: o.Name, Personal: o.Personal, Role: o.Role, Permissions: perms, CreatedAt: ts(o.CreatedAt)}
}

func toInvitationResponses(in []account.Invitation) listResponse[invitationResponse] {
	out := make([]invitationResponse, len(in))
	for i, v := range in {
		out[i] = invitationResponse{
			ID: v.ID, OrganisationID: v.OrganisationID, OrganisationName: v.OrganisationName,
			Email: v.Email, Role: v.Role, InvitedByName: v.InvitedByName,
			CreatedAt: ts(v.CreatedAt), ExpiresAt: ts(v.ExpiresAt),
		}
	}
	return listResponse[invitationResponse]{Data: out}
}

// ---- Helpers ----

const maxJSONBody = 16 << 10

// decodeJSON strictly decodes a single JSON object into dst, writing an
// error response and returning false on failure.
func decodeJSON(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBody)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(c, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "The request body is too large.")
		} else {
			writeError(c, http.StatusBadRequest, CodeInvalidRequest, "The request body must be a valid JSON object.")
		}
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, CodeInvalidRequest, "The request body must contain a single JSON object.")
		return false
	}
	return true
}

// fail maps account errors to HTTP responses.
func (h orgHandler) fail(c *gin.Context, err error) {
	var verr *account.ValidationError
	switch {
	case errors.As(err, &verr):
		writeErrorDetail(c, http.StatusUnprocessableEntity, ErrorDetail{
			Code: CodeValidation, Message: "Some fields need attention.",
			Fields: []FieldError{{Field: verr.Field, Message: verr.Message}},
		})
	case errors.Is(err, account.ErrNotFound):
		writeError(c, http.StatusNotFound, CodeNotFound, "The requested resource does not exist.")
	case errors.Is(err, account.ErrForbidden):
		writeError(c, http.StatusForbidden, CodeForbidden, "You don’t have permission to do that.")
	case errors.Is(err, account.ErrEmailNotVerified):
		writeError(c, http.StatusForbidden, CodeEmailNotVerified, "Verify your email address to accept this invitation.")
	case errors.Is(err, account.ErrLastOwner):
		writeError(c, http.StatusConflict, CodeLastOwner, "An organisation needs at least one owner. Make someone else an owner first.")
	case errors.Is(err, account.ErrPersonalOrg):
		writeError(c, http.StatusConflict, CodePersonalOrganisation, "Personal organisations can’t have other members. Create a team instead.")
	case errors.Is(err, account.ErrAlreadyMember):
		writeError(c, http.StatusConflict, CodeAlreadyMember, "That person is already a member.")
	case errors.Is(err, account.ErrInvitationExists):
		writeError(c, http.StatusConflict, CodeInvitationExists, "There’s already a pending invitation for that email.")
	default:
		h.logger.ErrorContext(c.Request.Context(), "organisation request failed",
			slog.String("request_id", requestIDFrom(c)), slog.String("error", err.Error()))
		writeError(c, http.StatusInternalServerError, CodeInternal, "Something went wrong. Please try again.")
	}
}

func actor(c *gin.Context) string { return c.GetString(userIDKey) }

// requireSameOriginWrites applies the Origin allowlist to state-changing
// methods on cookie-authenticated routes.
func requireSameOriginWrites(allowed []string) gin.HandlerFunc {
	check := requireAllowedOrigin(allowed)
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
		default:
			check(c)
		}
	}
}

// ---- Handlers ----

type nameRequest struct {
	Name string `json:"name"`
}

func (h orgHandler) create(c *gin.Context) {
	var req nameRequest
	if !decodeJSON(c, &req) {
		return
	}
	org, err := h.svc.CreateOrganisation(c.Request.Context(), actor(c), req.Name)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, toOrgResponse(org))
}

func (h orgHandler) get(c *gin.Context) {
	org, err := h.svc.Organisation(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toOrgResponse(org))
}

func (h orgHandler) rename(c *gin.Context) {
	var req nameRequest
	if !decodeJSON(c, &req) {
		return
	}
	org, err := h.svc.RenameOrganisation(c.Request.Context(), actor(c), c.Param("orgID"), req.Name)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toOrgResponse(org))
}

func (h orgHandler) members(c *gin.Context) {
	ms, err := h.svc.Members(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]memberResponse, len(ms))
	for i, m := range ms {
		out[i] = memberResponse{UserID: m.UserID, Email: m.Email, FirstName: m.FirstName, LastName: m.LastName,
			AvatarURL: m.AvatarURL, Role: m.Role, JoinedAt: ts(m.JoinedAt)}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, listResponse[memberResponse]{Data: out})
}

type roleRequest struct {
	Role string `json:"role"`
}

func (h orgHandler) changeRole(c *gin.Context) {
	var req roleRequest
	if !decodeJSON(c, &req) {
		return
	}
	if err := h.svc.ChangeRole(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("userID"), req.Role); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h orgHandler) removeMember(c *gin.Context) {
	if err := h.svc.RemoveMember(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("userID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h orgHandler) invite(c *gin.Context) {
	var req inviteRequest
	if !decodeJSON(c, &req) {
		return
	}
	if req.Role == "" {
		req.Role = authz.RoleEditor
	}
	inv, err := h.svc.CreateInvitation(c.Request.Context(), actor(c), c.Param("orgID"), req.Email, req.Role)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, toInvitationResponses([]account.Invitation{inv}).Data[0])
}

func (h orgHandler) invitations(c *gin.Context) {
	invs, err := h.svc.Invitations(c.Request.Context(), actor(c), c.Param("orgID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toInvitationResponses(invs))
}

func (h orgHandler) revoke(c *gin.Context) {
	if err := h.svc.RevokeInvitation(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("invitationID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h orgHandler) myInvitations(c *gin.Context) {
	invs, err := h.svc.PendingInvitations(c.Request.Context(), actor(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toInvitationResponses(invs))
}

func (h orgHandler) accept(c *gin.Context) {
	org, err := h.svc.AcceptInvitation(c.Request.Context(), actor(c), c.Param("invitationID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toOrgResponse(org))
}

func (h orgHandler) decline(c *gin.Context) {
	if err := h.svc.DeclineInvitation(c.Request.Context(), actor(c), c.Param("invitationID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

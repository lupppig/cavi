package httpapi

import (
	"github.com/gin-gonic/gin"
)

// ErrorResponse is the error envelope returned by every API endpoint.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes a single API error.
type ErrorDetail struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
}

// FieldError describes why one request field was rejected.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Stable error codes clients may branch on.
const (
	CodeInvalidRequest   = "invalid_request"
	CodeValidation       = "validation_failed"
	CodePayloadTooLarge  = "payload_too_large"
	CodeRateLimited      = "rate_limited"
	CodeUnauthenticated  = "unauthenticated"
	CodeForbidden        = "forbidden"
	CodeEmailNotVerified = "email_not_verified"
	// Conflicts with organisation rules.
	CodeLastOwner            = "last_owner"
	CodePersonalOrganisation = "personal_organisation"
	CodeAlreadyMember        = "already_member"
	CodeInvitationExists     = "invitation_exists"
	CodeNotFound             = "not_found"
	CodeMethodNotAllowed     = "method_not_allowed"
	CodeInternal             = "internal_error"
	CodeUnavailable          = "service_unavailable"
)

func writeError(c *gin.Context, status int, code, message string) {
	writeErrorDetail(c, status, ErrorDetail{Code: code, Message: message})
}

func writeErrorDetail(c *gin.Context, status int, detail ErrorDetail) {
	detail.RequestID = requestIDFrom(c)
	c.AbortWithStatusJSON(status, ErrorResponse{Error: detail})
}

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
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// Stable error codes clients may branch on.
const (
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeInternal         = "internal_error"
	CodeUnavailable      = "service_unavailable"
)

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		RequestID: requestIDFrom(c),
	}})
}

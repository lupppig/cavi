package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/waitlist"
)

// WaitlistService is the application behaviour the waitlist handler needs.
type WaitlistService interface {
	Join(ctx context.Context, email, discipline string) (waitlist.JoinResult, error)
}

const maxWaitlistBody = 4 << 10 // 4 KiB

type joinWaitlistRequest struct {
	Email      string `json:"email"`
	Discipline string `json:"discipline"`
	// CompanyWebsite is a honeypot. It is hidden from people in the web
	// form; automated submissions that fill it are accepted but discarded.
	CompanyWebsite string `json:"company_website"`
}

type joinWaitlistResponse struct {
	Status string `json:"status"`
}

type waitlistHandler struct {
	svc    WaitlistService
	logger *slog.Logger
}

// join adds an address to the waitlist. The response is identical whether
// or not the address was already present, so the endpoint cannot be used
// to discover who has signed up.
func (h waitlistHandler) join(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWaitlistBody)

	var req joinWaitlistRequest
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(c, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "The request body is too large.")
			return
		}
		writeError(c, http.StatusBadRequest, CodeInvalidRequest, "The request body must be a JSON object with an email field.")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, CodeInvalidRequest, "The request body must contain a single JSON object.")
		return
	}

	if req.CompanyWebsite != "" {
		h.logger.InfoContext(c.Request.Context(), "waitlist honeypot triggered",
			slog.String("request_id", requestIDFrom(c)))
		c.JSON(http.StatusAccepted, joinWaitlistResponse{Status: "joined"})
		return
	}

	res, err := h.svc.Join(c.Request.Context(), req.Email, req.Discipline)
	if err != nil {
		var verr *waitlist.ValidationError
		if errors.As(err, &verr) {
			writeErrorDetail(c, http.StatusUnprocessableEntity, ErrorDetail{
				Code:    CodeValidation,
				Message: "Some fields need attention.",
				Fields:  []FieldError{{Field: verr.Field, Message: verr.Err.Error()}},
			})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "waitlist join failed",
			slog.String("request_id", requestIDFrom(c)),
			slog.String("error", err.Error()))
		writeError(c, http.StatusInternalServerError, CodeInternal, "We couldn't save your signup. Please try again.")
		return
	}

	// Never log the address itself.
	h.logger.InfoContext(c.Request.Context(), "waitlist signup",
		slog.String("request_id", requestIDFrom(c)),
		slog.Bool("new", res.Created))
	c.JSON(http.StatusAccepted, joinWaitlistResponse{Status: "joined"})
}

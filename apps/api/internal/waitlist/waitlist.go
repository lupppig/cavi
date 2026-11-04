// Package waitlist records people who want early access to CAVI.
package waitlist

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// Discipline is how a person describes their creative work.
type Discipline string

// Supported disciplines. Keep in sync with the database check constraint
// and the OpenAPI contract.
const (
	DisciplineClipper  Discipline = "clipper"
	DisciplineCreator  Discipline = "creator"
	DisciplineMusician Discipline = "musician"
	DisciplineStudio   Discipline = "studio"
	DisciplineAgency   Discipline = "agency"
	DisciplineBrand    Discipline = "brand"
	DisciplineOther    Discipline = "other"
)

func (d Discipline) valid() bool {
	switch d {
	case "", DisciplineClipper, DisciplineCreator, DisciplineMusician,
		DisciplineStudio, DisciplineAgency, DisciplineBrand, DisciplineOther:
		return true
	}
	return false
}

// Validation errors returned by Join.
var (
	ErrInvalidEmail      = errors.New("enter a valid email address")
	ErrInvalidDiscipline = errors.New("choose one of the listed options")
)

// ValidationError reports which input field failed validation.
type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %v", e.Field, e.Err) }
func (e *ValidationError) Unwrap() error { return e.Err }

// Entry is a validated waitlist signup ready to persist.
type Entry struct {
	Email           string
	EmailNormalized string
	Discipline      Discipline
}

// Store persists waitlist entries.
type Store interface {
	// Insert stores the entry. It returns created=false, without error,
	// when the normalised email is already on the waitlist.
	Insert(ctx context.Context, e Entry) (created bool, err error)
}

// Service is the waitlist application service.
type Service struct {
	store Store
}

// NewService returns a waitlist service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// JoinResult describes the outcome of a join request.
type JoinResult struct {
	// Created is false when the address was already on the list.
	Created bool
}

// Join validates and records a signup. Joining twice is not an error.
func (s *Service) Join(ctx context.Context, email, discipline string) (JoinResult, error) {
	entry, err := newEntry(email, discipline)
	if err != nil {
		return JoinResult{}, err
	}
	created, err := s.store.Insert(ctx, entry)
	if err != nil {
		return JoinResult{}, fmt.Errorf("store waitlist entry: %w", err)
	}
	return JoinResult{Created: created}, nil
}

func newEntry(email, discipline string) (Entry, error) {
	email = strings.TrimSpace(email)
	if !validEmail(email) {
		return Entry{}, &ValidationError{Field: "email", Err: ErrInvalidEmail}
	}
	d := Discipline(strings.TrimSpace(discipline))
	if !d.valid() {
		return Entry{}, &ValidationError{Field: "discipline", Err: ErrInvalidDiscipline}
	}
	return Entry{
		Email:           email,
		EmailNormalized: strings.ToLower(email),
		Discipline:      d,
	}, nil
}

// validEmail accepts a bare addr-spec ("name@example.com") with a dotted
// domain. Display names, comments and angle brackets are rejected.
func validEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return false
	}
	at := strings.LastIndexByte(email, '@')
	domain := email[at+1:]
	return at > 0 && strings.Contains(domain, ".") &&
		!strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

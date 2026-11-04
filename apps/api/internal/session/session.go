// Package session stores browser sessions server-side.
//
// The browser holds an opaque random token in an HttpOnly cookie; the
// database stores only its SHA-256 hash, so a database leak does not leak
// usable sessions.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// ErrNotFound means the token is unknown, malformed or expired.
var ErrNotFound = errors.New("session: not found")

// Session is an active sign-in.
type Session struct {
	ID                string
	UserID            string
	ProviderSessionID string
	ExpiresAt         time.Time
}

// Store persists sessions.
type Store struct {
	db  bun.IDB
	ttl time.Duration
}

// NewStore returns a session store whose sessions last ttl.
func NewStore(db bun.IDB, ttl time.Duration) *Store {
	return &Store{db: db, ttl: ttl}
}

// Create starts a session and returns the token to give the browser.
func (s *Store) Create(ctx context.Context, userID, providerSessionID string) (token string, expiresAt time.Time, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	expiresAt = time.Now().Add(s.ttl).UTC().Truncate(time.Second)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, workos_session_id, expires_at)
		VALUES (?, ?, NULLIF(?, ''), ?)`,
		hash(token), userID, providerSessionID, expiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert session: %w", err)
	}
	return token, expiresAt, nil
}

// Lookup returns the unexpired session for token.
func (s *Store) Lookup(ctx context.Context, token string) (Session, error) {
	if !wellFormed(token) {
		return Session{}, ErrNotFound
	}
	var out Session
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, coalesce(workos_session_id, ''), expires_at
		FROM sessions
		WHERE token_hash = ? AND expires_at > now()`, hash(token)).
		Scan(&out.ID, &out.UserID, &out.ProviderSessionID, &out.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}
	return out, nil
}

// Delete ends the session for token and returns what was deleted.
func (s *Store) Delete(ctx context.Context, token string) (Session, error) {
	if !wellFormed(token) {
		return Session{}, ErrNotFound
	}
	var out Session
	err := s.db.QueryRowContext(ctx, `
		DELETE FROM sessions WHERE token_hash = ?
		RETURNING id, user_id, coalesce(workos_session_id, ''), expires_at`, hash(token)).
		Scan(&out.ID, &out.UserID, &out.ProviderSessionID, &out.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("delete session: %w", err)
	}
	return out, nil
}

// DeleteExpired removes expired sessions and reports how many were removed.
func (s *Store) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return res.RowsAffected()
}

func hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// wellFormed rejects obviously invalid tokens before touching the database.
func wellFormed(token string) bool {
	if len(token) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}

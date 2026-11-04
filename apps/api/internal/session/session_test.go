package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

func newUser(t *testing.T, store *Store) string {
	t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	var userID string
	if err := store.db.QueryRowContext(context.Background(),
		`INSERT INTO users (workos_user_id, email) VALUES (?, ?) RETURNING id`,
		"user_test_"+id, "s-"+id+"@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})
	return userID
}

func TestCreateLookupDelete(t *testing.T) {
	store := NewStore(testdb.Open(t), time.Hour)
	userID := newUser(t, store)
	ctx := context.Background()

	token, expires, err := store.Create(ctx, userID, "session_workos_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || time.Until(expires) < 59*time.Minute {
		t.Fatalf("token=%q expires=%v", token, expires)
	}

	var stored []byte
	_ = store.db.QueryRowContext(ctx, `SELECT token_hash FROM sessions WHERE user_id = ?`, userID).Scan(&stored)
	if string(stored) == token {
		t.Fatal("raw token stored in database")
	}

	s, err := store.Lookup(ctx, token)
	if err != nil || s.UserID != userID || s.ProviderSessionID != "session_workos_1" {
		t.Fatalf("lookup = %+v, %v", s, err)
	}

	deleted, err := store.Delete(ctx, token)
	if err != nil || deleted.ID != s.ID {
		t.Fatalf("delete = %+v, %v", deleted, err)
	}
	if _, err := store.Lookup(ctx, token); err != ErrNotFound {
		t.Errorf("lookup after delete: %v", err)
	}
	if _, err := store.Delete(ctx, token); err != ErrNotFound {
		t.Errorf("second delete: %v", err)
	}
}

func TestExpiredSessionsAreRejectedAndSwept(t *testing.T) {
	store := NewStore(testdb.Open(t), -time.Minute) // already expired
	userID := newUser(t, store)
	ctx := context.Background()

	token, _, err := store.Create(ctx, userID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup(ctx, token); err != ErrNotFound {
		t.Errorf("expired lookup: %v", err)
	}
	n, err := store.DeleteExpired(ctx)
	if err != nil || n < 1 {
		t.Errorf("DeleteExpired = %d, %v", n, err)
	}
}

func TestMalformedTokens(t *testing.T) {
	store := NewStore(nil, time.Hour) // never reaches the database
	for _, tok := range []string{"", "short", "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"} {
		if _, err := store.Lookup(context.Background(), tok); err != ErrNotFound {
			t.Errorf("Lookup(%q) = %v", tok, err)
		}
		if _, err := store.Delete(context.Background(), tok); err != ErrNotFound {
			t.Errorf("Delete(%q) = %v", tok, err)
		}
	}
}

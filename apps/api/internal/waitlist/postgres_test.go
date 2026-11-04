package waitlist

import (
	"context"
	"os"
	"testing"

	"github.com/lupppig/cavi/apps/api/internal/database"
)

// TestPostgresStore runs against a real, migrated database. It is skipped
// unless CAVI_TEST_DATABASE_URL is set (see `make api-test-integration`).
func TestPostgresStore(t *testing.T) {
	dsn := os.Getenv("CAVI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CAVI_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db := database.Open(dsn)
	t.Cleanup(func() { _ = db.Close() })

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	store := NewPostgresStore(tx)
	entry := Entry{Email: "Ada@Example.com", EmailNormalized: "ada@example.com", Discipline: DisciplineMusician}

	created, err := store.Insert(ctx, entry)
	if err != nil || !created {
		t.Fatalf("first insert: created=%v err=%v", created, err)
	}

	created, err = store.Insert(ctx, Entry{Email: "ada@example.com", EmailNormalized: "ada@example.com"})
	if err != nil || created {
		t.Fatalf("duplicate insert: created=%v err=%v", created, err)
	}

	var discipline *string
	if err := tx.NewSelect().Table("waitlist_signups").Column("discipline").
		Where("email_normalized = ?", "ada@example.com").Scan(ctx, &discipline); err != nil {
		t.Fatal(err)
	}
	if discipline == nil || *discipline != "musician" {
		t.Errorf("discipline = %v, want musician (duplicate must not overwrite)", discipline)
	}

	created, err = store.Insert(ctx, Entry{Email: "b@example.com", EmailNormalized: "b@example.com"})
	if err != nil || !created {
		t.Fatalf("insert without discipline: created=%v err=%v", created, err)
	}
	isNull, err := tx.NewSelect().Table("waitlist_signups").
		Where("email_normalized = ?", "b@example.com").
		Where("discipline IS NULL").
		Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !isNull {
		t.Error("empty discipline was not stored as NULL")
	}
}

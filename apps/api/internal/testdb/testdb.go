// Package testdb opens the migrated test database for integration tests.
package testdb

import (
	"os"
	"testing"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/database"
)

// Open returns a connection to CAVI_TEST_DATABASE_URL, or skips the test
// when it is not set (see `make api-test-integration`).
func Open(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("CAVI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CAVI_TEST_DATABASE_URL not set")
	}
	db := database.Open(dsn)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

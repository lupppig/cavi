// Package database opens the PostgreSQL connection used by the API.
package database

import (
	"database/sql"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// Open returns a Bun database handle. It does not connect eagerly;
// connectivity is reported through the readiness endpoint.
func Open(dsn string) *bun.DB {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	sqldb.SetMaxOpenConns(20)
	sqldb.SetMaxIdleConns(5)
	sqldb.SetConnMaxIdleTime(5 * time.Minute)
	return bun.NewDB(sqldb, pgdialect.New())
}

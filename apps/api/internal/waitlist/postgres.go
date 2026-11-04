package waitlist

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

type signupRow struct {
	bun.BaseModel `bun:"table:waitlist_signups"`

	ID              string    `bun:"id,pk,type:uuid,nullzero"`
	Email           string    `bun:"email,notnull"`
	EmailNormalized string    `bun:"email_normalized,notnull"`
	Discipline      string    `bun:"discipline,nullzero"`
	CreatedAt       time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}

// PostgresStore stores waitlist entries in PostgreSQL.
type PostgresStore struct {
	db bun.IDB
}

// NewPostgresStore returns a Store backed by db.
func NewPostgresStore(db bun.IDB) *PostgresStore {
	return &PostgresStore{db: db}
}

// Insert implements Store.
func (s *PostgresStore) Insert(ctx context.Context, e Entry) (bool, error) {
	row := &signupRow{
		Email:           e.Email,
		EmailNormalized: e.EmailNormalized,
		Discipline:      string(e.Discipline),
	}
	res, err := s.db.NewInsert().
		Model(row).
		ExcludeColumn("id", "created_at").
		On("CONFLICT (email_normalized) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

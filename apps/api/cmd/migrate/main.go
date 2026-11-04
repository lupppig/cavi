// Command migrate applies, rolls back, or reports SQL schema migrations.
//
// Migrations are versioned SQL files in the repository's migrations/
// directory. The API never migrates the schema on startup; run this
// command explicitly as part of deployment.
//
// Usage:
//
//	migrate [-dir path] up|down|status
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/uptrace/bun/migrate"

	"github.com/lupppig/cavi/apps/api/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "../../migrations", "directory containing *.up.sql and *.down.sql files")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: migrate [-dir path] up|down|status")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		return errors.New("expected exactly one command")
	}

	dsn := os.Getenv("CAVI_DATABASE_URL")
	if dsn == "" {
		return errors.New("CAVI_DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db := database.Open(dsn)
	defer db.Close()

	migrations := migrate.NewMigrations()
	if err := migrations.Discover(os.DirFS(*dir)); err != nil {
		return fmt.Errorf("discover migrations in %s: %w", *dir, err)
	}
	migrator := migrate.NewMigrator(db, migrations)
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("init migration tables: %w", err)
	}

	switch cmd := flag.Arg(0); cmd {
	case "up":
		return withLock(ctx, migrator, func() error {
			group, err := migrator.Migrate(ctx)
			if err != nil {
				return err
			}
			if group.IsZero() {
				fmt.Println("no new migrations")
				return nil
			}
			fmt.Printf("applied %s\n", group)
			return nil
		})
	case "down":
		return withLock(ctx, migrator, func() error {
			group, err := migrator.Rollback(ctx)
			if err != nil {
				return err
			}
			if group.IsZero() {
				fmt.Println("nothing to roll back")
				return nil
			}
			fmt.Printf("rolled back %s\n", group)
			return nil
		})
	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("applied:   %s\n", ms.Applied())
		fmt.Printf("pending:   %s\n", ms.Unapplied())
		fmt.Printf("last group: %s\n", ms.LastGroup())
		return nil
	default:
		flag.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func withLock(ctx context.Context, m *migrate.Migrator, fn func() error) (err error) {
	if err := m.Lock(ctx); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if unlockErr := m.Unlock(ctx); unlockErr != nil && err == nil {
			err = fmt.Errorf("release migration lock: %w", unlockErr)
		}
	}()
	return fn()
}

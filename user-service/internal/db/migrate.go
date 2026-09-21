package db

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers the "postgres" database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/equipment-rental-system/user-service/internal/config"
)

// Migrate applies every pending migration in cfg.MigrationsPath to the
// configured database, then returns.
//
// Nothing used to run migrations at all: golang-migrate was not a
// dependency, main.go never called it, and the Dockerfile copied
// /migrations into the image without a migrate binary to apply them. The
// only working recipe lived in a gitignored report. Running them here, at
// startup and before the HTTP server binds, means `docker compose up` on a
// fresh volume produces a usable service with no separate manual step.
//
// The source is read through io/fs rather than a file:// URL so the path
// works unchanged on Windows and Linux, and the DSN is assembled with
// net/url so a password containing URL-significant characters is escaped
// rather than corrupting the connection string.
func Migrate(cfg *config.Config) error {
	src, err := iofs.New(os.DirFS(cfg.MigrationsPath), ".")
	if err != nil {
		return fmt.Errorf("open migrations at %q: %w", cfg.MigrationsPath, err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, migrationDSN(cfg))
	if err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	// Close reports errors from the source and the database half separately.
	defer func() {
		srcErr, dbErr := m.Close()
		_ = srcErr
		_ = dbErr
	}()

	// ErrNoChange simply means the schema is already current, which is the
	// normal case on every restart after the first.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// migrationDSN builds the postgres:// URL golang-migrate expects. gorm uses
// the key/value DSN form instead (see Connect), so the two are kept
// separate rather than shared.
func migrationDSN(cfg *config.Config) string {
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.DBUser, cfg.DBPassword),
		Host:     fmt.Sprintf("%s:%s", cfg.DBHost, cfg.DBPort),
		Path:     "/" + cfg.DBName,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

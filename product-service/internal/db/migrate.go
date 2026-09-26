package db

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers the "postgres" database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/equipment-rental-system/product-service/internal/config"
)

// Migrate applies every pending migration in cfg.MigrationsPath, mirroring
// user-service/internal/db/migrate.go: run at startup, before the HTTP
// server binds, so `docker compose up` on a fresh volume needs no separate
// manual migration step.
func Migrate(cfg *config.Config) error {
	src, err := iofs.New(os.DirFS(cfg.MigrationsPath), ".")
	if err != nil {
		return fmt.Errorf("open migrations at %q: %w", cfg.MigrationsPath, err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, migrationDSN(cfg))
	if err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		_ = srcErr
		_ = dbErr
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

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

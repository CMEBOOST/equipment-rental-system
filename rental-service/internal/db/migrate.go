package db

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/equipment-rental-system/rental-service/internal/config"
)

func Migrate(cfg *config.Config) error {
	src, err := iofs.New(os.DirFS(cfg.MigrationsPath), ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrationDSN(cfg))
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func migrationDSN(cfg *config.Config) string {
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(cfg.DBUser, cfg.DBPassword), Host: fmt.Sprintf("%s:%s", cfg.DBHost, cfg.DBPort), Path: "/" + cfg.DBName, RawQuery: "sslmode=disable"}
	return u.String()
}

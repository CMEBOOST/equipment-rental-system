package main

import (
	"log"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/db"
	"github.com/equipment-rental-system/user-service/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	// Migrations run before anything serves traffic, so a fresh database
	// (e.g. `docker compose up` on an empty volume) is usable without a
	// separate manual step. Up() is a no-op once the schema is current.
	log.Printf("running migrations from %s", cfg.MigrationsPath)
	if err := db.Migrate(cfg); err != nil {
		log.Fatalf("migration error: %v", err)
	}
	log.Print("migrations up to date")

	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	r := router.New(database, cfg)
	log.Printf("user-service listening on :%s", cfg.AppPort)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

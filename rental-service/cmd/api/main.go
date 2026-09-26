package main

import (
	"log"

	"github.com/equipment-rental-system/rental-service/internal/config"
	"github.com/equipment-rental-system/rental-service/internal/db"
	"github.com/equipment-rental-system/rental-service/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	if err := db.Migrate(cfg); err != nil {
		log.Fatalf("migration error: %v", err)
	}
	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	r := router.New(database, cfg)
	log.Printf("rental-service listening on :%s", cfg.AppPort)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

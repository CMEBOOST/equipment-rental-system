package main

import (
	"log"

	"github.com/equipment-rental-system/product-service/internal/config"
	"github.com/equipment-rental-system/product-service/internal/db"
	"github.com/equipment-rental-system/product-service/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
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
	log.Printf("product-service listening on :%s", cfg.AppPort)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

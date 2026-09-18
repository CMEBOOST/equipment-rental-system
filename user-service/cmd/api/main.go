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
	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	r := router.New(database, cfg)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

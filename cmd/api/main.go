package main

import (
	"log"
	"net"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/ai"
	"marketmind-ai/internal/config"
	"marketmind-ai/internal/database"
	"marketmind-ai/internal/routes"
)

func main() {
	cfg := config.Load()

	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	aiClient := ai.NewClient(cfg)

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("PostgreSQL connection failed: %v", err)
	}
	defer db.Close()

	log.Println("PostgreSQL connected successfully!")

	app := gin.Default()

	if err := app.SetTrustedProxies(nil); err != nil {
		log.Fatalf("Trusted proxy configuration failed: %v", err)
	}

	routes.Register(app, db, aiClient)

	log.Printf(
		"MarketMind AI running on %s (%s)",
		net.JoinHostPort(cfg.Host, cfg.Port),
		cfg.Environment,
	)

	if err := app.Run(net.JoinHostPort(cfg.Host, cfg.Port)); err != nil {
		log.Fatal(err)
	}
}

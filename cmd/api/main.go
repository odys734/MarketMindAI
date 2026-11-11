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
	aiClient := ai.NewClient(cfg)

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("PostgreSQL connection failed: %v", err)
	}
	defer db.Close()

	log.Println("PostgreSQL connected successfully!")

	app := gin.Default()
	routes.Register(app, db, aiClient)

	log.Printf(
		"MarketMind AI running on port %s (%s)",
		cfg.Port,
		cfg.Environment,
	)

	if err := app.Run(net.JoinHostPort(cfg.Host, cfg.Port)); err != nil {
		log.Fatal(err)
	}
}

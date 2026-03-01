package main

import (
	"agentpay/internal/router"
	"log"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	cfg := router.LoadConfig()

	// Start background heartbeat goroutine
	router.StartHealthChecker(cfg.HealthCheckInterval)

	r := router.SetupRouter(cfg)

	log.Printf("AgentPay Router 正在运行在端口 :%s\n", cfg.Port)
	r.Run(":" + cfg.Port)
}

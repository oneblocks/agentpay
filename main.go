package main

import (
	"log"

	"github.com/joho/godotenv"
)

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	cfg := LoadConfig()

	r := SetupRouter(cfg)

	log.Println("AgentPay Router running on :8080")
	r.Run(":8080")
}

package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type Pricing struct {
	Mode  string `json:"mode"`
	Price int64  `json:"price"`
}

type Service struct {
	Name      string  `json:"name"`
	Endpoint  string  `json:"endpoint"`
	Recipient string  `json:"recipient"`
	Pricing   Pricing `json:"pricing"`
}

func main() {

	// 读取 .env
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system env")
	}

	port := getEnv("PORT", "9000")
	routerURL := getEnv("ROUTER_URL", "http://localhost:8080")

	serviceName := getEnv("SERVICE_NAME", "demo-ai")
	recipient := getEnv("SERVICE_RECIPIENT", "")
	priceStr := getEnv("SERVICE_PRICE", "1000000")
	mode := getEnv("SERVICE_MODE", "per_call")

	price, _ := strconv.ParseInt(priceStr, 10, 64)

	endpoint := "http://localhost:" + port + "/generate"

	// 自动注册
	go func() {
		registerService(routerURL, Service{
			Name:      serviceName,
			Endpoint:  endpoint,
			Recipient: recipient,
			Pricing: Pricing{
				Mode:  mode,
				Price: price,
			},
		})
	}()

	// 启动服务
	r := gin.Default()

	r.POST("/generate", func(c *gin.Context) {

		log.Println("Generate API called")

		proof := c.GetHeader("X-402-Proof")

		if proof == "" {
			log.Println("No payment proof, returning 402")
			c.Header("X-402-Cost", priceStr)
			c.Writer.WriteHeader(402)
			return
		}

		log.Println("Payment proof received:", proof)

		c.JSON(200, gin.H{
			"result": "AI Generated Content",
			"tx":     proof,
		})
	})

	log.Println("Demo service running on port", port)
	r.Run("0.0.0.0:" + port)
}

func registerService(routerURL string, service Service) {

	body, _ := json.Marshal(service)

	resp, err := http.Post(
		routerURL+"/register",
		"application/json",
		bytes.NewBuffer(body),
	)

	if err != nil {
		log.Println("Service registration failed:", err)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Println("Service registered successfully")
	} else {
		log.Println("Service registration failed, status:", resp.Status)
	}
}

func getEnv(key string, fallback string) string {

	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

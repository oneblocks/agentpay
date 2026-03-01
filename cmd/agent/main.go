package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	// 1. Load base config
	port := getEnv("PORT", "9000")
	routerURL := getEnv("ROUTER_URL", "http://localhost:8080")

	// 2. AgentPay registration info
	agentName := getEnv("AGENT_NAME", "agent")
	agentRecipient := getEnv("AGENT_RECIPIENT", "")
	agentPriceStr := getEnv("AGENT_PRICE", "1000000")
	agentEndpoint := getEnv("AGENT_ENDPOINT", "http://localhost:9000/chat")
	agentDescription := getEnv("AGENT_DESCRIPTION", "General AI assistant for daily chat and tasks")

	// 3. Third-party provider config
	apiKey := getEnv("PROVIDER_API_KEY", "")
	baseURL := getEnv("PROVIDER_BASE_URL", "https://api.openai.com/v1")
	model := getEnv("PROVIDER_MODEL", "gpt-3.5-turbo")
	autoPay := getEnv("AUTO_PAY", "false")

	log.Printf("⚙️ Auto-pay config: %s\n", autoPay)

	if apiKey == "" {
		log.Println("⚠️ Warning: PROVIDER_API_KEY is empty, AI calls will fail")
	}

	// Auto-register to Router
	go func() {
		// Wait for server to start
		time.Sleep(2 * time.Second)
		price, _ := strconv.ParseInt(agentPriceStr, 10, 64)
		autoRegister(routerURL, agentName, agentRecipient, agentEndpoint, agentDescription, price)
	}()

	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": agentName,
			"status":  "ok",
			"ts":      time.Now().Unix(),
		})
	})

	// Core: Agent task handling
	r.POST("/chat", func(c *gin.Context) {
		proof := c.GetHeader("X-402-Proof")
		if proof == "" {
			log.Println("❌ Reject: missing payment proof")
			c.Header("X-402-Cost", agentPriceStr)
			c.Writer.WriteHeader(402)
			return
		}

		var body struct {
			Query string `json:"query"`
		}
		if err := c.BindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		log.Printf("🤖 Processing task: [%s] -> %s\n", agentName, body.Query)

		// Call generic OpenAI-compatible model
		result, err := callLLM(apiKey, baseURL, model, body.Query)
		if err != nil {
			log.Println("❌ Provider Error:", err)
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"result": result,
			"tx":     proof, // Return payment proof as-is for tracing
		})
	})

	log.Printf("🚀 AgentPay node [%s] running on port %s...\n", agentName, port)
	r.Run("0.0.0.0:" + port)
}

// autoRegister Retry + heartbeat so node stays online while Router is up
func autoRegister(router, name, recipient, endpoint, description string, price int64) {
	if recipient == "" {
		log.Println("⚠️ Auto-register skipped: AGENT_RECIPIENT not set")
		return
	}
	payload := map[string]interface{}{
		"name":        name,
		"endpoint":    endpoint,
		"recipient":   recipient,
		"description": description,
		"pricing":     map[string]int64{"price": price},
	}

	data, _ := json.Marshal(payload)
	log.Printf("⏳ Connecting to AgentPay network: %s...\n", router)

	var successOnce bool

	for {
		resp, err := http.Post(router+"/register", "application/json", bytes.NewBuffer(data))
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			if !successOnce {
				log.Printf("✅ Node is now online on AgentPay network!")
				successOnce = true
			}
			// After successful register, this request acts as heartbeat so Router restart can recover
		} else {
			if err != nil {
				log.Printf("❌ Router connection failed (retrying...): %v\n", err)
			} else {
				log.Printf("❌ Register/heartbeat rejected (Status %d), retrying...\n", resp.StatusCode)
				resp.Body.Close()
			}
			successOnce = false
		}

		// Sync state to Router every 10s
		time.Sleep(10 * time.Second)
	}
}

// callLLM Generic OpenAI-compatible API
func callLLM(apiKey, baseURL, model, query string) (string, error) {
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Current server time: " + currentTime + " (UTC+8). Use this when answering time-sensitive questions."},
			{"role": "user", "content": query},
		},
	}
	jsonData, _ := json.Marshal(payload)

	url := baseURL + "/chat/completions"
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("provider error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}

	if len(res.Choices) == 0 {
		return "", fmt.Errorf("empty response from provider")
	}
	return res.Choices[0].Message.Content, nil
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

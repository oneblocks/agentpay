package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	godotenv.Load()

	port := getEnv("PORT", "9000")
	price := getEnv("SERVICE_PRICE", "1000000")
	moonshotKey := getEnv("MOONSHOT_API_KEY", "")
	moonshotModel := getEnv("MOONSHOT_MODEL", "moonshot-v1-8k")

	if moonshotKey == "" {
		log.Println("⚠️ WARNING: MOONSHOT_API_KEY is empty")
	}

	r := gin.Default()

	// /health 端点：供 AgentPay Router 心跳检测使用
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": "moonshot-agent",
			"status":  "ok",
			"ts":      fmt.Sprintf("%d", nowUnix()),
		})
	})

	r.POST("/chat", func(c *gin.Context) {

		log.Println("Generate API called")

		proof := c.GetHeader("X-402-Proof")

		if proof == "" {
			log.Println("No payment proof, returning 402")
			c.Header("X-402-Cost", price)
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

		result, err := callMoonshot(moonshotKey, moonshotModel, body.Query)
		if err != nil {
			log.Println("Moonshot error:", err)
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"result": result,
			"tx":     proof,
		})
	})

	log.Println("Demo service running on port", port)
	r.Run("0.0.0.0:" + port)
}

func callMoonshot(apiKey, model, query string) (string, error) {

	if apiKey == "" {
		return "", fmt.Errorf("moonshot api key not configured")
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(
		"POST",
		"https://api.moonshot.cn/v1/chat/completions",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	log.Println("Moonshot raw response:", string(bodyBytes))

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("moonshot http error %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", err
	}

	choicesRaw, ok := result["choices"]
	if !ok {
		return "", fmt.Errorf("moonshot response missing choices field: %s",
			string(bodyBytes))
	}

	choices, ok := choicesRaw.([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("invalid choices format")
	}

	first, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid first choice format")
	}

	messageRaw, ok := first["message"]
	if !ok {
		return "", fmt.Errorf("message field missing")
	}

	message, ok := messageRaw.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	contentRaw, ok := message["content"]
	if !ok {
		return "", fmt.Errorf("content field missing")
	}

	content, ok := contentRaw.(string)
	if !ok {
		return "", fmt.Errorf("invalid content format")
	}

	return content, nil
}

func getEnv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

// nowUnix 返回当前 Unix 时间戳（秒）
func nowUnix() int64 {
	return time.Now().Unix()
}

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

	// 1. 获取基础配置
	port := getEnv("PORT", "9000")
	routerURL := getEnv("ROUTER_URL", "http://localhost:8080")

	// 2. 获取 AgentPay 注册信息
	agentName := getEnv("AGENT_NAME", "agent")
	agentRecipient := getEnv("AGENT_RECIPIENT", "")
	agentPriceStr := getEnv("AGENT_PRICE", "1000000")
	agentEndpoint := getEnv("AGENT_ENDPOINT", "http://localhost:9000/chat")

	// 3. 获取第三方服务驱动配置
	apiKey := getEnv("PROVIDER_API_KEY", "")
	baseURL := getEnv("PROVIDER_BASE_URL", "https://api.openai.com/v1")
	model := getEnv("PROVIDER_MODEL", "gpt-3.5-turbo")

	if apiKey == "" {
		log.Println("⚠️ 警告: PROVIDER_API_KEY 为空，AI 调用将失败")
	}

	// 自动注册到 Router
	go func() {
		// 等待服务启动
		time.Sleep(2 * time.Second)
		price, _ := strconv.ParseInt(agentPriceStr, 10, 64)
		autoRegister(routerURL, agentName, agentRecipient, agentEndpoint, price)
	}()

	r := gin.Default()

	// 健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": agentName,
			"status":  "ok",
			"ts":      time.Now().Unix(),
		})
	})

	// 核心业务：Agent 任务处理
	r.POST("/chat", func(c *gin.Context) {
		proof := c.GetHeader("X-402-Proof")
		if proof == "" {
			log.Println("❌ 拒绝请求: 缺少支付凭证")
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

		log.Printf("🤖 处理任务: [%s] -> %s\n", agentName, body.Query)

		// 调用通用的 OpenAI 兼容模型
		result, err := callLLM(apiKey, baseURL, model, body.Query)
		if err != nil {
			log.Println("❌ Provider Error:", err)
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"result": result,
			"tx":     proof, // 原样返回支付凭证，供追踪
		})
	})

	log.Printf("🚀 AgentPay 节点 [%s] 正在端口 %s 运行...\n", agentName, port)
	r.Run("0.0.0.0:" + port)
}

// autoRegister 实现持续重试逻辑，直到成功向 Router 注册
func autoRegister(router, name, recipient, endpoint string, price int64) {
	if recipient == "" {
		log.Println("⚠️ 自动注册跳过: 未配置 AGENT_RECIPIENT")
		return
	}
	payload := map[string]interface{}{
		"name":      name,
		"endpoint":  endpoint,
		"recipient": recipient,
		"pricing":   map[string]int64{"price": price},
	}
	data, _ := json.Marshal(payload)

	log.Printf("⏳ 正在尝试连接 AgentPay 网络: %s...\n", router)

	for {
		resp, err := http.Post(router+"/register", "application/json", bytes.NewBuffer(data))
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			log.Printf("✅ 节点已成功上线 AgentPay 网络！")
			return
		}

		if err != nil {
			log.Printf("❌ 连接 Router 失败 (正在重试...): %v\n", err)
		} else {
			log.Printf("❌ 注册被拒绝 (Status %d), 正在重试...\n", resp.StatusCode)
			resp.Body.Close()
		}

		time.Sleep(5 * time.Second) // 失败后每 5 秒重试一次
	}
}

// callLLM 支持通用的 OpenAI 兼容接口
func callLLM(apiKey, baseURL, model, query string) (string, error) {
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
	}
	jsonData, _ := json.Marshal(payload)

	url := baseURL + "/chat/completions"
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
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

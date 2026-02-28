package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type AutoCallRequest struct {
	Capability string          `json:"capability"`
	Payload    json.RawMessage `json:"payload"`
	TxHash     string          `json:"txHash"` // Payment proof hash from frontend
}

type AutoCallResponse struct {
	Service       string      `json:"service"`
	Price         int64       `json:"price"`
	Tx            string      `json:"tx"`
	Explorer      string      `json:"explorer"`
	Confirmations int         `json:"confirmations"`
	Status        string      `json:"status"`
	Result        interface{} `json:"result"`
}

// PingResponse Pre-flight check response structure
type PingResponse struct {
	Service   string `json:"service"`
	Status    string `json:"status"` // ok / timeout / error
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms"`
}

func SetupRouter(cfg *Config) *gin.Engine {

	r := gin.Default()
	r.Use(cors.Default())

	// ─────────────────────────────────────────────
	// 【API 1】Get all services (with status, for frontend display)
	// ─────────────────────────────────────────────
	r.GET("/services", func(c *gin.Context) {
		c.JSON(200, ListServices())
	})

	// ─────────────────────────────────────────────
	// 【API 2】Register service node
	// ─────────────────────────────────────────────
	r.POST("/register", func(c *gin.Context) {

		var s Service

		if err := c.BindJSON(&s); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		fmt.Printf("=========register s: %#v=\n", s)
		RegisterService(s)

		c.JSON(200, gin.H{
			"message": "service registered",
		})
	})

	// ─────────────────────────────────────────────
	// 【API 3】Remove service node
	// ─────────────────────────────────────────────
	r.DELETE("/service/:name", func(c *gin.Context) {

		name := c.Param("name")
		RemoveService(name)

		c.JSON(200, gin.H{
			"message": "service removed",
		})
	})

	// ─────────────────────────────────────────────
	// 【API 3b】Re-enable node
	// ─────────────────────────────────────────────
	r.PUT("/service/:name/enable", func(c *gin.Context) {
		name := c.Param("name")
		if err := ReenableService(name); err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "service re-enabled"})
	})

	// ─────────────────────────────────────────────
	// 【API 4】Discover best service (online/busy nodes only)
	// ─────────────────────────────────────────────
	r.POST("/discover", func(c *gin.Context) {

		type DiscoverRequest struct {
			Capability string `json:"capability"`
		}

		var req DiscoverRequest

		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		// Only select from online/busy nodes
		onlineServices := ListOnlineServices()

		if len(onlineServices) == 0 {
			c.JSON(503, gin.H{
				"error": "no available services",
				"code":  "SERVICE_UNAVAILABLE",
			})
			return
		}

		// Strategy: pick cheapest among online nodes
		var selected Service
		var minPrice int64 = 0

		for i, s := range onlineServices {
			if i == 0 || s.Pricing.Price < minPrice {
				selected = s
				minPrice = s.Pricing.Price
			}
		}

		c.JSON(200, gin.H{
			"service":  selected.Name,
			"price":    selected.Pricing.Price,
			"endpoint": selected.Endpoint,
			"status":   selected.Status,
		})
	})

	// ─────────────────────────────────────────────
	// 【API 5】Pre-flight check — handshake before payment; ping target node, allow wallet only on success
	// ─────────────────────────────────────────────
	r.GET("/ping/:service", func(c *gin.Context) {

		serviceName := c.Param("service")

		target, err := GetService(serviceName)
		if err != nil {
			c.JSON(404, PingResponse{
				Service: serviceName,
				Status:  "error",
				Message: "Service node not found",
			})
			return
		}

		// If cached status is offline, reject directly
		if target.Status == StatusOffline {
			c.JSON(503, PingResponse{
				Service: serviceName,
				Status:  "offline",
				Message: "Node is offline (heartbeat unreachable), please choose another node",
			})
			return
		}

		// Real-time ping (independent of background heartbeat, 5s timeout)
		pingURL := buildPingURL(target.Endpoint)
		client := &http.Client{Timeout: 5 * time.Second}

		start := time.Now()
		resp, err := client.Get(pingURL)
		latencyMs := time.Since(start).Milliseconds()

		if err != nil {
			// Update status for heartbeat to pick up
			newFail := target.FailCount + 1
			newStatus := target.Status
			if newFail >= maxFailCount {
				newStatus = StatusOffline
			}
			updateServiceStatus(serviceName, newStatus, newFail, latencyMs)

			c.JSON(503, PingResponse{
				Service:   serviceName,
				Status:    "timeout",
				Message:   fmt.Sprintf("Service node response timeout (%dms), trying fallback route...", latencyMs),
				LatencyMs: latencyMs,
			})
			return
		}
		defer resp.Body.Close()

		// Ping success: reset status
		updateServiceStatus(serviceName, StatusOnline, 0, latencyMs)

		c.JSON(200, PingResponse{
			Service:   serviceName,
			Status:    "ok",
			Message:   fmt.Sprintf("Node healthy, latency %dms", latencyMs),
			LatencyMs: latencyMs,
		})
	})

	// ─────────────────────────────────────────────
	// 【API 6】Call specified service (with payment)
	// ─────────────────────────────────────────────
	r.POST("/call/:service", func(c *gin.Context) {

		serviceName := c.Param("service")

		selected, err := GetService(serviceName)
		if err != nil {
			fmt.Println("service not found")
			c.JSON(404, gin.H{"error": "service not found"})
			return
		}

		// Reject calls to offline nodes
		if selected.Status == StatusOffline {
			c.JSON(503, gin.H{
				"error": "Target node is offline, please use /discover to select an available node",
				"code":  "NODE_OFFLINE",
			})
			return
		}

		// Read body from frontend
		bodyBytes, err := c.GetRawData()
		if err != nil {
			fmt.Println("invalid body")
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}

		// Get payment proof (user already paid on frontend)
		txHash := c.GetHeader("X-402-Proof")
		if txHash == "" {
			c.JSON(402, gin.H{"error": "payment required", "cost": selected.Pricing.Price})
			return
		}

		// Forward request to downstream service
		req, err := http.NewRequest(
			"POST",
			selected.Endpoint,
			bytes.NewBuffer(bodyBytes),
		)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-402-Proof", txHash)

		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		c.Data(resp.StatusCode, "application/json", respBody)
	})

	// ─────────────────────────────────────────────
	// 【API 7】Smart auto-call (Pre-flight + routing + payment)
	// ─────────────────────────────────────────────
	r.POST("/auto-call", func(c *gin.Context) {

		var req AutoCallRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		// Route only among online nodes
		onlineServices := ListOnlineServices()
		if len(onlineServices) == 0 {
			c.JSON(503, gin.H{
				"error": "No available service nodes (all nodes offline)",
				"code":  "NO_AVAILABLE_NODES",
			})
			return
		}

		// AI smart routing decision
		selected, err := selectBestService(cfg, onlineServices, req.Capability)
		if err != nil {
			log.Printf("⚠️ Smart routing failed, fallback to price-first: %v\n", err)
			// Fallback: pick cheapest
			var minPrice int64
			for i, s := range onlineServices {
				if i == 0 || s.Pricing.Price < minPrice {
					selected = s
					minPrice = s.Pricing.Price
				}
			}
		}

		// 1️⃣ Use payment proof from frontend (user already paid)
		txHash := req.TxHash
		if txHash == "" {
			c.JSON(400, gin.H{"error": "missing payment proof (txHash)"})
			return
		}

		explorerURL := fmt.Sprintf(
			"https://testnet.monadexplorer.com/tx/%s",
			txHash,
		)

		// 2️⃣ Call downstream

		httpReq, err := http.NewRequest(
			"POST",
			selected.Endpoint,
			bytes.NewBuffer(req.Payload),
		)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-402-Proof", txHash)

		client := &http.Client{}
		resp, err := client.Do(httpReq)

		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		var result interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			result = string(respBody)
		}

		c.JSON(200, AutoCallResponse{
			Service:       selected.Name,
			Price:         selected.Pricing.Price,
			Tx:            txHash,
			Explorer:      explorerURL,
			Confirmations: 1,
			Status:        "confirmed",
			Result:        result,
		})
	})

	return r
}

// selectBestService Calls AI model to pick the best Agent by task intent
func selectBestService(cfg *Config, services []Service, query string) (Service, error) {
	if cfg.ProviderAPIKey == "" {
		return Service{}, fmt.Errorf("router AI config missing")
	}

	// Build candidate list description
	var options string
	for i, s := range services {
		options += fmt.Sprintf("[%d] Name: %s, Specialty: %s\n", i, s.Name, s.Description)
	}

	prompt := fmt.Sprintf(`### Task
You are a professional demand-distribution relay (Router). Analyze the user's question and pick the single best Agent from the candidate list to handle this task.

### Rules
1. **Semantic match**: Prefer nodes whose description contains core keywords of the user's question (e.g. travel, finance, legal, coding).
2. **Expertise first**: If the user asks about travel, choose "Travel Assistant" even if more expensive, not "General" or "Finance Assistant".
3. **Fallback**: If no clear match, choose the node named "Kimi" or containing "General".

### User question
"%s"

### Candidate Agent list
%s

### Output
Reply with ONLY the selected Agent **name** string. No reasoning, punctuation, or extra characters.`, query, options)

	selectedName, err := routerCallLLM(cfg, prompt)
	if err != nil {
		return Service{}, err
	}

	selectedName = strings.TrimSpace(selectedName)

	// Match result
	for _, s := range services {
		if strings.Contains(strings.ToLower(selectedName), strings.ToLower(s.Name)) {
			return s, nil
		}
	}

	return services[0], nil // Default: first one
}

func routerCallLLM(cfg *Config, prompt string) (string, error) {
	payload := map[string]interface{}{
		"model": cfg.ProviderModel,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a precise task dispatcher. Return only the selected Agent name."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", cfg.ProviderBaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.ProviderAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", fmt.Errorf("ai no response")
	}

	return fmt.Sprintf("%v", res.Choices[0].Message.Content), nil
}

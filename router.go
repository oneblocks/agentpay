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
	TxHash     string          `json:"txHash"` // 前端已完成支付的哈希凭证
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

// PingResponse 前置检查（Pre-flight）的响应结构
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
	// 【接口 1】获取所有服务（含状态，前端用于展示）
	// ─────────────────────────────────────────────
	r.GET("/services", func(c *gin.Context) {
		c.JSON(200, ListServices())
	})

	// ─────────────────────────────────────────────
	// 【接口 2】注册服务节点
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
	// 【接口 3】删除服务节点
	// ─────────────────────────────────────────────
	r.DELETE("/service/:name", func(c *gin.Context) {

		name := c.Param("name")
		RemoveService(name)

		c.JSON(200, gin.H{
			"message": "service removed",
		})
	})

	// ─────────────────────────────────────────────
	// 【接口 3b】重新上线节点
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
	// 【接口 4】发现最优服务（仅返回 online/busy 节点）
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

		// 只在 online/busy 节点中选择
		onlineServices := ListOnlineServices()

		if len(onlineServices) == 0 {
			c.JSON(503, gin.H{
				"error": "no available services",
				"code":  "SERVICE_UNAVAILABLE",
			})
			return
		}

		// 策略：在线节点中选最便宜的
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
	// 【接口 5】Pre-flight Check —— 支付前握手
	// 向目标节点发 ping，成功才允许前端调起钱包
	// ─────────────────────────────────────────────
	r.GET("/ping/:service", func(c *gin.Context) {

		serviceName := c.Param("service")

		target, err := GetService(serviceName)
		if err != nil {
			c.JSON(404, PingResponse{
				Service: serviceName,
				Status:  "error",
				Message: "服务节点不存在",
			})
			return
		}

		// 如果缓存状态即为 offline，直接拒绝
		if target.Status == StatusOffline {
			c.JSON(503, PingResponse{
				Service: serviceName,
				Status:  "offline",
				Message: "节点已离线（心跳检测不可达），请选择备用节点",
			})
			return
		}

		// 实时 ping（独立于后台心跳，超时 5s）
		pingURL := buildPingURL(target.Endpoint)
		client := &http.Client{Timeout: 5 * time.Second}

		start := time.Now()
		resp, err := client.Get(pingURL)
		latencyMs := time.Since(start).Milliseconds()

		if err != nil {
			// 更新状态以便心跳感知
			newFail := target.FailCount + 1
			newStatus := target.Status
			if newFail >= maxFailCount {
				newStatus = StatusOffline
			}
			updateServiceStatus(serviceName, newStatus, newFail, latencyMs)

			c.JSON(503, PingResponse{
				Service:   serviceName,
				Status:    "timeout",
				Message:   fmt.Sprintf("服务节点响应超时（%dms），正在尝试备用路由...", latencyMs),
				LatencyMs: latencyMs,
			})
			return
		}
		defer resp.Body.Close()

		// ping 成功：重置状态
		updateServiceStatus(serviceName, StatusOnline, 0, latencyMs)

		c.JSON(200, PingResponse{
			Service:   serviceName,
			Status:    "ok",
			Message:   fmt.Sprintf("节点健康，延迟 %dms", latencyMs),
			LatencyMs: latencyMs,
		})
	})

	// ─────────────────────────────────────────────
	// 【接口 6】调用指定服务（含支付）
	// ─────────────────────────────────────────────
	r.POST("/call/:service", func(c *gin.Context) {

		serviceName := c.Param("service")

		selected, err := GetService(serviceName)
		if err != nil {
			fmt.Println("service not found")
			c.JSON(404, gin.H{"error": "service not found"})
			return
		}

		// 拒绝调用 offline 节点
		if selected.Status == StatusOffline {
			c.JSON(503, gin.H{
				"error": "目标节点已离线，请通过 /discover 选择可用节点",
				"code":  "NODE_OFFLINE",
			})
			return
		}

		// 读取前端传入的 body
		bodyBytes, err := c.GetRawData()
		if err != nil {
			fmt.Println("invalid body")
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}

		// 获取支付凭证（用户已在前端完成支付）
		txHash := c.GetHeader("X-402-Proof")
		if txHash == "" {
			c.JSON(402, gin.H{"error": "payment required", "cost": selected.Pricing.Price})
			return
		}

		// 转发请求给下游服务
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
	// 【接口 7】智能自动调用（含 Pre-flight + 路由 + 支付）
	// ─────────────────────────────────────────────
	r.POST("/auto-call", func(c *gin.Context) {

		var req AutoCallRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		// 只在 online 节点中路由
		onlineServices := ListOnlineServices()
		if len(onlineServices) == 0 {
			c.JSON(503, gin.H{
				"error": "当前无可用服务节点（所有节点已离线）",
				"code":  "NO_AVAILABLE_NODES",
			})
			return
		}

		// AI 智能路由决策
		selected, err := selectBestService(cfg, onlineServices, req.Capability)
		if err != nil {
			log.Printf("⚠️ 智能路由失败，降级为价格优先策略: %v\n", err)
			// 降级：选最便宜的
			var minPrice int64
			for i, s := range onlineServices {
				if i == 0 || s.Pricing.Price < minPrice {
					selected = s
					minPrice = s.Pricing.Price
				}
			}
		}

		// 1️⃣ 使用前端传入的支付凭证（用户已直接付过钱了）
		txHash := req.TxHash
		if txHash == "" {
			c.JSON(400, gin.H{"error": "missing payment proof (txHash)"})
			return
		}

		explorerURL := fmt.Sprintf(
			"https://testnet.monadexplorer.com/tx/%s",
			txHash,
		)

		// 2️⃣ 调用下游

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

// selectBestService 调用 AI 模型通过任务意图选择最合适的 Agent
func selectBestService(cfg *Config, services []Service, query string) (Service, error) {
	if cfg.ProviderAPIKey == "" {
		return Service{}, fmt.Errorf("router AI config missing")
	}

	// 构建备选列表描述
	var options string
	for i, s := range services {
		options += fmt.Sprintf("[%d] 名称: %s, 擅长特点: %s\n", i, s.Name, s.Description)
	}

	prompt := fmt.Sprintf(`### 任务目标
你是一个专业的需求分发中继（Router）。请分析用户的问题，并从下面的候选 Agent 列表中选出最适合处理该具体任务的一个。

### 决策原则
1. **语义匹配**：优先选择描述中包含用户问题核心关键词（如：旅游、金融、法律、编程）的节点。
2. **专业度优先**：如果用户问的是旅游，即使价格高，也要选“旅游助手”而不是“通用助手”或“金融助手”。
3. **兜底策略**：如果没有明显匹配的专业节点，请选择名称为 "Kimi" 或包含 "通用" 字样的节点。

### 待处理问题
用户问题: "%s"

### 候选 Agent 列表
%s

### 输出要求
请仅回答选中的 Agent **名称**字符串，不要输出任何推理过程、标点符号或包裹字符。`, query, options)

	selectedName, err := routerCallLLM(cfg, prompt)
	if err != nil {
		return Service{}, err
	}

	selectedName = strings.TrimSpace(selectedName)

	// 匹配结果
	for _, s := range services {
		if strings.Contains(strings.ToLower(selectedName), strings.ToLower(s.Name)) {
			return s, nil
		}
	}

	return services[0], nil // 默认选第一个
}

func routerCallLLM(cfg *Config, prompt string) (string, error) {
	payload := map[string]interface{}{
		"model": cfg.ProviderModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个精准的任务调度员，只返回选中的 Agent 名字。"},
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

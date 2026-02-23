package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type AutoCallRequest struct {
	Capability string          `json:"capability"`
	Payload    json.RawMessage `json:"payload"`
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
	Status    string `json:"status"`  // ok / timeout / error
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
		status := StatusOnline
		if latencyMs > busyThreshold.Milliseconds() {
			status = StatusBusy
		}
		updateServiceStatus(serviceName, status, 0, latencyMs)

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

		// 计算价格
		price := selected.Pricing.Price

		// 发起链上支付
		txHash, err := Pay(cfg, selected.Recipient, price)
		if err != nil {
			fmt.Printf("Pay error: %#v\n", err)
			c.JSON(500, gin.H{"error": err.Error()})
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

		// 只在 online/busy 节点中路由
		onlineServices := ListOnlineServices()
		if len(onlineServices) == 0 {
			c.JSON(503, gin.H{
				"error": "当前无可用服务节点（所有节点已离线）",
				"code":  "NO_AVAILABLE_NODES",
			})
			return
		}

		// 选最便宜的在线节点
		var selected Service
		var minPrice int64

		for i, s := range onlineServices {
			if i == 0 || s.Pricing.Price < minPrice {
				selected = s
				minPrice = s.Pricing.Price
			}
		}

		// 1️⃣ 链上支付
		txHash, err := Pay(cfg, selected.Recipient, selected.Pricing.Price)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
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

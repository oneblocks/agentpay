package main

import (
	"bytes"
	"fmt"
	"net/http"
	"io"
	"encoding/json"

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

func SetupRouter(cfg *Config) *gin.Engine {

	r := gin.Default()
	r.Use(cors.Default())

	// 获取所有服务
	r.GET("/services", func(c *gin.Context) {
		c.JSON(200, ListServices())
	})

	// 注册服务
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

	// 删除服务
	r.DELETE("/service/:name", func(c *gin.Context) {

		name := c.Param("name")
		RemoveService(name)

		c.JSON(200, gin.H{
			"message": "service removed",
		})
	})

	// 查询最便宜的服务
	r.POST("/discover", func(c *gin.Context) {

		type DiscoverRequest struct {
			Capability string `json:"capability"`
		}

		var req DiscoverRequest

		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		services := ListServices()

		if len(services) == 0 {
			c.JSON(404, gin.H{"error": "no services registered"})
			return
		}

		// 简单策略：选最便宜
		var selected Service
		var minPrice int64 = 0

		for i, s := range services {
			if i == 0 || s.Pricing.Price < minPrice {
				selected = s
				minPrice = s.Pricing.Price
			}
		}

		c.JSON(200, gin.H{
			"service": selected.Name,
			"price":   selected.Pricing.Price,
			"endpoint": selected.Endpoint,
		})
	})

	// 调用服务
	r.POST("/call/:service", func(c *gin.Context) {

		serviceName := c.Param("service")

		selected, err := GetService(serviceName)
		if err != nil {
			fmt.Println("service not found")
			c.JSON(404, gin.H{"error": "service not found"})
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
			bytes.NewBuffer(bodyBytes), // ✅ 透传原始 body
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

	r.POST("/auto-call", func(c *gin.Context) {

		var req AutoCallRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		services := ListServices()
		if len(services) == 0 {
			c.JSON(404, gin.H{"error": "no services registered"})
			return
		}

		// 简单策略：选最便宜
		var selected Service
		var minPrice int64

		for i, s := range services {
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

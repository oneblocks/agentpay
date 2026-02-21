package main

import (
	"fmt"
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
)

func SetupRouter(cfg *Config) *gin.Engine {

	r := gin.Default()

	// 获取所有服务
	r.GET("/services", func(c *gin.Context) {
		c.JSON(200, GetAllServices())
	})

	// 注册服务
	r.POST("/register", func(c *gin.Context) {

		var s Service
		if err := c.BindJSON(&s); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		if err := RegisterService(s); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{"status": "service registered"})
	})

	// 调用服务
	r.POST("/call/:service", func(c *gin.Context) {

		serviceName := c.Param("service")

		selected, ok := GetService(serviceName)
		if !ok {
			c.JSON(404, gin.H{"error": "service not found"})
			return
		}

		price := CalculatePrice(selected.Pricing, 0)

		if !CheckPolicy(price) {
			c.JSON(403, gin.H{"error": "budget exceeded"})
			return
		}

		txHash, err := Pay(cfg, selected.Recipient, price)
		if err != nil {
			fmt.Printf("Pay error: %#v\n", err)
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		req, _ := http.NewRequest("POST", selected.Endpoint, bytes.NewBuffer([]byte(`{}`)))
		req.Header.Set("X-402-Proof", txHash)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"tx":     txHash,
			"status": resp.Status,
		})
	})

	return r
}

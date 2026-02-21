package main

import (
    "bytes"
    "net/http"

    "github.com/gin-gonic/gin"
)

func SetupRouter(cfg *Config) *gin.Engine {

    r := gin.Default()

    r.GET("/services", func(c *gin.Context) {
        c.JSON(200, Services)
    })

    r.POST("/call/:service", func(c *gin.Context) {

        serviceName := c.Param("service")

        var selected Service
        for _, s := range Services {
            if s.Name == serviceName {
                selected = s
            }
        }

        price := CalculatePrice(selected.Pricing, 0)

        if !CheckPolicy(price) {
            c.JSON(403, gin.H{"error": "budget exceeded"})
            return
        }

        txHash, err := Pay(cfg, "0xRecipientAddress", price)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

        req, _ := http.NewRequest("POST", selected.Endpoint, bytes.NewBuffer([]byte(`{}`)))
        req.Header.Set("X-402-Proof", txHash)

        client := &http.Client{}
        resp, _ := client.Do(req)

        c.JSON(200, gin.H{
            "tx": txHash,
            "status": resp.Status,
        })
    })

    return r
}

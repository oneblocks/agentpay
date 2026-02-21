package main

import (
    "github.com/gin-gonic/gin"
)

func main() {

    r := gin.Default()

    r.POST("/generate", func(c *gin.Context) {

        println("Generate API called")

        proof := c.GetHeader("X-402-Proof")

        if proof == "" {
            println("No payment proof, returning 402")
            c.Header("X-402-Cost", "1000000")
            c.Writer.WriteHeader(402)
            return
        }

        println("Payment proof received:", proof)

        c.JSON(200, gin.H{
            "result": "AI Generated Content",
            "tx": proof,
        })
    })

    r.Run("0.0.0.0:9000")
}

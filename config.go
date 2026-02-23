package main

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	RPCURL              string
	PrivateKey          string
	USDCAddress         string
	HealthCheckInterval time.Duration
}

func LoadConfig() *Config {
	intervalStr := os.Getenv("HEALTH_CHECK_INTERVAL")
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		interval = 10 // 默认 10 秒
	}

	return &Config{
		RPCURL:              os.Getenv("RPC_URL"),
		PrivateKey:          os.Getenv("PRIVATE_KEY"),
		USDCAddress:         os.Getenv("USDC_ADDRESS"),
		HealthCheckInterval: time.Duration(interval) * time.Second,
	}
}

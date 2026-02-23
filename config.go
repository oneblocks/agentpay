package main

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	RPCURL              string
	USDCAddress         string
	HealthCheckInterval time.Duration
	ProviderAPIKey      string
	ProviderBaseURL     string
	ProviderModel       string
}

func LoadConfig() *Config {
	intervalStr := os.Getenv("HEALTH_CHECK_INTERVAL")
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		interval = 3 // 黑客松推荐：3 秒快速监测
	}

	return &Config{
		RPCURL:              os.Getenv("RPC_URL"),
		USDCAddress:         os.Getenv("USDC_ADDRESS"),
		HealthCheckInterval: time.Duration(interval) * time.Second,
		ProviderAPIKey:      os.Getenv("ROUTER_PROVIDER_API_KEY"),
		ProviderBaseURL:     os.Getenv("ROUTER_PROVIDER_BASE_URL"),
		ProviderModel:       os.Getenv("ROUTER_PROVIDER_MODEL"),
	}
}

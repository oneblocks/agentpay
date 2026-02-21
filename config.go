package main

import (
	"os"
)

type Config struct {
	RPCURL      string
	PrivateKey  string
	USDCAddress string
}

func LoadConfig() *Config {

	return &Config{
		RPCURL:      os.Getenv("RPC_URL"),
		PrivateKey:  os.Getenv("PRIVATE_KEY"),
		USDCAddress: os.Getenv("USDC_ADDRESS"),
	}
}

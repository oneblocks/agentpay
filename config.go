package main

import "os"

type Config struct {
    RPC        string
    PrivateKey string
    USDC       string
}

func LoadConfig() *Config {
    return &Config{
        RPC:        os.Getenv("RPC_URL"),
        PrivateKey: os.Getenv("PRIVATE_KEY"),
        USDC:       os.Getenv("USDC_ADDRESS"),
    }
}

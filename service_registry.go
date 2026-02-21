package main

type Service struct {
    Name       string `json:"name"`
    Endpoint   string `json:"endpoint"`
    Pricing    Pricing `json:"pricing"`
}

var Services = []Service{
    {
        Name:     "demo-ai",
        Endpoint: "http://localhost:9000/generate",
        Pricing: Pricing{
            Model: "per_call",
            Price: 1000000, // 1 USDC (6 decimals)
        },
    },
}

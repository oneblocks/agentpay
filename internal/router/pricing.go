package router


type Pricing struct {
      Mode  string `json:"mode"`  // per_call / per_token
      Price int64  `json:"price"` // smallest unit (ex: 1000000 = 1 USDC if 6 decimals)
}

func CalculatePrice(p Pricing, tokens int64) int64 {
    switch p.Mode {
    case "per_call":
        return p.Price
    case "per_token":
        return tokens * p.Price
    case "subscription":
        return p.Price
    }
    return 0
}

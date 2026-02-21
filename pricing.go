package main

type Pricing struct {
    Model string
    Price int64
    PricePerToken int64
}

func CalculatePrice(p Pricing, tokens int64) int64 {
    switch p.Model {
    case "per_call":
        return p.Price
    case "per_token":
        return tokens * p.PricePerToken
    case "subscription":
        return p.Price
    }
    return 0
}

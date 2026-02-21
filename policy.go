package main

var DailyLimit int64 = 10000000 // 10 USDC

var SpentToday int64 = 0

func CheckPolicy(amount int64) bool {
    if SpentToday+amount > DailyLimit {
        return false
    }
    SpentToday += amount
    return true
}

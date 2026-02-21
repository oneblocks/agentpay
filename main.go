package main

func main() {

    cfg := LoadConfig()

    r := SetupRouter(cfg)

    r.Run(":8080")
}

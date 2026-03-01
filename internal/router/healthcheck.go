package router

import (
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// Single ping timeout
	pingTimeout = 5 * time.Second
	// Mark offline after this many consecutive failures
	// Hackathon: 2 for high sensitivity (~6s) while avoiding single glitch
	maxFailCount = 2
)

// StartHealthChecker starts background heartbeat goroutine; probes all registered nodes every interval
func StartHealthChecker(interval time.Duration) {
	go func() {
		log.Println("🫀 [HealthCheck] Heartbeat started, interval:", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run first check immediately, no wait for first tick
		runHealthCheck()

		for range ticker.C {
			runHealthCheck()
		}
	}()
}

// runHealthCheck runs one round of health probes for all registered nodes
func runHealthCheck() {
	allServices := ListServices()
	if len(allServices) == 0 {
		return
	}

	log.Printf("🔍 [HealthCheck] Starting probe round, %d nodes", len(allServices))

	for _, s := range allServices {
		// Each node in its own goroutine, non-blocking
		go pingService(s)
	}
}

// pingService pings a single node and updates its status
func pingService(s Service) {
	// Build ping URL: prefer /health; fallback to root if node does not support it
	pingURL := buildPingURL(s.Endpoint)

	client := &http.Client{
		Timeout: pingTimeout,
	}

	start := time.Now()
	resp, err := client.Get(pingURL)
	elapsed := time.Since(start)
	latencyMs := elapsed.Milliseconds()

	if err != nil {
		// Probe failed: increment fail count
		newFailCount := s.FailCount + 1
		newStatus := s.Status

		if newFailCount >= maxFailCount {
			newStatus = StatusOffline
			log.Printf("🔴 [HealthCheck] Node [%s] %d consecutive failures, marked offline (err: %v)",
				s.Name, newFailCount, err)
		} else {
			log.Printf("⚠️  [HealthCheck] Node [%s] probe failed (%d/%d): %v",
				s.Name, newFailCount, maxFailCount, err)
		}

		updateServiceStatus(s.Name, newStatus, newFailCount, latencyMs)
		return
	}
	defer resp.Body.Close()

	// On success: check status code and latency
	var newStatus NodeStatus
	if resp.StatusCode >= 500 {
		// Server error, treat as failure
		newFailCount := s.FailCount + 1
		newStatus = StatusOffline
		if newFailCount < maxFailCount {
			newStatus = s.Status
		}
		log.Printf("⚠️  [HealthCheck] Node [%s] returned HTTP %d (%d/%d)",
			s.Name, resp.StatusCode, newFailCount, maxFailCount)
		updateServiceStatus(s.Name, newStatus, newFailCount, latencyMs)
		return
	}

	// HTTP success
	newStatus = StatusOnline
	log.Printf("🟢 [HealthCheck] Node [%s] healthy (%dms)", s.Name, latencyMs)

	// Node recovered: reset fail count
	updateServiceStatus(s.Name, newStatus, 0, latencyMs)
}

// buildPingURL Build ping URL from Endpoint (e.g. http://host:9000/chat) by appending /health
func buildPingURL(endpoint string) string {
	// Try to find path and replace with /health
	// e.g. http://127.0.0.1:9000/chat -> http://127.0.0.1:9000/health
	if idx := strings.Index(endpoint, "://"); idx != -1 {
		rest := endpoint[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			base := endpoint[:idx+3] + rest[:slashIdx]
			return base + "/health"
		}
	}
	// If unparseable, append /health to original URL
	return strings.TrimRight(endpoint, "/") + "/health"
}

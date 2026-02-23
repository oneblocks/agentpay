package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// 单次 ping 超时时间
	pingTimeout = 5 * time.Second
	// 连续失败超过此次数自动 offline
	// 黑客松推荐调整为 2 次：在保持高灵敏度（约 6s 感知）的同时，有效避免单次网络抖动导致的误判
	maxFailCount = 2
)

// StartHealthChecker 启动后台心跳检测 goroutine
// 每隔 interval 探测一次所有已注册节点
func StartHealthChecker(interval time.Duration) {
	go func() {
		log.Println("🫀 [HealthCheck] 心跳监测协程已启动，检测间隔:", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// 启动后立即执行一次检测，无需等待第一个 tick
		runHealthCheck()

		for range ticker.C {
			runHealthCheck()
		}
	}()
}

// runHealthCheck 对当前所有已注册节点进行一轮健康探测
func runHealthCheck() {
	allServices := ListServices()
	if len(allServices) == 0 {
		return
	}

	log.Printf("🔍 [HealthCheck] 开始本轮探测，共 %d 个节点", len(allServices))

	for _, s := range allServices {
		// 每个节点独立 goroutine 并发探测，互不阻塞
		go pingService(s)
	}
}

// pingService 向单个节点发送 ping 请求并更新其状态
func pingService(s Service) {
	// 构建 ping 地址：优先使用 /health 端点，若节点不支持则 fallback 到根路径
	pingURL := buildPingURL(s.Endpoint)

	client := &http.Client{
		Timeout: pingTimeout,
	}

	start := time.Now()
	resp, err := client.Get(pingURL)
	elapsed := time.Since(start)
	latencyMs := elapsed.Milliseconds()

	if err != nil {
		// 探测失败：累计失败次数
		newFailCount := s.FailCount + 1
		newStatus := s.Status

		if newFailCount >= maxFailCount {
			newStatus = StatusOffline
			log.Printf("🔴 [HealthCheck] 节点 [%s] 连续 %d 次失败，已自动下线！(err: %v)",
				s.Name, newFailCount, err)
		} else {
			log.Printf("⚠️  [HealthCheck] 节点 [%s] 探测失败 (第 %d/%d 次): %v",
				s.Name, newFailCount, maxFailCount, err)
		}

		updateServiceStatus(s.Name, newStatus, newFailCount, latencyMs)
		return
	}
	defer resp.Body.Close()

	// 探测成功：判断响应码和延迟
	var newStatus NodeStatus
	if resp.StatusCode >= 500 {
		// 服务端错误，视为失败
		newFailCount := s.FailCount + 1
		newStatus = StatusOffline
		if newFailCount < maxFailCount {
			newStatus = s.Status
		}
		log.Printf("⚠️  [HealthCheck] 节点 [%s] 返回 HTTP %d (第 %d/%d 次)",
			s.Name, resp.StatusCode, newFailCount, maxFailCount)
		updateServiceStatus(s.Name, newStatus, newFailCount, latencyMs)
		return
	}

	// HTTP 成功
	newStatus = StatusOnline
	log.Printf("🟢 [HealthCheck] 节点 [%s] 健康 (%dms)", s.Name, latencyMs)

	// 节点恢复：重置失败计数
	updateServiceStatus(s.Name, newStatus, 0, latencyMs)
}

// buildPingURL 构建节点的 ping 检测 URL
// 从 Endpoint（如 http://host:9000/chat）中提取 base URL，拼接 /health
func buildPingURL(endpoint string) string {
	// 尝试找到路径部分并替换为 /health
	// 例：http://127.0.0.1:9000/chat -> http://127.0.0.1:9000/health
	if idx := strings.Index(endpoint, "://"); idx != -1 {
		rest := endpoint[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			base := endpoint[:idx+3] + rest[:slashIdx]
			return base + "/health"
		}
	}
	// 无法解析则直接在原 URL 加 /health
	return strings.TrimRight(endpoint, "/") + "/health"
}

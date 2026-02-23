package main

import (
	"errors"
	"sync"
	"time"
)

// NodeStatus 表示节点的健康状态
type NodeStatus string

const (
	StatusOnline  NodeStatus = "online"  // 节点存活
	StatusOffline NodeStatus = "offline" // 节点离线
	StatusBusy    NodeStatus = "busy"    // 节点繁忙（响应慢）
)

// Service 表示一个注册的 Agent 服务节点
type Service struct {
	Name         string     `json:"name"`
	Endpoint     string     `json:"endpoint"`
	Recipient    string     `json:"recipient"`
	Pricing      Pricing    `json:"pricing"`
	Capabilities []string   `json:"capabilities"`
	Status       NodeStatus `json:"status"`        // 节点健康状态
	FailCount    int        `json:"-"`             // 连续失败次数（不暴露给前端）
	LastChecked  time.Time  `json:"last_checked"`  // 上次心跳检测时间
	Latency      int64      `json:"latency_ms"`    // 上次 ping 延迟（毫秒）
}

var (
	services = make(map[string]Service)
	lock     sync.RWMutex
)

// RegisterService 注册一个新的服务节点，初始状态为 online
func RegisterService(s Service) {
	lock.Lock()
	defer lock.Unlock()
	s.Status = StatusOnline
	s.FailCount = 0
	s.LastChecked = time.Now()
	services[s.Name] = s
}

// GetService 获取单个服务（不过滤状态）
func GetService(name string) (Service, error) {
	lock.RLock()
	defer lock.RUnlock()

	s, ok := services[name]
	if !ok {
		return Service{}, errors.New("service not found")
	}
	return s, nil
}

// ListServices 返回所有服务（包含状态信息，前端按状态排序展示）
func ListServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0, len(services))
	for _, s := range services {
		list = append(list, s)
	}
	return list
}

// ListOnlineServices 返回状态为 online 或 busy 的服务（用于实际路由）
func ListOnlineServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0)
	for _, s := range services {
		if s.Status != StatusOffline {
			list = append(list, s)
		}
	}
	return list
}

// RemoveService 注销一个服务节点
func RemoveService(name string) {
	lock.Lock()
	defer lock.Unlock()
	delete(services, name)
}

// updateServiceStatus 内部更新节点状态（供心跳检测使用）
func updateServiceStatus(name string, status NodeStatus, failCount int, latency int64) {
	lock.Lock()
	defer lock.Unlock()

	s, ok := services[name]
	if !ok {
		return
	}
	s.Status = status
	s.FailCount = failCount
	s.LastChecked = time.Now()
	s.Latency = latency
	services[name] = s
}

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
)

// Service 表示一个注册的 Agent 服务节点
type Service struct {
	Name         string     `json:"name"`
	Endpoint     string     `json:"endpoint"`
	Recipient    string     `json:"recipient"`
	Pricing      Pricing    `json:"pricing"`
	Capabilities []string   `json:"capabilities"`
	Description  string     `json:"description"`   // Agent 特点描述
	Status       NodeStatus `json:"status"`        // 节点健康状态
	IsDisabled   bool       `json:"is_disabled"`   // 是否手动下线
	FailCount    int        `json:"-"`             // 连续失败次数（不暴露给前端）
	LastChecked  time.Time  `json:"last_checked"`  // 上次心跳检测时间
	RegisteredAt time.Time  `json:"registered_at"` // 注册时间
	Latency      int64      `json:"latency_ms"`    // 上次 ping 延迟（毫秒）
}

var (
	services = make(map[string]Service)
	lock     sync.RWMutex
)

// RegisterService 注册一个新的服务节点
func RegisterService(s Service) {
	lock.Lock()
	defer lock.Unlock()

	// 如果之前存在，保留手动下线状态，防止心跳强行恢复
	if existing, ok := services[s.Name]; ok {
		if existing.IsDisabled {
			s.IsDisabled = true
			s.Status = StatusOffline
		} else {
			s.Status = StatusOnline
			s.IsDisabled = false
		}
		s.RegisteredAt = existing.RegisteredAt // 保留初始注册时间
	} else {
		s.Status = StatusOnline
		s.IsDisabled = false
		s.RegisteredAt = time.Now()
	}

	s.LastChecked = time.Now()
	s.FailCount = 0
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

// ListServices 返回所有服务（包含状态信息）
func ListServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0, len(services))
	for _, s := range services {
		list = append(list, s)
	}
	return list
}

// ListOnlineServices 返回可调用的服务节点
func ListOnlineServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0)
	for _, s := range services {
		// 只有在线且未被禁用的节点才参与业务调度
		if s.Status != StatusOffline && !s.IsDisabled {
			list = append(list, s)
		}
	}
	return list
}

// RemoveService 标记一个服务节点手动下线（不再彻底删除）
func RemoveService(name string) {
	lock.Lock()
	defer lock.Unlock()
	if s, ok := services[name]; ok {
		s.Status = StatusOffline
		s.IsDisabled = true
		services[name] = s
	}
}

// updateServiceStatus 内部更新节点状态（供心跳检测使用）
func updateServiceStatus(name string, status NodeStatus, failCount int, latency int64) {
	lock.Lock()
	defer lock.Unlock()

	s, ok := services[name]
	if !ok {
		return
	}

	s.Latency = latency
	s.LastChecked = time.Now()

	// 如果节点处于手动下线状态，它依然会接受心跳轮询以展示延迟，但状态强行保持为离线
	if s.IsDisabled {
		s.Status = StatusOffline
	} else {
		s.Status = status
		s.FailCount = failCount
	}

	services[name] = s
}

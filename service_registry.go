package main

import (
	"errors"
	"sync"
	"time"
)

// NodeStatus represents node health status
type NodeStatus string

const (
	StatusOnline  NodeStatus = "online"  // Node alive
	StatusOffline NodeStatus = "offline" // Node offline
)

// Service represents a registered Agent service node
type Service struct {
	Name         string     `json:"name"`
	Endpoint     string     `json:"endpoint"`
	Recipient    string     `json:"recipient"`
	Pricing      Pricing    `json:"pricing"`
	Capabilities []string   `json:"capabilities"`
	Description  string     `json:"description"`   // Agent specialty description
	Status       NodeStatus `json:"status"`        // Node health status
	IsDisabled   bool       `json:"is_disabled"`   // Manually taken offline
	FailCount    int        `json:"-"`             // Consecutive failure count (not exposed to frontend)
	LastChecked  time.Time  `json:"last_checked"`  // Last heartbeat check time
	RegisteredAt time.Time  `json:"registered_at"` // Registration time
	Latency      int64      `json:"latency_ms"`    // Last ping latency (ms)
}

var (
	services = make(map[string]Service)
	lock     sync.RWMutex
)

// RegisterService registers a new service node
func RegisterService(s Service) {
	lock.Lock()
	defer lock.Unlock()

	// If already exists, keep manual offline state so heartbeat does not force recovery
	if existing, ok := services[s.Name]; ok {
		if existing.IsDisabled {
			s.IsDisabled = true
			s.Status = StatusOffline
		} else {
			s.Status = StatusOnline
			s.IsDisabled = false
		}
		s.RegisteredAt = existing.RegisteredAt // Keep initial registration time
	} else {
		s.Status = StatusOnline
		s.IsDisabled = false
		s.RegisteredAt = time.Now()
	}

	s.LastChecked = time.Now()
	s.FailCount = 0
	services[s.Name] = s
}

// GetService returns a single service (no status filter)
func GetService(name string) (Service, error) {
	lock.RLock()
	defer lock.RUnlock()

	s, ok := services[name]
	if !ok {
		return Service{}, errors.New("service not found")
	}
	return s, nil
}

// ListServices returns all services (with status)
func ListServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0, len(services))
	for _, s := range services {
		list = append(list, s)
	}
	return list
}

// ListOnlineServices returns callable service nodes
func ListOnlineServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0)
	for _, s := range services {
		// Only online and non-disabled nodes participate in dispatch
		if s.Status != StatusOffline && !s.IsDisabled {
			list = append(list, s)
		}
	}
	return list
}

// RemoveService marks a service node as manually offline (no longer hard delete)
func RemoveService(name string) {
	lock.Lock()
	defer lock.Unlock()
	if s, ok := services[name]; ok {
		s.Status = StatusOffline
		s.IsDisabled = true
		services[name] = s
	}
}

// ReenableService re-enables a manually offline node
func ReenableService(name string) error {
	lock.Lock()
	defer lock.Unlock()
	s, ok := services[name]
	if !ok {
		return errors.New("service not found")
	}
	s.IsDisabled = false
	// Mark offline, next heartbeat will restore to online
	s.Status = StatusOffline
	services[name] = s
	return nil
}

// updateServiceStatus updates node status internally (used by heartbeat)
func updateServiceStatus(name string, status NodeStatus, failCount int, latency int64) {
	lock.Lock()
	defer lock.Unlock()

	s, ok := services[name]
	if !ok {
		return
	}

	s.Latency = latency
	s.LastChecked = time.Now()

	// If node is manually offline, it still gets heartbeat for latency display but status stays offline
	if s.IsDisabled {
		s.Status = StatusOffline
	} else {
		s.Status = status
		s.FailCount = failCount
	}

	services[name] = s
}

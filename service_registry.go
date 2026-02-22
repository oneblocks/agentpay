package main

import (
	"errors"
	"sync"
)

type Service struct {
    Name        string   `json:"name"`
    Endpoint    string   `json:"endpoint"`
    Recipient   string   `json:"recipient"`
    Pricing     Pricing  `json:"pricing"`
    Capabilities []string `json:"capabilities"`
}

var (
	services = make(map[string]Service)
	lock     sync.RWMutex
)

// 注册服务
func RegisterService(s Service) {
	lock.Lock()
	defer lock.Unlock()
	services[s.Name] = s
}

// 获取单个服务
func GetService(name string) (Service, error) {
	lock.RLock()
	defer lock.RUnlock()

	s, ok := services[name]
	if !ok {
		return Service{}, errors.New("service not found")
	}
	return s, nil
}

// 获取所有服务
func ListServices() []Service {
	lock.RLock()
	defer lock.RUnlock()

	list := make([]Service, 0)
	for _, s := range services {
		list = append(list, s)
	}
	return list
}

// 删除服务
func RemoveService(name string) {
	lock.Lock()
	defer lock.Unlock()
	delete(services, name)
}

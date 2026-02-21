package main

import (
	"errors"
	"sync"
)

//type Pricing struct {
//	Mode  string `json:"mode"`  // per_call / per_token
//	Price int64  `json:"price"` // smallest unit (ex: 1000000 = 1 USDC if 6 decimals)
//}

type Service struct {
	Name      string  `json:"name"`
	Endpoint  string  `json:"endpoint"`
	Recipient string  `json:"recipient"`
	Pricing   Pricing `json:"pricing"`
}

var (
	serviceStore = make(map[string]Service)
	storeMutex   sync.RWMutex
)

func RegisterService(s Service) error {

	if s.Name == "" || s.Endpoint == "" || s.Recipient == "" {
		return errors.New("invalid service config")
	}

	storeMutex.Lock()
	defer storeMutex.Unlock()

	serviceStore[s.Name] = s
	return nil
}

func GetService(name string) (Service, bool) {

	storeMutex.RLock()
	defer storeMutex.RUnlock()

	s, ok := serviceStore[name]
	return s, ok
}

func GetAllServices() []Service {

	storeMutex.RLock()
	defer storeMutex.RUnlock()

	list := []Service{}
	for _, s := range serviceStore {
		list = append(list, s)
	}
	return list
}

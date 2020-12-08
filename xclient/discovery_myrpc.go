package xclient

import (
	"log"
	"myrpc/registry"
	"net/http"
	"strings"
	"time"
)

type MyRegistryDiscovery struct {
	*MultiServersDiscovery
	register   string
	timeout    time.Duration
	lastUpdate time.Time
}

const defaultUpdateTime = time.Second * 10

func NewMyRegistryDiscovery(registryAddr string, timeout time.Duration) *MyRegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTime
	}
	d := &MyRegistryDiscovery{
		MultiServersDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		register:              registryAddr,
		timeout:               timeout,
	}
	return d
}

func (m *MyRegistryDiscovery) Update(servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = servers
	m.lastUpdate = time.Now()
	return nil
}

// Refresh: 向注册中心拉取所有存活的 server 地址
// 然后更新到自己的 servers 列表
func (m *MyRegistryDiscovery) Refresh() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastUpdate.Add(m.timeout).After(time.Now()) {
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", m.register)
	resp, err := http.Get(m.register)
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}
	servers := strings.Split(resp.Header.Get(registry.RpcServerHeader), ",")
	m.servers = make([]string, 0, len(servers))
	// 去空格
	for _, server := range servers {
		s := strings.TrimSpace(server)
		if s != "" {
			m.servers = append(m.servers, s)
		}
	}
	m.lastUpdate = time.Now()
	return nil
}

func (m *MyRegistryDiscovery) Get(mode SelectMode) (string, error) {
	if err := m.Refresh(); err != nil{
		return "", err
	}
	return m.MultiServersDiscovery.Get(mode)
}

func (m *MyRegistryDiscovery) GetAll() ([]string, error) {
	if err := m.Refresh(); err != nil {
		return nil, err
	}
	return m.MultiServersDiscovery.servers, nil
}


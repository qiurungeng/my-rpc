package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const(
	RandomSelect SelectMode = iota
	RoundRobinSelect
)

const errorPrefix = "rpc recovery: "

type Discovery interface {
	Refresh() error
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}

var _ Discovery = (*MultiServersDiscovery)(nil)

type MultiServersDiscovery struct {
	r       *rand.Rand 		// r 是一个产生随机数的实例
	mu      sync.Mutex
	servers []string		// 存储有各 server 的 rpcAddr
	index   int				// index 记录 Round Robin 算法已经轮询到的位置
}

func NewMultiServerDiscovery(servers []string) *MultiServersDiscovery {
	m := &MultiServersDiscovery{
		servers: servers,
		// 初始化时使用时间戳设定随机数种子，避免每次产生相同的随机数序列
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	// 为了避免每次从 0 开始，初始化时随机设定一个值
	m.index = m.r.Intn(math.MaxInt32 - 1)
	return m
}

func (m *MultiServersDiscovery) Refresh() error {
	return nil
}

func (m *MultiServersDiscovery) Update(servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = servers
	return nil
}

func (m *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.servers)
	if n == 0 {
		return "", errors.New(errorPrefix + "no available server")
	}
	switch mode {
	case RandomSelect:
		return m.servers[m.r.Intn(n)], nil
	case RoundRobinSelect:
		server := m.servers[m.index % n]
		m.index = (m.index + 1) % n
		return server, nil
	default:
		return "", errors.New(errorPrefix + "not support select mode")
	}
}

func (m *MultiServersDiscovery) GetAll() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	servers := make([]string, len(m.servers), len(m.servers))
	copy(servers, m.servers)
	return servers, nil
}
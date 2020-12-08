package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type MyRegistry struct {
	timeout time.Duration
	mu sync.Mutex
	servers map[string]*ServerItem
}

type ServerItem struct {
	Addr string
	start time.Time
}

const (
	defaultPath     = "/_myrpc_/registry"
	defaultTimeout  = time.Minute * 5
	RpcServerHeader = "X-Myrpc-Servers"
)

func New(timeout time.Duration) *MyRegistry {
	return &MyRegistry{timeout: timeout, servers: make(map[string]*ServerItem)}
}

var DefaultMyRegistry = New(defaultTimeout)

func (r *MyRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil {
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else {
		r.servers[addr].start = time.Now() // 更新 start 时间, 保活
	}
}

func (r *MyRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *MyRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// 处理 client 的请求
		w.Header().Set(RpcServerHeader, strings.Join(r.aliveServers(), ","))
	case "POST":
		// 处理 server 的心跳消息
		addr := req.Header.Get(RpcServerHeader)
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *MyRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultMyRegistry.HandleHTTP(defaultPath)
}

// Heartbeat: addr 上存活的 server 实例每隔一端时间向注册中心发心跳消息,
// 证明自己还活着, 防止被注册中心定时清除
func Heartbeat(register, addr string, duration time.Duration) {
	if duration == 0 {
		// 在被移除之前发送心跳
		duration = defaultTimeout - time.Duration(1) * time.Minute
	}
	var err error
	err = sendHeartbeat(register, addr)
	go func() {
		ticker := time.NewTicker(duration)
		for err != nil {
			<- ticker.C
			err = sendHeartbeat(register, addr)
		}
	}()
}

func sendHeartbeat(registry, addr string) error {
	client := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set(RpcServerHeader, addr)
	_, err := client.Do(req)
	if err != nil {
		log.Println("rpc server: heartbeat error:", err)
		return err
	}
	return nil
}
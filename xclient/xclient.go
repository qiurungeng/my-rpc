package xclient

import (
	"context"
	"io"
	. "myrpc"
	"reflect"
	"sync"
)

type XClient struct {
	discovery Discovery
	mode      SelectMode
	opt       *Option
	mu        sync.Mutex
	clients   map[string]*Client
}

var _ io.Closer = (*XClient)(nil)

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

func NewXClient(d Discovery, mode SelectMode, opt *Option) *XClient {
	return &XClient{discovery: d, mode: mode, opt: opt, clients: make(map[string]*Client)}
}


func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, rpcAddr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = XDial(rpcAddr)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

// Call: 根据 SelectMode 从 discovery 中选择一个 server
// 然后 XDial 连接到该 server 得到 client 实例
func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	// 从服务中心获取 server 的 rpcAddr
	rpcAddr, err := xc.discovery.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// Broadcast: 对已在发现中注册的每个服务, 调用已命名函数
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.discovery.GetAll()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var e error
	replyDone := reply == nil
	ctx, cancelFunc := context.WithCancel(ctx)
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var cloneReply interface{}
			if reply != nil {
				cloneReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, ctx, serviceMethod, args, cloneReply)
			mu.Lock()
			if err != nil && e == nil{
				e = err
				cancelFunc() // if any call failed, cancel unfinished calls
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cloneReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}


package myrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"myrpc/codec"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 封装了结构体 Call 来承载一次 RPC 调用所需要的信息
// 其中, Done 字段用于当结束调用时异步通知对方
type Call struct {
	Seq           uint64
	ServiceMethod string		// format "<service>.<method>"
	Args          interface{}	// arguments to function
	Reply         interface{}	// reply from the function
	Error         error			// if error occurs, it will be set
	Done          chan *Call	// Strobes when call is templated
}

// 当调用结束时，会调用 call.done() 通知调用方
func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	codec        codec.Codec
	option       *Option
	sendingMutex sync.Mutex // 保证消息有序发送, 防止多请求报文混淆
	header       codec.Header // 只在请求发送时需要, 可以服用, 每个客户端只需要一个
	mutex        sync.Mutex // protect following
	seq          uint64
	pending      map[uint64]*Call // 存储未处理完的请求, 键是编号, 值是Call实例
	closing      bool // user has called Close
	shutdown     bool // server has told us to close
}

var ErrShutdown = errors.New("connection is shutdown")

var _ io.Closer = (*Client)(nil)

func (c *Client) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return c.codec.Close()
}

// Check if client is available
func (c *Client) IsAvailable() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return !c.shutdown && !c.closing
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

// terminateCalls: 服务端或客户端发生错误时调用
// shutdown = true, 将错误属性通知所有 pending 状态的 call
func (c *Client) terminateCalls(err error) {
	c.sendingMutex.Lock()
	defer c.sendingMutex.Unlock()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

//receive: 客户端接收消息
func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err := c.codec.ReadHeader(&h); err != nil {
			break
		}
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = c.codec.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = c.codec.ReadBody(nil)
			call.done()
		default:
			err = c.codec.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body" + err.Error())
			}
			call.done()
		}
	}
	c.terminateCalls(err)
}

func (c *Client) send(call *Call){
	c.sendingMutex.Lock()
	defer c.sendingMutex.Unlock()

	// register
	seq, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// prepare req header
	c.header.ServiceMethod = call.ServiceMethod
	c.header.Seq = seq
	c.header.Error = ""

	if err = c.codec.Write(&c.header, call.Args); err != nil {
		call := c.removeCall(seq)
		// call may be nil, it usually means that Write partially failed,
		// client has received the response and handled
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}


// Dial: 客户端发送连接请求
// 返回连接后的客户端实例 client
func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func DialHTTP(network, address string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

// XDial 由 rpcAddr 确定以何种协议连接到 RPC server
// rpcAddr 是代表 rpc 服务地址及连接方式的格式化字符 (protocol@addr)
// eg, http@10.0.0.1:7001, tcp@10.0.0.1:9999, unix@/tmp/myrpc.sock
func XDial(rpcAddr string, opt ...*Option) (*Client, error) {
	split := strings.Split(rpcAddr, "@")
	if len(split) != 2 {
		return nil, fmt.Errorf("rpc client err: wrong format '%s', expect 'protocol@addr'", rpcAddr)
	}
	protocol, addr := split[0], split[1]
	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opt...)
	default:
		return Dial(protocol, addr, opt...)
	}
}

func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	option, err := parseOption(opts...)
	if err != nil {
		return nil, err
	}

	// 与服务端连接, 需考虑到超时问题
	conn, err := net.DialTimeout(network, address, option.ConnectTimeout)
	if err != nil {
		log.Println("rpc client: fail to dial:", address, err)
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()

	// 创建Client需要将option发给服务端, 故也需超时校验
	ch:= make(chan dialResult)
	go func() {
		client, err := f(conn, option)
		ch <- dialResult{client: client, err: err}
	}()
	if option.ConnectTimeout == 0 {
		res := <- ch
		return res.client, res.err
	}
	select {
	case <-time.After(option.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s second", option.ConnectTimeout)
	case res := <-ch:
		return res.client, res.err
	}
}

type dialResult struct {
	client *Client
	err    error
}

type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

// NewHTTPClient: 通过HTTP传输协议新建一个 Client
func NewHTTPClient(conn net.Conn, opt *Option) (*Client, error) {
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		return NewClient(conn, opt)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
		log.Println(err)
	}
	return nil, err
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: codec error:", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(c codec.Codec, opt *Option) *Client{
	client := &Client{
		seq: 1,
		codec: c,
		option: opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOption(opts ...*Option) (*Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}


// Go: invokes the function asynchronously
// It returns the Call structure representing the invocation
func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args: args,
		Reply: reply,
		Done: done,
	}
	c.send(call)
	return call
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		c.removeCall(call.Seq)
		return errors.New("rpc client: call fail:" + ctx.Err().Error())
	case <-call.Done:
		return call.Error
	}
}
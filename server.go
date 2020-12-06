package myrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"myrpc/codec"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber    int
	CodecType      codec.Type
	ConnectTimeout time.Duration
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: time.Second * 10,
}


type Server struct{
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (server *Server) Accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("rpc server: accept error: ", err)
			return
		}
		go server.ServeConn(conn)
	}
}

func (server *Server) ServeConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	var option Option
	if err := json.NewDecoder(conn).Decode(&option); err != nil{
		log.Println("rpc server: options error:", err)
		return
	}
	if option.MagicNumber != MagicNumber {
		log.Println("rpc server: invalid magic number:", option.MagicNumber)
		return
	}
	codecFunc := codec.NewCodecFuncMap[option.CodecType]
	if codecFunc == nil {
		log.Println("rpc server: invalid codec type:", option.CodecType)
		return
	}
	server.ServeCodec(codecFunc(conn), &option)
}

func (server *Server) ServeCodec(c codec.Codec, opt *Option) {
	sendingMutex := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for{
		req, err := server.readRequest(c)
		if err != nil {
			if req == nil {
				 break // it's not possible to recover, so close the connection
			}
			req.h.Error = err.Error()
			server.sendResponse(c, req.h, invalidRequest, sendingMutex)
			continue
		}
		wg.Add(1)
		server.handleRequest(c, req, sendingMutex, wg, opt.HandleTimeout)
	}
}

// Register: 服务注册
func (server *Server) Register(receiver interface{}) error {
	s := newService(receiver)
	if _, loaded := server.serviceMap.LoadOrStore(s.name, s); loaded {
		return errors.New("rpc: service already defined:" + s.name)
	}
	return nil
}

// DefaultServer.Register
func Register(receiver interface{}) error {
	return DefaultServer.Register(receiver)
}

func (server *Server) findService(serviceMethod string) (svc *service, mType *methodType, err error) {
	docIdx := strings.LastIndex(serviceMethod, ".")
	if docIdx < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName := serviceMethod[:docIdx]
	methodName := serviceMethod[docIdx + 1:]
	s, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service:" + serviceName)
		return
	}
	svc = s.(*service)
	mType = svc.methods[methodName]
	if mType == nil {
		err = errors.New("rpc server: can't find method:" + methodName)
	}
	return
}


type request struct {
	h            *codec.Header
	argv, replyv reflect.Value
	mType        *methodType
	svc          *service
}

var invalidRequest = struct{}{} // a placeholder for response argv when error occurs

func (server *Server) readRequestHeader(c codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := c.ReadHeader(&h) ; err != nil {
		log.Println("rpc server: read header error:", err)
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(c codec.Codec) (*request, error) {
	header, err := server.readRequestHeader(c)
	if err != nil {
		return nil, err
	}

	req := &request{h: header}
	req.svc, req.mType, err = server.findService(header.ServiceMethod)
	if err != nil {
		return req, err
	}
	// 通过 newArgv() 和 newReplyv() 两个方法创建出两个入参实例
	req.argv = req.mType.newArgv()
	req.replyv = req.mType.newReplyv()

	// 确保 argvi 为 pointer
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	// 将请求报文反序列化为第一个入参 argv
	if err := c.ReadBody(argvi); err != nil {
		log.Println("rpc server: read argv err:", err)
		return req, err
	}

	return req, nil
}

func (server *Server) sendResponse(c codec.Codec, h *codec.Header, body interface{}, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()
	if err := c.Write(h, body); err != nil{
		log.Println("rpc server: write response err:", err)
	}
}

func (server *Server) handleRequest(c codec.Codec, req *request, sendingMutex *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	// 存在两个可能超时的地方: 调用映射的服务方法处理报文超时, 发送响应报文超时
	call := make(chan struct{})
	send := make(chan struct{})

	go func() {
		err := req.svc.call(req.mType, req.argv, req.replyv)
		call <- struct{}{}

		if err != nil {
			req.h.Error = err.Error()
			server.sendResponse(c, req.h, invalidRequest, sendingMutex)
			send <- struct{}{}
			return
		}
		server.sendResponse(c, req.h, req.replyv.Interface(), sendingMutex)
		send <- struct{}{}
	}()

	if timeout == 0 {
		<- call
		<- send
		return
	}
	select {
	case <-time.After(timeout):
		// TODO 是否有 goroutine 泄露的风险?
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		server.sendResponse(c, req.h, invalidRequest, sendingMutex)
		return
	case <-call:
		<-send
	}
}

func sendToChannel(c chan struct{}, msg struct{}, shutdown ...chan struct{}) {
	if len(shutdown) == 1  {
		closer := shutdown[0]
		select {
		case <-closer:
			return
		case c <- msg:
		}
	} else {
		c <- msg
	}
}

func Accept(listener net.Listener) {
	DefaultServer.Accept(listener)
}


package myrpc

import (
	"encoding/json"
	"fmt"
	"log"
	"myrpc/codec"
	"net"
	"reflect"
	"sync"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int
	CodecType   codec.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType: codec.GobType,
}


type Server struct{}

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
	server.ServeCodec(codecFunc(conn))
}

func (server *Server) ServeCodec(c codec.Codec) {
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
		server.handleRequest(c, req, sendingMutex, wg)
	}
}


type request struct {
	h            *codec.Header
	argv, replyv reflect.Value
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
	req.argv = reflect.New(reflect.TypeOf(""))	// 暂时假设参数类型都为 string
	if err := c.ReadBody(req.argv.Interface()); err != nil {
		log.Println("rpc server: read argv err:", err)
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

func (server *Server) handleRequest(c codec.Codec, req *request, mutex *sync.Mutex, wg *sync.WaitGroup) {
	// TODO, should call registered rpc methods to get the right replyv
	// day 1, just print argv and send a hello message
	defer wg.Done()
	log.Println(req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("myrpc resp %d", req.h.Seq))
	server.sendResponse(c, req.h, req.replyv.Interface(), mutex)
}



func Accept(listener net.Listener) {
	DefaultServer.Accept(listener)
}


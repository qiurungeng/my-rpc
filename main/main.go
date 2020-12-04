package main

import (
	"encoding/json"
	"fmt"
	"log"
	"myrpc"
	"myrpc/codec"
	"net"
	"time"
)

func startServer(addr chan string) {
	listen, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on:", listen.Addr())
	addr <- listen.Addr().String()
	myrpc.Accept(listen)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)
	//send options
	_ = json.NewEncoder(conn).Encode(myrpc.DefaultOption)
	c := codec.NewGobCodec(conn)

	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq: uint64(i),
		}
		_ = c.Write(h, fmt.Sprintf("myrpc seq %d", h.Seq))
		var reply string
		_ = c.ReadHeader(h)
		_ = c.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}
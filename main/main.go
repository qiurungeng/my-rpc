package main

import (
	"fmt"
	"log"
	"myrpc"
	"net"
	"sync"
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

	client, _ := myrpc.Dial("tcp", <- addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)
	//
	////send options
	//_ = json.NewEncoder(conn).Encode(myrpc.DefaultOption)
	//c := codec.NewGobCodec(conn)
	//
	//for i := 0; i < 5; i++ {
	//	h := &codec.Header{
	//		ServiceMethod: "Foo.Sum",
	//		Seq: uint64(i),
	//	}
	//	_ = c.Write(h, fmt.Sprintf("myrpc seq %d", h.Seq))
	//	var reply string
	//	_ = c.ReadHeader(h)
	//	_ = c.ReadBody(&reply)
	//	log.Println("reply:", reply)
	//}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int ) {
			defer wg.Done()
			args := fmt.Sprintf("myrpc req %d", i)
			var reply string

			if err := client.Call("", args, &reply); err != nil {
				log.Fatal(err)
			}
			log.Println("reply:", reply)
		}(i)
	}
	wg.Wait()
}
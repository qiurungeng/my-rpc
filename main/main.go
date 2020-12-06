package main

import (
	"log"
	"myrpc"
	"net"
	"sync"
	"time"
)

func startServer(addr chan string) {
	// Register
	var foo Foo
	if err := myrpc.Register(foo); err != nil {
		log.Fatal("register error:", err)
	}

	listen, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on:", listen.Addr())
	addr <- listen.Addr().String()
	myrpc.Accept(listen)
}

func main() {
	log.SetFlags(0)
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
			args := &Args{ Num1: i, Num2: i * i }
			var reply int

			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d\n", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}


type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}
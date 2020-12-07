package main

import (
	"context"
	"log"
	"myrpc"
	"net"
	"net/http"
	"sync"
	"time"
)

func startServer(addr chan string) {
	// Register
	var foo Foo
	listen, _ := net.Listen("tcp", ":9999")
	_ = myrpc.Register(&foo)
	myrpc.HandleHTTP()
	addr <- listen.Addr().String()
	_ = http.Serve(listen, nil)
}

func call(addrCh chan string) {
	client, _ := myrpc.DialHTTP("tcp", <- addrCh)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{ Num1: i, Num2: i * i }
			var reply int

			//ctx, _ := context.WithTimeout(context.Background(), time.Second)
			if err := client.Call(context.Background(), "Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d\n", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go call(addr)
	startServer(addr)

}


type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}
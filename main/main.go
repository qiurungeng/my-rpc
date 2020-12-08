package main

import (
	"context"
	"log"
	"myrpc"
	"myrpc/xclient"
	"net"
	"sync"
	"time"
)

//func startServer(addr chan string) {
//	// Register
//	var foo Foo
//	listen, _ := net.Listen("tcp", ":9999")
//	_ = myrpc.Register(&foo)
//	myrpc.HandleHTTP()
//	addr <- listen.Addr().String()
//	_ = http.Serve(listen, nil)
//}

//func call(addrCh chan string) {
//	client, _ := myrpc.DialHTTP("tcp", <- addrCh)
//	defer func() { _ = client.Close() }()
//
//	time.Sleep(time.Second)
//
//	var wg sync.WaitGroup
//	for i := 0; i < 5; i++ {
//		wg.Add(1)
//		go func(i int) {
//			defer wg.Done()
//			args := &Args{ Num1: i, Num2: i * i }
//			var reply int
//
//			//ctx, _ := context.WithTimeout(context.Background(), time.Second)
//			if err := client.Call(context.Background(), "Foo.Sum", args, &reply); err != nil {
//				log.Fatal("call Foo.Sum error:", err)
//			}
//			log.Printf("%d + %d = %d\n", args.Num1, args.Num2, reply)
//		}(i)
//	}
//	wg.Wait()
////}
//
//func main() {
//	log.SetFlags(0)
//	addr := make(chan string)
//	go call(addr)
//	startServer(addr)
//
//}


type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

// Sleep: 用于验证 XClient 的超时机制能否正常运作
func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func foo(xc *xclient.XClient, ctx context.Context, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func call(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "call", "Foo.Sum", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func broadcast(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i})
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			foo(xc, ctx, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	ch1 := make(chan string)
	ch2 := make(chan string)

	go startServer(ch1)
	go startServer(ch2)

	addr1 := <- ch1
	addr2 := <- ch2
	time.Sleep(time.Second)
	call(addr1, addr2)
	broadcast(addr1, addr2)
}

func startServer(addrCh chan string) {
	var foo Foo
	listen, _ := net.Listen("tcp", ":0")
	server := myrpc.NewServer()
	_ = server.Register(&foo)
	addrCh <- listen.Addr().String()
	server.Accept(listen)
}
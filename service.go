package myrpc

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	numCalls  uint64 // 后续统计方法调用次数时会用到
}

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

// newArgv: 创建 Argv 实例
func (m *methodType) newArgv() reflect.Value {
	if m.ArgType.Kind() == reflect.Ptr {
		return reflect.New(m.ArgType.Elem())
	} else {
		return reflect.New(m.ArgType).Elem()
	}
}

// newReplyv: 创建 Replyv 实例
func (m *methodType) newReplyv() reflect.Value {
	// because reply is a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}


type service struct {
	name         string                 // 被映射结构体的名称
	receiverType reflect.Type           // 被映射结构体类型
	receiver     reflect.Value          // 结构体实例本身, 保留是因为在调用方法时需要其自身作为第一个参数
	methods      map[string]*methodType // 存储结构体所有符合条件的方法
}

func newService(receiver interface{}) *service {
	s := new(service)
	s.receiver = reflect.ValueOf(receiver)
	s.receiverType = reflect.TypeOf(receiver)
	s.name = reflect.Indirect(s.receiver).Type().Name()
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethods()
	return s
}

// 注册符合条件的方法:
// 两个入参: argv, &replyv; 一个返回值: error
func (s *service) registerMethods() {
	s.methods = make(map[string]*methodType)
	for i := 0; i < s.receiverType.NumMethod(); i++ {
		m := s.receiverType.Method(i)
		mType := m.Type

		// 过滤出符合条件的方法进行注册
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if  !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType){
			continue
		}

		s.methods[m.Name] = &methodType{
			method: m,
			ArgType: argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, m.Name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.receiver, argv, replyv})
	if err := returnValues[0].Interface(); err != nil {
		return err.(error)
	}
	return nil
}
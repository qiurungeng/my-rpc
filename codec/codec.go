package codec

import "io"

type Header struct {
	ServiceMethod string
	Seq           uint64
	Error         string
}

// Codec: 消息的编码解码接口
type Codec interface {
	io.Closer	// 内嵌Closer接口
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

// Codec构造函数工厂
type NewCodecFunc func(closer io.ReadWriteCloser) Codec

type Type string

const(
	GobType  Type = "application/gob"
	JsonType Type = "application/json"
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
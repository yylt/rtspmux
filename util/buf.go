package util

import (
	"bytes"
	"io"
	"sync"
	"unsafe"
)

const (
	mbSize = 1024 * 1024
)

var (
	bufpool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
	bytespool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 32<<10)
			return &buffer
		},
	}
)


func IoCopy(src io.Reader,dst io.Writer) error{
	buf := GetBytes()
	_, err := io.CopyBuffer(dst,src,*buf)
	PutBytes(buf)
	return err
}

func GetBytes() *[]byte {
	return bytespool.Get().(*[]byte)
}

func PutBytes(b  *[]byte) {
	bytespool.Put(b)
}


func GetBuf() *bytes.Buffer {
	buf := bufpool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func PutBuf(b *bytes.Buffer) {
	bufpool.Put(b)
}

func Str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	b := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&b))
}

func Bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

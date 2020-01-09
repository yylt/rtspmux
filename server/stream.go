package server

import (
	"fmt"
	"net/url"
	"hash/crc32"
	"sync"
	"hash"

	"github.com/nareix/joy4/av"
	"io"
	"time"
)

type MuxStream struct {
	streams map[string]*Stream

	mu sync.RWMutex
}

type Stream struct {
	fp string
	remote *url.URL

	retry time.Duration
	started bool

}

var (
	ha hash.Hash32
)

func init(){
	ha = crc32.New(crc32.IEEETable)
}


func NewMuxStream(url2 ...string) *MuxStream{
	var (
		ms = new(MuxStream)
	)
	for _,u := range url2{
		s,err:=ms.SetStream(u)
		if err!=nil{
			panic(err)
		}
		ms.streams[s.fp]=s
	}
	return ms
}

func (mux *MuxStream) add(s *Stream) error {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	mux.streams[s.fp]=s
	return nil
}

func (mux *MuxStream) SetStream(s string) (*Stream,error) {
	st ,err := NewStream(s)
	if err!= nil{
		return nil,err
	}
	return st,mux.add(st)
}

func (mux *MuxStream) GetStream(key string) (*Stream) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()
	return mux.streams[key]
}


func (mux *MuxStream) Iter(fn func(key string, s *Stream)) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()
	for k,v:=range mux.streams{
		fn(k,v)
	}
}

func NewStream(s string) (*Stream,error) {
	u,err:=url.Parse(s)
	if err!= nil{
		return nil,err
	}
	news := &Stream{
		fp:fingerPrint([]byte(s)),
		remote:u,
		retry: time.Second * 3,
	}
	err = news.valid()
	if err!= nil{
		return nil,err
	}
	return news,nil
}


func (s *Stream) valid() error{
	var (
		url2 = s.remote
	)
	switch url2.Scheme{
	case "rtsp":
	case "rtmp":
	case "file":
	default:
		return fmt.Errorf("scheme %s not support",url2.Scheme)
	}
	if url2.Host==""{
		return fmt.Errorf("host is none")
	}
	return nil
}

func (s *Stream) CopyTo(det *Stream) {
	if s.started{

	}
}

func (s *Stream) CopyFrom(src *Stream) {

}

func (s *Stream) conn( ) error {

}



func fingerPrint(bs []byte) string{
	//need mutex
	ha.Reset()
	ha.Write([]byte(bs))
	return fmt.Sprintf("%x",ha.Sum32())
}

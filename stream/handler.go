package stream

import (
	"net/http"
	"errors"
	"bytes"
	"sync"
)
var (
	ErrHadAdd = errors.New("had add")
	ErrNotAdd = errors.New("not add")
	ErrNameIncorrect = errors.New("name is incorrect")
	bufPool           = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		}}
)

type Handler interface {
	AddStreams(s *Stream) error
	DelStreams(id string)
	HandlerStream(w http.ResponseWriter,req *http.Request)
	HandlerIndex(w http.ResponseWriter,req *http.Request)
	Stop()
}


type Saver interface {
	Start(s *Stream)
	Stop()
}

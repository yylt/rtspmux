package stream

import (
	"fmt"
	"net/url"
	"hash/crc32"

	"time"

	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/av/avutil"
	"github.com/nareix/joy4/av/pubsub"
	"github.com/nareix/joy4/format/rtsp"
	"github.com/nareix/joy4/format/rtmp"
	ants "github.com/panjf2000/ants/v2"

)

var (

	packetMaxSize = 64
)

type Stream struct {
	fp string
	remote *url.URL

	queue *pubsub.Queue

	demux av.Demuxer

	stopch chan struct{}
	startch chan struct{}
	pool *ants.Pool
}

func NewStream(s string,pool *ants.Pool) (*Stream,error) {
	u,err:=url.Parse(s)
	if err!= nil{
		return nil,err
	}
	queue:=pubsub.NewQueue()
	queue.SetMaxGopCount(packetMaxSize)
	news := &Stream{
		fp:fingerPrint([]byte(s)),
		remote:u,
		stopch: make(chan struct{}),
		startch:make(chan struct{}),
		queue:queue,
		pool:pool ,
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
	default:
		return fmt.Errorf("scheme %s not support",url2.Scheme)
	}
	if url2.Host==""{
		return fmt.Errorf("host is none")
	}
	return nil
}

func (s *Stream) Start( ) {
	go s.run(time.Minute * 5)
}

func (s *Stream) Stop( ) {
	close(s.stopch)
}

func (s *Stream) run(maxwait time.Duration) {
	var (
		err error
		retry = time.Second * 5
	)
	for {
		err = s.conn()
		if err != nil {
			fmt.Println("stream",s.Path(),"conn faile",err,"conn next time",time.Now().Add(retry).String())
			select {
			case <-time.NewTimer(retry).C:
				retry = retry * 2
			}
			if retry >=maxwait{
				retry=maxwait
			}
			continue
		}
		close(s.startch)
		err = avutil.CopyFile(s.queue,s.demux)
		if err != nil {
			fmt.Println("stream",s.Path(),"conn faile",err)
			continue
		}
		s.startch=make(chan struct{})
	}
}

func (s *Stream) WriteTo(mux av.Muxer,duration time.Duration) error{
	select {
	case <-s.stopch:
		return fmt.Errorf("stream stop")
	case <-s.startch:
	}

	cds,err := s.demux.Streams()
	if err!= nil{
		return err
	}
	mux.WriteHeader(cds)
	s.pool.Submit(func(){startwrite(s.startch,s.queue,duration,mux)})
	return nil
}

func (s *Stream) conn( ) error {
	var (
		cli interface{}
		err error
	)
	switch s.remote.Scheme {
	case "rtsp":
		cli,err = rtsp.Dial(s.remote.String())
	case "rtmp":
		cli, err = rtmp.Dial(s.remote.String())
	}
	if err!= nil{
		fmt.Println("remote",s.remote.String(),"failed",err,)
		return err
	}
	s.demux = cli.(av.Demuxer)
	return nil
}

func (s *Stream) Id() string{
	return s.fp
}

func (s *Stream) Clone() *Stream{
	news,_ := NewStream(s.remote.String(),s.pool)
	news.Start()
	return news
}

func (s *Stream) Path() string{
	return s.remote.String()
}

func fingerPrint(bs []byte) string{
	ha := crc32.New(crc32.IEEETable)
	ha.Reset()
	ha.Write([]byte(bs))
	return fmt.Sprintf("%x",ha.Sum32())
}

func startwrite(stopch <-chan struct{},reader *pubsub.Queue,du time.Duration , mux av.Muxer){
	var (
		cursor = reader.Oldest()
		err error
	)
	timer := time.NewTimer(du)
	for {
		select {
		case <-stopch:
			err = mux.WriteTrailer()
			fmt.Println("muxer","stop","message","context cancled")
			return
		case <-timer.C:
			err = mux.WriteTrailer()
			fmt.Println("muxer","time over","message",err)
			return
		default:
			packet , err := cursor.ReadPacket()
			if err!= nil{
				fmt.Println("queue","read","failed",err)
				return
			}
			err = mux.WritePacket(packet)
			if err!= nil{
				fmt.Println("muxer","write","failed",err)
				return
			}
		}
	}
}

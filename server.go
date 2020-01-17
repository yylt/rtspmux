package main

import (
	"fmt"
	"net/http"
	"github.com/yylt/rtspmux/config"
	"github.com/yylt/rtspmux/stream"
	"github.com/gorilla/mux"

	ants "github.com/panjf2000/ants/v2"
)

type Server struct {
	conf *config.Config
	r *mux.Router
	streams []*stream.Stream
	pool *ants.Pool
	save stream.Saver
	liveprefix string
	handle stream.Handler
}

func NewServer(conf *config.Config) *Server{
	route := mux.NewRouter()
	pool,err := ants.NewPool(10)
	if err!= nil{
		panic(err)
	}
	serv := &Server{
		conf:conf,
		r: route,
		pool: pool,
		liveprefix: "/live",
	}
	err = serv.probe()
	if err!= nil{
		panic(err)
	}
	return serv
}

func (s *Server) probe() error{
	var (
		err error
	)

	for _,st:=range s.conf.Froms{
		stm,err := stream.NewStream(st,s.pool)
		if err!= nil{
			return err
		}
		stm.Start()
		s.streams=append(s.streams,stm)
	}
	switch s.conf.Outformat {
	case config.HlsFmt:
		s.handle=stream.NewHlsHandler(s.liveprefix)
	default:
		return fmt.Errorf("%v not support",s.conf.Outformat)
	}
	s.save,err = stream.NewSaveMp4(&stream.Saveconf{
		Dir:s.conf.Save.Dir,
		Maxtime:s.conf.Save.Max,
		Fragtime:s.conf.Save.Interval,
	})
	if err!= nil{
		return err
	}
	return nil
}

func (s *Server) Stop() {
	for _,stm :=range s.streams{
		stm.Stop()
	}
	s.save.Stop()
	s.handle.Stop()
	s.pool.Release()
}

func (s *Server) starSave() {
	for _,stm :=range s.streams{
		s.save.Start(stm.Clone())
	}
}

func (s *Server) StartServer() error{
	var (
		server http.Server
	)
	s.starSave()
	s.r.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		s.handle.HandlerIndex(writer,request)
	})

	s.r.PathPrefix(s.liveprefix).HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		s.handle.HandlerStream(writer,request)
	})

	server.Addr=s.conf.Addr
	server.Handler=s.r
	if s.conf.Certf!="" &&s.conf.Keyf!=""{
		return server.ListenAndServeTLS(s.conf.Certf,s.conf.Keyf)
	}
	return server.ListenAndServe()
}

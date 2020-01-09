package main

import (
	"sync"
	"log"
	echo "github.com/labstack/echo/v4"
	"net/http"
	"errors"
	"path"
	"strconv"
)

var (
	ErrHadRegist = errors.New("had registe")
	ErrNotRegist = errors.New("not registe")
)

type RouteReg struct {
	mu sync.RWMutex
	routes map[int]http.HandlerFunc
	max int
	frees []int
	prefix string
}

func NotFoundHander() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}
}

func NewRouteReg(prefix string) *RouteReg{
	return &RouteReg{
		prefix:prefix,
		routes:make(map[int]http.HandlerFunc),
	}
}

func (R *RouteReg) Registe(endpoint int,fn http.HandlerFunc) error {
	log.Println("id",endpoint,"will","regist")
	return R.registe(endpoint,fn)
}

func (R *RouteReg) registe(endpoint int,fn http.HandlerFunc) error {
	R.mu.Lock()
	defer R.mu.Unlock()
	if _,ok:=R.routes[endpoint];!ok{
		if len(R.frees)==0{
			R.routes[R.max]=fn
			return nil
		}else {
			ep := R.frees[0]
			R.frees=R.frees[1:]
			R.routes[ep]=fn
		}
	}
	return ErrHadRegist
}

func (R *RouteReg) UnRegiste(endpoint int) error {
	R.mu.Lock()
	defer R.mu.Unlock()
	if _,ok:=R.routes[endpoint];ok{
		R.frees=append(R.frees,endpoint)
		delete(R.routes,endpoint)
	}
	return ErrNotRegist
}


func (R *RouteReg) Hander(router string) http.HandlerFunc{
	ids := path.Base(router)
	id ,err := strconv.Atoi(ids)
	if err!= nil{
		log.Println("not found","handler","router",router)
		return NotFoundHander()
	}
	R.mu.RLock()
	fn ,ok:=R.routes[id]
	R.mu.RUnlock()
	if ok{
		return fn
	}else{
		return NotFoundHander()
	}
}

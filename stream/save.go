package stream

import (
	"time"
	"os"
	"fmt"
	"path/filepath"
	"path"
	"strings"
	"strconv"
	"sync"

	"github.com/nareix/joy4/format/mp4"
)

type Saveconf struct {
	Dir string
	Maxtime time.Duration
	Fragtime time.Duration
}

func (c *Saveconf) valid() error{
	var (
		testfpath = path.Join(c.Dir,"test")
	)
	if info,err:=os.Stat(c.Dir);err!=nil{
		return err
	}else{
		if !info.IsDir(){
			return fmt.Errorf("%s is not dir",c.Dir)
		}
		f,err:=os.Create(testfpath)
		if err!=nil{
			return err
		}
		f.Close()
		os.RemoveAll(testfpath)
	}
	return nil
}

type SaveMp4 struct {
	c *Saveconf
	s *Stream
	mu sync.RWMutex
	stopch chan struct{}
}

func NewSaveMp4(c *Saveconf) (Saver,error){
	err := c.valid()
	if err != nil{
		return nil,err
	}
	return &SaveMp4{
		c:c,
		stopch: make(chan struct{}),
	},nil
}

func (m *SaveMp4) loopDelete() {
	for {
		select {
		case <-time.NewTimer(m.c.Fragtime / 2).C:
		}
		aftertime := time.Now()
		filepath.Walk(m.c.Dir, func(path string, info os.FileInfo, err error) error {
			id,t:=splitname( info.Name())
			if id==""{
				fmt.Println("file",path,"not create file")
				return nil
			}
			if !t.Add(m.c.Maxtime).After(aftertime){
				os.RemoveAll(path)
				fmt.Println("delete mp4 file",path)
				return nil
			}
			return nil
		})
	}
}

func genname(s *Stream) string{
	name := fmt.Sprintf("%s-%d.mp4",s.Id(),time.Now().Unix())
	return name
}

func splitname(name string) (string,time.Time){
	ids := strings.Split(name,"-")
	if len(ids)!=2{
		return "",time.Time{}
	}
	ids[1]=ids[1][:len(ids[1])-4]
	unixs,err := strconv.Atoi(ids[1])
	if err!= nil{
		return "",time.Time{}
	}
	return ids[0],time.Unix(int64(unixs),0)
}

func (m *SaveMp4) Stop() {
	close(m.stopch)
}

func (m *SaveMp4) Start(s *Stream) {
	m.mu.Lock()
	m.s = s
	go func(){
		defer m.mu.Unlock()
		name := genname(m.s)
		f,err:=os.OpenFile(path.Join(m.c.Dir,name),os.O_CREATE|os.O_RDWR,os.ModePerm)
		if err!= nil{
			fmt.Println("module","savemp4","file",path.Join(m.c.Dir,name),"failed",err)
			return
		}
		mux:=mp4.NewMuxer(f)
		err = s.WriteTo(mux,m.c.Fragtime)
		if err!= nil{
			fmt.Println("mp4 stream",m.s.Path(),"failed",err)
			writeUntilSuccess(m.stopch,m.s,mux)
		}
	}()
	return
}


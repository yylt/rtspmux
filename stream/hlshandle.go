package stream

import (
	"sync"
	"bytes"
	"net/http"
	"time"
	"fmt"
	"strings"
	"strconv"
	"path"

	"text/template"
	"github.com/nareix/joy4/format/ts"
	"github.com/nareix/joy4/av"
)


const (
	indexHtmlTmp =`
<html>
<head>
	<title>rtspmux</title>
	<meta charset="utf-8">
	<link href="https://unpkg.com/video.js/dist/video-js.css" rel="stylesheet">
	<script src="https://unpkg.com/video.js/dist/video.js"></script>
	<script src="https://unpkg.com/videojs-contrib-hls/dist/videojs-contrib-hls.js"></script>
</head>
<body>
{{- range $s := $.streams -}}
	<video id="{{- $s.Id }}" class="video-js vjs-default-skin" controls preload="none" width="640" height="264" data-setup="{}">
		<source src="{{- $s.Path}}" type='application/x-mpegURL'>
	</video>
	<script type="text/javascript">
		var {{- $s.Id -}}j=videojs('{{- $s.Id }}');
		videojs("{{- $s.Id }}").ready(function(){
			var player = this;
			player.play();
		});
	</script>
{{- end }}
</body>
</html>
`
)

type videoHtml struct {
	Id string
	Path string
}

var (
	tslen = 3
	Tslength = tslen * 2
	tstime = time.Second * 10
	tselepool = sync.Pool{
		New: func() interface{} {
			return &tsele{
				buf: new(bytes.Buffer),
			}
		}}
	tsmuxpool = sync.Pool{
		New: func() interface{} {
			return new(ts.Muxer)
		}}

	crossdomainxml = []byte(`<?xml version="1.0" ?>
<cross-domain-policy>
	<allow-access-from domain="*" />
	<allow-http-request-headers-from domain="*" headers="*"/>
</cross-domain-policy>`)
)

type HlsHandler struct {
	sts map[string]*hls
	mu sync.RWMutex
	prefix string
}

type hls struct {
	s *Stream
	mu sync.RWMutex
	tslist []*tsele

	m3uid int
	beginIndex int
	hls *HlsHandler
	stopch chan struct{}
}

func (h *HlsHandler) AddStreams(s *Stream) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _,ok:=h.sts[s.Id()];ok{
		return ErrHadAdd
	}
	h.sts[s.Id()]=newHls(s)
	h.sts[s.Id()].Start()
	return nil
}

func (h *HlsHandler) DelStreams(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _,ok:=h.sts[id];ok{
		h.sts[id].Stop()
	}
	return
}

//handle m3u8(m3u),ts request
func (h *HlsHandler) HandlerStream(w http.ResponseWriter,req *http.Request) {

	var (
		r = path.Clean(req.URL.Path)
		indexhls *hls
		ok bool
	)
	dir,name := path.Split(r)
	streamid := path.Base(dir)
	h.mu.RLock()
	indexhls,ok =h.sts[streamid]
	if !ok{
		h.mu.RUnlock()
		http.NotFound(w,req)
		return
	}
	h.mu.RUnlock()

	if name == "crossdomain.xml" {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(crossdomainxml)
		return
	}

	switch path.Ext(name){
	case ".m3u8":
		bs := indexhls.M3u8(dir)
		_,err:=w.Write(bs)

		if err!= nil {
			http.Error(w,err.Error(),http.StatusInternalServerError)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/x-mpegURL")
		w.Header().Set("Content-Length", strconv.Itoa(len(bs)))

	case ".ts":
		bs,err:=indexhls.Ts(name)
		if err!= nil {
			http.Error(w,err.Error(),http.StatusBadRequest)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "video/mp2ts")
		w.Header().Set("Content-Length", strconv.Itoa(len(bs)))
		w.Write(bs)
	default:
		http.NotFound(w,req)
	}

}

func (h *HlsHandler) Stop(){
	h.mu.Lock()
	defer h.mu.Unlock()
	for _,stm:=range h.sts{
		stm.Stop()
	}
}

//handle index html
func (h *HlsHandler) HandlerIndex(w http.ResponseWriter,req *http.Request) {
	var (
		data = make(map[string]interface{})
		videos []*videoHtml
		tmpl,_ = template.New("").Parse(indexHtmlTmp)
	)

	h.mu.RLock()
	for _,v:=range h.sts{
		name:=fmt.Sprintf("%s.m3u8",v.s.Id())
		videos=append(videos,&videoHtml{
			Id:v.s.Id(),
			Path:path.Join(h.prefix,v.s.Id(),name),
		})
	}
	h.mu.RUnlock()
	data["streams"]=videos

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	tmpl.Execute(buf,data)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(buf.Bytes())))
	w.Write(buf.Bytes())
	bufPool.Put(buf)
	return
}

func NewHlsHandler(preroute string) Handler {
	return &HlsHandler{
		sts:make(map[string]*hls),
		prefix:preroute,
	}
}

func newHls(s *Stream) *hls {
	h:= &hls{
		s:s,
		tslist:make([]*tsele,Tslength),
		stopch: make(chan struct{}),
	}
	for i:=0;i<Tslength;i++ {
		h.tslist[i]=tselepool.Get().(*tsele)
	}
	return h
}

func (h *hls) Stop(){
	close(h.stopch)
}

func (h *hls) releaseBuf() {
	fmt.Println("stop hls",h.s.Path())
	for _,v:=range h.tslist{
		tselepool.Put(v.buf)
	}
}

//截取3段10s视频
func (h *hls) Start() {
	go func(){
		var (
			i int
			count int
			tsmux = tsmuxpool.Get().(*ts.Muxer)
		)
		for {
			select {
			case <-h.stopch:
				h.releaseBuf()
				return
			}
			i = i% (Tslength +1)
			count = count % tslen
			h.tslist[i].name=fmt.Sprintf("%d_%d.ts",time.Now().Unix(),i)
			h.tslist[i].buf.Reset()
			tsmux.SetWriter(h.tslist[i].buf)
			err := h.s.WriteTo(tsmux,tstime)
			if err!= nil{
				fmt.Println("hls stream",h.s.Path(),"failed",err)
				writeUntilSuccess(h.stopch,h.s,tsmux)
			}
			count++
			i++
			h.updateid(i,count)
		}
	}()
}

func writeUntilSuccess(stopch <-chan struct{},s *Stream,mux av.Muxer) {
	var (
		mind = time.Second * 5
		maxd = time.Minute * 5
	)
	tmp := mind
	for {
		select {
		case <-stopch:
			return
		case <-time.NewTimer(tmp).C:
		}

		err := s.WriteTo(mux,tstime)
		if err!= nil{
			tmp = tmp * 2
			if tmp>=maxd{
				tmp = maxd
			}
			fmt.Println("stream",s.Path(),"try next time",time.Now().Add(tmp))
		}else{
			fmt.Println("stream",s.Path(),"recovered!")
			break
		}
	}
}

func (h *hls) updateid(in int,count int) {
	h.mu.Lock()
	h.beginIndex = in
	if count == tslen {
		h.m3uid++
	}
	h.mu.Unlock()
}

func (h *hls) Ts(name string) ([]byte,error) {
	id := strings.Split(name,"_")
	if len(id)!= 2{
		fmt.Println("name",name,"not")
		return nil,ErrNameIncorrect
	}
	num,err := strconv.Atoi(id[1])
	if err!= nil{
		fmt.Println("parse",id[1],"failed",err)
		return nil,err
	}
	return h.tslist[num].buf.Bytes(),nil
}

func (h *hls) M3u8(preroute string) []byte {
	var (
		times = tstime.Seconds()
		index int
	)
	buf:=bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.WriteString(fmt.Sprintf(
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-ALLOW-CACHE:NO\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n\n",
		times,h.m3uid))

	h.mu.RLock()
	if h.beginIndex<tslen{
		index = h.beginIndex + Tslength - tslen
	}else{
		index = h.beginIndex -tslen
	}
	h.mu.RUnlock()

	for i:=0;i<3;i++{
		index = index % Tslength
		buf.WriteString(fmt.Sprintf("#EXTINF:%0.3f,\n%s\n",times,  path.Join(preroute,h.tslist[index].name)))
		index ++
	}
	defer bufPool.Put(buf)
	return buf.Bytes()
}


type tsele struct {
	name string
	buf *bytes.Buffer
}

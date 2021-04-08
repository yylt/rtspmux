package pkg

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	backoff2 "github.com/cenkalti/backoff/v4"
	"github.com/yylt/rtspmux/util"
	klog "k8s.io/klog/v2"
)

const (
	defaultRetry = 3
	minWorkers   = 3

	tmpSuffix = ".ts.tmp"
	tsSuffix  = ".ts"

	mergeDirName = "merged"
	tsDirName    = "ts"

	defaultPerm = os.ModePerm
)

var _ MergerNode = &DlItem{}

type DlItems []*DlItem

func (dls DlItems) Swap(i, j int) {
	dls[i].url, dls[j].url = dls[j].url, dls[i].url
}

func (dls DlItems) Less(i, j int) bool {
	return dls[i].beginTime.Unix() < dls[j].beginTime.Unix()
}

func (dls DlItems) Len() int {
	return len(dls)
}

type DlItem struct {
	url       string
	beginTime time.Time
}

func (mi *DlItem) Path() string {
	return mi.url
}

func (mi *DlItem) Open() (io.ReadCloser, error) {
	return os.Open(mi.url)
}

func (mi *DlItem) Time() int64 {
	return mi.beginTime.Unix()
}

type retryItem struct {
	count int
	item  *Item
}

type Dl struct {
	ctx context.Context

	//tmp director
	dir string

	//worker number
	maxWorkerNumber int

	//worker pool
	workers *util.Pool

	dlItems chan *DlItem

	backoff backoff2.BackOff
	sendeds map[int64]struct{}
}

func NewDownload(ctx context.Context, workernumber int, tmpDir string) *Dl {
	if workernumber <= 0 {
		panic("worker number should be zero")
	}
	if workernumber < minWorkers {
		workernumber = minWorkers
	}
	dir := filepath.Join(filepath.Clean(tmpDir), tsDirName)
	err := os.MkdirAll(dir, defaultPerm)
	if err != nil {
		panic(err)
	}
	backof := backoff2.WithMaxRetries(backoff2.NewConstantBackOff(time.Millisecond*300), defaultRetry)
	dl := &Dl{
		ctx:             ctx,
		dir:             dir,
		maxWorkerNumber: workernumber,
		workers:         util.NewPool(workernumber),
		dlItems:         make(chan *DlItem, 16),
		backoff:         backof,
		sendeds:         make(map[int64]struct{}),
	}
	return dl
}

func (d *Dl) Start(sendDuration time.Duration, ch <-chan *Item) <-chan *DlItem {
	go d.worker(ch)
	go d.sendWorker(sendDuration)
	return d.dlItems
}

func (d *Dl) sendWorker(sendDuration time.Duration) {
	var (
		items []*DlItem
	)
	for {
		select {
		case <-time.NewTimer(sendDuration).C:
			nowtime := time.Now()
			items = items[:0]
			filepath.Walk(d.dir, func(path string, info os.FileInfo, err error) error {
				if filepath.Dir(path) != d.dir {
					return nil
				}
				if err != nil {
					klog.Errorf("walk path %v failed:%v", d.dir, err)
					return err
				}
				if info == nil {
					return nil
				}
				if info.ModTime().Add(time.Hour).Before(nowtime) {
					klog.Infof("remove file %v which before an hour", path)
					os.RemoveAll(path)
					return nil
				}
				fname := info.Name()
				if !strings.HasSuffix(fname, tsSuffix) {
					return nil
				}
				truename := fname[:len(fname)-len(tsSuffix)]
				begint, err := strconv.ParseInt(truename, 10, 64)
				if err != nil {
					klog.Errorf("parse %v failed:%v", fname, err)
					return nil
				}
				if _, ok := d.sendeds[begint]; ok {
					return nil
				}
				d.sendeds[begint] = struct{}{}
				btime := time.Unix(begint, 0)
				newdl := &DlItem{
					url:       path,
					beginTime: btime,
				}
				items = append(items, newdl)
				return nil
			})
			if len(items) == 0 {
				continue
			}
			sort.Sort(DlItems(items))
			for i := range items {
				d.dlItems <- items[i]
			}
		case <-d.ctx.Done():
			return
		}
	}
}

func (d *Dl) do(item *Item) (rerr error) {
	var (
		err      error
		tsname   = strconv.Itoa(int(item.Time().Unix()))
		tmpfpath = filepath.Join(d.dir, tsname+tmpSuffix)
		tsfpath  = filepath.Join(d.dir, tsname+tsSuffix)
	)
	//exist mean had download
	_, err = os.Stat(tsfpath)
	if err != nil && os.IsExist(err) {
		klog.Infof("file %v exist, skip", tsfpath)
		return nil
	}
	f, err := os.OpenFile(tmpfpath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, defaultPerm)
	if err != nil {
		klog.Errorf("open file %v failed:%v", tmpfpath, err)
		return err
	}
	defer func() {
		f.Close()
		if rerr != nil {
			klog.Errorf("download item failed:%v", rerr)
			os.Remove(tmpfpath)
		} else {
			klog.Infof("download success, rename %v to %v", tmpfpath, tsfpath)
			os.Rename(tmpfpath, tsfpath)
		}
	}()

	bufw := bufio.NewWriter(f)
	defer bufw.Flush()
	klog.Infof("start download item, path:%v", item.Path())
	return util.IoCopy(item, bufw)
}

func (d *Dl) worker(ch <-chan *Item) {
	var (
		err error
	)
	for {
		select {
		case <-d.ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if e.Time().IsZero() {
				klog.Infof("not found time in item url(%v), skip", e.Path())
				continue
			}
			err = backoff2.Retry(func() error {
				var rerr error
				err2 := d.workers.Submit(func() {
					rerr = d.do(e)
				})
				if err2 != nil {
					return err2
				}
				return rerr
			}, d.backoff)

			if err != nil {
				klog.Errorf("submit work failed:%v", err)
			}
		}
	}
}

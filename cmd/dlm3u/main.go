package main

import (
	"context"
	"flag"

	"os"
	"os/signal"
	"time"

	"github.com/yylt/rtspmux/pkg"
	"github.com/cenkalti/backoff/v4"
	"k8s.io/klog/v2"
)

const (
	MaxRetryM3u8Count = 10
)

var (
	ctx    context.Context
	cancle func()
)

func mergeWorker(ctx context.Context, mg *pkg.Merger, ch <-chan *pkg.DlItem) {
	var (
		cache = make(map[int64]struct{})
		err   error
	)
	for {
		select {
		case <-ctx.Done():
			return
		case v, ok := <-ch:
			if !ok {
				return
			}
			vt := v.Time()
			_, exist := cache[vt]
			if exist {
				continue
			}
			cache[vt] = struct{}{}

			err = mg.Merge(v)
			if err != nil {
				_, ishad := err.(*pkg.HadMergedError)
				if ishad {
					klog.Infof("item had merged, path:%v, time:%d", v.Path(), v.Time())
					continue
				}
				klog.Errorf("merge failed:%v, path:%v, time:%d", err, v.Path(), v.Time())
				continue
			}
			klog.Infof("merge success, path:%v, time:%d", v.Path(), v.Time())
		}
	}
}

func downloadWorker(ctx context.Context, m3u8url string, interval time.Duration, ch chan *pkg.Item) {
	var (
		cache = make(map[int64]struct{})
		err   error

		ebk = backoff.WithMaxRetries(backoff.NewExponentialBackOff(), MaxRetryM3u8Count)
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.NewTimer(interval).C:
			err = backoff.Retry(func() error {
				m38, err := pkg.Open(m3u8url)
				if err != nil {
					klog.Errorf("open url %v failed:%v", m3u8url, err)
					return err
				}
				return m38.Iter(func(i *pkg.Item) error {
					tu := i.Time().Unix()
					_, ok := cache[tu]
					if ok {
						return nil
					}
					cache[tu] = struct{}{}
					ch <- i
					return nil
				})
			}, ebk)
			if err != nil {
				panic(err)
			}
		}
	}
}

func main() {
	u8url := flag.String("m3u8", "", "m3u8 url address, mostly http://xxx or https://xxx ")
	workernum := flag.Int("workers", 5, "download worker number")
	fetchInterval := flag.Duration("feetch-interval", time.Second*2, "fetch m3u8 interval")
	mergeInterval := flag.Duration("merge-interval", time.Millisecond*300, "merge item interval, which is check directory duration")
	maxMbsize := flag.Int64("mb-max-size", 300, "the max Mb size in one merged file")
	dir := flag.String("dir", "/opt/dlm3u", "which directory saved all files")

	klog.InitFlags(flag.CommandLine)
	flag.Parse()
	done := make(chan struct{})

	ctx, cancle = context.WithCancel(context.Background())

	mr := pkg.NewMerge(ctx, *maxMbsize, *dir)
	dl := pkg.NewDownload(ctx, *workernum, *dir)
	dlitem := make(chan *pkg.Item, 16)

	rcvch := dl.Start(*mergeInterval, dlitem)

	go downloadWorker(ctx, *u8url, *fetchInterval, dlitem)
	go mergeWorker(ctx, mr, rcvch)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		klog.Infof("graceful exit...")
		cancle()
		close(done)
	}()
	<-done
}

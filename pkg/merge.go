package pkg

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Workiva/go-datastructures/augmentedtree"
	"github.com/yylt/rtspmux/util"

	"k8s.io/klog/v2"
)

var _ augmentedtree.Interval = &node{}

const (
	KiB int64 = 1024
	MiB int64 = 1024 * KiB
	GiB int64 = 1024 * MiB
	TiB int64 = 1024 * GiB
)

type HadMergedError struct{}

func (h *HadMergedError) Error() string {
	return "Had merged"
}

func NewMergedError() *HadMergedError {
	return &HadMergedError{}
}

type MergerNode interface {
	Open() (io.ReadCloser, error)
	Time() int64
}

type node struct {
	mu        sync.RWMutex
	beginTime int64
	endTime   int64

	path string
}

func (n *node) Open(fn func(closer io.ReadWriter) error) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	f, err := os.OpenFile(n.path, os.O_RDWR|os.O_APPEND, defaultPerm)
	if err != nil {
		return err
	}
	err = fn(f)
	f.Close()
	return err
}

func (n *node) UpdateEnd(end int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	dir := filepath.Dir(n.path)

	newname := nodeName(n.beginTime, end)
	newfpath := filepath.Join(dir, newname)
	err := os.Rename(n.path, newfpath)
	if err != nil {
		return err
	}
	n.path = newfpath
	return nil
}

func (n *node) Name() string {
	return filepath.Base(n.path)
}

func (n *node) Remove() {
	n.mu.Lock()
	defer n.mu.Unlock()
	os.RemoveAll(n.path)
}

func (n *node) Size() int64 {
	info, err := os.Lstat(n.path)
	if err != nil {
		klog.Errorf("stat file %v faild:%v", n.path, err)
		return 0
	}
	return info.Size()
}

// LowAtDimension returns an integer representing the lower bound
// at the requested dimension.
func (n *node) LowAtDimension(uint64) int64 {
	return n.beginTime
}

// HighAtDimension returns an integer representing the higher bound
// at the requested dimension.
func (n *node) HighAtDimension(uint64) int64 {
	if n.endTime == 0 {
		return n.beginTime
	}
	return n.endTime
}

// OverlapsAtDimension should return a bool indicating if the provided
// interval overlaps this interval at the dimension requested.
func (n *node) OverlapsAtDimension(intv augmentedtree.Interval, _ uint64) bool {
	lown := intv.LowAtDimension(1)
	highn := intv.HighAtDimension(1)
	if lown >= n.LowAtDimension(1) && lown <= n.HighAtDimension(1) {
		return true
	}
	if highn >= n.LowAtDimension(1) && highn <= n.HighAtDimension(1) {
		return true
	}
	return false
}

// ID should be a unique ID representing this interval.  This
// is used to identify which interval to delete from the tree if
// there are duplicates.
func (n *node) ID() uint64 {
	return uint64(n.beginTime)
}

type Merger struct {
	ctx context.Context

	dir string
	//max mb file size which to limit merged file size
	maxSizeBytes int64

	// interval tree
	tree augmentedtree.Tree

	nodes map[int64]*node
}

func NewMerge(ctx context.Context, maxMbSizePerfile int64, tmpDir string) *Merger {
	dir := filepath.Join(filepath.Clean(tmpDir), mergeDirName)
	err := os.MkdirAll(dir, defaultPerm)
	if err != nil {
		panic(err)
	}
	m := &Merger{
		ctx:          ctx,
		dir:          dir,
		maxSizeBytes: maxMbSizePerfile * MiB,
		tree:         augmentedtree.New(1),
	}
	err = m.probe()
	if err != nil {
		panic(err)
	}
	return m
}

func (m *Merger) probe() error {
	err := os.MkdirAll(m.dir, defaultPerm)
	if err != nil {
		return err
	}
	return filepath.Walk(m.dir, func(path string, info os.FileInfo, err error) error {
		if filepath.Dir(path) != m.dir {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		interv := parseMergeFileName(path)
		if interv != nil {
			klog.Infof("add merge file:%s", path)
			m.tree.Add(interv)
		}
		return nil
	})
}

// no should not be nil
func (m *Merger) append(newno *node, mergerNode MergerNode) (err error) {
	if newno == nil {
		panic("node must not be null")
	}

	srcf, err := mergerNode.Open()
	if err != nil {
		return err
	}
	defer srcf.Close()
	return newno.Open(func(rw io.ReadWriter) error {
		return util.IoCopy(srcf, rw)
	})
}

// Merge will take much io time, this should be async
// NOTE: nodes should be alpha sorted.
func (m *Merger) Merge(no MergerNode) (rerr error) {
	var (
		tmpno   = &node{}
		mintime int64
		insert  *node

		deltemp, ok bool
	)
	tmpno, err := newnode(m.dir, no)
	if err != nil {
		return err
	}
	defer func() {
		if rerr != nil || deltemp {
			tmpno.Remove()
		}
	}()

	m.tree.Traverse(func(interval augmentedtree.Interval) {
		lowtime := interval.LowAtDimension(1)
		if lowtime > mintime {
			mintime = lowtime
			insert, ok = interval.(*node)
			if !ok {
				klog.Errorf("can not trans interval to node")
			}
		}
	})

	overlaps := m.tree.Query(tmpno)
	if len(overlaps) != 0 {
		allid := make([]uint64, len(overlaps))
		for i := range overlaps {
			allid[i] = overlaps[i].ID()
		}
		klog.Errorf("find overlaps %v on node %d", allid, tmpno.ID())
		return NewMergedError()
	}
	if insert == nil || insert.Size() >= m.maxSizeBytes {
		klog.Infof("new interval node:%s, mergenode:%d", tmpno.Name(), no.Time())
		err = m.append(tmpno, no)
		if err == nil {
			m.tree.Add(tmpno)
		}
		return err
	}

	klog.Infof("insert node %d to %s", no.Time(), insert.Name())
	deltemp = true
	err = m.append(insert, no)
	if err == nil {
		return insert.UpdateEnd(no.Time())
	}
	return err
}

func parseMergeFileName(fpath string) *node {
	if fpath == "" {
		return nil
	}
	stat, err := os.Stat(fpath)
	if err != nil {
		klog.Errorf("file %v faild:%v", fpath, err)
		return nil
	}
	name := stat.Name()
	if !strings.HasSuffix(name, tsSuffix) {
		klog.Errorf("file %v is not suffix: %v", fpath, tsSuffix)
		return nil
	}
	truename := name[:len(name)-len(tsSuffix)]
	sten := strings.Split(truename, "-")
	if len(sten) != 2 {
		klog.Errorf("file name is invalid:%v", truename)
		return nil
	}
	starttime, err := strconv.ParseInt(sten[0], 10, 64)
	if err != nil {
		klog.Errorf("parse time %v failed:%v", sten[0], err)
		return nil
	}
	endtime, err := strconv.ParseInt(sten[1], 10, 64)
	if err != nil {
		klog.Errorf("parse time %v failed:%v", sten[1], err)
		return nil
	}
	return &node{
		mu:        sync.RWMutex{},
		beginTime: starttime,
		endTime:   endtime,
		path:      fpath,
	}
}

func nodeName(start, end int64) string {
	return fmt.Sprintf("%d-%d%s", start, end, tsSuffix)
}

func newnode(tmpdir string, no MergerNode) (*node, error) {
	var (
		tmpno = &node{}
	)
	notime := no.Time()
	if notime == 0 {
		return nil, fmt.Errorf("the time is zero")
	}
	tmpno.beginTime = notime
	tmpno.endTime = notime
	tmpno.path = filepath.Join(tmpdir, nodeName(notime, notime))

	f, err := os.OpenFile(tmpno.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, defaultPerm)
	if err != nil {
		return nil, err
	}
	f.Close()
	return tmpno, nil
}

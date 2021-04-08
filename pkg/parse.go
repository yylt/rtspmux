package pkg

import (
	"bytes"
	"fmt"
	"github.com/yylt/rtspmux/util"
	"io"
	"net/http"
	"time"
	"unicode"

	"github.com/oopsguy/m3u8/parse"
	"github.com/oopsguy/m3u8/tool"
)

const (
	SyncByte = uint8(71)
)

type Item struct {
	url       string
	beginTime time.Time

	bytes *bytes.Buffer
}

type M3Parse struct {
	result *parse.Result
}

func Get(url string) (io.ReadCloser, error) {
	c := http.Client{
		Timeout:   time.Duration(15) * time.Second,
		Transport: http.DefaultTransport,
	}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http error: status code %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func Open(url string) (*M3Parse, error) {
	res, err := parse.FromURL(url)
	if err != nil {
		return nil, err
	}
	return &M3Parse{result: res}, nil
}

func (m *M3Parse) Iter(fn func(i *Item) error) error {
	var (
		err   error
		ivkey string
	)
	for i, v := range m.result.M3u8.Segments {
		sf := m.result.M3u8.Segments[i]
		if sf == nil {
			return fmt.Errorf("invalid segment index: %d", i)
		}
		key, _ := m.result.Keys[sf.KeyIndex]
		if key != "" {
			ivkey = m.result.M3u8.Keys[sf.KeyIndex].IV
		} else {
			ivkey = ""
		}

		tsUrl := tool.ResolveURL(m.result.URL, v.URI)
		item := &Item{
			url:       tsUrl,
			beginTime: findTime([]byte(tsUrl)),
		}
		body, e := Get(item.url)
		if e != nil {
			return fmt.Errorf("request %s, %s", item.url, e.Error())
		}
		//noinspection GoUnhandledErrorResult
		defer body.Close()
		buf := util.GetBuf()
		err = util.IoCopy(body, buf)
		if err != nil {
			util.PutBuf(buf)
			return fmt.Errorf("read %v failed:%s", tsUrl, err.Error())
		}
		bufbs := buf.Bytes()
		if bufbs[0] == SyncByte && ivkey == "" {
			item.bytes = buf
			return fn(item)
		}

		if ivkey != "" {
			bufbs, err = tool.AES128Decrypt(bufbs, []byte(key), []byte(ivkey))
			if err != nil {
				util.PutBuf(buf)
				return err
			}
		}

		// https://en.wikipedia.org/wiki/MPEG_transport_stream
		// Some TS files do not start with SyncByte 0x47, they can not be played after merging,
		// Need to remove the bytes before the SyncByte 0x47(71).
		bLen := len(bufbs)
		for j := 0; j < bLen; j++ {
			if bufbs[j] == SyncByte {
				bufbs = bufbs[j:]
				break
			}
		}
		buf.Reset()
		buf.Write(bufbs)
		return fn(item)
	}
	return nil
}

func (i *Item) Time() time.Time {
	return i.beginTime
}

func (i *Item) Path() string {
	return i.url
}

func (i *Item) Read(p []byte) (int, error) {
	if i.bytes == nil {
		return 0, io.EOF
	}
	n, err := i.bytes.Read(p)
	if err == io.EOF {
		util.PutBuf(i.bytes)
	}
	return n, err
}

// mostly ts url is xxx-{time}.ts
// try find {time}
func findTime(url []byte) time.Time {
	var (
		i      = 1
		tmpnum int
		begin  = false
		zero   = time.Time{}
	)
	if len(url) == 0 {
		return zero
	}
	bytes.TrimRightFunc(url, func(r rune) bool {
		isnum := unicode.IsNumber(r)

		if isnum {
			if !begin {
				// find first number
				begin = true
			}
		}
		if !isnum {
			if begin {
				// find last is not number
				return false
			} else {
				// not find first number , do nothing
				return true
			}
		}

		a := i * int(r-'0')
		i *= 10
		tmpnum += a
		return true
	})
	if tmpnum == 0 {
		return zero
	}
	return time.Unix(int64(tmpnum), 0)
}

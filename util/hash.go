package util

import (
	"hash"
	"hash/crc64"
	"io/ioutil"
	"sync"
)

var (
	hash64 hash.Hash64
	hashmu sync.Mutex
)

func init() {
	hash64 = crc64.New(crc64.MakeTable(crc64.ECMA))
}

func Hashid(bs []byte) int64 {
	hashmu.Lock()
	defer hashmu.Unlock()
	hash64.Reset()
	hash64.Write(bs)
	return int64(hash64.Sum64())
}

func HashFile(p string) int64 {
	bs, err := ioutil.ReadFile(p)
	if err != nil {
		return 0
	}
	return Hashid(bs)
}

package util

import (
	"errors"
	"fmt"
	ants "github.com/panjf2000/ants/v2"
)

var (
	defaultP *Pool
	defaultCap    = 8
)


type Pool struct {
	pool *ants.Pool
}

func NewPool(capacity int) *Pool {
	pool, err := ants.NewPool(capacity)

	if err != nil {
		panic(err)
	}
	p := &Pool{
		pool: pool,
	}
	return p
}

func (p *Pool) Submit(fn func()) error {
	return p.pool.Submit(fn)
}

// Stop release pool
// TODO(y) graceful stop, wait running over
func (p *Pool) Stop() {
	p.pool.Release()
}

func (p *Pool) Running() int {
	return p.pool.Running()
}

func Submit(fn func()) error {
	if defaultP == nil{
		defaultP=NewPool(defaultCap)
	}
	return defaultP.Submit(fn)
}

func Shutdown() error {
	if defaultP == nil{
		return nil
	}
	if v := defaultP.Running(); v == 0 {
		defaultP.Stop()
		return nil
	} else {
		return errors.New(fmt.Sprintf("There are %d task in running", v))
	}
}

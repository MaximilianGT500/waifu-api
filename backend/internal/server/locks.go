package server

import (
	"context"
	"sync"
)

type keyedMutex struct {
	mu    sync.Mutex
	locks map[int64]*keyedLock
}

type keyedLock struct {
	token chan struct{}
	refs  int
}

func (k *keyedMutex) lock(ctx context.Context, id int64) (func(), error) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[int64]*keyedLock{}
	}
	l := k.locks[id]
	if l == nil {
		l = &keyedLock{token: make(chan struct{}, 1)}
		k.locks[id] = l
	}
	l.refs++
	k.mu.Unlock()

	select {
	case l.token <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() {
				<-l.token
				k.release(id, l)
			})
		}, nil
	case <-ctx.Done():
		k.release(id, l)
		return nil, ctx.Err()
	}
}

func (k *keyedMutex) release(id int64, l *keyedLock) {
	k.mu.Lock()
	l.refs--
	if l.refs == 0 {
		delete(k.locks, id)
	}
	k.mu.Unlock()
}

func (k *keyedMutex) size() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.locks)
}

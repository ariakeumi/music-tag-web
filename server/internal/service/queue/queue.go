// Package queue replaces Celery+Redis with an in-process goroutine pool.
// Job state lives in memory; persistent task rows stay in SQLite.
package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type JobInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	StartedAt string `json:"started_at"`
}

type job struct {
	info      JobInfo
	cancel    context.CancelFunc
	startedAt time.Time
}

type Queue struct {
	mu     sync.Mutex
	active map[string]*job
	wg     sync.WaitGroup
}

func New() *Queue {
	return &Queue{active: map[string]*job{}}
}

// Submit runs fn in a background goroutine with a cancellable context.
func (q *Queue) Submit(name string, fn func(ctx context.Context)) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	id := fmt.Sprintf("%s-%s", time.Now().Format("150405"), hex.EncodeToString(buf))

	ctx, cancel := context.WithCancel(context.Background())
	j := &job{
		info: JobInfo{
			ID:        id,
			Name:      name,
			State:     "ACTIVE",
			StartedAt: time.Now().Format("2006-01-02 15:04:05"),
		},
		cancel:    cancel,
		startedAt: time.Now(),
	}
	q.mu.Lock()
	q.active[id] = j
	q.mu.Unlock()

	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("queue: job %s panic: %v\n", id, r)
			}
			q.mu.Lock()
			delete(q.active, id)
			q.mu.Unlock()
		}()
		fn(ctx)
	}()
	return id
}

func (q *Queue) Active() []JobInfo {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := []JobInfo{}
	for _, j := range q.active {
		out = append(out, j.info)
	}
	return out
}

// CancelAll revokes every running job; the worker functions observe ctx.
func (q *Queue) CancelAll() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.active {
		j.cancel()
	}
	return len(q.active)
}

func (q *Queue) WaitIdle() { q.wg.Wait() }

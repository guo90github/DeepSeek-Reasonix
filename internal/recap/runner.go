package recap

import (
	"context"
	"sync"
)

const runnerQueueDepth = 64

// Runner drains recap requests on a single background goroutine. Submission is
// non-blocking and coalesced by path, so a close path never waits for a recap.
//
// Bulk closes (several tabs at once) are why there are two exits: Close drains
// for batch callers that want every queued session finished, while Stop abandons
// queued work and cancels the call in flight for a process that is shutting down.
type Runner struct {
	generator *Generator
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc

	mu       sync.Mutex
	queued   map[string]bool
	closed   bool
	stopped  bool
	wg       sync.WaitGroup
	closeOne sync.Once
}

// NewRunner starts the lane. Call Close to drain it or Stop to abandon it.
func NewRunner(generator *Generator) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		generator: generator,
		queue:     make(chan string, runnerQueueDepth),
		ctx:       ctx,
		cancel:    cancel,
		queued:    map[string]bool{},
	}
	r.wg.Add(1)
	go r.work()
	return r
}

// Submit queues one session. It reports false when the path is already queued,
// already running, the lane is closing, or the queue is full; callers treat that
// as "not now", never as a failure.
func (r *Runner) Submit(sessionPath string) bool {
	r.mu.Lock()
	if r.closed || sessionPath == "" || r.queued[sessionPath] {
		r.mu.Unlock()
		return false
	}
	r.queued[sessionPath] = true
	select {
	case r.queue <- sessionPath:
		r.mu.Unlock()
		return true
	default:
		delete(r.queued, sessionPath)
		r.mu.Unlock()
		// Closing many tabs at once must not lose a session silently: the pending
		// marker is what a later sweep or backfill looks for.
		r.generator.markPending(context.Background(), sessionPath, "queue full")
		return false
	}
}

// Close stops accepting work and waits for the lane to drain.
func (r *Runner) Close() {
	r.shutdown(false)
	r.wg.Wait()
}

// Stop abandons queued work and cancels the call in flight, returning as soon as
// the lane is closed. A session that just ended must never wait for a model.
func (r *Runner) Stop() {
	r.shutdown(true)
}

func (r *Runner) shutdown(abandon bool) {
	r.closeOne.Do(func() {
		r.mu.Lock()
		r.closed = true
		if abandon {
			r.stopped = true
		}
		close(r.queue)
		r.mu.Unlock()
		if abandon {
			r.cancel()
		}
	})
}

func (r *Runner) work() {
	defer r.wg.Done()
	for path := range r.queue {
		_, _ = r.generator.Generate(r.ctx, path)
		r.mu.Lock()
		delete(r.queued, path)
		r.mu.Unlock()
		if r.ctx.Err() != nil {
			return
		}
	}
}

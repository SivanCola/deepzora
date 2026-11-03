package agent

import (
	"context"
	"fmt"
	"sync"
)

type SubagentSlotStatus string

const (
	SubagentSlotQueued  SubagentSlotStatus = "queued"
	SubagentSlotRunning SubagentSlotStatus = "running"
	SubagentSlotDone    SubagentSlotStatus = "done"
	SubagentSlotFailed  SubagentSlotStatus = "failed"
)

type AcquireRequest struct {
	Writer     bool
	WritePaths WritePathSet
	Nested     bool
	Label      string
}

// SubagentScheduler owns a bounded pool and one exclusive writer. Waiting
// writers consume neither pool capacity nor another writer's turn.
type SubagentScheduler struct {
	mu       sync.Mutex
	maxTotal int
	active   map[int64]bool
	parents  map[int64]bool
	nextID   int64
	waiters  []*schedulerWaiter
}
type schedulerWaiter struct {
	writer bool
	parent int64
	ready  chan struct{}
	id     int64
}

func NewSubagentScheduler(total, _ int) *SubagentScheduler {
	total, _ = NormalizeConcurrencyLimits(total, 1)
	return &SubagentScheduler{maxTotal: total, active: map[int64]bool{}, parents: map[int64]bool{}}
}
func (s *SubagentScheduler) Limits() (int, int) {
	if s == nil {
		return DefaultMaxSubagentConcurrency, 1
	}
	return s.maxTotal, 1
}
func (s *SubagentScheduler) Acquire(ctx context.Context, req AcquireRequest) (func(), error) {
	release, _, err := s.AcquireWithID(ctx, req)
	return release, err
}
func (s *SubagentScheduler) AcquireWithID(ctx context.Context, req AcquireRequest) (func(), int64, error) {
	noop := func() {}
	if s == nil {
		return noop, 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return noop, 0, err
	}
	if req.Nested {
		return noop, 0, fmt.Errorf("subagents cannot delegate")
	}
	w := &schedulerWaiter{writer: req.Writer, parent: ParentWriteClaimID(ctx), ready: make(chan struct{})}
	s.mu.Lock()
	s.waiters = append(s.waiters, w)
	s.pumpLocked()
	s.mu.Unlock()
	select {
	case <-w.ready:
		return s.release(w.id), w.id, nil
	case <-ctx.Done():
		s.mu.Lock()
		for i, v := range s.waiters {
			if v == w {
				s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
				break
			}
		}
		delete(s.active, w.id)
		s.pumpLocked()
		s.mu.Unlock()
		return noop, 0, ctx.Err()
	}
}
func (s *SubagentScheduler) writerBusyLocked(parent int64) bool {
	for _, writer := range s.active {
		if writer {
			return true
		}
	}
	for id := range s.parents {
		if id != parent {
			return true
		}
	}
	return false
}
func (s *SubagentScheduler) pumpLocked() {
	remaining := s.waiters[:0]
	writerWaiting := false
	for _, w := range s.waiters {
		ownsParent := w.parent != 0 && s.parents[w.parent]
		if len(s.active) >= s.maxTotal || (w.writer && ((writerWaiting && !ownsParent) || s.writerBusyLocked(w.parent))) {
			remaining = append(remaining, w)
			writerWaiting = writerWaiting || w.writer
			continue
		}
		s.nextID++
		w.id = s.nextID
		s.active[w.id] = w.writer
		close(w.ready)
	}
	s.waiters = remaining
}
func (s *SubagentScheduler) release(id int64) func() {
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); delete(s.active, id); s.pumpLocked(); s.mu.Unlock() }) }
}
func (s *SubagentScheduler) ReserveParentWrite(paths WritePathSet) (func(), error) {
	r, _, e := s.ReserveParentWriteWithID(paths)
	return r, e
}
func (s *SubagentScheduler) ReserveParentWriteWithID(_ WritePathSet) (func(), int64, error) {
	noop := func() {}
	if s == nil {
		return noop, 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writerBusyLocked(0) {
		return noop, 0, fmt.Errorf("a writable subagent or parent tool is still running")
	}
	for _, w := range s.waiters {
		if w.writer {
			return noop, 0, fmt.Errorf("a writable subagent is queued")
		}
	}
	s.nextID++
	id := s.nextID
	s.parents[id] = true
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); delete(s.parents, id); s.pumpLocked(); s.mu.Unlock() }) }, id, nil
}

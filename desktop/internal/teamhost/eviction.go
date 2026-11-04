package teamhost

import (
	"context"
	"errors"
	"sort"

	"deepzora/internal/team"
)

func (h *Host) evict(ctx context.Context) error {
	h.mu.Lock()
	refs := make([]team.SeatRef, 0, len(h.seats))
	for ref, e := range h.seats {
		if !e.visible {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return h.seats[refs[i]].used.Before(h.seats[refs[j]].used) })
	h.mu.Unlock()
	var failures error
	for _, ref := range refs {
		lock := h.seatLock(ref)
		lock.Lock()
		h.mu.Lock()
		e := h.seats[ref]
		background := 0
		for _, current := range h.seats {
			if !current.visible {
				background++
			}
		}
		eligible := e != nil && !e.visible
		if eligible {
			eligible = background > BackgroundLimit || h.opts.Now().Sub(e.used) >= IdleTimeout
		}
		h.mu.Unlock()
		if e != nil && !e.runtime.Idle() {
			// Idle age starts after activity, not when a long turn was opened.
			h.mu.Lock()
			e.used = h.opts.Now()
			h.mu.Unlock()
		} else if eligible {
			if closed, err := e.runtime.CloseIdle(ctx); err != nil {
				failures = errors.Join(failures, err)
			} else if closed {
				h.mu.Lock()
				if h.seats[ref] == e {
					delete(h.seats, ref)
				}
				h.mu.Unlock()
			}
		}
		lock.Unlock()
	}
	return failures
}

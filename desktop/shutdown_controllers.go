package main

import (
	"context"
	"fmt"

	"deepzora/internal/control"
)

type shutdownControllerClose struct {
	done chan struct{}
	err  error // immutable once done closes
}

func (c *desktopShutdownCoordinator) startControllerClose(item desktopShutdownItem) *shutdownControllerClose {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closes == nil {
		c.closes = map[string]*shutdownControllerClose{}
	}
	previous := c.closes[item.tab.ID]
	if previous != nil {
		select {
		case <-previous.done:
			if previous.err == nil {
				return previous
			}
		default:
			return previous
		}
	}
	attempt := &shutdownControllerClose{done: make(chan struct{})}
	c.closes[item.tab.ID] = attempt
	go attempt.run(item.ctrl)
	return attempt
}

func (attempt *shutdownControllerClose) run(ctrl control.SessionAPI) {
	defer close(attempt.done)
	defer func() {
		if recovered := recover(); recovered != nil {
			attempt.err = &shutdownStepError{code: "cleanup_panic", err: fmt.Errorf("controller close panic: %v", recovered)}
		}
	}()
	ctrl.Close()
	if closed, ok := ctrl.(interface{ Closed() <-chan struct{} }); ok {
		<-closed.Closed()
	}
}

func (c *desktopShutdownCoordinator) closeControllers(ctx context.Context, items []desktopShutdownItem) error {
	attempts := make([]*shutdownControllerClose, len(items))
	for i, item := range items {
		attempts[i] = c.startControllerClose(item)
	}
	for i, item := range items {
		select {
		case <-attempts[i].done:
			if attempts[i].err != nil {
				return attempts[i].err
			}
		case <-ctx.Done():
			return fmt.Errorf("wait for session %s to close: %w", item.tab.ID, ctx.Err())
		}
	}
	return nil
}

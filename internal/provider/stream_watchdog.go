package provider

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// StreamStall says which watchdog deadline ended a stream read.
type StreamStall int32

const (
	StreamStallNone StreamStall = iota
	// StreamStallIdle: no bytes at all — a half-open connection.
	StreamStallIdle
	// StreamStallNoOutput: the connection stayed alive (keep-alive comments,
	// empty events) but the model produced nothing.
	StreamStallNoOutput
)

// StreamWatchConfig holds the watchdog deadlines; zero fields take the defaults.
type StreamWatchConfig struct {
	Idle        time.Duration // no bytes
	FirstOutput time.Duration // no model output since the stream opened or since reasoning was last seen
	Progress    time.Duration // no model output since the previous output
}

func (c StreamWatchConfig) withDefaults() StreamWatchConfig {
	if c.Idle <= 0 {
		c.Idle = DefaultStreamIdleTimeout
	}
	if c.FirstOutput <= 0 {
		c.FirstOutput = DefaultStreamFirstOutputTimeout
	}
	if c.Progress <= 0 {
		c.Progress = DefaultStreamProgressTimeout
	}
	return c
}

// StreamWatchdog closes a response body when its read stops making progress, so
// a blocked scanner returns instead of hanging. It tracks two signals because
// bytes alone do not prove progress: a server can keep a dead generation's
// connection open with keep-alive lines indefinitely. The watchdog goroutine
// owns both timers; the read loop only pings buffered channels, so there is no
// Timer.Reset race.
type StreamWatchdog struct {
	cfg      StreamWatchConfig
	activity chan struct{}
	output   chan struct{}
	// thinking is the phase of the latest output ping. It is a field read on
	// receipt, not a second channel, so Output/Thinking pings cannot reorder.
	thinking atomic.Bool
	done     chan struct{}
	stall    atomic.Int32
	window   atomic.Int64 // the deadline that fired, for the error text
}

// WatchStream starts a watchdog over body. It also closes body when ctx is
// canceled. The caller must call Stop when the read returns — otherwise the
// goroutine outlives the call on a context whose Done() is nil.
func WatchStream(ctx context.Context, body io.Closer, cfg StreamWatchConfig) *StreamWatchdog {
	w := &StreamWatchdog{
		cfg:      cfg.withDefaults(),
		activity: make(chan struct{}, 1),
		output:   make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go w.run(ctx, body)
	return w
}

func (w *StreamWatchdog) run(ctx context.Context, body io.Closer) {
	idle := time.NewTimer(w.cfg.Idle)
	defer idle.Stop()
	armed := w.cfg.FirstOutput
	progress := time.NewTimer(armed)
	defer progress.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = body.Close()
			return
		case <-w.done:
			return
		case <-idle.C:
			w.window.Store(int64(w.cfg.Idle))
			w.stall.Store(int32(StreamStallIdle))
			_ = body.Close()
			return
		case <-progress.C:
			w.window.Store(int64(armed))
			w.stall.Store(int32(StreamStallNoOutput))
			_ = body.Close()
			return
		case <-w.activity:
			resetTimer(idle, w.cfg.Idle)
		case <-w.output:
			armed = w.cfg.Progress
			if w.thinking.Load() {
				armed = w.cfg.FirstOutput
			}
			resetTimer(idle, w.cfg.Idle)
			resetTimer(progress, armed)
		}
	}
}

func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// Activity records that bytes arrived. Non-blocking: a full buffer already
// carries a pending ping.
func (w *StreamWatchdog) Activity() {
	select {
	case w.activity <- struct{}{}:
	default:
	}
}

// Output records that the model produced text, tool-call data, or token-level
// reasoning.
// Protocol scaffolding (role-only deltas, usage, keep-alives) must not call it:
// providers emit those while a hidden-reasoning model is still thinking.
func (w *StreamWatchdog) Output() { w.ping(false) }

// Thinking records that the model is in a reasoning phase whose text may be
// hidden or arrive in sparse summaries. It re-arms the long first-output window
// instead of the progress window, so a legitimate silent think is not cut.
func (w *StreamWatchdog) Thinking() { w.ping(true) }

func (w *StreamWatchdog) ping(thinking bool) {
	w.thinking.Store(thinking)
	select {
	case w.output <- struct{}{}:
	default:
	}
}

// Stop ends the watchdog without closing the body.
func (w *StreamWatchdog) Stop() { close(w.done) }

// Stalled reports which deadline closed the body, if any.
func (w *StreamWatchdog) Stalled() StreamStall { return StreamStall(w.stall.Load()) }

// StallWindow returns the deadline that closed the body, for the error text.
func (w *StreamWatchdog) StallWindow() time.Duration { return time.Duration(w.window.Load()) }

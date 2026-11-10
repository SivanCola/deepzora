package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// A step still running after this long is reported with every goroutine's
// stack. The shell abandons a phase that stays unchanged for 30 seconds, so the
// report has to reach the log before then, and a stalled step never returns to
// log its own outcome.
var (
	shutdownStepStallAfter           = 10 * time.Second
	shutdownStallOutput    io.Writer = os.Stderr
)

func reportShutdownStepStall(name string, started time.Time) {
	slog.Warn("desktop: shutdown step stalled", "step", name, "elapsed_ms", time.Since(started).Milliseconds())
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	fmt.Fprintf(shutdownStallOutput, "desktop: shutdown step %q goroutines:\n%s\n", name, buf)
}

func (c *desktopShutdownCoordinator) setStep(name string) {
	c.mu.Lock()
	c.status.Step = name
	c.mu.Unlock()
}

func (c *desktopShutdownCoordinator) runStep(name string, run func()) {
	_ = c.runErrorStep(name, func() error { run(); return nil })
}

// The coordinator serializes attempts. Only successful steps are checkpointed;
// a retry must revisit a failed service while preserving completed resources.
func (c *desktopShutdownCoordinator) runErrorStep(name string, run func() error) (err error) {
	c.mu.Lock()
	done := c.finished[name]
	c.mu.Unlock()
	if done {
		return nil
	}
	started := time.Now()
	// Info, not Debug: the service keeps slog's default Info level, and these
	// lines are the only record of which step a stalled shutdown reached (#126).
	slog.Info("desktop: shutdown step", "step", name, "outcome", "started")
	c.setStep(name)
	stall := time.AfterFunc(shutdownStepStallAfter, func() { reportShutdownStepStall(name, started) })
	completed := false
	defer func() {
		stall.Stop()
		outcome := "failed"
		if completed {
			outcome = "completed"
		}
		if completed {
			slog.Info("desktop: shutdown step", "step", name, "outcome", outcome, "duration_ms", time.Since(started).Milliseconds())
		} else {
			slog.Warn("desktop: shutdown step failed", "step", name, "outcome", outcome, "duration_ms", time.Since(started).Milliseconds(), "err", err)
		}
	}()
	if err = run(); err != nil {
		return err
	}
	c.mu.Lock()
	c.finished[name] = true
	c.status.Step = ""
	c.mu.Unlock()
	completed = true
	return nil
}

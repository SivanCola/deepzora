package service

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A stalled step never returns, so the step in flight and the goroutine that
// blocks it must be observable while it is still running (#126).
func TestStalledShutdownStepIsNamedAndDumped(t *testing.T) {
	out := &lockedBuffer{}
	previousAfter, previousOutput := shutdownStepStallAfter, shutdownStallOutput
	shutdownStepStallAfter, shutdownStallOutput = 10*time.Millisecond, out
	defer func() { shutdownStepStallAfter, shutdownStallOutput = previousAfter, previousOutput }()

	c := &desktopShutdownCoordinator{finished: map[string]bool{}}
	release, entered, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		c.runStep("stalled-step", func() { close(entered); <-release })
	}()
	<-entered
	c.mu.Lock()
	step := c.status.Step
	c.mu.Unlock()
	if step != "stalled-step" {
		t.Fatalf("status step = %q while the step is in flight", step)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "TestStalledShutdownStepIsNamedAndDumped") {
		if time.Now().After(deadline) {
			t.Fatalf("stall report lacks the blocked goroutine: %q", out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(out.String(), `"stalled-step"`) {
		t.Fatalf("stall report does not name the step: %q", out.String())
	}
	close(release)
	<-done
	c.mu.Lock()
	step, finished := c.status.Step, c.finished["stalled-step"]
	c.mu.Unlock()
	if step != "" || !finished {
		t.Fatalf("completed step left step=%q finished=%v", step, finished)
	}
}

func TestFastShutdownStepIsNotReportedAsStalled(t *testing.T) {
	out := &lockedBuffer{}
	previousAfter, previousOutput := shutdownStepStallAfter, shutdownStallOutput
	shutdownStepStallAfter, shutdownStallOutput = 50*time.Millisecond, out
	defer func() { shutdownStepStallAfter, shutdownStallOutput = previousAfter, previousOutput }()

	c := &desktopShutdownCoordinator{finished: map[string]bool{}}
	c.runStep("fast-step", func() {})
	time.Sleep(150 * time.Millisecond)
	if got := out.String(); got != "" {
		t.Fatalf("fast step produced a stall report: %q", got)
	}
}

package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJoinPromptAndStdinKeepsBothParts(t *testing.T) {
	if got := joinPromptAndStdin("review this", "diff --git a b"); got != "review this\n\ndiff --git a b" {
		t.Fatalf("joined prompt = %q", got)
	}
	if got := joinPromptAndStdin("review this", ""); got != "review this" {
		t.Fatalf("prompt without stdin = %q", got)
	}
}

func TestReadPipedStdinReadsClosedPipe(t *testing.T) {
	r, w := testPipe(t)
	if _, err := w.WriteString("line1\nline2\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	if got := readPipedStdin(r, time.Second); got != "line1\nline2" {
		t.Fatalf("piped stdin = %q", got)
	}
}

func TestReadPipedStdinAbandonsSilentOpenPipe(t *testing.T) {
	r, _ := testPipe(t)
	started := time.Now()
	if got := readPipedStdin(r, 50*time.Millisecond); got != "" {
		t.Fatalf("silent pipe = %q, want empty", got)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("silent pipe blocked for %s", elapsed)
	}
}

// Input that starts inside the wait but finishes after it must arrive whole;
// the wait only bounds the time to the first byte.
func TestReadPipedStdinWaitsForStartedSlowWriter(t *testing.T) {
	r, w := testPipe(t)
	go func() {
		_, _ = w.WriteString("first ")
		time.Sleep(200 * time.Millisecond)
		_, _ = w.WriteString("second")
		_ = w.Close()
	}()
	if got := readPipedStdin(r, 100*time.Millisecond); got != "first second" {
		t.Fatalf("slow piped stdin = %q", got)
	}
}

func TestReadPipedStdinReadsRedirectedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("  from file \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := readPipedStdin(f, time.Second); got != "from file" {
		t.Fatalf("redirected stdin = %q", got)
	}
}

func testPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return r, w
}

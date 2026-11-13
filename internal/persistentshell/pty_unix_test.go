//go:build !windows

package persistentshell

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestUnixPTYReadDeadlineAndClose(t *testing.T) {
	conn, err := startPTY([]string{"/bin/sh", "-c", "sleep 60"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	file := conn.(*unixPTY).file
	if err := file.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("PTY reads must support cancellation: %v", err)
	}
	read := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			var buf [1]byte
			_, err := conn.Read(buf[:])
			done <- err
		}()
		return done
	}
	await := func(done <-chan error) error {
		t.Helper()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("PTY reader remained blocked")
			return nil
		}
	}
	if err := await(read()); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("idle read did not honor its deadline: %v", err)
	}
	if err := file.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	done := read()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := await(done); err == nil {
		t.Fatal("closed PTY read reported success")
	}
}

//go:build !windows

package persistentshell

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"deepzora/internal/proc"
)

type unixPTY struct {
	file      *os.File
	cmd       *exec.Cmd
	closeOnce sync.Once
}

func startPTY(argv []string, dir string, env []string) (ptyConn, error) {
	if len(argv) == 0 {
		return nil, errEmptyArgv
	}
	cmd := proc.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return nil, err
	}
	// PTY setup obtains Fd(), which can leave the master in blocking mode.
	// Register a fresh nonblocking descriptor with Go's poller so Close can
	// interrupt Read even when another process still holds the slave open.
	fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err == nil {
		err = unix.SetNonblock(fd, true)
		if err != nil {
			_ = unix.Close(fd)
		}
	}
	if err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = file.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("persistent shell PTY polling: %w", err)
	}
	pollable := os.NewFile(uintptr(fd), file.Name())
	_ = file.Close()
	file = pollable
	// Reap the shell without blocking: its exit surfaces to callers as a read
	// error on the master side, so the wait status itself carries no decision.
	go func() { _ = cmd.Wait() }()
	return &unixPTY{file: file, cmd: cmd}, nil
}

func (c *unixPTY) Read(p []byte) (int, error) {
	return c.file.Read(p)
}

func (c *unixPTY) Write(p []byte) (int, error) {
	return c.file.Write(p)
}

func (c *unixPTY) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		if c.cmd != nil && c.cmd.Process != nil {
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		}
		if c.file != nil {
			closeErr = c.file.Close()
		}
	})
	return closeErr
}

package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"time"
)

// pipedStdinStartWait bounds how long a run that already has a prompt argument
// waits for piped input to start. Parents that hand over an open pipe and never
// write or close it would otherwise hang a run that never needed stdin.
const pipedStdinStartWait = 2 * time.Second

// headlessPrompt joins the prompt arguments with any piped stdin, so
// `git diff | deepzora -p "review this"` sends both. Without arguments stdin is
// the whole prompt, read to EOF as before.
func headlessPrompt(args []string) string {
	prompt := strings.TrimSpace(strings.Join(args, " "))
	if prompt == "" {
		return readStdin()
	}
	return joinPromptAndStdin(prompt, readPipedStdin(os.Stdin, pipedStdinStartWait))
}

func joinPromptAndStdin(prompt, piped string) string {
	if piped == "" {
		return prompt
	}
	return prompt + "\n\n" + piped
}

// readPipedStdin returns the trimmed contents of a pipe or redirected file on
// f. Terminals, sockets and other descriptors yield "" so spawners that leave
// stdio attached keep their old behavior. A pipe that produces no byte and no
// EOF within startWait is abandoned; once data starts it is read to EOF.
func readPipedStdin(f *os.File, startWait time.Duration) string {
	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	mode := stat.Mode()
	switch {
	case mode.IsRegular():
		data, _ := io.ReadAll(f)
		return strings.TrimSpace(string(data))
	case mode&os.ModeNamedPipe == 0:
		return ""
	}
	started := make(chan struct{})
	done := make(chan []byte, 1)
	go func() {
		var buf bytes.Buffer
		chunk := make([]byte, 32*1024)
		signalled := false
		for {
			n, err := f.Read(chunk)
			if n > 0 {
				buf.Write(chunk[:n])
				if !signalled {
					signalled = true
					close(started)
				}
			}
			if err != nil {
				break
			}
		}
		done <- buf.Bytes()
	}()
	timer := time.NewTimer(startWait)
	defer timer.Stop()
	select {
	case data := <-done:
		return strings.TrimSpace(string(data))
	case <-started:
		return strings.TrimSpace(string(<-done))
	case <-timer.C:
		return ""
	}
}

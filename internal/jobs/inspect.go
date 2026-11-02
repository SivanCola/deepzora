package jobs

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// InspectForSession is a bounded, non-consuming UI read. It never moves the
// agent's job_output cursor or consumes the pending task result/evidence.
func (m *Manager) InspectForSession(parentSession, id string) (Result, bool) {
	if parentSession == "" {
		return Result{}, false
	}
	j := m.get(parentSession, id)
	if j == nil {
		return Result{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	text := string(j.tail)
	if j.outcome.text != "" {
		text = j.outcome.text
	}
	// Completed artifact-backed jobs release their in-memory output. Read only
	// the durable tail, without touching readOffset, outcome.read or evidence.
	if j.artifactPath != "" {
		if tail, err := inspectArtifactTail(j.artifactPath); err == nil {
			text = tail
		}
	}
	if len(text) > defaultTailBytes {
		text = text[len(text)-defaultTailBytes:]
	}
	// jobWriter may already have clipped exactly to the byte budget. It can
	// also be observed between writes of a multi-byte rune.
	for len(text) > 0 && !utf8.RuneStart(text[0]) {
		text = text[1:]
	}
	text = strings.ToValidUTF8(text, "")
	return Result{ID: j.ID, Kind: j.Kind, Label: j.Label, Status: j.status, Output: text}, true
}

func inspectArtifactTail(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := max(int64(0), info.Size()-defaultTailBytes)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, defaultTailBytes))
	return string(b), err
}

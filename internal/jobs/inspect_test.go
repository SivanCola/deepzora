package jobs

import (
	"context"
	"deepzora/internal/event"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestInspectDoesNotConsumeAgentOutputAndKeepsSessionScope(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()
	written := make(chan struct{})
	job := m.StartForSession("parent", "bash", "streaming", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = io.WriteString(out, "first output\n")
		close(written)
		<-ctx.Done()
		return "", ctx.Err()
	})
	<-written
	for range 2 {
		result, ok := m.InspectForSession("parent", job.ID)
		if !ok || result.Output != "first output\n" {
			t.Fatalf("inspection: %+v %v", result, ok)
		}
	}
	for _, parent := range []string{"", "other"} {
		if _, ok := m.InspectForSession(parent, job.ID); ok {
			t.Fatal("foreign output exposed")
		}
	}
	text, _, _ := m.OutputForSession("parent", job.ID)
	if text != "first output\n" {
		t.Fatal("inspection consumed the agent cursor")
	}
	if text, _, _ := m.OutputForSession("parent", job.ID); text != "" {
		t.Fatal("agent incremental cursor did not advance")
	}
	result, _ := m.InspectForSession("parent", job.ID)
	if result.Output != "first output\n" {
		t.Fatal("agent reads consumed the UI snapshot")
	}
	m.KillForSession("parent", job.ID)
}

func TestInspectBoundsTerminalTaskTextWithoutConsumingResult(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()
	m.BindSession("parent", filepath.Join(t.TempDir(), "parent.jsonl"))
	text := strings.Repeat("中文", 100_000)
	job := m.StartForSession("parent", "task", "answer", func(context.Context, io.Writer) (string, error) { return text, nil })
	<-job.done
	result, ok := m.InspectForSession("parent", job.ID)
	if !ok || result.Output == "" || len(result.Output) > defaultTailBytes || !utf8.ValidString(result.Output) || !strings.HasSuffix(text, result.Output) {
		t.Fatal("inspection is not a bounded UTF-8 tail")
	}
	output, _, _ := m.OutputForSession("parent", job.ID)
	if output != text {
		t.Fatal("inspection consumed the task answer")
	}
}

func TestInspectCompletedShellReadsDurableOutputWithoutConsuming(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()
	m.BindSession("parent", filepath.Join(t.TempDir(), "parent.jsonl"))
	const output = "build completed successfully\n"
	j := m.StartForSession("parent", "bash", "build", func(_ context.Context, out io.Writer) (string, error) {
		_, err := io.WriteString(out, output)
		return "", err
	})
	<-j.done
	if len(j.tail) != 0 || j.outcome.text != "" || j.artifactPath == "" {
		t.Fatal("expected output to have been released to the durable artifact")
	}
	for range 2 {
		result, ok := m.InspectForSession("parent", j.ID)
		if !ok || result.Status != Done || result.Output != output {
			t.Fatalf("durable inspection: %+v %v", result, ok)
		}
	}
	text, _, _ := m.OutputForSession("parent", j.ID)
	if text != output {
		t.Fatal("UI reads consumed the durable agent output")
	}
	if text, _, _ := m.OutputForSession("parent", j.ID); text != "" {
		t.Fatal("agent cursor did not advance")
	}
	result, _ := m.InspectForSession("parent", j.ID)
	if result.Output != output {
		t.Fatal("agent read changed the UI snapshot")
	}
}

func TestInspectRunningOutputRepairsAlreadyClippedUTF8(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "artifact"}[durable], func(t *testing.T) {
			m := NewManager(event.Discard)
			defer m.Close()
			if durable {
				m.BindSession("parent", filepath.Join(t.TempDir(), "parent.jsonl"))
			} else {
				// Exercise the memory fallback as well as artifact-backed jobs.
				tempRoot := m.tempRoot
				t.Cleanup(func() { _ = os.RemoveAll(tempRoot) })
				m.tempRoot = ""
			}
			text := strings.Repeat("中", 23_000)
			written := make(chan struct{})
			j := m.StartForSession("parent", "bash", "UTF-8", func(ctx context.Context, out io.Writer) (string, error) {
				_, _ = io.WriteString(out, text)
				close(written)
				<-ctx.Done()
				return "", ctx.Err()
			})
			<-written
			result, ok := m.InspectForSession("parent", j.ID)
			if !ok || result.Status != Running || result.Output == "" || !utf8.ValidString(result.Output) || len(result.Output) > defaultTailBytes || !strings.HasSuffix(text, result.Output) {
				t.Fatalf("running tail: status=%s bytes=%d valid=%v", result.Status, len(result.Output), utf8.ValidString(result.Output))
			}
			if durable {
				output, _, _ := m.OutputForSession("parent", j.ID)
				if output != text {
					t.Fatal("inspection altered original UTF-8 bytes or consumed output")
				}
			}
		})
	}
}

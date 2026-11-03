package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestAStreamMasksAValueSplitAcrossWrites(t *testing.T) {
	watch("STREAM_KEY", "sk-stream-0123456789")
	var out bytes.Buffer
	s := NewStream(&out)
	for _, piece := range []string{"token=sk-st", "ream-01234", "56789 done\n"} {
		if _, err := s.Write([]byte(piece)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "token=[secret:STREAM_KEY] done\n" {
		t.Fatalf("stream = %q", got)
	}
}

// Text that cannot begin a value passes on at once: a job waiting for a line
// must see it without the stream ending.
func TestAStreamPassesOnWhatCannotBeginAValue(t *testing.T) {
	watch("STREAM_KEY2", "sk-other-0123456789")
	var out bytes.Buffer
	s := NewStream(&out)
	_, _ = s.Write([]byte("server ready\n"))
	if out.String() != "server ready\n" {
		t.Fatalf("held back %q", out.String())
	}
	_, _ = s.Write([]byte("next sk-oth"))
	if out.String() != "server ready\nnext " {
		t.Fatalf("a possible value's start was passed on: %q", out.String())
	}
	_ = s.Flush()
	if !strings.HasSuffix(out.String(), "next sk-oth") {
		t.Fatalf("flush lost the held fragment: %q", out.String())
	}
}

func TestRunningStreamMasksNewlySavedValuesImmediately(t *testing.T) {
	var current []Labeled
	WatchValues(t.Name(), func() []Labeled { return current })
	t.Cleanup(func() { WatchValues(t.Name(), func() []Labeled { return nil }) })
	var out bytes.Buffer
	s := NewStream(&out)
	_, _ = s.Write([]byte("ready\n"))
	current = []Labeled{{Label: "STREAM_NEW", Value: "newly-saved-stream-value"}}
	_, _ = s.Write([]byte("newly-saved-stream-value\n"))
	_ = s.Flush()
	if got := out.String(); got != "ready\n[secret:STREAM_NEW]\n" {
		t.Fatalf("stream leaked a newly saved value: %q", got)
	}
}

func TestStreamShortValuesKeepOriginalWordBoundariesAtEverySplit(t *testing.T) {
	watch("STREAM_PIN", "4267")
	for _, input := range []string{"4267", "x4267", "4267x", "x4267x", " 4267 ", "4267 4267", "x4267 4267y 4267"} {
		want := Masker().String(input)
		for split := 0; split <= len(input); split++ {
			var out bytes.Buffer
			stream := NewStream(&out)
			_, _ = stream.Write([]byte(input[:split]))
			_, _ = stream.Write([]byte(input[split:]))
			_ = stream.Flush()
			if out.String() != want {
				t.Fatalf("%q split at %d: %q, want %q", input, split, out.String(), want)
			}
		}
	}
}

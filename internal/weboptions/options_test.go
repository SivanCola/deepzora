package weboptions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLocalWebOptions(t *testing.T) {
	dir := t.TempDir()
	o, err := ParseOptions([]string{"--dir", dir, "--port", "8080", "--no-open"})
	if err != nil || o.Dir != dir || o.Port != 8080 || !o.NoOpen {
		t.Fatalf("options = %+v, %v", o, err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--host", "0.0.0.0"}, {"--port", "65536"}, {"--port", "-1"}, {"--port", "x"}, {"--dir", file}, {"--dir", filepath.Join(dir, "missing")}, {"task"}} {
		if _, err := ParseOptions(args); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		if o, err := ParseOptions(args); err != nil || !o.Help {
			t.Fatalf("help = %+v, %v", o, err)
		}
	}
}

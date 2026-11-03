package builtin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/secrets"
)

func TestRecursiveSearchKeepsHostSecretNamespacePrivateWithoutOptionalDenylist(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "team-slots")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "opaque-slot"), []byte("needle private-value"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("needle public-value"), 0600); err != nil {
		t.Fatal(err)
	}
	secrets.RegisterHostSecretPath(private)
	// An overlapping tree uses the native confinement owner even when rg is
	// configured; an external walker must not open protected credential files.
	reader := grepTool{workDir: dir, rg: "not-an-executable"}
	args, _ := json.Marshal(map[string]string{"pattern": "needle", "path": dir})
	out, err := reader.Execute(t.Context(), args)
	if err != nil || strings.Contains(out, "private-value") || !strings.Contains(out, "public-value") {
		t.Fatalf("protected recursive search: %q %v", out, err)
	}
}

package installsource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/pluginpkg"
)

func TestMarketRecoveryBetweenTreeSwapAndStateCommit(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncommitted", true: "committed"}[committed], func(t *testing.T) {
			home := t.TempDir()
			target := pluginpkg.InstallRoot(home, "kit")
			d := txnDir(home)
			old := pluginpkg.InstalledPlugin{Name: "kit", Root: "plugins/kit", Version: "1.0.0", Commit: strings.Repeat("a", 40)}
			next := old
			next.Version = "1.1.0"
			next.Commit = strings.Repeat("b", 40)
			for file, body := range map[string]string{filepath.Join(target, "marker"): "new", filepath.Join(d, "previous", "marker"): "old"} {
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			b, _ := json.Marshal(catalogTransaction{Previous: &old, Next: next})
			if err := os.WriteFile(filepath.Join(d, "journal.json"), b, 0600); err != nil {
				t.Fatal(err)
			}
			record, want := old, "old"
			if committed {
				record, want = next, "new"
			}
			if err := pluginpkg.Upsert(home, record); err != nil {
				t.Fatal(err)
			}
			if err := RecoverCatalog(t.Context(), home); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(target, "marker"))
			if err != nil || string(got) != want {
				t.Fatalf("got %s %v", got, err)
			}
			if err := RecoverCatalog(t.Context(), home); err != nil {
				t.Fatal("not idempotent", err)
			}
		})
	}
}

package plugincatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryIsNotApproval(t *testing.T) {
	h := t.TempDir()
	market, topics := filepath.Join(h, "catalog"), filepath.Join(h, "topic")
	t.Setenv("DEEPZORA_PLUGIN_CATALOG", market)
	t.Setenv("DEEPZORA_PLUGIN_TAGGED", topics)
	row := Release{ID: "community/a", Name: "Tool", Version: "1.0.0", Repo: "A/Repo", Commit: strings.Repeat("a", 40), ManifestName: "tool", ManifestKind: "deepzora", Summary: map[string]string{"en": "Tool", "zh": "工具"}}
	put := func(p string, rows ...Release) {
		b, _ := json.Marshal(Index{SchemaVersion: 1, Releases: rows})
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	upstream := row
	upstream.ID = "a/repo"
	upstream.Version = "2.0.0"
	upstream.Commit = strings.Repeat("b", 40)
	other := upstream
	other.ID = "b/repo"
	other.Repo = "b/repo"
	put(market, row)
	put(topics, upstream, other)
	got, err := Search(t.Context(), h, "", true)
	if err != nil || len(got.Entries) != 2 {
		t.Fatal(got, err)
	}
	e := got.Entries[0]
	if e.Channel != "custom" || e.Commit != row.Commit || e.Upstream == nil || e.Upstream.Version != "2.0.0" {
		t.Fatal(e)
	}
	row.Yanked = true
	put(market, row)
	if _, err := Resolve(t.Context(), h, e.Spec); err == nil {
		t.Fatal("cached result authorized a withdrawn release")
	}
	if err = os.Remove(market); err != nil {
		t.Fatal(err)
	}
	cache, _ := filepath.Glob(filepath.Join(h, "cache", "plugin-market", "*.json"))
	for _, p := range cache {
		past := time.Now().Add(-24 * time.Hour)
		if err = os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}
	got, err = Search(t.Context(), h, "", false)
	if err != nil || got.Warning == "" {
		t.Fatal("offline browse", got, err)
	}
	if _, err = Resolve(t.Context(), h, e.Spec); err == nil {
		t.Fatal("offline cache authorized installation")
	}
}

func TestMalformedRowsDoNotHideValidReleases(t *testing.T) {
	good := Release{ID: "a/tool", Name: "Tool", Version: "1.0.0", Repo: "a/tool", Commit: strings.Repeat("a", 40), ManifestName: "tool", ManifestKind: "deepzora", Summary: map[string]string{"en": "Tool", "zh": "工具"}}
	badVersion, badName, squatter := good, good, good
	badVersion.ID, badVersion.Repo, badVersion.Version = "b/tool", "b/tool", "1.0.0-a..b"
	badName.ID, badName.Repo, badName.ManifestName = "c/tool", "c/tool", "has space"
	squatter.ID = "d/tool" // same repository and path as good, under another ID
	b, _ := json.Marshal(Index{SchemaVersion: 1, Releases: []Release{good, badVersion, badName, squatter}})
	idx, err := Parse(b)
	if err != nil || len(idx.Releases) != 1 || idx.Releases[0].ID != "a/tool" || idx.Skipped != 3 {
		t.Fatal(idx, err)
	}
	file := filepath.Join(t.TempDir(), "tagged.json")
	if err = os.WriteFile(file, b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPZORA_PLUGIN_TAGGED", file)
	if _, err = Resolve(t.Context(), t.TempDir(), "topic:b/tool"); err == nil {
		t.Fatal("a dropped release must not be installable")
	}
}

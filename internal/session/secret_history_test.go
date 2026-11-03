package session

import (
	"bytes"
	"context"
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"deepzora/internal/sessioncontent"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryFirstSecretHitCreatesNewContentIdentityAndMasksEscapedValues(t *testing.T) {
	root := t.TempDir()
	store := contentStoreForSessionDir(root)
	value := "history-private-value-3581"
	body := json.RawMessage(`{"content":"history-private-value-3581","seq":3581}`)
	ref, err := store.Put(t.Context(), bytes.NewReader(body), sessioncontent.Metadata{MediaType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	prior, err := projectHistoryContent(t.Context(), root, ref)
	if err != nil || prior != ref {
		t.Fatalf("clean content changed: %+v %v", prior, err)
	}
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "HISTORY", Value: value}} })
	projected, err := projectHistoryContent(t.Context(), root, ref)
	if err != nil || projected.Digest == ref.Digest {
		t.Fatalf("new secret reused unmasked content identity: %+v %v", projected, err)
	}
	masked, err := store.ReadRange(t.Context(), projected, 0, projected.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(masked), value) || !strings.Contains(string(masked), "[secret:HISTORY]") || !strings.Contains(string(masked), `"seq":3581`) {
		t.Fatalf("history projection: %s", masked)
	}
	original, err := store.ReadRange(t.Context(), ref, 0, ref.Bytes)
	if err != nil || !bytes.Equal(original, body) {
		t.Fatal("history masking rewrote the original immutable object")
	}
}

func TestLegacyHistoryAndSearchMaskFirstRegistrationWithoutRewritingLogs(t *testing.T) {
	service, err := NewService("local", NewFilesystemPersistence(filepath.Join(t.TempDir(), "sessions")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.CloseAll(context.Background()) })
	runtime, err := service.Create(t.Context(), CreateOptions{SessionID: "first-secret"})
	if err != nil {
		t.Fatal(err)
	}
	value := "legacy-history-private-915073"
	raw, _ := json.Marshal(map[string]any{"message": provider.Message{ID: "first-user", Role: provider.RoleUser, Content: "findable " + value}})
	if _, err := runtime.Session().Append(t.Context(), Batch{OperationID: "before-registration", Events: []Event{{Kind: "message/complete", Payload: raw}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Session().Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	query, ref := service.Query(), runtime.Ref()
	// Build indexes while the value is not yet registered: first-hit reads
	// must protect old derived previews as well as new writes.
	searchHistoryReady(t, query, ref, "findable", "", 10)
	windowReady(t, query, ref, HistoryWindowRequest{Anchor: "newest"})
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "LEGACY", Value: value}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	messages, err := query.History(t.Context(), ref)
	if err != nil || len(messages) != 1 || strings.Contains(messages[0].Content, value) {
		t.Fatalf("legacy history: %+v %v", messages, err)
	}
	page := searchHistoryReady(t, query, ref, "findable", "", 10)
	if len(page.Hits) != 1 || strings.Contains(page.Hits[0].Preview, value) {
		t.Fatalf("search preview leaked: %+v", page)
	}
	outline, err := query.ReadHistoryOutline(t.Context(), ref, HistoryOutlineRequest{})
	if err != nil || len(outline.Entries) != 1 || strings.Contains(outline.Entries[0].Prompt, value) {
		t.Fatalf("outline leaked: %+v %v", outline, err)
	}
}

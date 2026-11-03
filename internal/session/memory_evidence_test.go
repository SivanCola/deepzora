package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"deepzora/internal/provider"
)

func TestMemoryEvidencePagesOnlyNewHumanTurnsWithoutLoadingAssistantBodies(t *testing.T) {
	service, err := NewService("local", NewFilesystemPersistence(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.CloseAll(context.Background()) })
	runtime, err := service.Create(t.Context(), CreateOptions{SessionID: "memory-evidence"})
	if err != nil {
		t.Fatal(err)
	}
	epoch := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	for i := range 42 {
		message := provider.Message{ID: fmt.Sprintf("message-%02d", i), Role: provider.RoleUser, Content: fmt.Sprintf("human %d", i), CreatedAt: epoch.Add(time.Duration(i) * time.Second).UnixMilli()}
		if i%3 == 0 {
			message.Origin = provider.MessageOriginHost
			message.Content = "scheduler text is not human evidence"
		}
		if i%3 == 1 {
			message.Role = provider.RoleAssistant
			message.Content = "agent output is not human evidence"
		}
		payload, _ := json.Marshal(map[string]any{"message": message})
		if _, err := runtime.Session().AppendBatch(t.Context(), message.ID, []Event{{Kind: "message/complete", Payload: payload}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.Session().Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := service.Query().ReadUserEvidenceAfter(t.Context(), runtime.Ref(), 0, "", epoch.Add(10*time.Second), 4)
	if err != nil || len(page.Lines) != 4 || !page.More {
		t.Fatalf("first evidence: %+v %v", page, err)
	}
	if page.Lines[0].MessageID != "message-11" || page.Lines[3].MessageID != "message-20" {
		t.Fatalf("newest-page traversal lost earlier unreviewed evidence: %+v", page.Lines)
	}
	next, err := service.Query().ReadUserEvidenceAfter(t.Context(), runtime.Ref(), page.Through, page.Generation, epoch.Add(10*time.Second), 4)
	if err != nil || len(next.Lines) != 4 || next.Lines[0].MessageID != "message-23" {
		t.Fatalf("cursor replayed evidence: %+v %v", next, err)
	}
	if _, err := service.Query().ReadUserEvidenceAfter(t.Context(), runtime.Ref(), page.Through, "replaced-generation", epoch, 4); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("replacement accepted cursor: %v", err)
	}
}

package history

import (
	"context"
	"slices"
	"strings"
	"testing"

	"deepzora/internal/historycatalog"
	"deepzora/internal/provider"
)

func TestHistorySurfacesExcludePinnedContextRevisions(t *testing.T) {
	service := currentHistoryStore(t)
	revision := provider.Message{
		Role: provider.RoleUser, Origin: provider.MessageOriginHost,
		Content: "<pinned_context_revision>private pinned body</pinned_context_revision>",
	}
	saveCurrentHistory(t, service, "pinned", revision, provider.Message{Role: provider.RoleUser, Content: "visible question"})

	searcher := newReadyIndexedSearcher(t, currentOptions(service))
	hits, err := searcher.Search(context.Background(), SearchRequest{Query: "private pinned body"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Search() returned pinned revision hits: %#v", hits)
	}
	around, err := searcher.Around(context.Background(), AroundRequest{CatalogID: catalogID(service.Query()), SessionID: "pinned", ContentDigest: currentDigest(t, service, "pinned"), MessageIndex: 1, Before: 1, After: 1})
	if err != nil {
		t.Fatalf("Around() error = %v", err)
	}
	if len(around) != 1 || strings.Contains(around[0].Text, "private pinned body") {
		t.Fatalf("Around() exposed pinned revision: %#v", around)
	}
	if _, ok := candidateText([]provider.Message{revision}, historycatalog.Candidate{MessageIndex: 0, Kind: string(KindUserText)}); ok {
		t.Fatal("candidateText accepted a stale pinned-revision index entry")
	}
}

func TestContextIndicesBoundsWindowAndSkipsPinnedRevisions(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "first"},
		{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: "<pinned_context_revision>pinned</pinned_context_revision>"},
		{Role: provider.RoleAssistant, Content: "hit"},
		{Role: provider.RoleUser, Content: "after"},
	}
	if got, want := ContextIndices(messages, 2, 5, 5), []int{0, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("ContextIndices = %v, want %v", got, want)
	}
	if got, want := ContextIndices(messages, 2, 0, -1), []int{2}; !slices.Equal(got, want) {
		t.Fatalf("zero/negative window = %v, want %v", got, want)
	}
	if got := ContextIndices(messages, 4, 1, 1); len(got) != 0 {
		t.Fatalf("out-of-range index = %v, want empty", got)
	}
}

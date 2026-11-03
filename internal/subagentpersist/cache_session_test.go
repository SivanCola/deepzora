package subagentpersist

import (
	"testing"

	"deepzora/internal/agent"
)

func TestChildCacheIdentityPersistsAndFreshChildrenHaveDistinctIdentities(t *testing.T) {
	store, _ := testStore(t, t.TempDir())
	handle, err := store.Create(t.Context(), agent.SubagentMeta{Ref: "sa_cache"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	identity := handle.(agent.SessionCacheIdentity).ProviderCacheSessionIdentity()
	if identity == "" {
		t.Fatal("child has no cache identity")
	}
	if err := handle.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	resumed, err := store.Open(t.Context(), "sa_cache")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.(agent.SessionCacheIdentity).ProviderCacheSessionIdentity() != identity {
		t.Fatal("reopening the archive changed identity")
	}
	if err := resumed.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Create(t.Context(), agent.SubagentMeta{Ref: "sa_fresh"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close(t.Context())
	if fresh.(agent.SessionCacheIdentity).ProviderCacheSessionIdentity() == identity {
		t.Fatal("fresh child reused another child's identity")
	}
}

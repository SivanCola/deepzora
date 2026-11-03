package teamroster

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestExchangeAdmissionSerializesSeparateStores(t *testing.T) {
	dir := t.TempDir()
	full := errors.New("full")
	start, entered, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	done := make(chan error, 2)
	go func() {
		_, _, err := (Store{Dir: dir}).OpenExchangeChecked(t.Context(), ask(ByUser, newAgentID(), ""), "first", func(book []Exchange) error {
			close(entered)
			<-release
			return nil
		})
		done <- err
	}()
	<-entered
	go func() {
		close(start)
		_, _, err := (Store{Dir: dir}).OpenExchangeChecked(t.Context(), ask(ByUser, newAgentID(), ""), "second", func(book []Exchange) error {
			if len(book) >= 1 {
				return full
			}
			return nil
		})
		done <- err
	}()
	<-start
	close(release)
	a, b := <-done, <-done
	if !((a == nil && errors.Is(b, full)) || (b == nil && errors.Is(a, full))) {
		t.Fatalf("admission results: %v / %v", a, b)
	}
	book, err := (Store{Dir: dir}).Exchanges()
	if err != nil || len(book) != 1 {
		t.Fatalf("book=%+v err=%v", book, err)
	}
}

func TestCompactionRetainsUnconsumedRepliesAndActiveAncestry(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	ctx := context.Background()
	parent, _, err := s.OpenExchange(ctx, ask(ByUser, newAgentID(), ""), "parent")
	if err != nil {
		t.Fatal(err)
	}
	childSpec := ask(parent.To, newAgentID(), "")
	childSpec.Parent = parent.ID
	child, _, err := s.OpenExchange(ctx, childSpec, "child")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetExchangeState(ctx, parent.ID, ExchangeInterrupted, parent.At); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReturned(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetExchangeState(ctx, child.ID, ExchangeReplied, child.At); err != nil {
		t.Fatal(err)
	}
	// More recent, consumed work must not displace an unread reply or its ancestry.
	book, err := s.Exchanges()
	if err != nil {
		t.Fatal(err)
	}
	for range keepClosed + 1 {
		x := parent
		x.ID, x.Parent = "x-"+randomHex(), ""
		x.State, x.Returned = ExchangeInterrupted, true
		book = append(book, x)
	}
	if err := compactExchanges(filepath.Join(s.Dir, ExchangeFile), book, compactAt+1); err != nil {
		t.Fatal(err)
	}
	book, err = s.Exchanges()
	if err != nil {
		t.Fatal(err)
	}
	if len(book) != keepClosed+2 || book[0].ID != parent.ID || book[1].ID != child.ID || book[1].Returned {
		t.Fatalf("lost active dependency: got %d records", len(book))
	}
}

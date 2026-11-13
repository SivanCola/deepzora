package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedServiceLifecycleAndContract(t *testing.T) {
	t.Setenv("DEEPZORA_HOME", t.TempDir())
	s := NewService(ServiceOptions{Headless: true})
	if _, err := s.Invoke(t.Context(), "SubmitForTab", nil); err == nil {
		t.Fatal("commands admitted before startup")
	}
	contract, err := s.Contract()
	if err != nil || len(contract.Commands) == 0 {
		t.Fatalf("contract = %+v, %v", contract, err)
	}
	for _, command := range contract.Commands {
		if command.Name == "Start" || command.Name == "Shutdown" || command.Name == "SetEventSink" {
			t.Fatalf("transport exposed service lifecycle method %s", command.Name)
		}
	}
	if err := s.SetEventSink(func(context.Context, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := s.Start(t.Context()); err == nil {
		t.Fatal("service started twice")
	}
	if err := s.SetEventSink(nil); err == nil {
		t.Fatal("event owner replaced after startup")
	}
	result, err := s.Invoke(t.Context(), "Version", nil)
	if err != nil || result != version {
		t.Fatalf("shared invocation = %v, %v", result, err)
	}
	current, err := s.Contract()
	if err != nil || current.Digest() != contract.Digest() {
		t.Fatal("startup changed the command contract")
	}
	if s.app.desktopShell.coordinator != nil || !s.app.localWeb {
		t.Fatal("headless service started native integrations")
	}
}

func TestSharedServiceOwnsApplicationAndSessionEvents(t *testing.T) {
	first, second := NewService(ServiceOptions{Headless: true}), NewService(ServiceOptions{Headless: true})
	firstEvents, secondEvents := make(chan string, 4), make(chan string, 4)
	if err := first.SetEventSink(func(_ context.Context, name string, _ ...any) { firstEvents <- name }); err != nil {
		t.Fatal(err)
	}
	if err := second.SetEventSink(func(_ context.Context, name string, _ ...any) { secondEvents <- name }); err != nil {
		t.Fatal(err)
	}
	first.app.runtimeEvents.Emit(t.Context(), "application:first")
	firstSession := &tabEventSink{app: first.app, ctx: t.Context()}
	firstSession.emitRuntimeEvent("agent:first", map[string]string{"kind": "approval"})
	secondSession := &tabEventSink{app: second.app, ctx: t.Context()}
	secondSession.emitRuntimeEvent("agent:second")
	for _, want := range []string{"application:first", "agent:first"} {
		select {
		case got := <-firstEvents:
			if got != want {
				t.Fatalf("first service event = %s, want %s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("first service lost %s", want)
		}
	}
	select {
	case got := <-secondEvents:
		if got != "agent:second" {
			t.Fatalf("second service received %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second service lost its session event")
	}
	select {
	case got := <-firstEvents:
		t.Fatalf("cross-service event leak: %s", got)
	default:
	}
}

func TestSharedServiceEventRebindingIsConcurrentSafe(t *testing.T) {
	s := NewService(ServiceOptions{Headless: true})
	var received atomic.Int32
	delivered := make(chan struct{})
	if err := s.SetEventSink(func(context.Context, string, ...any) {
		if received.Add(1) == 1000 {
			close(delivered)
		}
	}); err != nil {
		t.Fatal(err)
	}
	sink := &tabEventSink{app: s.app, ctx: t.Context()}
	rebound := make(chan struct{})
	go func() {
		for range 1000 {
			sink.setBinding("target", s.app)
		}
		close(rebound)
	}()
	for range 1000 {
		sink.emitRuntimeEvent("agent:event")
	}
	<-rebound
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatalf("service received %d/1000 events during rebinding", received.Load())
	}
}

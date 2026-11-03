package secrets

import (
	"sync/atomic"
	"testing"
)

func TestMaskSnapshotIsReusedUntilSourceValuesChange(t *testing.T) {
	values := []Labeled{{Label: "CACHE_PROBE", Value: "cache-probe-value-0001"}}
	WatchValues(t.Name(), func() []Labeled { return values })
	first := Masker()
	if second := Masker(); first != second {
		t.Fatal("unchanged sources rebuilt the mask")
	}
	values = []Labeled{{Label: "CACHE_PROBE", Value: "cache-probe-value-0002"}}
	changed := Masker()
	if changed == first {
		t.Fatal("changed source reused the old mask")
	}
	for _, value := range []string{"cache-probe-value-0001", "cache-probe-value-0002"} {
		if changed.String(value) == value {
			t.Fatal("new or retired value escaped masking")
		}
	}
	values = nil
	if Masker() != changed {
		t.Fatal("retiring a value rebuilt an unchanged retained mask")
	}
}

func TestConfirmedForgetFencesAlreadyReadingSourceSnapshot(t *testing.T) {
	value := Labeled{Label: "FORGET_RACE", Value: "forget-race-private-value"}
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	WatchValues(t.Name(), func() []Labeled {
		if reads.Add(1) == 1 {
			close(started)
			<-release
			return []Labeled{value}
		}
		return nil
	})
	t.Cleanup(func() { UnwatchValues(t.Name()) })
	done := make(chan *Mask, 1)
	go func() { done <- Masker() }()
	<-started
	ForgetValues([]Labeled{value})
	close(release)
	if mask := <-done; mask.String(value.Value) != value.Value {
		t.Fatal("in-flight source reintroduced a confirmed forgotten value")
	}
}

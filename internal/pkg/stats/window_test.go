package stats

import (
	"testing"
	"time"
)

func TestQuantile(t *testing.T) {
	w := NewWindow(100)
	if w.Quantile(0.95) != 0 {
		t.Fatal("empty window should report 0")
	}
	for i := 1; i <= 100; i++ {
		w.Add(time.Duration(i) * time.Millisecond)
	}
	if got := w.Quantile(0.95); got != 95*time.Millisecond {
		t.Fatalf("p95 = %s, want 95ms", got)
	}
	if got := w.Quantile(0.5); got != 50*time.Millisecond {
		t.Fatalf("p50 = %s, want 50ms", got)
	}
}

func TestWindowOverwritesOldest(t *testing.T) {
	w := NewWindow(3)
	for _, ms := range []int{100, 100, 100, 1, 2, 3} {
		w.Add(time.Duration(ms) * time.Millisecond)
	}
	if got := w.Quantile(1); got != 3*time.Millisecond {
		t.Fatalf("max = %s, want 3ms (old values must be gone)", got)
	}
}

func BenchmarkWindowAdd(b *testing.B) {
	w := NewWindow(512)
	for i := 0; i < b.N; i++ {
		w.Add(time.Duration(i))
	}
}

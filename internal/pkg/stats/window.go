// Package stats provides small, allocation-light statistics helpers.
package stats

import (
	"sort"
	"sync"
	"time"
)

// Window keeps the last N durations and answers quantile queries over them.
// It is safe for concurrent use.
type Window struct {
	mu   sync.Mutex
	buf  []time.Duration
	next int
	full bool
}

func NewWindow(size int) *Window {
	return &Window{buf: make([]time.Duration, size)}
}

func (w *Window) Add(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf[w.next] = d
	w.next = (w.next + 1) % len(w.buf)
	if w.next == 0 {
		w.full = true
	}
}

// Quantile returns the q-quantile (0..1) using the nearest-rank method, or 0
// when the window is empty.
func (w *Window) Quantile(q float64) time.Duration {
	w.mu.Lock()
	n := w.next
	if w.full {
		n = len(w.buf)
	}
	sorted := make([]time.Duration, n)
	copy(sorted, w.buf[:n])
	w.mu.Unlock()

	if n == 0 {
		return 0
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int(q*float64(n)+0.999999) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= n {
		rank = n - 1
	}
	return sorted[rank]
}

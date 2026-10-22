package bbolt

import (
	"encoding/json"
	"fmt"

	"github.com/OubeidHanini/plantorq/internal/app/collector"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// ReadingBuffer adapts the byte-oriented Queue to collector.Buffer. The queue
// stays generic and reusable; serialisation is this adapter's only concern.
type ReadingBuffer struct{ q *Queue }

func NewReadingBuffer(q *Queue) *ReadingBuffer { return &ReadingBuffer{q: q} }

var _ collector.Buffer = (*ReadingBuffer)(nil)

func (b *ReadingBuffer) Push(r telemetry.Reading) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return b.q.Push(data)
}

func (b *ReadingBuffer) Peek(n int) ([]collector.Buffered, error) {
	items, err := b.q.Peek(n)
	if err != nil {
		return nil, err
	}
	out := make([]collector.Buffered, 0, len(items))
	for _, it := range items {
		var r telemetry.Reading
		if err := json.Unmarshal(it.Value, &r); err != nil {
			return nil, fmt.Errorf("bbolt: corrupt item %d: %w", it.Key, err)
		}
		out = append(out, collector.Buffered{ID: it.Key, Reading: r})
	}
	return out, nil
}

func (b *ReadingBuffer) Ack(items []collector.Buffered) error {
	keys := make([]Item, len(items))
	for i, it := range items {
		keys[i] = Item{Key: it.ID}
	}
	return b.q.Ack(keys)
}

func (b *ReadingBuffer) Len() int { return b.q.Len() }

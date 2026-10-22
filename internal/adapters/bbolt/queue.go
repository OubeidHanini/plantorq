// Package buffer implements a persistent, bounded FIFO queue backed by bbolt.
//
// The edge agent stores messages here while the broker is unreachable so that
// a network outage or a restart does not lose measurements. When the queue is
// full the oldest entries are dropped: for monitoring data, recent values are
// worth more than old ones.
package bbolt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucket = []byte("queue")

// Item is a queued message together with the key needed to acknowledge it.
type Item struct {
	Key   uint64
	Value []byte
}

type Queue struct {
	mu      sync.Mutex
	db      *bolt.DB
	max     int
	count   int
	dropped uint64
}

// Open opens (or creates) the queue file at path, holding at most max items.
func Open(path string, max int) (*Queue, error) {
	if max <= 0 {
		return nil, errors.New("buffer: max must be positive")
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("buffer: open %s: %w", path, err)
	}
	q := &Queue{db: db, max: max}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		q.count = b.Stats().KeyN
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return q, nil
}

// Push appends value, evicting the oldest items if the queue is full.
func (q *Queue) Push(value []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		if err := b.Put(encodeKey(seq), value); err != nil {
			return err
		}
		q.count++
		c := b.Cursor()
		for k, _ := c.First(); k != nil && q.count > q.max; k, _ = c.Next() {
			if err := c.Delete(); err != nil {
				return err
			}
			q.count--
			q.dropped++
		}
		return nil
	})
}

// Peek returns up to n of the oldest items without removing them.
func (q *Queue) Peek(n int) ([]Item, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := make([]Item, 0, n)
	err := q.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucket).Cursor()
		for k, v := c.First(); k != nil && len(items) < n; k, v = c.Next() {
			items = append(items, Item{Key: binary.BigEndian.Uint64(k), Value: append([]byte(nil), v...)})
		}
		return nil
	})
	return items, err
}

// Ack removes the given items once they have been delivered.
func (q *Queue) Ack(items []Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		for _, it := range items {
			k := encodeKey(it.Key)
			if b.Get(k) == nil {
				continue // already evicted
			}
			if err := b.Delete(k); err != nil {
				return err
			}
			q.count--
		}
		return nil
	})
}

// Len returns the number of queued items.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.count
}

// Dropped returns how many items were evicted because the queue was full.
func (q *Queue) Dropped() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dropped
}

func (q *Queue) Close() error { return q.db.Close() }

func encodeKey(seq uint64) []byte {
	k := make([]byte, 8)
	binary.BigEndian.PutUint64(k, seq)
	return k
}

package bbolt

import (
	"fmt"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T, max int) (*Queue, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q.db")
	q, err := Open(path, max)
	if err != nil {
		t.Fatal(err)
	}
	return q, path
}

func TestFIFOOrderAndAck(t *testing.T) {
	q, _ := openTemp(t, 10)
	defer q.Close()
	for i := 0; i < 3; i++ {
		if err := q.Push([]byte(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	items, err := q.Peek(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || string(items[0].Value) != "0" || string(items[1].Value) != "1" {
		t.Fatalf("unexpected peek: %+v", items)
	}
	if err := q.Ack(items); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 1 {
		t.Fatalf("len = %d, want 1", q.Len())
	}
	rest, _ := q.Peek(10)
	if string(rest[0].Value) != "2" {
		t.Fatalf("remaining = %q, want 2", rest[0].Value)
	}
}

func TestEvictsOldestWhenFull(t *testing.T) {
	q, _ := openTemp(t, 3)
	defer q.Close()
	for i := 0; i < 5; i++ {
		_ = q.Push([]byte(fmt.Sprint(i)))
	}
	if q.Len() != 3 || q.Dropped() != 2 {
		t.Fatalf("len=%d dropped=%d, want 3 and 2", q.Len(), q.Dropped())
	}
	items, _ := q.Peek(3)
	if string(items[0].Value) != "2" {
		t.Fatalf("oldest kept = %q, want 2", items[0].Value)
	}
}

func TestSurvivesRestart(t *testing.T) {
	q, path := openTemp(t, 10)
	_ = q.Push([]byte("a"))
	_ = q.Push([]byte("b"))
	q.Close()

	q2, err := Open(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Close()
	if q2.Len() != 2 {
		t.Fatalf("len after reopen = %d, want 2", q2.Len())
	}
	_ = q2.Push([]byte("c"))
	items, _ := q2.Peek(3)
	if string(items[2].Value) != "c" {
		t.Fatalf("order broken after reopen: %+v", items)
	}
}

func TestAckOfEvictedItemIsHarmless(t *testing.T) {
	q, _ := openTemp(t, 1)
	defer q.Close()
	_ = q.Push([]byte("old"))
	items, _ := q.Peek(1)
	_ = q.Push([]byte("new")) // evicts "old"
	if err := q.Ack(items); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 1 {
		t.Fatalf("len = %d, want 1", q.Len())
	}
}

package opcua

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/gopcua/opcua/ua"
)

type fakeSession struct {
	results []*ua.DataValue
	err     error
	reads   int
	closed  bool
}

func (f *fakeSession) Read(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
	f.reads++
	if f.err != nil {
		return nil, f.err
	}
	return &ua.ReadResponse{Results: f.results}, nil
}

func (f *fakeSession) Close(context.Context) error { f.closed = true; return nil }

func value(v any) *ua.DataValue {
	return &ua.DataValue{Value: ua.MustVariant(v), Status: ua.StatusOK}
}

var nodes = []Node{
	{Name: "temperature", NodeID: "ns=2;s=robot-07.temperature"},
	{Name: "power_kw", NodeID: "ns=2;s=robot-07.power_kw", Scale: 0.001}, // device reports W
	{Name: "running", NodeID: "ns=2;s=robot-07.running"},
}

func TestReadDecodesAllNodeTypes(t *testing.T) {
	sess := &fakeSession{results: []*ua.DataValue{value(41.5), value(int32(8400)), value(true)}}
	dials := 0
	r, err := newReader(func(context.Context) (session, error) { dials++; return sess, nil }, nodes)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := r.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got["temperature"] != 41.5 || math.Abs(got["power_kw"]-8.4) > 1e-9 || got["running"] != 1 {
			t.Fatalf("unexpected values %v", got)
		}
	}
	if dials != 1 {
		t.Fatalf("session should be reused across reads, dialled %d times", dials)
	}
}

func TestResetForcesReconnect(t *testing.T) {
	first := &fakeSession{err: errors.New("connection reset")}
	second := &fakeSession{results: []*ua.DataValue{value(1.0), value(1.0), value(false)}}
	sessions := []*fakeSession{first, second}
	r, _ := newReader(func(context.Context) (session, error) {
		s := sessions[0]
		sessions = sessions[1:]
		return s, nil
	}, nodes)

	if _, err := r.Read(context.Background()); err == nil {
		t.Fatal("expected read error from broken session")
	}
	r.Reset() // what the collector's poller does after a failure
	if !first.closed {
		t.Fatal("broken session was not closed")
	}
	if _, err := r.Read(context.Background()); err != nil {
		t.Fatalf("read after reconnect failed: %v", err)
	}
}

func TestBadStatusFailsWholeReading(t *testing.T) {
	bad := &ua.DataValue{Status: ua.StatusBadNodeIDUnknown}
	sess := &fakeSession{results: []*ua.DataValue{value(1.0), bad, value(true)}}
	r, _ := newReader(func(context.Context) (session, error) { return sess, nil }, nodes)
	_, err := r.Read(context.Background())
	if !errors.Is(err, ua.StatusBadNodeIDUnknown) {
		t.Fatalf("err = %v, want StatusBadNodeIDUnknown", err)
	}
}

func TestDialFailureIsReported(t *testing.T) {
	r, _ := newReader(func(context.Context) (session, error) { return nil, errors.New("refused") }, nodes)
	if _, err := r.Read(context.Background()); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestInvalidConfigFailsFast(t *testing.T) {
	if _, err := newReader(nil, nil); err == nil {
		t.Fatal("expected error for empty node list")
	}
	for _, bad := range []string{"temperature", "not a node id", "ns=x;s=a", "ns=2;temperature"} {
		if _, err := newReader(nil, []Node{{Name: "x", NodeID: bad}}); err == nil {
			t.Errorf("node id %q accepted, want error", bad)
		}
	}
	for _, good := range []string{"ns=2;s=robot-07.temperature", "i=2258", "ns=3;i=1001"} {
		if _, err := newReader(nil, []Node{{Name: "x", NodeID: good}}); err != nil {
			t.Errorf("node id %q rejected: %v", good, err)
		}
	}
}

func TestToFloatRejectsStrings(t *testing.T) {
	if _, err := toFloat("42"); err == nil {
		t.Fatal("string values must be rejected, not silently parsed")
	}
}

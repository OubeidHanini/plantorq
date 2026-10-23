package opcua

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestAgainstRealServer reads from a live OPC UA server, typically the
// Python simulator. It runs only when PLANTORQ_OPCUA_ENDPOINT is set:
//
//	PLANTORQ_OPCUA_ENDPOINT=opc.tcp://localhost:4840 go test ./internal/adapters/opcua/
func TestAgainstRealServer(t *testing.T) {
	endpoint := os.Getenv("PLANTORQ_OPCUA_ENDPOINT")
	if endpoint == "" {
		t.Skip("PLANTORQ_OPCUA_ENDPOINT not set")
	}
	nodes := []Node{
		{Name: "temperature", NodeID: "ns=2;s=robot-07.temperature"},
		{Name: "power_kw", NodeID: "ns=2;s=robot-07.power_kw"},
		{Name: "running", NodeID: "ns=2;s=robot-07.running"},
	}
	r, err := NewReader(endpoint, Security{}, 5*time.Second, nodes)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Reset()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	values, err := r.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read from %s: %v", endpoint, values)
	if values["temperature"] < 30 || values["temperature"] > 60 || values["running"] != 1 {
		t.Fatalf("implausible values %v", values)
	}
}

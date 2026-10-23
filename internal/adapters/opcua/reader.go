// Package opcua reads machine values from OPC UA servers. It is the second
// collector.DeviceReader strategy next to Modbus TCP: adding it required no
// change to the collector, only a new factory entry in cmd/edge-agent.
package opcua

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"

	"github.com/OubeidHanini/plantorq/internal/app/collector"
)

// Node maps one OPC UA variable to a named measurement:
// value = raw*Scale + Offset (Scale 0 means 1).
type Node struct {
	Name   string  `yaml:"name"`
	NodeID string  `yaml:"node_id"`
	Scale  float64 `yaml:"scale"`
	Offset float64 `yaml:"offset"`
	Unit   string  `yaml:"unit"`
}

// Security selects the OPC UA security policy and message mode. The demo
// uses None; production plants use Basic256Sha256 with SignAndEncrypt and
// client certificates.
type Security struct {
	Policy string `yaml:"policy"` // e.g. "None", "Basic256Sha256"
	Mode   string `yaml:"mode"`   // e.g. "None", "Sign", "SignAndEncrypt"
}

// session is the part of *opcua.Client the reader uses. Keeping it as an
// interface lets tests substitute a fake server.
type session interface {
	Read(ctx context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error)
	Close(ctx context.Context) error
}

type dialer func(ctx context.Context) (session, error)

// nodeIDSyntax requires an explicit identifier type. gopcua alone accepts a
// bare "temperature" as a string id in namespace 0 (reserved for standard
// nodes), which would turn a config typo into silent read failures.
var nodeIDSyntax = regexp.MustCompile(`^(ns=\d+;)?[isgb]=.+$`)

// Reader reads all configured nodes of one machine in a single Read service
// call, reconnecting lazily after Reset.
type Reader struct {
	mu      sync.Mutex
	dial    dialer
	sess    session
	nodes   []Node
	request *ua.ReadRequest
}

var _ collector.DeviceReader = (*Reader)(nil)

// NewReader validates the node ids up front, so a typo in the configuration
// fails at startup instead of on every poll.
func NewReader(endpoint string, sec Security, timeout time.Duration, nodes []Node) (*Reader, error) {
	dial := func(ctx context.Context) (session, error) {
		c, err := opcua.NewClient(endpoint,
			opcua.SecurityPolicy(orDefault(sec.Policy, "None")),
			opcua.SecurityModeString(orDefault(sec.Mode, "None")),
			opcua.AuthAnonymous(),
			opcua.DialTimeout(timeout),
			opcua.RequestTimeout(timeout),
		)
		if err != nil {
			return nil, err
		}
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
		return c, nil
	}
	return newReader(dial, nodes)
}

func newReader(dial dialer, nodes []Node) (*Reader, error) {
	if len(nodes) == 0 {
		return nil, errors.New("opcua: no nodes configured")
	}
	req := &ua.ReadRequest{TimestampsToReturn: ua.TimestampsToReturnSource}
	for _, n := range nodes {
		if !nodeIDSyntax.MatchString(n.NodeID) {
			return nil, fmt.Errorf("opcua: node %q: %q must use the explicit form [ns=<n>;]<i|s|g|b>=<id>", n.Name, n.NodeID)
		}
		id, err := ua.ParseNodeID(n.NodeID)
		if err != nil {
			return nil, fmt.Errorf("opcua: node %q: %w", n.Name, err)
		}
		req.NodesToRead = append(req.NodesToRead, &ua.ReadValueID{NodeID: id, AttributeID: ua.AttributeIDValue})
	}
	return &Reader{dial: dial, nodes: nodes, request: req}, nil
}

func (r *Reader) Read(ctx context.Context) (map[string]float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess == nil {
		sess, err := r.dial(ctx)
		if err != nil {
			return nil, fmt.Errorf("opcua connect: %w", err)
		}
		r.sess = sess
	}
	resp, err := r.sess.Read(ctx, r.request)
	if err != nil {
		return nil, fmt.Errorf("opcua read: %w", err)
	}
	return decode(r.nodes, resp.Results)
}

// Reset drops the session; the next Read reconnects.
func (r *Reader) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.sess.Close(ctx)
		r.sess = nil
	}
}

// decode converts read results, in request order, into engineering values.
// A single bad node fails the whole reading: a partial snapshot would make
// derived values (and alert rules) inconsistent.
func decode(nodes []Node, results []*ua.DataValue) (map[string]float64, error) {
	if len(results) != len(nodes) {
		return nil, fmt.Errorf("opcua: expected %d results, got %d", len(nodes), len(results))
	}
	out := make(map[string]float64, len(nodes))
	for i, n := range nodes {
		dv := results[i]
		if dv == nil || dv.Status != ua.StatusOK {
			status := ua.StatusBad
			if dv != nil {
				status = dv.Status
			}
			return nil, fmt.Errorf("opcua: node %q: %w", n.Name, status)
		}
		v, err := toFloat(dv.Value.Value())
		if err != nil {
			return nil, fmt.Errorf("opcua: node %q: %w", n.Name, err)
		}
		scale := n.Scale
		if scale == 0 {
			scale = 1
		}
		out[n.Name] = v*scale + n.Offset
	}
	return out, nil
}

// toFloat accepts every numeric OPC UA built-in type and booleans.
func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int8:
		return float64(x), nil
	case int16:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint8:
		return float64(x), nil
	case uint16:
		return float64(x), nil
	case uint32:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported value type %T", v)
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

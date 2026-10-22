package influx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

func TestPointLine(t *testing.T) {
	p := Point{
		Measurement: "telemetry",
		Tags:        map[string]string{"tenant": "acme", "machine": "cnc 01"},
		Fields:      map[string]float64{"temperature": 73.5, "power_kw": 22},
		Time:        time.UnixMilli(1700000000123),
	}
	want := `telemetry,machine=cnc\ 01,tenant=acme power_kw=22,temperature=73.5 1700000000123`
	if got := p.Line(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestQuoteStringPreventsInjection(t *testing.T) {
	got := quoteString(`acme' OR 1=1 --`)
	if got != `'acme\' OR 1=1 --'` {
		t.Fatalf("unexpected quoting: %s", got)
	}
}

func TestHistoryRejectsBadMetric(t *testing.T) {
	c := New("http://127.0.0.1:1", "db")
	if _, err := c.History(context.Background(), "acme", "cnc-01", `x") FROM y; DROP`, time.Hour, time.Minute); err == nil {
		t.Fatal("expected invalid metric error")
	}
}

func TestHistoryParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, `"machine"='cnc-01'`) || !strings.Contains(q, `mean("temperature")`) {
			t.Errorf("unexpected query: %s", q)
		}
		_, _ = io.WriteString(w, `{"results":[{"series":[{"name":"telemetry","columns":["time","mean"],"values":[[1000,50.5],[2000,null],[3000,52]]}]}]}`)
	}))
	defer srv.Close()

	samples, err := New(srv.URL, "db").History(context.Background(), "acme", "cnc-01", "temperature", time.Hour, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0] != (telemetry.Sample{Time: 1000, Value: 50.5}) || samples[1] != (telemetry.Sample{Time: 3000, Value: 52}) {
		t.Fatalf("unexpected samples: %+v", samples)
	}
}

func TestToPoint(t *testing.T) {
	p := ToPoint(telemetry.Reading{Tenant: "acme", Machine: "cnc-01", Source: "edge", Timestamp: 5, Values: map[string]float64{"x": 1}})
	if got := p.Line(); got != "telemetry,machine=cnc-01,source=edge,tenant=acme x=1 5" {
		t.Fatalf("unexpected line %q", got)
	}
}

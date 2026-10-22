// Package prom implements the application Metrics ports with Prometheus.
// Use cases emit domain-level signals ("a poll completed"); only this package
// knows about counters, histograms and label names.
package prom

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	appalerting "github.com/OubeidHanini/plantorq/internal/app/alerting"
	"github.com/OubeidHanini/plantorq/internal/app/collector"
	"github.com/OubeidHanini/plantorq/internal/app/ingest"
	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Collector implements collector.Metrics.
type Collector struct {
	polls     *prometheus.CounterVec
	pollTime  *prometheus.HistogramVec
	published *prometheus.CounterVec
	buffered  prometheus.Counter
	depth     prometheus.Gauge
	online    prometheus.Gauge
	total     prometheus.Gauge
	connected prometheus.Gauge
}

var _ collector.Metrics = (*Collector)(nil)

func NewCollector(reg prometheus.Registerer) *Collector {
	f := promauto.With(reg)
	return &Collector{
		polls: f.NewCounterVec(prometheus.CounterOpts{
			Name: "plantorq_agent_polls_total", Help: "Device polls by machine and result.",
		}, []string{"machine", "result"}),
		pollTime: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "plantorq_agent_poll_duration_seconds", Help: "Device poll latency.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 3},
		}, []string{"machine"}),
		published: f.NewCounterVec(prometheus.CounterOpts{
			Name: "plantorq_agent_published_total", Help: "Readings delivered, by path.",
		}, []string{"path"}),
		buffered: f.NewCounter(prometheus.CounterOpts{
			Name: "plantorq_agent_buffered_total", Help: "Readings written to the offline buffer.",
		}),
		depth: f.NewGauge(prometheus.GaugeOpts{
			Name: "plantorq_agent_buffer_depth", Help: "Readings currently waiting in the offline buffer.",
		}),
		online: f.NewGauge(prometheus.GaugeOpts{
			Name: "plantorq_agent_machines_online", Help: "Machines that answered recently.",
		}),
		total: f.NewGauge(prometheus.GaugeOpts{
			Name: "plantorq_agent_machines_total", Help: "Machines configured on this agent.",
		}),
		connected: f.NewGauge(prometheus.GaugeOpts{
			Name: "plantorq_agent_broker_connected", Help: "1 when the MQTT session is up.",
		}),
	}
}

func (m *Collector) PollCompleted(machine string, seconds float64, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	m.polls.WithLabelValues(machine, result).Inc()
	if err == nil {
		m.pollTime.WithLabelValues(machine).Observe(seconds)
	}
}

func (m *Collector) Published(fromBuffer bool) {
	path := "direct"
	if fromBuffer {
		path = "replay"
	}
	m.published.WithLabelValues(path).Inc()
}

func (m *Collector) Buffered() { m.buffered.Inc() }

func (m *Collector) StatusReported(s telemetry.GatewayStatus) {
	m.depth.Set(float64(s.Buffered))
	m.online.Set(float64(s.MachinesOnline))
	m.total.Set(float64(s.MachinesTotal))
	connected := 0.0
	if s.Connected {
		connected = 1
	}
	m.connected.Set(connected)
}

// Ingest implements ingest.Metrics.
type Ingest struct {
	written prometheus.Counter
	dropped *prometheus.CounterVec
	flush   prometheus.Histogram
}

var _ ingest.Metrics = (*Ingest)(nil)

func NewIngest(reg prometheus.Registerer) *Ingest {
	f := promauto.With(reg)
	return &Ingest{
		written: f.NewCounter(prometheus.CounterOpts{
			Name: "plantorq_ingestor_readings_written_total", Help: "Readings persisted.",
		}),
		dropped: f.NewCounterVec(prometheus.CounterOpts{
			Name: "plantorq_ingestor_readings_dropped_total", Help: "Readings dropped, by reason.",
		}, []string{"reason"}),
		flush: f.NewHistogram(prometheus.HistogramOpts{
			Name: "plantorq_ingestor_flush_duration_seconds", Help: "Batch write latency.",
		}),
	}
}

func (m *Ingest) Written(n int)                 { m.written.Add(float64(n)) }
func (m *Ingest) Dropped(reason string, n int)  { m.dropped.WithLabelValues(reason).Add(float64(n)) }
func (m *Ingest) FlushObserved(seconds float64) { m.flush.Observe(seconds) }

// Rules implements alerting.Metrics for the rules engine.
type Rules struct {
	evaluated   prometheus.Counter
	transitions *prometheus.CounterVec
}

var _ appalerting.Metrics = (*Rules)(nil)

func NewRules(reg prometheus.Registerer) *Rules {
	f := promauto.With(reg)
	return &Rules{
		evaluated: f.NewCounter(prometheus.CounterOpts{
			Name: "plantorq_rules_readings_evaluated_total", Help: "Readings evaluated against alert rules.",
		}),
		transitions: f.NewCounterVec(prometheus.CounterOpts{
			Name: "plantorq_rules_alert_transitions_total", Help: "Alerts fired or resolved, by severity.",
		}, []string{"kind", "severity"}),
	}
}

func (m *Rules) ReadingEvaluated() { m.evaluated.Inc() }

func (m *Rules) AlertChanged(kind alerting.EventKind, severity alerting.Severity) {
	m.transitions.WithLabelValues(string(kind), string(severity)).Inc()
}

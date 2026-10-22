// Command edge-agent runs at the plant: it polls industrial devices, forwards
// readings to the platform over MQTT and buffers them on disk while offline.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/OubeidHanini/plantorq/internal/adapters/bbolt"
	"github.com/OubeidHanini/plantorq/internal/adapters/mqtt"
	"github.com/OubeidHanini/plantorq/internal/adapters/prom"
	"github.com/OubeidHanini/plantorq/internal/app/collector"
	"github.com/OubeidHanini/plantorq/internal/pkg/config"
	"github.com/OubeidHanini/plantorq/internal/pkg/obs"
)

func main() {
	log := obs.NewLogger("edge-agent")
	configPath := flag.String("config", config.String("AGENT_CONFIG", "/etc/plantorq/agent.yaml"), "path to agent config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	machines, err := buildMachines(cfg)
	if err != nil {
		log.Error("cannot build device readers", "err", err)
		os.Exit(1)
	}

	queue, err := bbolt.Open(cfg.Buffer.Path, cfg.Buffer.MaxItems)
	if err != nil {
		log.Error("cannot open offline buffer", "err", err)
		os.Exit(1)
	}
	defer queue.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := mqtt.Dial(log, mqtt.Options{
		Broker: cfg.MQTT.Broker, ClientID: "edge-" + cfg.Gateway,
		Username: cfg.MQTT.Username, Password: cfg.MQTT.Password,
	})
	defer client.Close()

	agent := collector.NewAgent(collector.Config{
		Tenant: cfg.Tenant, Gateway: cfg.Gateway,
		PollInterval: cfg.Polling.Interval, MaxBackoff: cfg.Polling.MaxBackoff,
		StatusInterval: 5 * time.Second, DrainInterval: time.Second, DrainBatch: 200,
	}, machines, mqtt.TelemetryPublisher{Client: client}, bbolt.NewReadingBuffer(queue),
		prom.NewCollector(prometheus.DefaultRegisterer), log)

	// The agent is "ready" as soon as it polls: buffering covers broker outages.
	ready := &obs.Readiness{}
	ready.Set(true)
	go func() {
		if err := obs.Serve(ctx, log, config.String("OPS_ADDR", ":9100"), obs.OpsMux(ready)); err != nil {
			log.Error("ops server failed", "err", err)
		}
	}()

	log.Info("edge agent started", "tenant", cfg.Tenant, "gateway", cfg.Gateway, "machines", len(machines))
	agent.Run(ctx)
	log.Info("edge agent stopped", "buffered", queue.Len())
}

// Command ingestor consumes telemetry from MQTT and writes it to InfluxDB in
// batches. It is stateless and scales horizontally behind a shared
// subscription.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/OubeidHanini/plantorq/internal/adapters/influx"
	"github.com/OubeidHanini/plantorq/internal/adapters/mqtt"
	"github.com/OubeidHanini/plantorq/internal/adapters/prom"
	"github.com/OubeidHanini/plantorq/internal/app/ingest"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
	"github.com/OubeidHanini/plantorq/internal/pkg/config"
	"github.com/OubeidHanini/plantorq/internal/pkg/obs"
)

func main() {
	log := obs.NewLogger("ingestor")
	if err := run(log); err != nil {
		log.Error("ingestor failed", "err", err)
		os.Exit(1)
	}
}

// run owns the service lifetime, so deferred cleanups complete before main
// chooses the exit code.
func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ready := obs.StartOps(ctx, log, config.String("OPS_ADDR", ":9101"))

	store := influx.New(config.String("INFLUX_URL", "http://localhost:8086"), config.String("INFLUX_DB", "plantorq"))
	if err := waitFor(ctx, func(ctx context.Context) error { return store.EnsureDatabase(ctx) }); err != nil {
		return fmt.Errorf("influxdb unavailable: %w", err)
	}

	svc := ingest.NewService(ingest.Config{
		BatchSize:     config.Int("BATCH_SIZE", 500),
		FlushInterval: config.Duration("FLUSH_INTERVAL", time.Second),
		QueueSize:     config.Int("QUEUE_SIZE", 20_000),
		MaxRetries:    3,
		RetryBackoff:  500 * time.Millisecond,
	}, store, prom.NewIngest(prometheus.DefaultRegisterer), log)

	onReading := mqtt.JSONHandler(log, func(topic string, r telemetry.Reading) {
		tenant, machine, ok := mqtt.ParseReadingTopic(topic)
		if !ok {
			return
		}
		r.Tenant, r.Machine = tenant, machine // the topic is authoritative
		svc.Accept(r)
	})
	// $share/ingestors/... is an MQTT shared subscription (supported by the
	// Mosquitto broker): with several replicas each message reaches only one.
	client := mqtt.Dial(log, mqtt.Options{
		Broker:   config.String("MQTT_BROKER", "tcp://localhost:1883"),
		ClientID: "ingestor-" + config.String("HOSTNAME", "local"),
		Username: config.String("MQTT_USERNAME", ""), Password: config.String("MQTT_PASSWORD", ""),
		Handlers: map[string]paho.MessageHandler{
			"$share/ingestors/" + mqtt.SubAllTelemetry: onReading,
			"$share/ingestors/" + mqtt.SubAllAnalytics: onReading,
		},
	})
	defer client.Close()

	// Ready once every dependency is up: databases above, the broker here.
	go func() {
		if err := client.WaitConnected(ctx); err == nil {
			ready.Set(true)
		}
	}()

	log.Info("ingestor started")
	svc.Run(ctx)
	log.Info("ingestor stopped")
	return nil
}

// waitFor retries fn every 2s until it succeeds or ctx is cancelled.
func waitFor(ctx context.Context, fn func(context.Context) error) error {
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(2 * time.Second):
		}
	}
}

// Command ingestor consumes telemetry from MQTT and writes it to InfluxDB in
// batches. It is stateless and scales horizontally behind a shared
// subscription.
package main

import (
	"context"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := influx.New(config.String("INFLUX_URL", "http://localhost:8086"), config.String("INFLUX_DB", "plantorq"))
	if err := waitFor(ctx, func(ctx context.Context) error { return store.EnsureDatabase(ctx) }); err != nil {
		log.Error("influxdb unavailable", "err", err)
		os.Exit(1)
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
	// $share/ingestors/... is an MQTT 5 / mosquitto shared subscription:
	// with several replicas each message is delivered to only one of them.
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

	ready := &obs.Readiness{}
	go func() {
		if err := client.WaitConnected(ctx); err == nil {
			ready.Set(true)
		}
	}()
	go func() {
		if err := obs.Serve(ctx, log, config.String("OPS_ADDR", ":9101"), obs.OpsMux(ready)); err != nil {
			log.Error("ops server failed", "err", err)
		}
	}()

	log.Info("ingestor started")
	svc.Run(ctx)
	log.Info("ingestor stopped")
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

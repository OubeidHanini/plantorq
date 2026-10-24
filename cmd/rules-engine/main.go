// Command rules-engine evaluates alert rules against live telemetry, records
// alerts in PostgreSQL and announces them on MQTT.
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

	"github.com/OubeidHanini/plantorq/internal/adapters/mqtt"
	"github.com/OubeidHanini/plantorq/internal/adapters/postgres"
	"github.com/OubeidHanini/plantorq/internal/adapters/prom"
	"github.com/OubeidHanini/plantorq/internal/app/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
	"github.com/OubeidHanini/plantorq/internal/pkg/config"
	"github.com/OubeidHanini/plantorq/internal/pkg/obs"
)

func main() {
	log := obs.NewLogger("rules-engine")
	if err := run(log); err != nil {
		log.Error("rules engine failed", "err", err)
		os.Exit(1)
	}
}

// run owns the service lifetime, so deferred cleanups (database pool, broker
// session) complete before main chooses the exit code.
func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ready := obs.StartOps(ctx, log, config.String("OPS_ADDR", ":9102"))

	db, err := postgres.Open(ctx, config.String("DATABASE_URL", "postgres://plantorq:plantorq@localhost:5432/plantorq"), log)
	if err != nil {
		return fmt.Errorf("database unavailable: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// The service needs the notifier, and the MQTT handlers need the service:
	// a late-bound variable breaks the cycle without globals.
	var svc *alerting.Service
	onReading := mqtt.JSONHandler(log, func(topic string, r telemetry.Reading) {
		tenant, machine, ok := mqtt.ParseReadingTopic(topic)
		if !ok {
			return
		}
		r.Tenant, r.Machine = tenant, machine
		svc.Accept(r)
	})
	client := mqtt.Dial(log, mqtt.Options{
		Broker:   config.String("MQTT_BROKER", "tcp://localhost:1883"),
		ClientID: "rules-engine-" + config.String("HOSTNAME", "local"),
		Username: config.String("MQTT_USERNAME", ""), Password: config.String("MQTT_PASSWORD", ""),
		Handlers: map[string]paho.MessageHandler{
			mqtt.SubAllTelemetry: onReading,
			mqtt.SubAllAnalytics: onReading,
		},
	})
	defer client.Close()
	svc = alerting.NewService(db, db, mqtt.AlertNotifier{Client: client}, prom.NewRules(prometheus.DefaultRegisterer), log)

	// Ready once every dependency is up: databases above, the broker here.
	go func() {
		if err := client.WaitConnected(ctx); err == nil {
			ready.Set(true)
		}
	}()

	log.Info("rules engine started")
	if err := svc.Run(ctx, config.Duration("RULE_RELOAD_INTERVAL", 15*time.Second)); err != nil {
		return fmt.Errorf("rules engine: %w", err)
	}
	log.Info("rules engine stopped")
	return nil
}

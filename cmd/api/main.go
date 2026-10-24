// Command api serves the Plantorq REST + WebSocket API for the web
// dashboard.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/OubeidHanini/plantorq/internal/adapters/influx"
	"github.com/OubeidHanini/plantorq/internal/adapters/jwt"
	"github.com/OubeidHanini/plantorq/internal/adapters/mqtt"
	"github.com/OubeidHanini/plantorq/internal/adapters/password"
	"github.com/OubeidHanini/plantorq/internal/adapters/postgres"
	"github.com/OubeidHanini/plantorq/internal/app/platform"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
	"github.com/OubeidHanini/plantorq/internal/pkg/config"
	"github.com/OubeidHanini/plantorq/internal/pkg/obs"
	transport "github.com/OubeidHanini/plantorq/internal/transport/http"
)

func main() {
	log := obs.NewLogger("api")
	if err := run(log); err != nil {
		log.Error("api failed", "err", err)
		os.Exit(1)
	}
}

// run owns the service lifetime, so deferred cleanups (database pool, broker
// session) complete before main chooses the exit code.
func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ready := obs.StartOps(ctx, log, config.String("OPS_ADDR", ":9103"))

	// --- infrastructure ----------------------------------------------------
	db, err := postgres.Open(ctx, config.String("DATABASE_URL", "postgres://plantorq:plantorq@localhost:5432/plantorq"), log)
	if err != nil {
		return fmt.Errorf("database unavailable: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	hasher := password.Bcrypt{}
	if demoPassword := config.String("SEED_DEMO_PASSWORD", ""); demoPassword != "" {
		hash, err := hasher.Hash(demoPassword)
		if err == nil {
			err = db.SeedDemo(ctx, hash)
		}
		if err != nil {
			return fmt.Errorf("seeding failed: %w", err)
		}
	}

	tokens, err := jwt.New(config.String("JWT_SECRET", ""), config.Duration("JWT_TTL", 12*time.Hour))
	if err != nil {
		return fmt.Errorf("invalid JWT configuration: %w", err)
	}
	history := influx.New(config.String("INFLUX_URL", "http://localhost:8086"), config.String("INFLUX_DB", "plantorq"))

	// --- application -------------------------------------------------------
	live := platform.NewLiveView()
	hub := transport.NewHub(log)

	// Bus events update the live projection and are relayed to browsers.
	handlers := map[string]paho.MessageHandler{
		mqtt.SubAllTelemetry: mqtt.JSONHandler(log, func(topic string, r telemetry.Reading) {
			if tenant, machine, ok := mqtt.ParseReadingTopic(topic); ok {
				r.Tenant, r.Machine = tenant, machine
				live.ApplyReading(r)
				hub.Broadcast(tenant, transport.Event{Type: "reading", Data: r})
			}
		}),
		mqtt.SubAllAnalytics: mqtt.JSONHandler(log, func(topic string, r telemetry.Reading) {
			if tenant, machine, ok := mqtt.ParseReadingTopic(topic); ok {
				r.Tenant, r.Machine = tenant, machine
				live.ApplyReading(r)
				hub.Broadcast(tenant, transport.Event{Type: "reading", Data: r})
			}
		}),
		mqtt.SubAllGateways: mqtt.JSONHandler(log, func(_ string, s telemetry.GatewayStatus) {
			live.ApplyGateway(s)
			hub.Broadcast(s.Tenant, transport.Event{Type: "gateway", Data: s})
		}),
		mqtt.SubAllAlerts: mqtt.JSONHandler(log, func(_ string, a mqtt.AlertMessage) {
			hub.Broadcast(a.Tenant, transport.Event{Type: "alert", Data: a})
		}),
	}
	client := mqtt.Dial(log, mqtt.Options{
		Broker:   config.String("MQTT_BROKER", "tcp://localhost:1883"),
		ClientID: "api-" + config.String("HOSTNAME", "local"),
		Username: config.String("MQTT_USERNAME", ""), Password: config.String("MQTT_PASSWORD", ""),
		Handlers: handlers,
	})
	defer client.Close()

	server := transport.NewServer(transport.Deps{
		Auth:    platform.NewAuthService(db, hasher, tokens),
		Tokens:  tokens,
		Fleet:   platform.NewFleetService(db, db, history, live, 15*time.Second),
		Rules:   platform.NewRuleService(db, db),
		Alerts:  platform.NewAlertService(db),
		Hub:     hub,
		Log:     log,
		Origins: splitNonEmpty(config.String("CORS_ORIGINS", "")),
	})

	// --- servers -----------------------------------------------------------
	// Ready once every dependency is up: databases above, the broker here.
	go func() {
		if err := client.WaitConnected(ctx); err == nil {
			ready.Set(true)
		}
	}()
	// obs.Serve already treats a graceful shutdown as success.
	if err := obs.Serve(ctx, log, config.String("HTTP_ADDR", ":8080"), server.Routes()); err != nil {
		return fmt.Errorf("api server: %w", err)
	}
	log.Info("api stopped")
	return nil
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

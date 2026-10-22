package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("TEST_MQTT_PASSWORD", "s3cret")
	cfg, err := loadConfig(writeConfig(t, `
tenant: acme
gateway: gw-munich-01
mqtt:
  broker: tcp://mosquitto:1883
  password: ${TEST_MQTT_PASSWORD}
profiles:
  cnc:
    - { name: temperature, address: 0, scale: 0.1 }
machines:
  - { id: cnc-01, protocol: modbus-tcp, address: "sim:5020", unit_id: 1, profile: cnc }
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MQTT.Password != "s3cret" {
		t.Errorf("env var not expanded: %q", cfg.MQTT.Password)
	}
	if cfg.Polling.Interval != time.Second || cfg.Buffer.MaxItems != 100_000 {
		t.Errorf("defaults not applied: %+v", cfg.Polling)
	}
	machines, err := buildMachines(cfg)
	if err != nil || len(machines) != 1 {
		t.Fatalf("buildMachines: %v", err)
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	_, err := loadConfig(writeConfig(t, `
tenant: "Bad Tenant"
gateway: gw
machines:
  - { id: cnc-01, profile: missing }
  - { id: cnc-01, profile: missing }
`))
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"tenant", "mqtt.broker", "unknown profile", "declared twice"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestUnsupportedProtocol(t *testing.T) {
	cfg := &fileConfig{Machines: []machineConfig{{ID: "x", Protocol: "profinet"}}}
	if _, err := buildMachines(cfg); err == nil || !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("err = %v, want unsupported protocol", err)
	}
}

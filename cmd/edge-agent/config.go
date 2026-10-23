package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/OubeidHanini/plantorq/internal/adapters/modbus"
	"github.com/OubeidHanini/plantorq/internal/adapters/opcua"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// fileConfig is the YAML layout of the agent configuration (deploy/config).
type fileConfig struct {
	Tenant  string `yaml:"tenant"`
	Gateway string `yaml:"gateway"`
	MQTT    struct {
		Broker   string `yaml:"broker"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"mqtt"`
	Polling struct {
		Interval   time.Duration `yaml:"interval"`
		Timeout    time.Duration `yaml:"timeout"`
		MaxBackoff time.Duration `yaml:"max_backoff"`
	} `yaml:"polling"`
	Buffer struct {
		Path     string `yaml:"path"`
		MaxItems int    `yaml:"max_items"`
	} `yaml:"buffer"`
	Profiles profiles        `yaml:"profiles"`
	Machines []machineConfig `yaml:"machines"`
}

// profiles describe how a device type exposes its values, per protocol:
// register maps for Modbus, node ids for OPC UA.
type profiles struct {
	Modbus map[string][]modbus.Register `yaml:"modbus"`
	OPCUA  map[string][]opcua.Node      `yaml:"opcua"`
}

type machineConfig struct {
	ID       string `yaml:"id"`
	Protocol string `yaml:"protocol"`
	Profile  string `yaml:"profile"`

	// Modbus TCP
	Address string `yaml:"address"`
	UnitID  byte   `yaml:"unit_id"`

	// OPC UA
	Endpoint string         `yaml:"endpoint"`
	Security opcua.Security `yaml:"security"`
}

func loadConfig(path string) (*fileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Secrets and per-environment values come from the environment, never
	// from the committed file: ${MQTT_PASSWORD} etc. are expanded here.
	raw = []byte(os.ExpandEnv(string(raw)))

	var c fileConfig
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	c.applyDefaults()
	return &c, c.validate()
}

func (c *fileConfig) applyDefaults() {
	if c.Polling.Interval == 0 {
		c.Polling.Interval = time.Second
	}
	if c.Polling.Timeout == 0 {
		c.Polling.Timeout = 2 * time.Second
	}
	if c.Polling.MaxBackoff == 0 {
		c.Polling.MaxBackoff = 30 * time.Second
	}
	if c.Buffer.Path == "" {
		c.Buffer.Path = "/data/buffer.db"
	}
	if c.Buffer.MaxItems == 0 {
		c.Buffer.MaxItems = 100_000
	}
}

// validate reports every problem at once instead of failing on the first.
func (c *fileConfig) validate() error {
	var errs []error
	if !telemetry.ValidSlug(c.Tenant) {
		errs = append(errs, fmt.Errorf("tenant %q is not a valid id", c.Tenant))
	}
	if !telemetry.ValidSlug(c.Gateway) {
		errs = append(errs, fmt.Errorf("gateway %q is not a valid id", c.Gateway))
	}
	if c.MQTT.Broker == "" {
		errs = append(errs, errors.New("mqtt.broker is required"))
	}
	if len(c.Machines) == 0 {
		errs = append(errs, errors.New("at least one machine is required"))
	}
	seen := map[string]bool{}
	for _, m := range c.Machines {
		p, known := protocols[m.Protocol]
		switch {
		case !telemetry.ValidSlug(m.ID):
			errs = append(errs, fmt.Errorf("machine id %q is invalid", m.ID))
		case seen[m.ID]:
			errs = append(errs, fmt.Errorf("machine %q is declared twice", m.ID))
		case !known:
			errs = append(errs, fmt.Errorf("machine %q: unsupported protocol %q", m.ID, m.Protocol))
		default:
			if err := p.validate(m, c); err != nil {
				errs = append(errs, fmt.Errorf("machine %q: %w", m.ID, err))
			}
		}
		seen[m.ID] = true
	}
	return errors.Join(errs...)
}

package main

import (
	"errors"
	"fmt"

	"github.com/OubeidHanini/plantorq/internal/adapters/modbus"
	"github.com/OubeidHanini/plantorq/internal/adapters/opcua"
	"github.com/OubeidHanini/plantorq/internal/app/collector"
)

// protocol bundles everything protocol-specific: how to validate a machine's
// configuration and how to build its DeviceReader. Supporting a new protocol
// means registering one entry here; the collector never changes
// (open/closed principle).
type protocol struct {
	validate func(m machineConfig, cfg *fileConfig) error
	build    func(m machineConfig, cfg *fileConfig) (collector.DeviceReader, error)
}

var protocols = map[string]protocol{
	"modbus-tcp": {
		validate: func(m machineConfig, cfg *fileConfig) error {
			if m.Address == "" {
				return errors.New("address is required")
			}
			if len(cfg.Profiles.Modbus[m.Profile]) == 0 {
				return fmt.Errorf("unknown modbus profile %q", m.Profile)
			}
			return nil
		},
		build: func(m machineConfig, cfg *fileConfig) (collector.DeviceReader, error) {
			return modbus.NewTCPReader(m.Address, m.UnitID, cfg.Polling.Timeout, cfg.Profiles.Modbus[m.Profile])
		},
	},
	"opcua": {
		validate: func(m machineConfig, cfg *fileConfig) error {
			if m.Endpoint == "" {
				return errors.New("endpoint is required")
			}
			if len(cfg.Profiles.OPCUA[m.Profile]) == 0 {
				return fmt.Errorf("unknown opcua profile %q", m.Profile)
			}
			return nil
		},
		build: func(m machineConfig, cfg *fileConfig) (collector.DeviceReader, error) {
			return opcua.NewReader(m.Endpoint, m.Security, cfg.Polling.Timeout, cfg.Profiles.OPCUA[m.Profile])
		},
	},
}

func buildMachines(cfg *fileConfig) ([]collector.Machine, error) {
	machines := make([]collector.Machine, 0, len(cfg.Machines))
	for _, m := range cfg.Machines {
		p, ok := protocols[m.Protocol]
		if !ok {
			return nil, fmt.Errorf("machine %q: unsupported protocol %q", m.ID, m.Protocol)
		}
		reader, err := p.build(m, cfg)
		if err != nil {
			return nil, fmt.Errorf("machine %q: %w", m.ID, err)
		}
		machines = append(machines, collector.Machine{ID: m.ID, Reader: reader})
	}
	return machines, nil
}

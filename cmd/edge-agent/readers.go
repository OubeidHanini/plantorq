package main

import (
	"fmt"

	"github.com/OubeidHanini/plantorq/internal/adapters/modbus"
	"github.com/OubeidHanini/plantorq/internal/app/collector"
)

// readerFactory builds a DeviceReader for a protocol. Supporting a new
// protocol (OPC UA is next) means registering one more factory here; the
// collector itself never changes (open/closed principle).
type readerFactory func(m machineConfig, cfg *fileConfig) (collector.DeviceReader, error)

var readerFactories = map[string]readerFactory{
	"modbus-tcp": func(m machineConfig, cfg *fileConfig) (collector.DeviceReader, error) {
		return modbus.NewTCPReader(m.Address, m.UnitID, cfg.Polling.Timeout, cfg.Profiles[m.Profile])
	},
}

func buildMachines(cfg *fileConfig) ([]collector.Machine, error) {
	machines := make([]collector.Machine, 0, len(cfg.Machines))
	for _, m := range cfg.Machines {
		factory, ok := readerFactories[m.Protocol]
		if !ok {
			return nil, fmt.Errorf("machine %q: unsupported protocol %q", m.ID, m.Protocol)
		}
		reader, err := factory(m, cfg)
		if err != nil {
			return nil, fmt.Errorf("machine %q: %w", m.ID, err)
		}
		machines = append(machines, collector.Machine{ID: m.ID, Reader: reader})
	}
	return machines, nil
}

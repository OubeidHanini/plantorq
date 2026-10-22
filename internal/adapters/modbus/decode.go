// Package modbus turns raw holding-register blocks into engineering values.
package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Register maps one 16-bit holding register to a named measurement:
// value = raw*Scale + Offset. Signed registers are read as two's complement.
type Register struct {
	Name    string  `yaml:"name"`
	Address uint16  `yaml:"address"`
	Scale   float64 `yaml:"scale"`
	Offset  float64 `yaml:"offset"`
	Signed  bool    `yaml:"signed"`
	Unit    string  `yaml:"unit"`
}

// Span returns the smallest contiguous block covering all registers, so a
// machine can be read in a single Modbus transaction.
func Span(regs []Register) (start, quantity uint16, err error) {
	if len(regs) == 0 {
		return 0, 0, errors.New("modbus: no registers")
	}
	lo, hi := regs[0].Address, regs[0].Address
	for _, r := range regs[1:] {
		lo = min(lo, r.Address)
		hi = max(hi, r.Address)
	}
	quantity = hi - lo + 1
	if quantity > 125 { // Modbus limit for function code 0x03
		return 0, 0, fmt.Errorf("modbus: span of %d registers exceeds 125", quantity)
	}
	return lo, quantity, nil
}

// Decode converts a raw register block starting at address start.
func Decode(raw []byte, start uint16, regs []Register) (map[string]float64, error) {
	out := make(map[string]float64, len(regs))
	for _, r := range regs {
		if r.Address < start {
			return nil, fmt.Errorf("modbus: register %q at %d is before block start %d", r.Name, r.Address, start)
		}
		i := int(r.Address-start) * 2
		if i+2 > len(raw) {
			return nil, fmt.Errorf("modbus: register %q at %d outside %d-byte block", r.Name, r.Address, len(raw))
		}
		word := binary.BigEndian.Uint16(raw[i : i+2])
		v := float64(word)
		if r.Signed {
			v = float64(int16(word))
		}
		scale := r.Scale
		if scale == 0 {
			scale = 1
		}
		out[r.Name] = v*scale + r.Offset
	}
	return out, nil
}

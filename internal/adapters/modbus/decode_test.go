package modbus

import (
	"math"
	"testing"
)

func TestSpan(t *testing.T) {
	start, qty, err := Span([]Register{{Address: 4}, {Address: 1}, {Address: 2}})
	if err != nil || start != 1 || qty != 4 {
		t.Fatalf("got start=%d qty=%d err=%v, want 1,4,nil", start, qty, err)
	}
	if _, _, err := Span(nil); err == nil {
		t.Fatal("expected error for empty register list")
	}
	if _, _, err := Span([]Register{{Address: 0}, {Address: 200}}); err == nil {
		t.Fatal("expected error for span > 125")
	}
}

func TestDecode(t *testing.T) {
	// temperature=735 (x0.1), vibration=-12 signed (x0.01), status=1
	raw := []byte{0x02, 0xDF, 0xFF, 0xF4, 0x00, 0x01}
	regs := []Register{
		{Name: "temperature", Address: 10, Scale: 0.1},
		{Name: "vibration", Address: 11, Scale: 0.01, Signed: true},
		{Name: "status", Address: 12},
	}
	got, err := Decode(raw, 10, regs)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"temperature": 73.5, "vibration": -0.12, "status": 1}
	for k, w := range want {
		if math.Abs(got[k]-w) > 1e-9 {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
}

func TestDecodeOutOfRange(t *testing.T) {
	if _, err := Decode([]byte{0, 1}, 0, []Register{{Name: "x", Address: 1}}); err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3}, uint16(0))
	f.Fuzz(func(t *testing.T, raw []byte, start uint16) {
		regs := []Register{{Name: "a", Address: start}, {Name: "b", Address: start + 1, Signed: true}}
		// Must never panic, whatever the device sends back.
		_, _ = Decode(raw, start, regs)
	})
}

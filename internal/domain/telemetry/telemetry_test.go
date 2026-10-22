package telemetry

import "testing"

func TestValidSlug(t *testing.T) {
	for s, want := range map[string]bool{
		"acme": true, "cnc-01": true, "0a": true,
		"": false, "-cnc": false, "CNC": false, "a/b": false, "a b": false, "..": false,
	} {
		if got := ValidSlug(s); got != want {
			t.Errorf("ValidSlug(%q) = %v, want %v", s, got, want)
		}
	}
}

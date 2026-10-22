package mqtt

import "testing"

func TestParseReadingTopic(t *testing.T) {
	tests := []struct {
		topic           string
		tenant, machine string
		ok              bool
	}{
		{"plantorq/acme/cnc-01/telemetry", "acme", "cnc-01", true},
		{"plantorq/acme/cnc-01/analytics", "acme", "cnc-01", true},
		{"plantorq/acme/alerts", "", "", false},
		{"plantorq/acme/gateways/gw-1", "", "", false},
		{"other/acme/cnc-01/telemetry", "", "", false},
		{"plantorq/ACME/cnc-01/telemetry", "", "", false},
		{"plantorq/acme/../telemetry", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			tenant, machine, ok := ParseReadingTopic(tt.topic)
			if ok != tt.ok || tenant != tt.tenant || machine != tt.machine {
				t.Fatalf("got (%q,%q,%v), want (%q,%q,%v)", tenant, machine, ok, tt.tenant, tt.machine, tt.ok)
			}
		})
	}
}

func TestTopicRoundTrip(t *testing.T) {
	tenant, machine, ok := ParseReadingTopic(TelemetryTopic("acme", "press-p04"))
	if !ok || tenant != "acme" || machine != "press-p04" {
		t.Fatalf("round trip failed: %q %q %v", tenant, machine, ok)
	}
}

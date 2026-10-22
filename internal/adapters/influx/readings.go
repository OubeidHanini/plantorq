package influx

import (
	"context"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// WriteReadings implements ingest.Store: each reading becomes one point
// tagged by tenant, machine and source. Tags are indexed in InfluxDB, which
// keeps per-machine history queries fast.
func (c *Client) WriteReadings(ctx context.Context, readings []telemetry.Reading) error {
	points := make([]Point, 0, len(readings))
	for _, r := range readings {
		points = append(points, ToPoint(r))
	}
	return c.Write(ctx, points)
}

// ToPoint maps the domain reading onto the storage model.
func ToPoint(r telemetry.Reading) Point {
	return Point{
		Measurement: Measurement,
		Tags:        map[string]string{"tenant": r.Tenant, "machine": r.Machine, "source": r.Source},
		Fields:      r.Values,
		Time:        r.Time(),
	}
}

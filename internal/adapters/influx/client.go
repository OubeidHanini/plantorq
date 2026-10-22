// Package influx is a minimal InfluxDB 1.x HTTP client: line-protocol writes
// and InfluxQL queries, with no dependency beyond net/http.
package influx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Measurement is where all machine telemetry is stored.
const Measurement = "telemetry"

type Point struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]float64
	Time        time.Time
}

var (
	tagEscaper  = strings.NewReplacer(",", `\,`, "=", `\=`, " ", `\ `)
	measEscaper = strings.NewReplacer(",", `\,`, " ", `\ `)
)

// Line renders the point in InfluxDB line protocol with millisecond precision.
// Keys are sorted so output is deterministic.
func (p Point) Line() string {
	var b strings.Builder
	b.WriteString(measEscaper.Replace(p.Measurement))
	for _, k := range sortedKeys(p.Tags) {
		b.WriteByte(',')
		b.WriteString(tagEscaper.Replace(k))
		b.WriteByte('=')
		b.WriteString(tagEscaper.Replace(p.Tags[k]))
	}
	b.WriteByte(' ')
	for i, k := range sortedKeys(p.Fields) {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(tagEscaper.Replace(k))
		b.WriteByte('=')
		b.WriteString(strconv.FormatFloat(p.Fields[k], 'f', -1, 64))
	}
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(p.Time.UnixMilli(), 10))
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type Client struct {
	base string
	db   string
	http *http.Client
}

func New(baseURL, db string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		db:   db,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/ping", nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// EnsureDatabase creates the database if it does not exist (idempotent).
func (c *Client) EnsureDatabase(ctx context.Context) error {
	form := url.Values{"q": {fmt.Sprintf("CREATE DATABASE %q", c.db)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/query", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, nil)
}

// Write sends points in a single batch.
func (c *Client) Write(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	var body bytes.Buffer
	for _, p := range points {
		if len(p.Fields) == 0 {
			continue
		}
		body.WriteString(p.Line())
		body.WriteByte('\n')
	}
	u := fmt.Sprintf("%s/write?db=%s&precision=ms", c.base, url.QueryEscape(c.db))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &body)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,63}$`)

// History returns the mean of metric for one machine over the last rng,
// bucketed by step.
func (c *Client) History(ctx context.Context, tenant, machine, metric string, rng, step time.Duration) ([]telemetry.Sample, error) {
	if !identRe.MatchString(metric) {
		return nil, fmt.Errorf("invalid metric name %q", metric)
	}
	q := fmt.Sprintf(
		`SELECT mean(%q) FROM %q WHERE "tenant"=%s AND "machine"=%s AND time > now() - %ds GROUP BY time(%ds) fill(none)`,
		metric, Measurement, quoteString(tenant), quoteString(machine),
		int(rng.Seconds()), max(1, int(step.Seconds())),
	)
	res, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}
	samples := []telemetry.Sample{}
	for _, s := range res.Results {
		if s.Error != "" {
			return nil, fmt.Errorf("influx: %s", s.Error)
		}
		for _, series := range s.Series {
			for _, row := range series.Values {
				if len(row) < 2 || row[1] == nil {
					continue
				}
				t, _ := row[0].(json.Number).Int64()
				v, _ := row[1].(json.Number).Float64()
				samples = append(samples, telemetry.Sample{Time: t, Value: v})
			}
		}
	}
	return samples, nil
}

func quoteString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

type queryResponse struct {
	Results []struct {
		Error  string `json:"error"`
		Series []struct {
			Name    string          `json:"name"`
			Columns []string        `json:"columns"`
			Values  [][]interface{} `json:"values"`
		} `json:"series"`
	} `json:"results"`
}

func (c *Client) query(ctx context.Context, q string) (*queryResponse, error) {
	u := fmt.Sprintf("%s/query?db=%s&epoch=ms&q=%s", c.base, url.QueryEscape(c.db), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var out queryResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("influx %s %s: %s: %s", req.Method, req.URL.Path, resp.Status, bytes.TrimSpace(msg))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	return dec.Decode(out)
}

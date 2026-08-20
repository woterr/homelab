// Package metric defines the normalized metric model shared by collectors,
// storage and the API.
package metric

import (
	"sort"
	"strings"
	"time"
)

// Kind classifies how a metric behaves over time.
type Kind string

const (
	Gauge   Kind = "gauge"
	Counter Kind = "counter"
	Derived Kind = "derived"
)

// Sample is a single timestamped value for a named series with optional labels.
type Sample struct {
	Series string            `json:"series"`
	Labels map[string]string `json:"labels,omitempty"`
	Time   time.Time         `json:"ts"`
	Value  float64           `json:"value"`
}

// New returns a sample with nil labels.
func New(series string, ts time.Time, value float64) Sample {
	return Sample{Series: series, Time: ts, Value: value}
}

// NewL returns a sample with labels.
func NewL(series string, labels map[string]string, ts time.Time, value float64) Sample {
	return Sample{Series: series, Labels: labels, Time: ts, Value: value}
}

// Point is a resolved value at a timestamp, optionally carrying aggregation
// bounds from downsampled buckets.
type Point struct {
	Time  time.Time `json:"ts"`
	Avg   float64   `json:"avg"`
	Min   float64   `json:"min,omitempty"`
	Max   float64   `json:"max,omitempty"`
	Sum   float64   `json:"sum,omitempty"`
	Count int       `json:"count,omitempty"`
}

// Value returns the representative value of a point (its average).
func (p Point) Value() float64 { return p.Avg }

// Key builds the canonical storage key for a series, embedding labels in a
// Prometheus-like form: `cpu.utilization{core="0"}`. Unlabeled series key to
// their bare name. This keeps labeled dimensions unique in SQLite while letting
// the frontend reconstruct labels from the key.
func Key(base string, labels map[string]string) string {
	if len(labels) == 0 {
		return base
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(base)
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(labels[k])
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// ParseKey splits a storage key into its base name and labels.
func ParseKey(key string) (string, map[string]string) {
	if i := strings.IndexByte(key, '{'); i >= 0 {
		base := key[:i]
		labels := map[string]string{}
		body := strings.TrimSuffix(key[i+1:], "}")
		for _, pair := range strings.Split(body, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				labels[kv[0]] = strings.Trim(kv[1], `"`)
			}
		}
		return base, labels
	}
	return key, nil
}

// Package store defines the storage abstractions for homelab.
//
// MetricStore is the time-series boundary. The API and frontend only ever talk
// to this interface, so the SQLite V1 implementation can be replaced by a
// Prometheus-backed or other backend without architectural changes.
package store

import (
	"context"
	"time"

	"homelab/backend/internal/metric"
)

// Aggregate is a bucketing/aggregation strategy.
type Aggregate string

const (
	AggAvg   Aggregate = "avg"
	AggMin   Aggregate = "min"
	AggMax   Aggregate = "max"
	AggSum   Aggregate = "sum"
	AggCount Aggregate = "count"
)

// Query selects a window of points for a series.
type Query struct {
	Series     string        `json:"series"`
	Start      time.Time     `json:"start"`
	End        time.Time     `json:"end"`
	Resolution time.Duration `json:"resolution"` // 0 selects raw
	Aggregate  Aggregate     `json:"aggregate"`
	Limit      int           `json:"limit"`
}

// SeriesMeta describes a stored series for discovery/listing.
type SeriesMeta struct {
	Name    string  `json:"name"`
	Samples int64   `json:"samples"`
	First   int64   `json:"first_ts"` // unix seconds
	Last    int64   `json:"last_ts"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

// MetricStore is the time-series storage abstraction.
type MetricStore interface {
	// Append writes samples, batching internally where possible.
	Append(ctx context.Context, samples []metric.Sample) error
	// Query resolves a window of points, applying bucketed aggregation when a
	// resolution is requested.
	Query(ctx context.Context, q Query) ([]metric.Point, error)
	// ListSeries returns metadata for all stored series.
	ListSeries(ctx context.Context) ([]SeriesMeta, error)
	// Latest returns the most recent point for a series, if any.
	Latest(ctx context.Context, series string) (metric.Point, bool, error)
	// Trim removes data older than the retention window.
	Trim(ctx context.Context, retention time.Duration) error
	// Rollup folds raw samples older than cutoff into coarse rollup buckets.
	Rollup(ctx context.Context, cutoff time.Time) error
	// Close releases resources.
	Close() error
}

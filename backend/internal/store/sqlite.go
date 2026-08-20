package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"

	_ "modernc.org/sqlite"
)

// sqliteStore is the V1 MetricStore implementation.
//
// Raw samples are stored in a WITHOUT ROWID table clustered on (series_id, ts)
// so range scans are cheap. Coarse rollup buckets (60s / 300s) are maintained
// by the janitor for old data so long-range queries stay fast and small.
type sqliteStore struct {
	db *sql.DB

	insSeries *sql.Stmt
	insSample *sql.Stmt
	insRollup *sql.Stmt
	getSeries *sql.Stmt
	updStats  *sql.Stmt
	insStats  *sql.Stmt
	latest    *sql.Stmt

	rollupRes []int64 // bucket sizes in seconds maintained by Rollup

	mu       sync.Mutex
	cutoffMs int64 // fold boundary: raw data older than this lives in rollups
}

const (
	sqlSchema = `
CREATE TABLE IF NOT EXISTS series (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS samples (
    series_id INTEGER NOT NULL,
    ts        INTEGER NOT NULL,
    value     REAL NOT NULL,
    PRIMARY KEY (series_id, ts)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_samples_ts ON samples(ts);
CREATE TABLE IF NOT EXISTS rollup (
    series_id INTEGER NOT NULL,
    res       INTEGER NOT NULL,
    bucket    INTEGER NOT NULL,
    count     INTEGER NOT NULL,
    sum       REAL NOT NULL,
    min       REAL NOT NULL,
    max       REAL NOT NULL,
    PRIMARY KEY (series_id, res, bucket)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS series_stats (
    series_id INTEGER PRIMARY KEY,
    samples   INTEGER NOT NULL DEFAULT 0,
    first_ts  INTEGER,
    last_ts   INTEGER,
    min       REAL,
    max       REAL
);
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
)

// metaRollupCutoff is the meta key storing the last rollup fold boundary.
const metaRollupCutoff = "rollup_cutoff"

func OpenMetricStore(ctx context.Context, path string, lg *log.Logger) (MetricStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.ExecContext(ctx, sqlSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("metric schema: %w", err)
	}

	s := &sqliteStore{db: db, rollupRes: []int64{60, 300}}
	if err := s.prepare(ctx); err != nil {
		db.Close()
		return nil, err
	}
	// Restore the persisted fold boundary (or default to "nothing folded").
	s.cutoffMs = time.Now().UnixMilli()
	var v string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, metaRollupCutoff).Scan(&v); err == nil {
		if ms, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			s.cutoffMs = ms
		}
	}
	return s, nil
}

func (s *sqliteStore) prepare(ctx context.Context) error {
	var err error
	prep := func(name, q string) (*sql.Stmt, error) {
		st, e := s.db.PrepareContext(ctx, q)
		if e != nil {
			return nil, fmt.Errorf("%s: %w", name, e)
		}
		return st, nil
	}
	if s.insSeries, err = prep("insSeries", `INSERT OR IGNORE INTO series(name) VALUES(?)`); err != nil {
		return err
	}
	if s.getSeries, err = prep("getSeries", `SELECT id FROM series WHERE name = ?`); err != nil {
		return err
	}
	if s.insSample, err = prep("insSample", `INSERT OR REPLACE INTO samples(series_id, ts, value) VALUES(?,?,?)`); err != nil {
		return err
	}
	if s.insRollup, err = prep("insRollup", `INSERT OR REPLACE INTO rollup(series_id, res, bucket, count, sum, min, max) VALUES(?,?,?,?,?,?,?)`); err != nil {
		return err
	}
	if s.latest, err = prep("latest", `SELECT ts, value FROM samples WHERE series_id = ? ORDER BY ts DESC LIMIT 1`); err != nil {
		return err
	}
	if s.updStats, err = prep("updStats", `UPDATE series_stats SET samples = samples + ?, first_ts = COALESCE(first_ts, ?), last_ts = ?, min = MIN(COALESCE(min, ?), ?), max = MAX(COALESCE(max, ?), ?) WHERE series_id = ?`); err != nil {
		return err
	}
	if s.insStats, err = prep("insStats", `INSERT INTO series_stats(series_id, samples, first_ts, last_ts, min, max) VALUES(?,?,?,?,?,?)`); err != nil {
		return err
	}
	return nil
}

// seriesID resolves a series name to its integer id, creating it on first use.
func (s *sqliteStore) seriesID(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	exec := func(q string, args ...any) (sql.Result, error) {
		if tx != nil {
			return tx.ExecContext(ctx, q, args...)
		}
		return s.db.ExecContext(ctx, q, args...)
	}
	if _, err := exec(`INSERT OR IGNORE INTO series(name) VALUES(?)`, name); err != nil {
		return 0, err
	}
	var id int64
	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(ctx, `SELECT id FROM series WHERE name = ?`, name)
	} else {
		row = s.db.QueryRowContext(ctx, `SELECT id FROM series WHERE name = ?`, name)
	}
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// Append batches samples into a single transaction per call. The scheduler
// calls this once per collector tick, so flushing cost is amortized.
func (s *sqliteStore) Append(ctx context.Context, samples []metric.Sample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ids := make(map[string]int64, 8)
	var tsMs int64
	for _, sm := range samples {
		key := metric.Key(sm.Series, sm.Labels)
		id, ok := ids[key]
		if !ok {
			id, err = s.seriesID(ctx, tx, key)
			if err != nil {
				return err
			}
			ids[key] = id
		}
		tsMs = sm.Time.UnixMilli()
		if _, err := tx.StmtContext(ctx, s.insSample).ExecContext(ctx, id, tsMs, sm.Value); err != nil {
			return fmt.Errorf("insert %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.updateStats(ctx, ids, tsMs, samples)
}

func (s *sqliteStore) updateStats(ctx context.Context, ids map[string]int64, tsMs int64, samples []metric.Sample) error {
	// Collapse batch extremes per series so stats stay cheap to maintain.
	type st struct{ mn, mx float64 }
	extremes := map[string]*st{}
	for _, sm := range samples {
		key := metric.Key(sm.Series, sm.Labels)
		e, ok := extremes[key]
		if !ok {
			extremes[key] = &st{mn: sm.Value, mx: sm.Value}
			continue
		}
		if sm.Value < e.mn {
			e.mn = sm.Value
		}
		if sm.Value > e.mx {
			e.mx = sm.Value
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for name, e := range extremes {
		id := ids[name]
		res, err := tx.StmtContext(ctx, s.updStats).ExecContext(ctx, 1, tsMs, tsMs, e.mn, e.mn, e.mx, e.mx, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if _, err := tx.StmtContext(ctx, s.insStats).ExecContext(ctx, id, 1, tsMs, tsMs, e.mn, e.mx); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// resolveSeries returns the id for a series name, or 0 if unknown.
func (s *sqliteStore) resolveSeries(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM series WHERE name = ?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// Query resolves a window using the finest appropriate source:
//
//	resolution <  60s  -> raw samples, SQL-bucketed
//	resolution >= 60s  -> rollup rows (60s/300s) for old data + raw for recent,
//	                      merged and re-bucketed in Go
func (s *sqliteStore) Query(ctx context.Context, q Query) ([]metric.Point, error) {
	agg := q.Aggregate
	if agg == "" {
		agg = AggAvg
	}
	id, err := s.resolveSeries(ctx, q.Series)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}

	startMs := q.Start.UnixMilli()
	endMs := q.End.UnixMilli()

	// Raw path: finest resolution.
	if q.Resolution <= 0 || q.Resolution < time.Second*60 {
		return s.queryRaw(ctx, id, q, startMs, endMs, agg)
	}

	// Rollup path.
	resSec := int64(q.Resolution / time.Second)
	res := s.pickRollupRes(resSec)
	buckets := [][]metric.Point{}

	if startMs < s.rollupCutoff() {
		rollEnd := endMs
		if rollEnd > s.rollupCutoff() {
			rollEnd = s.rollupCutoff()
		}
		if pts, err := s.queryRollup(ctx, id, q, res, startMs, rollEnd, agg); err != nil {
			return nil, err
		} else {
			buckets = append(buckets, pts)
		}
	}
	if endMs > s.rollupCutoff() {
		rawStart := startMs
		if rawStart < s.rollupCutoff() {
			rawStart = s.rollupCutoff()
		}
		// Bucket the recent raw data at the requested resolution.
		rq := q
		rq.Start = time.UnixMilli(rawStart)
		rq.End = time.UnixMilli(endMs)
		rq.Resolution = q.Resolution
		if pts, err := s.queryRaw(ctx, id, rq, rawStart, endMs, agg); err != nil {
			return nil, err
		} else {
			buckets = append(buckets, pts)
		}
	}
	return mergePoints(buckets...), nil
}

// rollupCutoff is the fold boundary: data older than this lives in rollups,
// data newer than this is in the raw samples table. It is persisted when the
// janitor runs so query routing stays consistent with the configured window.
func (s *sqliteStore) rollupCutoff() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cutoffMs
}

func (s *sqliteStore) pickRollupRes(resSec int64) int64 {
	for _, r := range s.rollupRes {
		if resSec <= r {
			return r
		}
	}
	return s.rollupRes[len(s.rollupRes)-1]
}

// queryRaw runs a bucketed SQL aggregation over raw samples.
func (s *sqliteStore) queryRaw(ctx context.Context, id int64, q Query, startMs, endMs int64, agg Aggregate) ([]metric.Point, error) {
	step := int64(time.Second)
	if q.Resolution > 0 {
		step = int64(q.Resolution / time.Millisecond)
	}
	aggCol := "AVG(value)"
	switch agg {
	case AggMin:
		aggCol = "MIN(value)"
	case AggMax:
		aggCol = "MAX(value)"
	case AggSum:
		aggCol = "SUM(value)"
	case AggCount:
		aggCol = "COUNT(value)"
	}
	query := fmt.Sprintf(
		`SELECT (ts / ?) * ?, COUNT(*), AVG(value), MIN(value), MAX(value), SUM(value), %s
		 FROM samples WHERE series_id = ? AND ts >= ? AND ts <= ?
		 GROUP BY (ts / ?) ORDER BY (ts / ?)`, aggCol)
	rows, err := s.db.QueryContext(ctx, query, step, step, id, startMs, endMs, step, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []metric.Point
	var extra float64
	for rows.Next() {
		var p metric.Point
		var b, cnt int64
		if err := rows.Scan(&b, &cnt, &p.Avg, &p.Min, &p.Max, &p.Sum, &extra); err != nil {
			return nil, err
		}
		p.Time = time.UnixMilli(b)
		p.Count = int(cnt)
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// queryRollup fetches pre-aggregated buckets and re-buckets them at the
// requested resolution in Go.
func (s *sqliteStore) queryRollup(ctx context.Context, id int64, q Query, res int64, startMs, endMs int64, agg Aggregate) ([]metric.Point, error) {
	startB := startMs / 1000 / res * res
	endB := endMs / 1000 / res * res
	rows, err := s.db.QueryContext(ctx,
		`SELECT bucket, count, sum, min, max FROM rollup
		 WHERE series_id = ? AND res = ? AND bucket >= ? AND bucket <= ?
		 ORDER BY bucket`, id, res, startB, endB)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rb struct {
		cnt         int64
		sum, mn, mx float64
	}
	var groups []rb
	var groupB []int64
	step := q.Resolution.Seconds()
	for rows.Next() {
		var b int64
		var r rb
		if err := rows.Scan(&b, &r.cnt, &r.sum, &r.mn, &r.mx); err != nil {
			return nil, err
		}
		bucketIdx := b / int64(step)
		if len(groupB) == 0 || groupB[len(groupB)-1] != bucketIdx {
			groupB = append(groupB, bucketIdx)
			groups = append(groups, r)
		} else {
			g := &groups[len(groups)-1]
			g.cnt += r.cnt
			g.sum += r.sum
			if r.mn < g.mn {
				g.mn = r.mn
			}
			if r.mx > g.mx {
				g.mx = r.mx
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pts := make([]metric.Point, 0, len(groups))
	for i, g := range groups {
		var v float64
		switch agg {
		case AggMin:
			v = g.mn
		case AggMax:
			v = g.mx
		case AggSum:
			v = g.sum
		case AggCount:
			v = float64(g.cnt)
		default:
			if g.cnt > 0 {
				v = g.sum / float64(g.cnt)
			}
		}
		pts = append(pts, metric.Point{
			Time:  time.UnixMilli(groupB[i] * int64(step) * 1000),
			Avg:   v,
			Min:   g.mn,
			Max:   g.mx,
			Sum:   g.sum,
			Count: int(g.cnt),
		})
	}
	return pts, nil
}

func mergePoints(buckets ...[]metric.Point) []metric.Point {
	total := 0
	for _, b := range buckets {
		total += len(b)
	}
	out := make([]metric.Point, 0, total)
	for _, b := range buckets {
		out = append(out, b...)
	}
	// Simple insertion into sorted order (sources are individually sorted).
	if len(out) < 2 {
		return out
	}
	// Merge-sort since sources are sorted and boundaries are contiguous.
	sorted := make([]metric.Point, 0, len(out))
	for _, b := range buckets {
		sorted = mergeSorted(sorted, b)
	}
	return sorted
}

func mergeSorted(a, b []metric.Point) []metric.Point {
	out := make([]metric.Point, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].Time.Before(b[j].Time) {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}

func (s *sqliteStore) Latest(ctx context.Context, series string) (metric.Point, bool, error) {
	id, err := s.resolveSeries(ctx, series)
	if err != nil || id == 0 {
		return metric.Point{}, false, err
	}
	var ts int64
	var v float64
	err = s.latest.QueryRowContext(ctx, id).Scan(&ts, &v)
	if err == sql.ErrNoRows {
		return metric.Point{}, false, nil
	}
	if err != nil {
		return metric.Point{}, false, err
	}
	return metric.Point{Time: time.UnixMilli(ts), Avg: v}, true, nil
}

func (s *sqliteStore) ListSeries(ctx context.Context) ([]SeriesMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ss.series_id, s.name, ss.samples, COALESCE(ss.first_ts,0), COALESCE(ss.last_ts,0), COALESCE(ss.min,0), COALESCE(ss.max,0)
		FROM series_stats ss JOIN series s ON s.id = ss.series_id
		ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesMeta
	for rows.Next() {
		var m SeriesMeta
		var sid int64
		if err := rows.Scan(&sid, &m.Name, &m.Samples, &m.First, &m.Last, &m.Min, &m.Max); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *sqliteStore) Trim(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention).UnixMilli()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM samples WHERE ts < ?`, cutoff); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rollup WHERE bucket * 1000 < ?`, cutoff); err != nil {
		return err
	}
	return nil
}

// Rollup folds raw samples older than cutoff into rollup buckets, then removes
// the folded raw rows in the same transaction. Buckets at 60s and 300s are
// maintained so long-range queries never scan raw data.
func (s *sqliteStore) Rollup(ctx context.Context, cutoff time.Time) error {
	cutMs := cutoff.UnixMilli()
	for _, res := range s.rollupRes {
		// Aggregate raw rows older than cutoff into buckets of `res` seconds.
		rows, err := s.db.QueryContext(ctx, `
			SELECT series_id, (ts / 1000 / ?) * ?, COUNT(*), SUM(value), MIN(value), MAX(value)
			FROM samples
			WHERE ts < ?
			GROUP BY series_id, (ts / 1000 / ?)
			ORDER BY series_id`, res, res, cutMs, res)
		if err != nil {
			return err
		}
		type agg struct {
			series int64
			bucket int64
			count  int64
			sum    float64
			mn     float64
			mx     float64
		}
		var aggs []agg
		for rows.Next() {
			var a agg
			if err := rows.Scan(&a.series, &a.bucket, &a.count, &a.sum, &a.mn, &a.mx); err != nil {
				rows.Close()
				return err
			}
			aggs = append(aggs, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, a := range aggs {
			if _, err := tx.StmtContext(ctx, s.insRollup).ExecContext(ctx, a.series, res, a.bucket, a.count, a.sum, a.mn, a.mx); err != nil {
				tx.Rollback()
				return err
			}
		}
		// Remove raw rows that have been folded into this resolution. Rows are
		// shared across resolutions; only delete after the coarsest resolution
		// has folded them.
		if res == s.rollupRes[len(s.rollupRes)-1] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM samples WHERE ts < ?`, cutMs); err != nil {
				tx.Rollback()
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES(?,?)
				ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
				metaRollupCutoff, strconv.FormatInt(cutMs, 10)); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.cutoffMs = cutMs
	s.mu.Unlock()
	return nil
}

func (s *sqliteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

var _ MetricStore = (*sqliteStore)(nil)

// SeriesName is a small helper for building normalized series names.
func SeriesName(parts ...string) string {
	return strings.Join(parts, ".")
}

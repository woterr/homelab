package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"homelab/backend/internal/events"
)

// StateStore persists application state: events, devices, services, rules and
// settings. It is intentionally a small, typed layer over SQLite.
type StateStore struct {
	db *sql.DB
}

const sqlStateSchema = `
CREATE TABLE IF NOT EXISTS events (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    ts       INTEGER NOT NULL,
    type     TEXT NOT NULL,
    severity TEXT NOT NULL,
    source   TEXT NOT NULL,
    message  TEXT NOT NULL,
    data     TEXT
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);

CREATE TABLE IF NOT EXISTS devices (
    mac         TEXT PRIMARY KEY,
    ip          TEXT,
    hostname    TEXT,
    vendor      TEXT,
    device_type TEXT,
    first_seen  INTEGER,
    last_seen   INTEGER,
    online      INTEGER NOT NULL DEFAULT 0,
    metadata    TEXT
);

CREATE TABLE IF NOT EXISTS services (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    status     TEXT NOT NULL,
    image      TEXT,
    started_at INTEGER,
    restarts   INTEGER NOT NULL DEFAULT 0,
    cpu        REAL,
    memory     REAL,
    last_error TEXT
);

CREATE TABLE IF NOT EXISTS rules (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    metric      TEXT NOT NULL,
    operator    TEXT NOT NULL,
    threshold   REAL NOT NULL,
    duration_s  INTEGER NOT NULL DEFAULT 0,
    notify      INTEGER NOT NULL DEFAULT 1,
    channels    TEXT NOT NULL DEFAULT '[]',
    message     TEXT,
    created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// OpenStateStore opens the application-state database.
func OpenStateStore(ctx context.Context, path string) (*StateStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	if _, err := db.ExecContext(ctx, sqlStateSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("state schema: %w", err)
	}
	if err := migrateRulesChannels(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("state schema migration: %w", err)
	}
	return &StateStore{db: db}, nil
}

// migrateRulesChannels adds the channels column to rules on databases created
// before per-rule notification targeting was added.
func migrateRulesChannels(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(rules)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "channels" {
			return nil
		}
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE rules ADD COLUMN channels TEXT NOT NULL DEFAULT '[]'`)
	return err
}

// Close releases the underlying connection pool.
func (s *StateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ---- Events ----

func (s *StateStore) InsertEvent(ctx context.Context, ev *events.Event) error {
	var data []byte
	if len(ev.Data) > 0 {
		b, err := json.Marshal(ev.Data)
		if err != nil {
			return err
		}
		data = b
	}
	sev := ev.Severity
	if sev == "" {
		sev = events.SeverityInfo
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events(ts, type, severity, source, message, data) VALUES(?,?,?,?,?,?)`,
		ev.Time.UnixMilli(), ev.Type, sev, ev.Source, ev.Message, data)
	return err
}

// EventFilter narrows ListEvents.
type EventFilter struct {
	Limit    int
	Since    time.Time
	Types    []string
	Severity string
}

func (s *StateStore) ListEvents(ctx context.Context, f EventFilter) ([]*events.Event, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 5000 {
		limit = 5000
	}
	q := `SELECT ts, type, severity, source, message, data FROM events`
	conds := []string{}
	args := []any{}
	if !f.Since.IsZero() {
		conds = append(conds, `ts >= ?`)
		args = append(args, f.Since.UnixMilli())
	}
	if len(f.Types) > 0 {
		place := ""
		for i, t := range f.Types {
			if i > 0 {
				place += ","
			}
			place += "?"
			args = append(args, t)
		}
		conds = append(conds, fmt.Sprintf(`type IN (%s)`, place))
	}
	if f.Severity != "" {
		conds = append(conds, `severity = ?`)
		args = append(args, f.Severity)
	}
	if len(conds) > 0 {
		q += " WHERE " + joinConds(conds)
	}
	q += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out = make([]*events.Event, 0)
	for rows.Next() {
		var ts int64
		var etype, sev, src, msg string
		var data []byte
		if err := rows.Scan(&ts, &etype, &sev, &src, &msg, &data); err != nil {
			return nil, err
		}
		ev := events.New(etype, sev, src, msg)
		ev.Time = time.UnixMilli(ts)
		if len(data) > 0 {
			_ = json.Unmarshal(data, &ev.Data)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func joinConds(conds []string) string {
	out := ""
	for i, c := range conds {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}

// ---- Devices ----

type Device struct {
	MAC        string            `json:"mac"`
	IP         string            `json:"ip"`
	Hostname   string            `json:"hostname"`
	Vendor     string            `json:"vendor"`
	DeviceType string            `json:"device_type"`
	FirstSeen  time.Time         `json:"first_seen"`
	LastSeen   time.Time         `json:"last_seen"`
	Online     bool              `json:"online"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func (s *StateStore) UpsertDevice(ctx context.Context, d *Device) error {
	meta := "{}"
	if len(d.Metadata) > 0 {
		if b, err := json.Marshal(d.Metadata); err == nil {
			meta = string(b)
		}
	}
	online := 0
	if d.Online {
		online = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO devices(mac, ip, hostname, vendor, device_type, first_seen, last_seen, online, metadata)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(mac) DO UPDATE SET
			ip=excluded.ip,
			hostname=excluded.hostname,
			vendor=excluded.vendor,
			device_type=excluded.device_type,
			last_seen=excluded.last_seen,
			online=excluded.online,
			metadata=excluded.metadata`,
		d.MAC, d.IP, d.Hostname, d.Vendor, d.DeviceType,
		d.FirstSeen.UnixMilli(), d.LastSeen.UnixMilli(), online, meta)
	return err
}

func (s *StateStore) ListDevices(ctx context.Context) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT mac, COALESCE(ip,''), COALESCE(hostname,''), COALESCE(vendor,''),
		       COALESCE(device_type,''), first_seen, last_seen, online, COALESCE(metadata,'{}')
		FROM devices ORDER BY online DESC, last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out = make([]*Device, 0)
	for rows.Next() {
		var d Device
		var fs, ls int64
		var online int
		var meta string
		if err := rows.Scan(&d.MAC, &d.IP, &d.Hostname, &d.Vendor, &d.DeviceType, &fs, &ls, &online, &meta); err != nil {
			return nil, err
		}
		d.FirstSeen = time.UnixMilli(fs)
		d.LastSeen = time.UnixMilli(ls)
		d.Online = online == 1
		_ = json.Unmarshal([]byte(meta), &d.Metadata)
		out = append(out, &d)
	}
	return out, rows.Err()
}

func (s *StateStore) GetDevice(ctx context.Context, mac string) (*Device, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT mac, COALESCE(ip,''), COALESCE(hostname,''), COALESCE(vendor,''),
		       COALESCE(device_type,''), first_seen, last_seen, online, COALESCE(metadata,'{}')
		FROM devices WHERE mac = ?`, mac)
	var d Device
	var fs, ls int64
	var online int
	var meta string
	if err := row.Scan(&d.MAC, &d.IP, &d.Hostname, &d.Vendor, &d.DeviceType, &fs, &ls, &online, &meta); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	d.FirstSeen = time.UnixMilli(fs)
	d.LastSeen = time.UnixMilli(ls)
	d.Online = online == 1
	_ = json.Unmarshal([]byte(meta), &d.Metadata)
	return &d, nil
}

// ---- Services ----

type Service struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Image     string    `json:"image,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Restarts  int       `json:"restarts"`
	CPU       float64   `json:"cpu"`
	Memory    float64   `json:"memory"`
	LastError string    `json:"last_error,omitempty"`
}

func (s *StateStore) UpsertService(ctx context.Context, svc *Service) error {
	return s.UpsertServices(ctx, []*Service{svc})
}

// UpsertServices persists many service snapshots in a single transaction so the
// periodic 100+ unit refresh doesn't pay an fsync per row (SD-card friendly).
func (s *StateStore) UpsertServices(ctx context.Context, svcs []*Service) error {
	if len(svcs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO services(id, name, kind, status, image, started_at, restarts, cpu, memory, last_error)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, kind=excluded.kind, status=excluded.status,
			image=excluded.image, started_at=excluded.started_at, restarts=excluded.restarts,
			cpu=excluded.cpu, memory=excluded.memory, last_error=excluded.last_error`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, svc := range svcs {
		var st int64
		if !svc.StartedAt.IsZero() {
			st = svc.StartedAt.UnixMilli()
		}
		if _, err := stmt.ExecContext(ctx, svc.ID, svc.Name, svc.Kind, svc.Status, svc.Image, st, svc.Restarts, svc.CPU, svc.Memory, svc.LastError); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *StateStore) ListServices(ctx context.Context) ([]*Service, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, kind, status, COALESCE(image,''), COALESCE(started_at,0),
		       restarts, COALESCE(cpu,0), COALESCE(memory,0), COALESCE(last_error,'')
		FROM services ORDER BY kind, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out = make([]*Service, 0)
	for rows.Next() {
		var sv Service
		var st int64
		if err := rows.Scan(&sv.ID, &sv.Name, &sv.Kind, &sv.Status, &sv.Image, &st, &sv.Restarts, &sv.CPU, &sv.Memory, &sv.LastError); err != nil {
			return nil, err
		}
		if st > 0 {
			sv.StartedAt = time.UnixMilli(st)
		}
		out = append(out, &sv)
	}
	return out, rows.Err()
}

func (s *StateStore) GetService(ctx context.Context, id string) (*Service, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, kind, status, COALESCE(image,''), COALESCE(started_at,0),
		       restarts, COALESCE(cpu,0), COALESCE(memory,0), COALESCE(last_error,'')
		FROM services WHERE id = ?`, id)
	var sv Service
	var st int64
	if err := row.Scan(&sv.ID, &sv.Name, &sv.Kind, &sv.Status, &sv.Image, &st, &sv.Restarts, &sv.CPU, &sv.Memory, &sv.LastError); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if st > 0 {
		sv.StartedAt = time.UnixMilli(st)
	}
	return &sv, nil
}

// ---- Rules ----

type Rule struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Metric    string    `json:"metric"`
	Operator  string    `json:"operator"` // gt, gte, lt, lte, eq, neq
	Threshold float64   `json:"threshold"`
	DurationS int       `json:"duration_s"`
	Notify    bool      `json:"notify"`
	Channels  []string  `json:"channels,omitempty"` // notify targets; empty = all configured
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *StateStore) UpsertRule(ctx context.Context, r *Rule) error {
	if r.ID == "" {
		r.ID = fmt.Sprintf("rule_%d", time.Now().UnixNano())
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	en := 0
	if r.Enabled {
		en = 1
	}
	nt := 0
	if r.Notify {
		nt = 1
	}
	ch := "[]"
	if len(r.Channels) > 0 {
		if b, err := json.Marshal(r.Channels); err == nil {
			ch = string(b)
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rules(id, name, enabled, metric, operator, threshold, duration_s, notify, channels, message, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, enabled=excluded.enabled, metric=excluded.metric,
			operator=excluded.operator, threshold=excluded.threshold,
			duration_s=excluded.duration_s, notify=excluded.notify, channels=excluded.channels, message=excluded.message`,
		r.ID, r.Name, en, r.Metric, r.Operator, r.Threshold, r.DurationS, nt, ch, r.Message, r.CreatedAt.UnixMilli())
	return err
}

func (s *StateStore) ListRules(ctx context.Context) ([]*Rule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, enabled, metric, operator, threshold, duration_s, notify, COALESCE(channels,'[]'), COALESCE(message,''), created_at
		FROM rules ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out = make([]*Rule, 0)
	for rows.Next() {
		var r Rule
		var en, nt, ca int64
		var ch string
		if err := rows.Scan(&r.ID, &r.Name, &en, &r.Metric, &r.Operator, &r.Threshold, &r.DurationS, &nt, &ch, &r.Message, &ca); err != nil {
			return nil, err
		}
		r.Enabled = en == 1
		r.Notify = nt == 1
		if ch != "" && ch != "[]" {
			_ = json.Unmarshal([]byte(ch), &r.Channels)
		}
		r.CreatedAt = time.UnixMilli(ca)
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (s *StateStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	return err
}

// ---- Settings ----

func (s *StateStore) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *StateStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings(key, value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *StateStore) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

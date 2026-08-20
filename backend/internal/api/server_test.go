package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"homelab/backend/internal/collect"
	"homelab/backend/internal/config"
	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"
	"homelab/backend/internal/notify"
	"homelab/backend/internal/rules"
	"homelab/backend/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	lg := log.New(4, nil)
	cfg := &config.Config{DataDir: t.TempDir(), HistoryWindow: 15 * time.Minute, Demo: true}
	ms, err := store.OpenMetricStore(ctx, cfg.DataDir+"/m.db", lg)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenStateStore(ctx, cfg.DataDir+"/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ms.Close(); st.Close() })
	bus := events.NewBus()
	tee := &events.Tee{Store: st, Bus: bus}
	nf := notify.NewManager(notify.Config{
		NtfyServer: "https://ntfy.sh",
		NtfyTopic:  "x",
	}, st.ListSettings, st.SetSetting)
	eng := rules.NewEngine(st, tee, nf.Registry(), lg)
	demo := collect.NewDemoHost()

	// Seed a couple of samples so queries return data.
	now := time.Now()
	var samples []metric.Sample
	for i := 0; i < 120; i++ {
		ts := now.Add(-time.Duration(120-i) * time.Second)
		samples = append(samples, metric.New("cpu.utilization", ts, float64(i)))
	}
	if err := ms.Append(ctx, samples); err != nil {
		t.Fatal(err)
	}

	src := Sources{
		System: demo, CPU: demo, Memory: demo, Temperature: demo,
		Network: demo, Disk: demo, Filesystem: demo, Services: demo,
		Devices: demo, Power: demo,
	}
	return NewServer(cfg, lg, ms, st, bus, tee, eng, nf, src)
}

func get(t *testing.T, s *Server, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	return rr, rr.Body.String()
}

func TestQueryEndpoint(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	start := now.Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	end := now.Add(time.Second).UTC().Format(time.RFC3339)
	rr, body := get(t, s, "/api/v1/metrics/query?series=cpu.utilization&start="+start+"&end="+end+"&resolution=30s")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %q", rr.Code, body)
	}
	if body == "" {
		t.Fatal("empty body")
	}
	var resp struct {
		Points []metric.Point `json:"points"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("bad json: %v (%q)", err, body)
	}
	if len(resp.Points) == 0 {
		t.Fatal("expected points")
	}
}

func TestQueryEndpointMilliseconds(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	start := now.Add(-2 * time.Minute).UnixMilli()
	end := now.Add(time.Second).UnixMilli()
	rr, body := get(t, s, "/api/v1/metrics/query?series=cpu.utilization&start="+itoa(start)+"&end="+itoa(end)+"&resolution=30s")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %q", rr.Code, body)
	}
	if body == "" {
		t.Fatal("empty body")
	}
}

func TestEndpointsSmoke(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{
		"/api/v1/system",
		"/api/v1/system/cpu",
		"/api/v1/system/memory",
		"/api/v1/system/temperature",
		"/api/v1/network/interfaces",
		"/api/v1/network/devices",
		"/api/v1/network/topology",
		"/api/v1/storage",
		"/api/v1/storage/disks",
		"/api/v1/services",
		"/api/v1/power",
		"/api/v1/events",
		"/api/v1/metrics/series",
		"/api/v1/metrics/latest?series=cpu.utilization",
		"/api/v1/rules",
		"/api/v1/settings",
		"/api/v1/notify/providers",
		"/api/v1/internet",
	} {
		rr, body := get(t, s, p)
		if rr.Code != http.StatusOK {
			t.Errorf("%s -> %d: %s", p, rr.Code, body)
			continue
		}
		if body == "" {
			t.Errorf("%s -> empty body", p)
		}
	}
}

func itoa(n int64) string {
	b := [20]byte{}
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

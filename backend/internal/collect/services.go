package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"
	"homelab/backend/internal/store"
)

// ServiceMeta is a discovered service/container snapshot.
type ServiceMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // docker | systemd
	Status    string    `json:"status"`
	Image     string    `json:"image,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Restarts  int       `json:"restarts"`
	CPU       float64   `json:"cpu"`
	Memory    float64   `json:"memory"`
}

// ServicesCollector discovers Docker containers and systemd services and emits
// service.started/stopped events on transitions. Container CPU/memory stats are
// sampled each tick; the discovery list refreshes on a slower cadence.
type ServicesCollector struct {
	Host    Host
	store   *store.StateStore
	tee     *events.Tee
	log     *log.Logger
	refresh time.Duration

	mu          sync.Mutex
	services    map[string]*store.Service
	docker      *dockerClient
	lastRefresh time.Time
}

func NewServicesCollector(h Host, st *store.StateStore, tee *events.Tee, lg *log.Logger) *ServicesCollector {
	c := &ServicesCollector{
		Host:     h,
		store:    st,
		tee:      tee,
		log:      lg,
		refresh:  30 * time.Second,
		services: map[string]*store.Service{},
	}
	c.docker, _ = newDockerClient(h.Docker)
	return c
}

func (c *ServicesCollector) Name() string { return "services" }

// ---- Docker ----

type dockerClient struct{ c *http.Client }

func newDockerClient(sock string) (*dockerClient, error) {
	if _, err := os.Stat(sock); err != nil {
		return nil, err
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
	return &dockerClient{c: &http.Client{Transport: tr, Timeout: 5 * time.Second}}, nil
}

func (dc *dockerClient) getJSON(ctx context.Context, path string, v any) error {
	resp, err := dc.c.Get("http://docker" + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("docker: %s -> %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

type dockerContainer struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
}

func (c *ServicesCollector) collectDocker(ctx context.Context, now time.Time) ([]metric.Sample, []ServiceMeta, []*store.Service) {
	if c.docker == nil {
		return nil, nil, nil
	}
	var list []dockerContainer
	if err := c.docker.getJSON(ctx, "/containers/json?all=1", &list); err != nil {
		c.log.Warn("docker list failed", "error", err)
		return nil, nil, nil
	}
	var samples []metric.Sample
	var metas []ServiceMeta
	var svcs []*store.Service
	for _, ct := range list {
		name := ct.Names[0]
		name = strings.TrimPrefix(name, "/")
		id := "docker." + name
		started := time.Unix(ct.Created, 0)

		svc := &store.Service{
			ID:        id,
			Name:      name,
			Kind:      "docker",
			Status:    ct.State,
			Image:     ct.Image,
			StartedAt: started,
		}
		metas = append(metas, ServiceMeta{ID: id, Name: name, Kind: "docker", Status: ct.State, Image: ct.Image, StartedAt: started})

		up := 0.0
		if ct.State == "running" {
			up = 1
		}
		samples = append(samples, metric.NewL("service.up", map[string]string{"service": name, "kind": "docker"}, now, up))

		if ct.State == "running" {
			var st dockerStats
			if err := c.docker.getJSON(ctx, "/containers/"+ct.ID+"/stats?stream=false", &st); err == nil {
				cpu, mem := dockerCPUMem(&st)
				svc.CPU, svc.Memory = cpu, mem
				samples = append(samples,
					metric.NewL("service.cpu_percent", map[string]string{"service": name, "kind": "docker"}, now, cpu),
					metric.NewL("service.memory_bytes", map[string]string{"service": name, "kind": "docker"}, now, mem),
				)
			}
		}
		svcs = append(svcs, svc)
	}
	return samples, metas, svcs
}

func dockerCPUMem(st *dockerStats) (cpu, mem float64) {
	mem = float64(st.MemoryStats.Usage)
	delta := st.CPUStats.CPUUsage.TotalUsage - st.PreCPUStats.CPUUsage.TotalUsage
	sysDelta := st.CPUStats.SystemUsage - st.PreCPUStats.SystemUsage
	if sysDelta > 0 && delta > 0 {
		cores := st.CPUStats.OnlineCPUs
		if cores == 0 {
			cores = 1
		}
		cpu = float64(delta) / float64(sysDelta) * float64(cores) * 100
	}
	return
}

// ---- systemd ----

func (c *ServicesCollector) collectSystemd(ctx context.Context, now time.Time) ([]metric.Sample, []ServiceMeta, []*store.Service) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil, nil, nil
	}
	out, err := exec.CommandContext(ctx, "systemctl", "--no-pager", "--output=json", "list-units", "--type=service", "--all").Output()
	if err != nil {
		return nil, nil, nil
	}
	var units []struct {
		Unit        string `json:"unit"`
		Active      string `json:"active"`
		Sub         string `json:"sub"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(out, &units); err != nil {
		return nil, nil, nil
	}
	var samples []metric.Sample
	var metas []ServiceMeta
	var svcs []*store.Service
	for _, u := range units {
		name := strings.TrimSuffix(u.Unit, ".service")
		id := "systemd." + name
		status := u.Active
		if u.Active == "active" && u.Sub != "running" {
			status = u.Sub
		}
		up := 0.0
		if status == "running" || status == "active" {
			up = 1
		}
		samples = append(samples, metric.NewL("service.up", map[string]string{"service": name, "kind": "systemd"}, now, up))
		svc := &store.Service{
			ID:     id,
			Name:   name,
			Kind:   "systemd",
			Status: status,
		}
		metas = append(metas, ServiceMeta{ID: id, Name: name, Kind: "systemd", Status: status})
		svcs = append(svcs, svc)
	}
	return samples, metas, svcs
}

// persist diffs the snapshots against the in-memory map, emits transition
// events, and batches all upserts into a single transaction.
func (c *ServicesCollector) persist(ctx context.Context, svcs []*store.Service) {
	for _, svc := range svcs {
		c.diffAndEmit(ctx, svc)
	}
	if err := c.store.UpsertServices(ctx, svcs); err != nil {
		c.log.Warn("service persist failed", "error", err)
	}
}

// diffAndEmit updates the in-memory snapshot and emits state-transition events.
// Persistence is handled separately (batched) by persist().
func (c *ServicesCollector) diffAndEmit(ctx context.Context, cur *store.Service) {
	c.mu.Lock()
	prev, existed := c.services[cur.ID]
	c.services[cur.ID] = cur
	c.mu.Unlock()

	if !existed {
		c.tee.Emit(ctx, events.New(events.TypeServiceStarted, events.SeverityInfo, "services",
			"Service "+cur.Name+" is now "+cur.Status))
		return
	}
	if prev.Status != cur.Status {
		ev := events.New(events.TypeServiceStopped, events.SeverityWarning, "services",
			"Service "+cur.Name+" changed: "+prev.Status+" -> "+cur.Status)
		if cur.Status == "running" || cur.Status == "active" {
			ev = events.New(events.TypeServiceStarted, events.SeverityInfo, "services",
				"Service "+cur.Name+" is now "+cur.Status)
		}
		c.tee.Emit(ctx, ev)
	}
}

// refreshSync refreshes the discovery list on a slow cadence.
func (c *ServicesCollector) refreshSync(ctx context.Context) {
	now := time.Now()
	_, _, dsvcs := c.collectDocker(ctx, now)
	_, _, ssvcs := c.collectSystemd(ctx, now)
	c.persist(ctx, append(dsvcs, ssvcs...))
}

func (c *ServicesCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	now := time.Now()
	c.mu.Lock()
	stale := now.Sub(c.lastRefresh) > c.refresh
	c.lastRefresh = now
	c.mu.Unlock()
	if stale {
		c.refreshSync(ctx)
	}
	samples, _, svcs := c.collectDocker(ctx, now)
	c.persist(ctx, svcs)

	var countRunning, countTotal int
	c.mu.Lock()
	for _, s := range c.services {
		countTotal++
		if s.Status == "running" || s.Status == "active" {
			countRunning++
		}
	}
	c.mu.Unlock()
	samples = append(samples,
		metric.New("services.running", now, float64(countRunning)),
		metric.New("services.total", now, float64(countTotal)),
	)
	return samples, nil
}

// List returns the current service table.
func (c *ServicesCollector) List(ctx context.Context) ([]*store.Service, error) {
	return c.store.ListServices(ctx)
}

// Services satisfies ServiceSource.
func (c *ServicesCollector) Services(ctx context.Context) ([]*store.Service, error) {
	return c.List(ctx)
}

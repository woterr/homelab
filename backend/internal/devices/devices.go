// Package devices implements LAN device discovery. It combines passive
// observation of the ARP cache with a gentle, configurable ICMP sweep so that
// silent devices are also discovered. Everything is read-only: discovery only
// emits packets and reads tables; it never modifies the host.
package devices

import (
	"bufio"
	"context"
	"encoding/binary"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/store"
)

// Manager discovers and tracks LAN devices, persisting them and emitting
// device.joined / device.left events.
type Manager struct {
	store      *store.StateStore
	log        *log.Logger
	tee        *events.Tee
	proc       string
	sweepOn    bool
	sweepEvery time.Duration
	sweepRetry time.Duration

	mu       sync.Mutex
	devices  map[string]*store.Device // mac -> device
	gateway  string
	lastSeen map[string]time.Time
}

// Options configures discovery.
type Options struct {
	Proc       string
	Sweep      bool
	SweepEvery time.Duration
	SweepRetry time.Duration
}

func NewManager(st *store.StateStore, lg *log.Logger, tee *events.Tee, opts Options) *Manager {
	m := &Manager{
		store:      st,
		log:        lg,
		tee:        tee,
		proc:       opts.Proc,
		sweepOn:    opts.Sweep,
		sweepEvery: opts.SweepEvery,
		sweepRetry: opts.SweepRetry,
		devices:    map[string]*store.Device{},
		lastSeen:   map[string]time.Time{},
	}
	// Seed from persisted devices.
	if ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second); cancel != nil {
		defer cancel()
		if list, err := st.ListDevices(ctx); err == nil {
			for _, d := range list {
				m.devices[d.MAC] = d
				m.lastSeen[d.MAC] = d.LastSeen
			}
		}
	}
	return m
}

// SetGateway records the discovered default gateway so devices can be typed.
func (m *Manager) SetGateway(gw string) {
	m.mu.Lock()
	m.gateway = gw
	m.mu.Unlock()
}

// Run blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	sweepTick := time.NewTicker(m.sweepEvery)
	defer sweepTick.Stop()
	arpTick := time.NewTicker(20 * time.Second)
	defer arpTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-arpTick.C:
			m.pollARP(ctx)
		case <-sweepTick.C:
			if m.sweepOn {
				m.sweep(ctx)
			}
		}
	}
}

// seen is one discovered device observation.
type seen struct {
	ip    string
	mac   string
	iface string
}

func (m *Manager) pollARP(ctx context.Context) {
	f, err := os.Open(m.proc + "/net/arp")
	if err != nil {
		return
	}
	defer f.Close()
	var list []seen
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 6 {
			continue
		}
		ip, hw := fields[0], fields[3]
		if hw == "" || strings.EqualFold(hw, "00:00:00:00:00:00") || len(hw) != 17 {
			continue
		}
		list = append(list, seen{ip: ip, mac: strings.ToLower(hw), iface: fields[5]})
	}
	if sc.Err() != nil {
		return
	}
	m.observe(ctx, list)
}

// sweep performs a gentle ICMP echo sweep of the primary subnet. It uses
// unprivileged ICMP when possible and degrades to passive discovery otherwise.
func (m *Manager) sweep(ctx context.Context) {
	iface, cidr, err := m.primarySubnet()
	if err != nil || cidr == nil {
		return
	}
	hosts, err := subnetHosts(cidr)
	if err != nil || len(hosts) == 0 {
		return
	}
	sweepLAN(ctx, m, iface, hosts, m.sweepRetry)
}

func (m *Manager) primarySubnet() (string, *net.IPNet, error) {
	m.mu.Lock()
	gw := m.gateway
	m.mu.Unlock()
	if gw == "" {
		return "", nil, nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", nil, err
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			if ipn.Contains(net.ParseIP(gw)) {
				return i.Name, ipn, nil
			}
		}
	}
	return "", nil, nil
}

func subnetHosts(ipn *net.IPNet) ([]net.IP, error) {
	mask := ipn.Mask
	ones, bits := mask.Size()
	if ones == 0 || bits == 0 || ones > 30 {
		return nil, nil // avoid sweeping absurdly large networks
	}
	base := ipn.IP.To4()
	if base == nil {
		return nil, nil
	}
	start := binary.BigEndian.Uint32(base) & binary.BigEndian.Uint32(mask)
	end := start | ^binary.BigEndian.Uint32(mask)
	var out []net.IP
	for ip := start + 1; ip < end; ip++ {
		b := make(net.IP, 4)
		binary.BigEndian.PutUint32(b, ip)
		out = append(out, b)
	}
	return out, nil
}

// observe reconciles discovered observations with known devices, persisting
// changes and emitting join/leave events. DB writes happen outside the lock so
// the devices API (Snapshot) never blocks on SQLite contention.
func (m *Manager) observe(ctx context.Context, list []seen) {
	now := time.Now()

	type act struct {
		d       *store.Device
		ev      *events.Event
		persist bool
	}
	var acts []act

	m.mu.Lock()
	seenIPs := map[string]string{} // ip -> mac
	for _, s := range list {
		seenIPs[s.ip] = s.mac
	}

	online := map[string]bool{}
	for _, s := range list {
		d, ok := m.devices[s.mac]
		if !ok {
			d = &store.Device{
				MAC:        s.mac,
				IP:         s.ip,
				FirstSeen:  now,
				LastSeen:   now,
				Online:     true,
				Vendor:     OUI.Lookup(s.mac),
				DeviceType: m.inferType(s.mac, s.ip, ""),
				Metadata:   map[string]string{"interface": s.iface},
			}
			m.devices[s.mac] = d
			m.lastSeen[s.mac] = now
			online[s.mac] = true
			acts = append(acts, act{d: d, persist: true,
				ev: events.New(events.TypeDeviceJoined, events.SeverityInfo, "discovery",
					"New device detected: "+s.ip)})
			continue
		}
		wasOnline := d.Online
		d.Online = true
		d.LastSeen = now
		if d.IP != s.ip && s.ip != "" {
			d.IP = s.ip
		}
		if d.Metadata == nil {
			d.Metadata = map[string]string{}
		}
		d.Metadata["interface"] = s.iface
		m.lastSeen[s.mac] = now
		online[s.mac] = true
		if !wasOnline {
			acts = append(acts, act{d: d, persist: true,
				ev: events.New(events.TypeDeviceJoined, events.SeverityInfo, "discovery",
					"Device online: "+s.ip)})
		} else {
			acts = append(acts, act{d: d, persist: true})
		}
	}

	// Mark devices missing for > leaveGrace as offline.
	const leaveGrace = 4 * time.Minute
	for mac, d := range m.devices {
		if online[mac] {
			continue
		}
		last, ok := m.lastSeen[mac]
		if !ok {
			last = d.LastSeen
		}
		if d.Online && now.Sub(last) > leaveGrace {
			d.Online = false
			acts = append(acts, act{d: d, persist: true,
				ev: events.New(events.TypeDeviceLeft, events.SeverityInfo, "discovery",
					"Device offline: "+d.IP)})
		}
	}
	m.mu.Unlock()

	for _, a := range acts {
		if a.persist {
			m.persist(ctx, a.d)
		}
		if a.ev != nil {
			a.ev.Data = map[string]any{"mac": a.d.MAC, "ip": a.d.IP, "hostname": a.d.Hostname, "vendor": a.d.Vendor}
			m.tee.Emit(ctx, a.ev)
		}
	}
}

func (m *Manager) persist(ctx context.Context, d *store.Device) {
	if err := m.store.UpsertDevice(ctx, d); err != nil {
		m.log.Warn("device persist failed", "mac", d.MAC, "error", err)
	}
}

func (m *Manager) inferType(mac, ip, hostname string) string {
	m.mu.Lock()
	gw := m.gateway
	m.mu.Unlock()
	if gw != "" && ip == gw {
		return "router"
	}
	switch {
	case strings.Contains(strings.ToLower(hostname), "pi") || strings.Contains(strings.ToLower(hostname), "rasp"):
		return "server"
	case strings.Contains(strings.ToLower(hostname), "nas") || strings.Contains(strings.ToLower(hostname), "stash"):
		return "nas"
	case strings.Contains(strings.ToLower(hostname), "phone") || strings.Contains(strings.ToLower(hostname), "pixel"):
		return "mobile"
	}
	prefix := strings.ToUpper(strings.ReplaceAll(mac, ":", ""))[:6]
	switch {
	case OUI.Lookup(mac) == "Apple":
		return "mobile"
	case OUI.Lookup(mac) == "Google":
		return "mobile"
	case OUI.Lookup(mac) == "Samsung":
		return "mobile"
	case OUI.Lookup(mac) == "Raspberry Pi":
		return "server"
	case OUI.Lookup(mac) == "TP-Link" || OUI.Lookup(mac) == "Xiaomi":
		return "router"
	case OUI.Lookup(mac) == "Intel":
		return "laptop"
	}
	_ = prefix
	return "unknown"
}

// Snapshot returns the current device table.
func (m *Manager) Snapshot() []*store.Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*store.Device, 0, len(m.devices))
	for _, d := range m.devices {
		cp := *d
		out = append(out, &cp)
	}
	return out
}

// Devices satisfies collect.DeviceSource.
func (m *Manager) Devices() []*store.Device { return m.Snapshot() }

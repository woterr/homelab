package collect

import (
	"bufio"
	"context"
	"encoding/binary"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/metric"
	"homelab/backend/internal/util"
)

// NetInterface is a discovered network interface snapshot.
type NetInterface struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac"`
	MTU       int      `json:"mtu"`
	IPv4      []string `json:"ipv4"`
	IPv6      []string `json:"ipv6"`
	SpeedMbps int64    `json:"speed_mbps"`
	OperState string   `json:"operstate"`
	Carrier   bool     `json:"carrier"`
	Up        bool     `json:"up"`
	Primary   bool     `json:"primary"`

	RxBytes   uint64  `json:"rx_bytes"`
	TxBytes   uint64  `json:"tx_bytes"`
	RxPackets uint64  `json:"rx_packets"`
	TxPackets uint64  `json:"tx_packets"`
	RxErrors  uint64  `json:"rx_errors"`
	RxDrops   uint64  `json:"rx_drops"`
	TxErrors  uint64  `json:"tx_errors"`
	TxDrops   uint64  `json:"tx_drops"`
	RxBps     float64 `json:"rx_bps"`
	TxBps     float64 `json:"tx_bps"`
}

type netCounters struct {
	rxBytes, txBytes, rxPackets, txPackets uint64
	rxErrors, rxDrops, txErrors, txDrops   uint64
}

// NetworkCollector reports interface status, addresses, link and traffic.
type NetworkCollector struct {
	Host     Host
	mu       sync.Mutex
	prev     map[string]netCounters
	prevTime time.Time
	cur      []NetInterface
	Gateway  string
}

func NewNetworkCollector(h Host) *NetworkCollector { return &NetworkCollector{Host: h} }

func (c *NetworkCollector) Name() string { return "network" }

// primaryInterface returns the interface owning the default route, if any.
func (c *NetworkCollector) primaryInterface() (string, string) {
	f, err := os.Open(c.Host.Proc + "/net/route")
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		gw, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			return fields[0], ""
		}
		return fields[0], intToIP(uint32(gw))
	}
	return "", ""
}

func intToIP(v uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return net.IP(b).String()
}

func (c *NetworkCollector) readCounters() map[string]netCounters {
	out := map[string]netCounters{}
	f, err := os.Open(c.Host.Proc + "/net/dev")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		vals := strings.Fields(line[idx+1:])
		if len(vals) < 16 {
			continue
		}
		out[name] = netCounters{
			rxBytes:   atou(vals[0]),
			rxPackets: atou(vals[1]),
			rxErrors:  atou(vals[2]),
			rxDrops:   atou(vals[3]),
			txBytes:   atou(vals[8]),
			txPackets: atou(vals[9]),
			txErrors:  atou(vals[10]),
			txDrops:   atou(vals[11]),
		}
	}
	return out
}

func (c *NetworkCollector) snapshotInterfaces() ([]NetInterface, string) {
	primary, gw := c.primaryInterface()
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, gw
	}
	now := time.Now()
	c.mu.Lock()
	deltaSec := 5.0
	if !c.prevTime.IsZero() {
		if d := now.Sub(c.prevTime).Seconds(); d > 0 {
			deltaSec = d
		}
	}
	prev := c.prev
	counters := c.readCounters()
	c.prev = counters
	c.prevTime = now
	c.mu.Unlock()

	var out []NetInterface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		base := c.Host.Sys + "/class/net/" + iface.Name
		n := NetInterface{
			Name:      iface.Name,
			MAC:       iface.HardwareAddr.String(),
			MTU:       iface.MTU,
			OperState: util.ReadFirstLine(base + "/operstate"),
			Up:        iface.Flags&net.FlagUp != 0,
			Carrier:   util.ReadFirstLine(base+"/carrier") == "1",
			SpeedMbps: util.ParseIntOr(util.ReadFirstLine(base+"/speed"), 0),
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip := strings.Split(a.String(), "/")[0]
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if parsed.To4() != nil {
				n.IPv4 = append(n.IPv4, ip)
			} else {
				n.IPv6 = append(n.IPv6, ip)
			}
		}
		if cnt, ok := counters[iface.Name]; ok {
			n.RxBytes, n.TxBytes = cnt.rxBytes, cnt.txBytes
			n.RxPackets, n.TxPackets = cnt.rxPackets, cnt.txPackets
			n.RxErrors, n.RxDrops = cnt.rxErrors, cnt.rxDrops
			n.TxErrors, n.TxDrops = cnt.txErrors, cnt.txDrops
			if p, ok := prev[iface.Name]; ok && deltaSec > 0 {
				n.RxBps = float64(cnt.rxBytes-p.rxBytes) / deltaSec
				n.TxBps = float64(cnt.txBytes-p.txBytes) / deltaSec
			}
		}
		if iface.Name == primary {
			n.Primary = true
		}
		out = append(out, n)
	}
	return out, gw
}

func (c *NetworkCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	ifaces, gw := c.snapshotInterfaces()
	c.mu.Lock()
	c.cur = ifaces
	c.Gateway = gw
	c.mu.Unlock()

	now := time.Now()
	var out []metric.Sample
	primaryFound := false
	for _, n := range ifaces {
		labels := map[string]string{"interface": n.Name}
		out = append(out,
			metric.NewL("network.rx_bps", labels, now, n.RxBps),
			metric.NewL("network.tx_bps", labels, now, n.TxBps),
			metric.NewL("network.rx_packets", labels, now, float64(n.RxPackets)),
			metric.NewL("network.tx_packets", labels, now, float64(n.TxPackets)),
			metric.NewL("network.rx_errors", labels, now, float64(n.RxErrors)),
			metric.NewL("network.rx_drops", labels, now, float64(n.RxDrops)),
			metric.NewL("network.tx_errors", labels, now, float64(n.TxErrors)),
			metric.NewL("network.tx_drops", labels, now, float64(n.TxDrops)),
		)
		if n.Primary {
			primaryFound = true
			up := 0.0
			if n.Up && n.Carrier {
				up = 1
			}
			out = append(out,
				metric.New("network.link_up", now, up),
				metric.New("network.link_speed_mbps", now, float64(n.SpeedMbps)),
				metric.New("network.primary_rx_bps", now, n.RxBps),
				metric.New("network.primary_tx_bps", now, n.TxBps),
			)
		}
	}
	if !primaryFound && len(ifaces) > 0 {
		// Fall back to the first carrier-up interface as primary.
		for _, n := range ifaces {
			if n.Up && n.Carrier {
				up := 1.0
				out = append(out,
					metric.New("network.link_up", now, up),
					metric.New("network.link_speed_mbps", now, float64(n.SpeedMbps)),
					metric.New("network.primary_rx_bps", now, n.RxBps),
					metric.New("network.primary_tx_bps", now, n.TxBps),
				)
				break
			}
		}
	}
	return out, nil
}

// Snapshot returns interface state and the discovered gateway.
func (c *NetworkCollector) Snapshot() ([]NetInterface, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]NetInterface, len(c.cur))
	copy(out, c.cur)
	return out, c.Gateway
}

// Network satisfies NetworkSource.
func (c *NetworkCollector) Network() ([]NetInterface, string) { return c.Snapshot() }

package api

import (
	"net/http"

	"homelab/backend/internal/store"
)

// TopologyNode is a node in the network topology graph.
type TopologyNode struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Type    string  `json:"type"` // internet | router | server | nas | laptop | mobile | device
	IP      string  `json:"ip,omitempty"`
	MAC     string  `json:"mac,omitempty"`
	Vendor  string  `json:"vendor,omitempty"`
	Online  bool    `json:"online"`
	Primary bool    `json:"primary,omitempty"`
	RxBps   float64 `json:"rx_bps,omitempty"`
	TxBps   float64 `json:"tx_bps,omitempty"`
}

// TopologyLink is a connection between two nodes.
type TopologyLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // wan | lan | usb | gpio
}

// handleNetworkTopology builds the LAN graph from the device table, the
// primary interface and the discovered gateway.
func (s *Server) handleNetworkTopology(w http.ResponseWriter, r *http.Request) {
	devices := []*store.Device{}
	if s.sources.Devices != nil {
		devices = s.sources.Devices.Devices()
	}
	ifaces, gw := s.sources.Network.Network()
	info := s.sources.System.Info()

	var selfIP, selfMAC string
	for _, n := range ifaces {
		if n.Primary && len(n.IPv4) > 0 {
			selfIP = n.IPv4[0]
		}
		if n.Primary {
			selfMAC = n.MAC
		}
	}
	if selfIP == "" && info.Hostname != "" {
		// Demo host reports the primary interface directly.
		for _, n := range ifaces {
			if len(n.IPv4) > 0 && n.Up {
				selfIP = n.IPv4[0]
				break
			}
		}
	}

	nodes := []TopologyNode{
		{ID: "internet", Label: "Internet", Type: "internet", Online: true},
	}
	links := []TopologyLink{}

	// Router node from gateway.
	routerOnline := gw != ""
	nodes = append(nodes, TopologyNode{
		ID: "router", Label: "router", Type: "router", IP: gw, Online: routerOnline,
	})
	links = append(links, TopologyLink{Source: "internet", Target: "router", Type: "wan"})

	// Self node.
	nodes = append(nodes, TopologyNode{
		ID: "self", Label: info.Hostname, Type: "server", IP: selfIP, MAC: selfMAC,
		Online: true, Primary: true,
	})
	links = append(links, TopologyLink{Source: "router", Target: "self", Type: "lan"})

	// Discovered devices.
	seen := map[string]bool{}
	for _, d := range devices {
		if d.MAC == selfMAC || d.IP == selfIP || d.IP == gw {
			continue
		}
		if seen[d.MAC] {
			continue
		}
		seen[d.MAC] = true
		label := d.Hostname
		if label == "" {
			label = d.IP
		}
		ntype := d.DeviceType
		if ntype == "" || ntype == "unknown" {
			ntype = "device"
		}
		nodes = append(nodes, TopologyNode{
			ID: "dev_" + d.MAC, Label: label, Type: ntype, IP: d.IP, MAC: d.MAC,
			Vendor: d.Vendor, Online: d.Online,
		})
		links = append(links, TopologyLink{Source: "router", Target: "dev_" + d.MAC, Type: "lan"})
	}

	// Containers attach to the docker bridge interface when visible.
	for _, n := range ifaces {
		if n.Name == "docker0" || len(n.Name) >= 4 && n.Name[:4] == "br-" {
			nodes = append(nodes, TopologyNode{ID: "docker", Label: "Docker", Type: "server", Online: true})
			links = append(links, TopologyLink{Source: "self", Target: "docker", Type: "lan"})
			break
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"nodes":   nodes,
		"links":   links,
		"gateway": gw,
		"self_ip": selfIP,
	})
}

package devices

import (
	"context"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// sweepLAN sends ICMP echo requests to every host in the subnet and waits for
// replies. Replies populate the kernel neighbor/ARP table, which pollARP then
// observes to map IPs to MACs. It prefers unprivileged ICMP and falls back to
// a raw socket (needs NET_RAW / root); if neither is permitted it silently
// degrades to passive discovery.
func sweepLAN(ctx context.Context, m *Manager, ifaceName string, hosts []net.IP, retry time.Duration) {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	raw := false
	if err != nil {
		conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
		raw = true
		if err != nil {
			return // unprivileged and raw ICMP both unavailable
		}
	}
	defer conn.Close()

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: 1, Data: []byte("3p14-sweep")},
	}
	payload, err := msg.Marshal(nil)
	if err != nil {
		return
	}

	for _, h := range hosts {
		if err := ctx.Err(); err != nil {
			return
		}
		if raw {
			conn.WriteTo(payload, &net.IPAddr{IP: h})
		} else {
			conn.WriteTo(payload, &net.UDPAddr{IP: h})
		}
	}

	// Collect replies until the probe window elapses.
	deadline := time.Now().Add(retry)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetReadDeadline(deadline)
	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < 4 {
			continue
		}
		rm, err := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), buf[:n])
		if err != nil {
			continue
		}
		if rm.Type == ipv4.ICMPTypeEchoReply {
			// Replies populate the kernel neighbor table; the next pollARP pass
			// observes the new IP->MAC mappings.
			continue
		}
	}
}

func peerIP(peer net.Addr) string {
	return peer.String()
}

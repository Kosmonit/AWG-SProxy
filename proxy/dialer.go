package proxy

import (
	"context"
	"net"

	"github.com/amnezia-vpn/amneziawg-go/tun/netstack"
)

// TunnelDialer routes TCP connections through the AmneziaWG netstack.
// DNS resolution uses the DNS servers from the AWG config (same as the main client).
type TunnelDialer struct {
	Net *netstack.Net
}

func (d *TunnelDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network == "" {
		network = "tcp"
	}
	return d.Net.DialContext(ctx, network, address)
}

package p2p

import (
	"net"

	"github.com/ethereum/go-ethereum/log"
)

var rabbitCGNAT = &net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

// rabbitAdvertisableNATIP reports whether a NAT-reported external IP
// is suitable for publication in the node record.
func rabbitAdvertisableNATIP(ip net.IP) bool {
	if ip == nil ||
		ip.IsUnspecified() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil && rabbitCGNAT.Contains(ip4) {
		return false
	}
	return true
}

func (srv *Server) setRabbitStaticIP(ip net.IP) {
	if rabbitAdvertisableNATIP(ip) {
		srv.localnode.SetStaticIP(ip)
		return
	}
	log.Warn("Ignoring non-public NAT external IP", "ip", ip, "interface", srv.NAT)
}

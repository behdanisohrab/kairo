// Package netutil holds small helpers for working with network addresses.
package netutil

import "net"

// RemoteIPAddr pulls the IP out of an arbitrary net.Addr (typically a remote
// connection address).
func RemoteIPAddr(addr net.Addr) net.IP {
	if addr == nil {
		return nil
	}
	return ParseHostIP(addr.String())
}

// ParseHostIP pulls the IP out of "host" or "host:port".
func ParseHostIP(hostport string) net.IP {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return net.ParseIP(hostport)
	}
	return net.ParseIP(host)
}

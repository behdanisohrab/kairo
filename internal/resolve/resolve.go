// Package resolve implements upstream DNS forwarding and the default domain
// resolver used by the allowlist generator.
package resolve

import (
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/miekg/dns"
)

// Exchange asks each upstream until one answers. UDP first, TCP when truncated.
func Exchange(req *dns.Msg, upstreams []string) (*dns.Msg, error) {
	var lastErr error
	for _, up := range upstreams {
		resp, err := queryUpstream(req, up)
		if err == nil && resp != nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all upstreams failed: %w", lastErr)
}

func queryUpstream(req *dns.Msg, upstream string) (*dns.Msg, error) {
	udp := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	resp, _, err := udp.Exchange(req, upstream)
	if err == nil && resp != nil && !resp.Truncated {
		return resp, nil
	}
	tcp := &dns.Client{Net: "tcp", Timeout: 5 * time.Second}
	resp, _, err = tcp.Exchange(req, upstream)
	if err == nil && resp != nil {
		return resp, nil
	}
	return nil, err
}

// DefaultResolver resolves a domain through the upstreams, both families. This
// is the stock generator resolver; tests swap in a fake.
func DefaultResolver(upstreams []string) func(domain string) []net.IP {
	return func(domain string) []net.IP {
		var out []net.IP
		for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn(domain), qtype)
			resp, err := Exchange(req, upstreams)
			if err != nil {
				slog.Warn("ip-source: resolve failed", "domain", domain, "type", dns.TypeToString[qtype], "error", err)
				continue
			}
			for _, rr := range resp.Answer {
				switch a := rr.(type) {
				case *dns.A:
					out = append(out, a.A)
				case *dns.AAAA:
					out = append(out, a.AAAA)
				}
			}
		}
		return out
	}
}

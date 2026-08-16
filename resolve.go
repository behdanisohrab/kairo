package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/miekg/dns"
)

// exchange asks each upstream until one answers. UDP first, TCP when truncated.
func exchange(req *dns.Msg) (*dns.Msg, error) {
	var lastErr error
	for _, up := range cfg.Upstream {
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

// defaultResolver resolves a domain through the upstreams, both families. This
// is the stock generator resolver; tests swap in a fake.
func defaultResolver(domain string) []net.IP {
	var out []net.IP
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(domain), qtype)
		resp, err := exchange(req)
		if err != nil {
			log.Printf("ip-source: resolve %q (%s): %v", domain, dns.TypeToString[qtype], err)
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

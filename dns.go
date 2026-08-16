package main

import (
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"net"

	"github.com/miekg/dns"
)

// processQuery is the whole point of this binary. Restricted domain plus an
// allowlisted client: answer with our own IP and silence IPv6 so the traffic
// is forced over the SNI router. Anything else goes upstream.
func (s *State) processQuery(req *dns.Msg, clientIP net.IP) *dns.Msg {
	if req == nil || len(req.Question) == 0 {
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeServerFailure)
		return m
	}

	q := req.Question[0]
	name := normalizeDomain(q.Name)

	if s.isRestricted(name) && s.isAllowedIP(clientIP) {
		switch q.Qtype {
		case dns.TypeA:
			m := new(dns.Msg)
			m.SetReply(req)
			if ip := net.ParseIP(cfg.VPSIP); ip != nil {
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    cfg.TTL,
					},
					A: ip,
				})
			}
			return m
		case dns.TypeAAAA:
			m := new(dns.Msg)
			m.SetReply(req)
			m.Ns = append(m.Ns, noDataSOA(q.Name))
			return m
		}
	}

	resp, err := exchange(req)
	if err != nil {
		log.Printf("exchange %q: %v", name, err)
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeServerFailure)
		return m
	}
	return resp
}

// noDataSOA is the standard "nothing here, don't ask again" marker.
func noDataSOA(owner string) *dns.SOA {
	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   owner,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Ns:      "ns1.invalid.",
		Mbox:    "hostmaster.invalid.",
		Serial:  1,
		Refresh: 7200,
		Retry:   900,
		Expire:  86400,
		Minttl:  60,
	}
}

// ---------------------------------------------------------------------------
// Plain DNS (:53, UDP + TCP)
// ---------------------------------------------------------------------------

func (s *State) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	if !dnsLimiter.Allow() {
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(m)
		return
	}
	resp := s.processQuery(req, remoteIPAddr(w.RemoteAddr()))
	_ = w.WriteMsg(resp)
}

func StartDNS(listen string, s *State) {
	udp := &dns.Server{Addr: listen, Net: "udp", Handler: s}
	tcp := &dns.Server{Addr: listen, Net: "tcp", Handler: s}
	log.Printf("plain DNS (UDP) on %s", listen)
	log.Printf("plain DNS (TCP) on %s", listen)
	go func() {
		if err := udp.ListenAndServe(); err != nil {
			log.Fatalf("dns udp: %v", err)
		}
	}()
	go func() {
		if err := tcp.ListenAndServe(); err != nil {
			log.Fatalf("dns tcp: %v", err)
		}
	}()
}

// ---------------------------------------------------------------------------
// DNS-over-TLS (:853)
// ---------------------------------------------------------------------------

func StartDoT(s *State) {
	if cfg.TLS.Cert == "" || cfg.TLS.Key == "" {
		log.Printf("DoT on %s disabled (no TLS certificate configured)", cfg.Listen.DoT)
		return
	}
	cer, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key)
	if err != nil {
		log.Printf("DoT on %s disabled: %v", cfg.Listen.DoT, err)
		return
	}
	ln, err := tls.Listen("tcp", cfg.Listen.DoT, &tls.Config{
		Certificates: []tls.Certificate{cer},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		log.Fatalf("dot: %v", err)
	}
	log.Printf("DoT on %s", cfg.Listen.DoT)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleDoTConn(conn)
	}
}

// handleDoTConn serves one DoT connection. Queries are framed with a
// two-octet length prefix, per RFC 7858.
func (s *State) handleDoTConn(conn net.Conn) {
	defer conn.Close()

	if !dnsLimiter.Allow() {
		return
	}

	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return
	}
	length := binary.BigEndian.Uint16(lenBuf)
	if length == 0 {
		return
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(conn, raw); err != nil {
		return
	}

	var req dns.Msg
	if err := req.Unpack(raw); err != nil {
		return
	}

	resp := s.processQuery(&req, remoteIPAddr(conn.RemoteAddr()))
	out, err := resp.Pack()
	if err != nil {
		return
	}

	binary.BigEndian.PutUint16(lenBuf, uint16(len(out)))
	if _, err := conn.Write(lenBuf); err != nil {
		return
	}
	_, _ = conn.Write(out)
}

package server

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strconv"

	"github.com/miekg/dns"

	"kairo/internal/config"
	"kairo/internal/netutil"
	"kairo/internal/resolve"
)

// dnsServer adapts miekg/dns.Server to carry the owning Server.
type dnsServer struct {
	Server *Server
	Net    string
}

func (d *dnsServer) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	d.Server.serveDNS(w, req)
}

func (d *dnsServer) ListenAndServe(addr string) error {
	srv := &dns.Server{Addr: addr, Net: d.Net, Handler: d}
	return srv.ListenAndServe()
}

func (s *Server) serveDNS(w dns.ResponseWriter, req *dns.Msg) {
	if !s.dnsLimiter.Allow() {
		s.Metrics.DNSRateLimited.Inc()
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(m)
		return
	}
	resp := s.processQuery(req, netutil.RemoteIPAddr(w.RemoteAddr()))
	_ = w.WriteMsg(resp)
}

// processQuery is the whole point of this binary. Restricted domain plus an
// allowlisted client: answer with our own IP and silence IPv6 so the traffic
// is forced over the SNI router. Everyone else gets the upstream answer, so an
// unallowlisted client still resolves normally and simply is not routed.
func isAllowedForIP(s *Server, ip net.IP) bool {
	if ip == nil {
		return false
	}
	if s.st.IsAllowedIP(ip) {
		return true
	}
	if s.db != nil {
		if ok, _ := s.db.IsIPAllowlistedAny(ip.String()); ok {
			return true
		}
	}
	return false
}

func (s *Server) processQuery(req *dns.Msg, clientIP net.IP) *dns.Msg {
	if req == nil || len(req.Question) == 0 {
		s.recordDNS("", "error")
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeServerFailure)
		return m
	}

	q := req.Question[0]
	name := config.NormalizeDomain(q.Name)
	qtype := dns.TypeToString[q.Qtype]
	if qtype == "" {
		qtype = "TYPE" + strconv.Itoa(int(q.Qtype))
	}

	if s.st.IsRestricted(name) && isAllowedForIP(s, clientIP) {
		switch q.Qtype {
		case dns.TypeA:
			s.recordDNS(qtype, "split")
			m := new(dns.Msg)
			m.SetReply(req)
			if ip := net.ParseIP(s.cfg.VPSIP); ip != nil {
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    s.cfg.TTL,
					},
					A: ip,
				})
			}
			return m
		case dns.TypeAAAA:
			s.recordDNS(qtype, "split")
			m := new(dns.Msg)
			m.SetReply(req)
			m.Ns = append(m.Ns, noDataSOA(q.Name))
			return m
		}
	}

	resp, err := resolve.Exchange(req, s.cfg.Upstream)
	if err != nil {
		slog.Debug("dns exchange failed", "domain", name, "error", err)
		s.recordDNS(qtype, "error")
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeServerFailure)
		return m
	}
	s.recordDNS(qtype, "proxy")
	return resp
}

// recordDNS counts one answered query by record type and routing outcome.
func (s *Server) recordDNS(qtype, outcome string) {
	if s.Metrics != nil {
		s.Metrics.DNSQueries.WithLabelValues(qtype, outcome).Inc()
	}
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

// handleDoTConn serves one DoT connection. Queries are framed with a
// two-octet length prefix, per RFC 7858.
func (s *Server) handleDoTConn(conn net.Conn) {
	defer conn.Close()

	if !s.dnsLimiter.Allow() {
		s.Metrics.DNSRateLimited.Inc()
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

	resp := s.processQuery(&req, netutil.RemoteIPAddr(conn.RemoteAddr()))
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

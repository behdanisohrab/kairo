package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"kairo/internal/config"
	"kairo/internal/metrics"
	"kairo/internal/state"
)

func newTestServer(t *testing.T) (*Server, *state.State) {
	t.Helper()
	dir := t.TempDir()
	st, err := state.NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	m := metrics.New(func() int { return st.AllowedCount() }, func() int { return st.RestrictedCount() })
	srv := New(&config.Config{VPSIP: "203.0.113.10", TTL: 60}, st, "test", m)
	return srv, st
}

func TestProcessQuerySplitRouting(t *testing.T) {
	srv, st := newTestServer(t)
	if _, err := st.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}
	if _, err := st.AddRestricted("youtube.com"); err != nil {
		t.Fatalf("AddRestricted: %v", err)
	}

	newQuery := func(name string, qtype uint16) *dns.Msg {
		m := new(dns.Msg)
		m.SetQuestion(name, qtype)
		return m
	}

	allowed := net.ParseIP("198.51.100.7")
	disallowed := net.ParseIP("198.51.100.8")

	t.Run("allowlisted client gets VPS IP for A query", func(t *testing.T) {
		resp := srv.processQuery(newQuery("www.youtube.com.", dns.TypeA), allowed)
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("rcode = %v, want success", resp.Rcode)
		}
		if len(resp.Answer) != 1 {
			t.Fatalf("answers = %d, want 1", len(resp.Answer))
		}
		a, ok := resp.Answer[0].(*dns.A)
		if !ok {
			t.Fatalf("answer is %T, want *dns.A", resp.Answer[0])
		}
		if a.A.String() != srv.cfg.VPSIP {
			t.Errorf("A = %s, want %s", a.A, srv.cfg.VPSIP)
		}
	})

	t.Run("allowlisted client gets NODATA for AAAA query", func(t *testing.T) {
		resp := srv.processQuery(newQuery("youtube.com.", dns.TypeAAAA), allowed)
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("rcode = %v, want success", resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("answers = %d, want 0 (NODATA)", len(resp.Answer))
		}
	})

	t.Run("disallowed client never receives the VPS IP", func(t *testing.T) {
		// Point upstream at a dead resolver so the answer cannot be the VPS IP.
		srv.cfg.Upstream = []string{"127.0.0.1:9"}
		resp := srv.processQuery(newQuery("youtube.com.", dns.TypeA), disallowed)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("rcode = %v, want servfail", resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("disallowed client must not receive a VPS IP answer, got %d answers", len(resp.Answer))
		}
		resp = srv.processQuery(newQuery("google.com.", dns.TypeA), disallowed)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("non-restricted query rcode = %v, want servfail (upstream unreachable)", resp.Rcode)
		}
	})

	t.Run("loopback is always allowed", func(t *testing.T) {
		srv.cfg.Upstream = []string{"127.0.0.1:9"}
		resp := srv.processQuery(newQuery("youtube.com.", dns.TypeA), net.ParseIP("127.0.0.1"))
		if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
			t.Fatalf("loopback query failed: rcode=%v answers=%d", resp.Rcode, len(resp.Answer))
		}
		a, ok := resp.Answer[0].(*dns.A)
		if !ok || a.A.String() != srv.cfg.VPSIP {
			t.Fatalf("loopback answer = %v, want VPS IP", resp.Answer)
		}
	})

	t.Run("allowlisted client gets normal answer for non-restricted domain", func(t *testing.T) {
		srv.cfg.Upstream = []string{"127.0.0.1:9"}
		resp := srv.processQuery(newQuery("google.com.", dns.TypeA), allowed)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("rcode = %v, want servfail (upstream unreachable)", resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("non-restricted query must not be answered with the VPS IP")
		}
	})
}

func TestMetricsEndToEnd(t *testing.T) {
	srv, st := newTestServer(t)
	if _, err := st.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}
	if _, err := st.AddRestricted("youtube.com"); err != nil {
		t.Fatalf("AddRestricted: %v", err)
	}

	q := new(dns.Msg)
	q.SetQuestion("www.youtube.com.", dns.TypeA)
	srv.processQuery(q, net.ParseIP("198.51.100.7"))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(srv.Metrics.Reg, promhttp.HandlerOpts{}))
	go http.Serve(ln, mux)

	resp, err := http.Get("http://" + ln.Addr().String() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	want := []string{
		`kairo_dns_queries_total{outcome="split"`,
		`kairo_allowlisted_clients 1`,
		`kairo_restricted_domains 1`,
	}
	for _, m := range want {
		if !strings.Contains(body, m) {
			t.Errorf("metrics body missing %q", m)
		}
	}

	public := srv.BuildHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	public.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatalf("public handler must not serve /metrics, got 200")
	}
}

func TestMetricsEndpointWorksWithNilMetrics(t *testing.T) {
	dir := t.TempDir()
	st, err := state.NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	srv := New(&config.Config{VPSIP: "203.0.113.10", TTL: 60}, st, "test", nil)

	handler := srv.BuildHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/status?key=x", nil)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 401 or 429 (nil metrics must not panic)", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/dns-query", nil)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusTooManyRequests {
		t.Fatalf("dns-query status = %d, want 400 or 429 (nil metrics must not panic)", rr.Code)
	}
}

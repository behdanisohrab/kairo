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
	"kairo/internal/database"
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
	srv := New(&config.Config{VPSIP: "203.0.113.10", TTL: 60}, st, "test", m, nil)
	return srv, st
}

func TestProcessQuerySplitRouting(t *testing.T) {
	srv, st := newTestServer(t)
	if !st.AddAllowed(net.ParseIP("198.51.100.7")) {
		t.Fatal("AddAllowed: IP reported as not new")
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
	if !st.AddAllowed(net.ParseIP("198.51.100.7")) {
		t.Fatal("AddAllowed: IP reported as not new")
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
	srv := New(&config.Config{VPSIP: "203.0.113.10", TTL: 60}, st, "test", nil, nil)

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

// TestAllowAddsIPToCallerAccount pins the v0.3 contract: POST /api/allow with
// a user's API key stores the IP on THAT user's account (panel-visible), and
// DELETE removes it again. The legacy single admin key maps onto the real
// admin account row.
func TestAllowAddsIPToCallerAccount(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/kairo.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	bob, err := db.CreateUser("bob", "secret-password", "user")
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	if err := db.EnsureAdmin("admin", "secret-password"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	adminRow, err := db.GetUserByUsername("admin")
	if err != nil || adminRow == nil {
		t.Fatalf("resolve admin row: %v", err)
	}

	st, err := state.NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	srv := New(&config.Config{VPSIP: "203.0.113.10", TTL: 60, APIKey: "legacy-key"}, st, "test",
		metrics.New(func() int { return st.AllowedCount() }, func() int { return st.RestrictedCount() }), nil)
	srv.SetDB(db)
	handler := srv.BuildHandler()

	do := func(method, key, ip string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/allow?ip="+ip, nil)
		req.Header.Set("X-API-Key", key)
		handler.ServeHTTP(rr, req)
		return rr
	}
	rowsOf := func(id int) map[string]bool {
		rows, _ := db.ListUserIPs(id)
		out := map[string]bool{}
		for _, r := range rows {
			out[r.IP] = true
		}
		return out
	}

	// bob adds an IP with his own API key → lands in bob's account.
	if rr := do(http.MethodPost, bob.APIKey, "203.0.113.7"); rr.Code != http.StatusOK {
		t.Fatalf("bob POST status = %d body = %s, want 200", rr.Code, rr.Body.String())
	}
	if !rowsOf(bob.ID)["203.0.113.7"] {
		t.Errorf("IP missing from bob's panel-visible rows: %v", rowsOf(bob.ID))
	}
	if st.AllowedCount() != 1 {
		t.Errorf("cache size = %d, want 1", st.AllowedCount())
	}

	// Duplicate → conflict, no second row.
	if rr := do(http.MethodPost, bob.APIKey, "203.0.113.7"); rr.Code != http.StatusConflict {
		t.Errorf("duplicate POST status = %d, want 409", rr.Code)
	}

	// The legacy single admin key resolves to the real admin row.
	if rr := do(http.MethodPost, "legacy-key", "198.51.100.9"); rr.Code != http.StatusOK {
		t.Fatalf("legacy-key POST status = %d body = %s, want 200", rr.Code, rr.Body.String())
	}
	if !rowsOf(adminRow.ID)["198.51.100.9"] {
		t.Errorf("IP missing from the real admin account rows: %v", rowsOf(adminRow.ID))
	}

	// bob removes his IP; the cache entry survives while admin still holds his.
	if rr := do(http.MethodDelete, bob.APIKey, "203.0.113.7"); rr.Code != http.StatusOK {
		t.Fatalf("bob DELETE status = %d, want 200", rr.Code)
	}
	if len(rowsOf(bob.ID)) != 0 {
		t.Errorf("bob rows after delete = %v, want empty", rowsOf(bob.ID))
	}
	if st.AllowedCount() != 1 {
		t.Errorf("cache size = %d, want 1 while admin still holds an IP", st.AllowedCount())
	}
}

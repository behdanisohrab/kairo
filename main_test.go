package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"youtube.com":    "youtube.com",
		"  YouTube.COM":  "youtube.com",
		"YouTube.COM.":   "youtube.com",
		"...":            "",
		" ":              "",
		"  .google.com.": "google.com",
	}
	for in, want := range cases {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func newTestState(t *testing.T) *State {
	t.Helper()
	dir := t.TempDir()
	s, err := NewState(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	return s
}

func TestIsRestricted(t *testing.T) {
	s := newTestState(t)
	if _, err := s.AddRestricted("youtube.com"); err != nil {
		t.Fatalf("AddRestricted: %v", err)
	}

	match := []string{
		"youtube.com",
		"www.youtube.com",
		"a.b.youtube.com",
	}
	noMatch := []string{
		"google.com",
		"notyoutube.com",
		"youtube.com.evil.net",
		"evil-youtube.com",
		"",
	}

	for _, d := range match {
		if !s.isRestricted(d) {
			t.Errorf("isRestricted(%q) = false, want true", d)
		}
	}
	for _, d := range noMatch {
		if s.isRestricted(d) {
			t.Errorf("isRestricted(%q) = true, want false", d)
		}
	}
}

func TestIsAllowedIP(t *testing.T) {
	s := newTestState(t)
	if _, err := s.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}

	if !s.isAllowedIP(net.ParseIP("127.0.0.1")) {
		t.Error("loopback must always be allowed")
	}
	if !s.isAllowedIP(net.ParseIP("::1")) {
		t.Error("loopback must always be allowed")
	}
	if !s.isAllowedIP(net.ParseIP("198.51.100.7")) {
		t.Error("allowlisted IP must be allowed")
	}
	if s.isAllowedIP(net.ParseIP("198.51.100.8")) {
		t.Error("non-allowlisted IP must be denied")
	}
	if s.isAllowedIP(nil) {
		t.Error("nil IP must be denied")
	}
}

func TestProcessQuerySplitRouting(t *testing.T) {
	cfg = &Config{VPSIP: "203.0.113.10", TTL: 60}
	s := newTestState(t)
	if _, err := s.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}
	if _, err := s.AddRestricted("youtube.com"); err != nil {
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
		resp := s.processQuery(newQuery("www.youtube.com.", dns.TypeA), allowed)
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
		if a.A.String() != cfg.VPSIP {
			t.Errorf("A = %s, want %s", a.A, cfg.VPSIP)
		}
	})

	t.Run("allowlisted client gets NODATA for AAAA query", func(t *testing.T) {
		resp := s.processQuery(newQuery("youtube.com.", dns.TypeAAAA), allowed)
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("rcode = %v, want success", resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("answers = %d, want 0 (NODATA)", len(resp.Answer))
		}
	})

	t.Run("disallowed client never receives the VPS IP", func(t *testing.T) {
		// Point upstream at a dead resolver so the answer cannot be the VPS IP.
		cfg.Upstream = []string{"127.0.0.1:9"}
		resp := s.processQuery(newQuery("youtube.com.", dns.TypeA), disallowed)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("rcode = %v, want servfail", resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("disallowed client must not receive a VPS IP answer, got %d answers", len(resp.Answer))
		}
		resp = s.processQuery(newQuery("google.com.", dns.TypeA), disallowed)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("non-restricted query rcode = %v, want servfail (upstream unreachable)", resp.Rcode)
		}
	})

	t.Run("loopback is always allowed", func(t *testing.T) {
		cfg.Upstream = []string{"127.0.0.1:9"}
		resp := s.processQuery(newQuery("youtube.com.", dns.TypeA), net.ParseIP("127.0.0.1"))
		if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
			t.Fatalf("loopback query failed: rcode=%v answers=%d", resp.Rcode, len(resp.Answer))
		}
		a, ok := resp.Answer[0].(*dns.A)
		if !ok || a.A.String() != cfg.VPSIP {
			t.Fatalf("loopback answer = %v, want VPS IP", resp.Answer)
		}
	})

	t.Run("allowlisted client gets normal answer for non-restricted domain", func(t *testing.T) {
		cfg.Upstream = []string{"127.0.0.1:9"}
		resp := s.processQuery(newQuery("google.com.", dns.TypeA), allowed)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("rcode = %v, want servfail (upstream unreachable)", resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("non-restricted query must not be answered with the VPS IP")
		}
	})
}

func TestStatePersistence(t *testing.T) {
	dir := t.TempDir()

	s, err := NewState(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if _, err := s.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}
	if _, err := s.AddRestricted("youtube.com"); err != nil {
		t.Fatalf("AddRestricted: %v", err)
	}

	s2, err := NewState(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("reload NewState: %v", err)
	}
	if got := s2.AllowedList(); len(got) != 1 || got[0] != "198.51.100.7" {
		t.Errorf("reloaded allowlist = %v, want [198.51.100.7]", got)
	}
	if got := s2.RestrictedList(); len(got) != 1 || got[0] != "youtube.com" {
		t.Errorf("reloaded domains = %v, want [youtube.com]", got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, domainsFilename))
	if err != nil {
		t.Fatalf("read domains.txt: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "youtube.com" {
		t.Errorf("domains.txt = %q, want youtube.com", string(raw))
	}
}

func TestStateListFilesIgnoreComments(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, domainsFilename), []byte("# comment\n\nyoutube.com\n  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewState(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if got := s.RestrictedList(); len(got) != 1 || got[0] != "youtube.com" {
		t.Errorf("RestrictedList = %v, want [youtube.com]", got)
	}
}

func TestGenerateIPs(t *testing.T) {
	dir := t.TempDir()
	cfg = &Config{DataDir: dir}
	s, err := NewState(cfg)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	s.resolver = func(domain string) []net.IP {
		switch domain {
		case "router.home":
			return []net.IP{net.ParseIP("198.51.100.10"), net.ParseIP("198.51.100.10")}
		case "nas.home":
			return []net.IP{net.ParseIP("198.51.100.11")}
		default:
			return nil
		}
	}

	if err := os.WriteFile(s.IPSourcePath(), []byte("router.home\nnas.home\ndead.domain\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	added, failed, err := s.GenerateIPs()
	if err != nil {
		t.Fatalf("GenerateIPs: %v", err)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2 (deduplicated)", added)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}

	list := s.AllowedList()
	if len(list) != 2 {
		t.Fatalf("allowlist = %v, want 2 entries", list)
	}
	if list[0] != "198.51.100.10" || list[1] != "198.51.100.11" {
		t.Errorf("allowlist = %v, want [198.51.100.10 198.51.100.11]", list)
	}

	// Generation is idempotent: a second run adds nothing.
	added, _, err = s.GenerateIPs()
	if err != nil {
		t.Fatalf("GenerateIPs: %v", err)
	}
	if added != 0 {
		t.Errorf("second run added = %d, want 0", added)
	}
}

func TestGenerateIPsMissingFileIsNoOp(t *testing.T) {
	s := newTestState(t)
	added, failed, err := s.GenerateIPs()
	if err != nil {
		t.Fatalf("GenerateIPs: %v", err)
	}
	if added != 0 || failed != 0 {
		t.Errorf("added = %d, failed = %d, want 0 0", added, failed)
	}
}

func TestReadProxyHeader(t *testing.T) {
	t.Run("plain TLS bytes are not a PROXY header", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("\x16\x03\x01...tls"))
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip != nil {
			t.Errorf("ip = %v, want nil", ip)
		}
		// The bytes must still be readable after the no-op.
		rest, _ := io.ReadAll(r)
		if string(rest) != "\x16\x03\x01...tls" {
			t.Errorf("rest = %q, want original bytes", rest)
		}
	})

	t.Run("parses TCP4 header", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("PROXY TCP4 198.51.100.7 84.245.19.105 50210 443\r\nTLSBYTES"))
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip == nil || ip.String() != "198.51.100.7" {
			t.Errorf("ip = %v, want 198.51.100.7", ip)
		}
		rest, _ := io.ReadAll(r)
		if string(rest) != "TLSBYTES" {
			t.Errorf("rest = %q, want TLSBYTES", rest)
		}
	})

	t.Run("parses TCP6 header", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("PROXY TCP6 2001:db8::7 2001:db8::1 1234 443\r\n"))
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip == nil || ip.String() != "2001:db8::7" {
			t.Errorf("ip = %v, want 2001:db8::7", ip)
		}
	})

	t.Run("UNKNOWN form carries no address", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("PROXY UNKNOWN\r\n"))
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip != nil {
			t.Errorf("ip = %v, want nil", ip)
		}
	})

	for _, bad := range []string{"PROXY TCP4 nope 1.2.3.4 1 2\r\n", "PROXY TCP7 1.2.3.4 1.2.3.4 1 2\r\n", "PROXY TCP4 1.2.3.4 1.2.3.4 1\r\n"} {
		t.Run("rejects garbage "+bad[:20], func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(bad))
			if _, err := readProxyHeader(r); err == nil {
				t.Errorf("readProxyHeader(%q) should fail", bad)
			}
		})
	}
}

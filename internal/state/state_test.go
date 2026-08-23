package state

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kairo/internal/config"
)

func newTestState(t *testing.T) *State {
	t.Helper()
	dir := t.TempDir()
	s, err := NewState(&config.Config{DataDir: dir})
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

	match := []string{"youtube.com", "www.youtube.com", "a.b.youtube.com"}
	noMatch := []string{"google.com", "notyoutube.com", "youtube.com.evil.net", "evil-youtube.com", ""}
	for _, d := range match {
		if !s.IsRestricted(d) {
			t.Errorf("IsRestricted(%q) = false, want true", d)
		}
	}
	for _, d := range noMatch {
		if s.IsRestricted(d) {
			t.Errorf("IsRestricted(%q) = true, want false", d)
		}
	}
}

func TestIsAllowedIP(t *testing.T) {
	s := newTestState(t)
	s.AddAllowed(net.ParseIP("198.51.100.7"))
	if !s.IsAllowedIP(net.ParseIP("127.0.0.1")) {
		t.Error("loopback must always be allowed")
	}
	if !s.IsAllowedIP(net.ParseIP("::1")) {
		t.Error("loopback must always be allowed")
	}
	if !s.IsAllowedIP(net.ParseIP("198.51.100.7")) {
		t.Error("allowlisted IP must be allowed")
	}
	if s.IsAllowedIP(net.ParseIP("198.51.100.8")) {
		t.Error("non-allowlisted IP must be denied")
	}
	if s.IsAllowedIP(nil) {
		t.Error("nil IP must be denied")
	}
}

func TestDomainsPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if _, err := s.AddRestricted("youtube.com"); err != nil {
		t.Fatalf("AddRestricted: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, DomainsFilename))
	if err != nil {
		t.Fatalf("read domains.txt: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "youtube.com" {
		t.Errorf("domains.txt = %q, want youtube.com", string(raw))
	}

	s2, err := NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("reload NewState: %v", err)
	}
	if got := s2.RestrictedList(); len(got) != 1 || got[0] != "youtube.com" {
		t.Errorf("reloaded domains = %v, want [youtube.com]", got)
	}
}

// The 0.2.x-era allowed.txt is retired: it must not be read back into the
// cache, and mutations must not resurrect the file. Persistence now lives in
// the database and reaches the cache through SeedAllowed.
func TestAllowlistFileIsRetired(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "allowed.txt"), []byte("198.51.100.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if s.AllowedCount() != 0 {
		t.Errorf("legacy allowed.txt leaked into the cache: %v", s.AllowedList())
	}
	if _, err := os.Stat(filepath.Join(dir, "allowed.txt")); err != nil {
		t.Errorf("stat allowed.txt: %v", err)
	}
}

func TestSeedAllowed(t *testing.T) {
	s := newTestState(t)
	s.SeedAllowed([]string{"198.51.100.7", "2001:db8::1", "not-an-ip"})
	if got := s.AllowedList(); len(got) != 2 || got[0] != "198.51.100.7" || got[1] != "2001:db8::1" {
		t.Errorf("SeedAllowed cache = %v, want [198.51.100.7 2001:db8::1]", got)
	}
	if !s.IsAllowedIP(net.ParseIP("2001:db8::1")) {
		t.Error("seeded IPv6 address must be allowed")
	}
	// Re-seeding replaces rather than appends.
	s.SeedAllowed(nil)
	if s.AllowedCount() != 0 {
		t.Errorf("cache = %v, want empty after re-seed", s.AllowedList())
	}
}

func TestListFilesIgnoreComments(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DomainsFilename), []byte("# comment\n\nyoutube.com\n  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if got := s.RestrictedList(); len(got) != 1 || got[0] != "youtube.com" {
		t.Errorf("RestrictedList = %v, want [youtube.com]", got)
	}
}

func TestGenerateIPs(t *testing.T) {
	dir := t.TempDir()
	s, err := NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	s.Resolver = func(domain string) []net.IP {
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
	if len(list) != 2 || list[0] != "198.51.100.10" || list[1] != "198.51.100.11" {
		t.Errorf("allowlist = %v, want [198.51.100.10 198.51.100.11]", list)
	}
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

func TestGenerateIPsPersistsViaHook(t *testing.T) {
	s := newTestState(t)
	s.Resolver = func(string) []net.IP {
		return []net.IP{net.ParseIP("198.51.100.20")}
	}
	if err := os.WriteFile(s.IPSourcePath(), []byte("router.home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var persisted []string
	s.PersistIP = func(ip net.IP) error {
		persisted = append(persisted, ip.String())
		return nil
	}

	added, _, err := s.GenerateIPs()
	if err != nil {
		t.Fatalf("GenerateIPs: %v", err)
	}
	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}
	if len(persisted) != 1 || persisted[0] != "198.51.100.20" {
		t.Errorf("PersistIP calls = %v, want [198.51.100.20]", persisted)
	}

	// Idempotent: a second run neither adds nor persists again.
	persisted = nil
	added, _, err = s.GenerateIPs()
	if err != nil {
		t.Fatalf("second GenerateIPs: %v", err)
	}
	if added != 0 || len(persisted) != 0 {
		t.Errorf("second run: added = %d, persisted = %v, want 0 and none", added, persisted)
	}
}

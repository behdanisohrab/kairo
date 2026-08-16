package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	allowedFilename = "allowed.txt" // client IPs allowed to be split-routed
	domainsFilename = "domains.txt" // restricted domains
)

// State owns the two policy lists, persisted as plain text in the data dir and
// hot-reloaded whenever the files change on disk.
type State struct {
	mu           sync.RWMutex
	allowed      map[string]struct{}
	domains      map[string]struct{}
	allowedM     time.Time
	domainsM     time.Time
	allowedPath  string
	domainsPath  string
	ipSourcePath string

	// resolver is used by the IP generator; replaceable in tests.
	resolver func(domain string) []net.IP
}

func NewState(cfg *Config) (*State, error) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "."
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	ipSource := cfg.IPSource.DomainsFile
	if ipSource == "" {
		ipSource = "domain.txt"
	}
	if !filepath.IsAbs(ipSource) {
		ipSource = filepath.Join(dataDir, ipSource)
	}

	s := &State{
		allowed:      make(map[string]struct{}),
		domains:      make(map[string]struct{}),
		allowedPath:  filepath.Join(dataDir, allowedFilename),
		domainsPath:  filepath.Join(dataDir, domainsFilename),
		ipSourcePath: ipSource,
		resolver:     defaultResolver,
	}

	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads both policy files. Missing files are just an empty policy; a
// fresh data dir must not be a showstopper.
func (s *State) load() error {
	allowed, err := readStateFile(s.allowedPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load %s: %w", s.allowedPath, err)
	}
	domains, err := readStateFile(s.domainsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load %s: %w", s.domainsPath, err)
	}

	s.mu.Lock()
	s.allowed = normalizeIPSet(allowed)
	s.domains = normalizeDomainSet(domains)
	if info, err := os.Stat(s.allowedPath); err == nil {
		s.allowedM = info.ModTime()
	}
	if info, err := os.Stat(s.domainsPath); err == nil {
		s.domainsM = info.ModTime()
	}
	s.mu.Unlock()

	log.Printf("state: %d restricted domains, %d allowlisted clients", len(s.domains), len(s.allowed))
	return nil
}

// Watch reloads the policy files when they change. Polling is cheap and
// avoids another dependency; edits show up within a few seconds.
func (s *State) Watch(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reloadIfChanged()
		}
	}
}

func (s *State) reloadIfChanged() {
	s.reloadFile(s.allowedPath, &s.allowedM, "allowlist")
	s.reloadFile(s.domainsPath, &s.domainsM, "domains")
}

func (s *State) reloadFile(path string, mtime *time.Time, name string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.ModTime().Equal(*mtime) {
		return
	}
	lines, err := readStateFile(path)
	if err != nil {
		log.Printf("reload %s: %v", name, err)
		return
	}
	s.mu.Lock()
	switch name {
	case "allowlist":
		s.allowed = normalizeIPSet(lines)
	default:
		s.domains = normalizeDomainSet(lines)
	}
	*mtime = info.ModTime()
	count := len(lines)
	s.mu.Unlock()
	log.Printf("reloaded %s: %d entries", name, count)
}

// ---------------------------------------------------------------------------
// Allowlist
// ---------------------------------------------------------------------------

func (s *State) isAllowedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// local tools and health checks always pass
	if ip.IsLoopback() {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.allowed[ip.String()]
	return ok
}

func (s *State) AddAllowed(ip net.IP) (added bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ip.String()
	if _, ok := s.allowed[key]; ok {
		return false, nil
	}
	s.allowed[key] = struct{}{}
	return true, s.saveAllowed()
}

func (s *State) RemoveAllowed(ip net.IP) (removed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ip.String()
	if _, ok := s.allowed[key]; !ok {
		return false, nil
	}
	delete(s.allowed, key)
	return true, s.saveAllowed()
}

func (s *State) AllowedList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedKeys(s.allowed)
}

func (s *State) AllowedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.allowed)
}

func (s *State) IPSourcePath() string {
	return s.ipSourcePath
}

func (s *State) saveAllowed() error {
	return writeStateFile(s.allowedPath, sortedKeys(s.allowed))
}

// ---------------------------------------------------------------------------
// Restricted domains
// ---------------------------------------------------------------------------

// isRestricted matches a name and its parents, so restricting youtube.com
// also catches www.youtube.com and friends.
func (s *State) isRestricted(name string) bool {
	name = normalizeDomain(name)
	if name == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for {
		if _, ok := s.domains[name]; ok {
			return true
		}
		idx := strings.IndexByte(name, '.')
		if idx < 0 {
			return false
		}
		name = name[idx+1:]
	}
}

func (s *State) AddRestricted(domain string) (added bool, err error) {
	domain = normalizeDomain(domain)
	if domain == "" {
		return false, fmt.Errorf("empty domain")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[domain]; ok {
		return false, nil
	}
	s.domains[domain] = struct{}{}
	return true, s.saveDomains()
}

func (s *State) RemoveRestricted(domain string) (removed bool, err error) {
	domain = normalizeDomain(domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[domain]; !ok {
		return false, nil
	}
	delete(s.domains, domain)
	return true, s.saveDomains()
}

func (s *State) RestrictedList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedKeys(s.domains)
}

func (s *State) RestrictedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.domains)
}

func (s *State) saveDomains() error {
	return writeStateFile(s.domainsPath, sortedKeys(s.domains))
}

// ---------------------------------------------------------------------------
// IP generation from the ip_source file
// ---------------------------------------------------------------------------

// GenerateIPs resolves every domain in the ip source file and merges the
// addresses into the allowlist. Merging (not replacing) means a temporarily
// dead domain does not drop working clients. Idempotent on repeat runs.
func (s *State) GenerateIPs() (added int, failed int, err error) {
	domains, err := readStateFile(s.ipSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	var resolved []net.IP
	for _, d := range domains {
		ips := s.resolver(d)
		if len(ips) == 0 {
			failed++
			log.Printf("ip-source: no addresses for %q", d)
			continue
		}
		resolved = append(resolved, ips...)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ip := range resolved {
		if ip.IsLoopback() {
			continue
		}
		key := ip.String()
		if _, ok := s.allowed[key]; ok {
			continue
		}
		s.allowed[key] = struct{}{}
		added++
	}
	if added > 0 {
		if err := s.saveAllowed(); err != nil {
			return added, failed, err
		}
	}
	return added, failed, nil
}

func (s *State) RunGenerator(ctx context.Context) {
	if cfg.IPSource.Interval <= 0 {
		return
	}
	ticker := time.NewTicker(time.Duration(cfg.IPSource.Interval) * time.Second)
	defer ticker.Stop()
	log.Printf("ip-source: generating allowlist every %ds from %s", cfg.IPSource.Interval, s.ipSourcePath)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			added, failed, err := s.GenerateIPs()
			if err != nil {
				log.Printf("ip-source: %v", err)
				continue
			}
			log.Printf("ip-source: %d new addresses, %d unresolved", added, failed)
		}
	}
}

// ---------------------------------------------------------------------------
// File helpers
// ---------------------------------------------------------------------------

// readStateFile reads a list file, skipping blank lines and # comments.
func readStateFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, scanner.Err()
}

func writeStateFile(path string, list []string) error {
	var sb strings.Builder
	for _, v := range list {
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	return atomicWrite(path, []byte(sb.String()))
}

// atomicWrite writes to a temp file and renames, so a crash mid-write never
// leaves a half-baked policy file behind.
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func normalizeIPSet(list []string) map[string]struct{} {
	out := make(map[string]struct{}, len(list))
	for _, v := range list {
		if ip := net.ParseIP(v); ip != nil {
			out[ip.String()] = struct{}{}
		}
	}
	return out
}

func normalizeDomainSet(list []string) map[string]struct{} {
	out := make(map[string]struct{}, len(list))
	for _, v := range list {
		if d := normalizeDomain(v); d != "" {
			out[d] = struct{}{}
		}
	}
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Package config defines Kairo's runtime configuration (a single YAML file),
// plus loading, validation, migration and sample-config generation. The
// routing policy lives in separate plain-text files, owned by the state package.
package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds every runtime setting for Kairo. The routing policy is kept in
// separate text files under data_dir (see the state package), not here, so the
// API can edit it freely without rewriting the config.
type Config struct {
	Host        string       `yaml:"host"`         // public hostname for DoH and the API (matched via SNI)
	HostBackend string       `yaml:"host_backend"` // where host SNI goes when we are not terminating TLS
	VPSIP       string       `yaml:"vps_ip"`       // public IP of this box, handed out for restricted domains
	APIKey      string       `yaml:"api_key"`      // shared secret for /api/*, keep it long and ugly
	Upstream    []string     `yaml:"upstream_dns"`
	Listen      ListenConfig `yaml:"listen"`
	TLS         TLSConfig    `yaml:"tls"`
	DataDir     string       `yaml:"data_dir"`
	TTL         uint32       `yaml:"ttl"` // TTL for the fake A records
	Rate        RateConfig   `yaml:"rate"`
	IPSource    IPSource     `yaml:"ip_source"`

	// ProxyProtocol makes the SNI router trust the client address in a PROXY
	// protocol v1 header, but only from loopback peers (a local nginx). Use it
	// when nginx fronts :443 and forwards unknown SNIs to us, otherwise the
	// allowlist gate would see nginx's own address instead of the client's.
	ProxyProtocol bool `yaml:"proxy_protocol"`
}

// IPSource feeds the allowlist generator: resolve the domains in DomainsFile,
// allowlist the results, repeat every Interval seconds. Zero disables the
// background job.
type IPSource struct {
	DomainsFile string `yaml:"domains_file"`
	Interval    int    `yaml:"interval"`
}

type ListenConfig struct {
	DNS     string `yaml:"dns"`
	DoT     string `yaml:"dot"`
	HTTPS   string `yaml:"https"` // the SNI router: DoH, API and the tunnel
	HTTP    string `yaml:"http"`  // loopback backend for reverse proxies
	Metrics string `yaml:"metrics"` // Prometheus endpoint, loopback-only by default
}

type TLSConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// RateConfig is a blunt instrument: global per-second limits for DNS and API.
type RateConfig struct {
	DNS      int `yaml:"dns"`
	DNSBurst int `yaml:"dns_burst"`
	API      int `yaml:"api"`
	APIBurst int `yaml:"api_burst"`
}

func DefaultConfig() *Config {
	return &Config{
		Upstream: []string{"1.1.1.1:53", "8.8.8.8:53"},
		Listen: ListenConfig{
			DNS:     ":53",
			DoT:     ":853",
			HTTPS:   ":443",
			HTTP:    "127.0.0.1:8080",
			Metrics: "127.0.0.1:9090",
		},
		DataDir: ".",
		TTL:     300,
		Rate: RateConfig{
			DNS:      200,
			DNSBurst: 400,
			API:      20,
			APIBurst: 40,
		},
	}
}

// Load reads the YAML config file and complains loudly when it is nonsense.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := NormalizeAndValidate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// NormalizeAndValidate normalizes the user's values, fills any fallback
// defaults, and checks that the config can actually run.
func NormalizeAndValidate(cfg *Config) error {
	cfg.Host = NormalizeDomain(cfg.Host)
	cfg.HostBackend = strings.TrimSpace(cfg.HostBackend)
	cfg.VPSIP = strings.TrimSpace(cfg.VPSIP)
	cfg.IPSource.DomainsFile = strings.TrimSpace(cfg.IPSource.DomainsFile)
	if cfg.IPSource.Interval < 0 {
		cfg.IPSource.Interval = 0
	}

	if net.ParseIP(cfg.VPSIP) == nil {
		return fmt.Errorf("invalid vps_ip %q", cfg.VPSIP)
	}
	if len(cfg.Upstream) == 0 {
		return fmt.Errorf("upstream_dns must not be empty")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("api_key must not be empty")
	}
	if cfg.TTL == 0 {
		cfg.TTL = 300
	}
	if cfg.Rate.DNS <= 0 {
		cfg.Rate.DNS = 200
	}
	if cfg.Rate.DNSBurst <= 0 {
		cfg.Rate.DNSBurst = 400
	}
	if cfg.Rate.API <= 0 {
		cfg.Rate.API = 20
	}
	if cfg.Rate.APIBurst <= 0 {
		cfg.Rate.APIBurst = 40
	}
	return nil
}

// NormalizeDomain strips the noise off a domain: lowercase, no trailing dots.
func NormalizeDomain(d string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
}

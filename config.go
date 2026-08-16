package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Config holds every runtime setting for Kairo.
type Config struct {
	Host        string       `json:"host"`         // public hostname for DoH and the API (matched via SNI)
	HostBackend string       `json:"host_backend"` // where host SNI goes when we are not terminating TLS
	VPSIP       string       `json:"vps_ip"`       // public IP of this box, handed out for restricted domains
	APIKey      string       `json:"api_key"`      // shared secret for /api/*, keep it long and ugly
	Upstream    []string     `json:"upstream_dns"`
	IPSource    IPSource     `json:"ip_source"` // the allowlist-from-domain.txt generator
	Listen      ListenConfig `json:"listen"`
	TLS         TLSConfig    `json:"tls"`
	DataDir     string       `json:"data_dir"`
	TTL         uint32       `json:"ttl"` // TTL for the fake A records
	Rate        RateConfig   `json:"rate"`

	// ProxyProtocol makes the SNI router trust the client address in a PROXY
	// protocol v1 header, but only from loopback peers (a local nginx). Use it
	// when nginx fronts :443 and forwards unknown SNIs to us, otherwise the
	// allowlist gate would see nginx's own address instead of the client's.
	ProxyProtocol bool `json:"proxy_protocol"`
}

// IPSource feeds the allowlist generator: resolve these domains, allowlist the
// results, repeat every Interval seconds. Zero disables the background job.
type IPSource struct {
	DomainsFile string `json:"domains_file"`
	Interval    int    `json:"interval"`
}

type ListenConfig struct {
	DNS   string `json:"dns"`
	DoT   string `json:"dot"`
	HTTPS string `json:"https"` // the SNI router: DoH, API and the tunnel
	HTTP  string `json:"http"`  // loopback backend for reverse proxies
}

type TLSConfig struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// RateConfig is a blunt instrument: global per-second limits for DNS and API.
type RateConfig struct {
	DNS      int `json:"dns"`
	DNSBurst int `json:"dns_burst"`
	API      int `json:"api"`
	APIBurst int `json:"api_burst"`
}

func DefaultConfig() *Config {
	return &Config{
		Upstream: []string{"1.1.1.1:53", "8.8.8.8:53"},
		Listen: ListenConfig{
			DNS:   ":53",
			DoT:   ":853",
			HTTPS: ":443",
			HTTP:  "127.0.0.1:8080",
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

// LoadConfig reads config.json and complains loudly when it is nonsense.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := applyDefaultsAndValidate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyDefaultsAndValidate normalizes the user's values, fills any fallback
// defaults, and checks that the config can actually run.
func applyDefaultsAndValidate(cfg *Config) error {
	cfg.Host = normalizeDomain(cfg.Host)
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

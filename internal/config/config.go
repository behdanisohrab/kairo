// Package config defines Kairo's runtime configuration (a single YAML file),
// plus loading, validation, migration and sample-config generation. The
// routing policy lives in separate plain-text files, owned by the state package.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
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
	ACME        ACMEConfig   `yaml:"acme"`
	DataDir     string       `yaml:"data_dir"`
	TTL         uint32       `yaml:"ttl"` // TTL for the fake A records
	Rate        RateConfig   `yaml:"rate"`
	IPSource    IPSource     `yaml:"ip_source"`
	AdminURL    string       `yaml:"admin_url"` // public URL for admin panel, e.g. https://panel.example.com
	DoHURL      string       `yaml:"doh_url"`   // public URL for DoH, e.g. https://dns.example.com/dns-query

	// ProxyProtocol makes the SNI router trust the client address in a PROXY
	// protocol v1 header, but only from loopback peers (a local nginx). Use it
	// when nginx fronts :443 and forwards unknown SNIs to us, otherwise the
	// allowlist gate would see nginx's own address instead of the client's.
	ProxyProtocol bool `yaml:"proxy_protocol"`

	// Admin password for web UI login. Falls back to api_key when empty.
	AdminPassword string `yaml:"admin_password"`
	// WebDir is the path to the built frontend files (served as static files).
	WebDir string `yaml:"web_dir"`
	// SessionTTL is the session lifetime in hours. Defaults to 24.
	SessionTTL int `yaml:"session_ttl"`
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

// ACMEConfig drives automatic certificate management through Let's Encrypt
// (via github.com/go-acme/lego) with the http-01 challenge. Leave email empty
// to disable ACME and fall back to tls.cert/tls.key.
type ACMEConfig struct {
	// Email is the address used to register the ACME account.
	Email string `yaml:"email"`
	// Storage is where the account key and issued certificate are kept.
	// Defaults to <data_dir>/certs when empty.
	Storage string `yaml:"storage"`
	// Directory is the ACME directory URL. Empty means Let's Encrypt production.
	Directory string `yaml:"directory"`
	// RenewBeforeDays renews the certificate when it expires within this many
	// days. Defaults to 30.
	RenewBeforeDays int `yaml:"renew_before_days"`
	// HTTPListen is the address the http-01 challenge is served on. It must be
	// reachable from the internet on port 80. Defaults to ":80".
	HTTPListen string `yaml:"http_listen"`
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
		ACME: ACMEConfig{
			RenewBeforeDays: 30,
			HTTPListen:      ":80",
		},
		WebDir:      "./web/dist",
		SessionTTL:  24,
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
	cfg.AdminURL = strings.TrimSpace(cfg.AdminURL)
	cfg.DoHURL = strings.TrimSpace(cfg.DoHURL)
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

	// Validate admin_url and doh_url if set
	if cfg.AdminURL != "" {
		u, err := url.Parse(cfg.AdminURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("invalid admin_url %q: must be http(s) URL", cfg.AdminURL)
		}
	}
	if cfg.DoHURL != "" {
		u, err := url.Parse(cfg.DoHURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("invalid doh_url %q: must be http(s) URL", cfg.DoHURL)
		}
	}
	// Derive defaults from host if not set
	if cfg.AdminURL == "" && cfg.Host != "" {
		cfg.AdminURL = "https://" + cfg.Host + "/"
	}
	if cfg.DoHURL == "" && cfg.Host != "" {
		cfg.DoHURL = "https://" + cfg.Host + "/dns-query"
	}

	if cfg.ACME.Email != "" {
		if cfg.Host == "" {
			return fmt.Errorf("acme.email requires host")
		}
		if cfg.TLS.Cert != "" || cfg.TLS.Key != "" {
			return fmt.Errorf("configure either acme.email (automatic certificates) or tls.cert/tls.key (static certificates), not both")
		}
		if cfg.ACME.Storage == "" {
			cfg.ACME.Storage = filepath.Join(cfg.DataDir, "certs")
		}
		if cfg.ACME.HTTPListen == "" {
			cfg.ACME.HTTPListen = ":80"
		}
		if cfg.ACME.RenewBeforeDays <= 0 {
			cfg.ACME.RenewBeforeDays = 30
		}
	}

	if cfg.AdminPassword == "" {
		cfg.AdminPassword = cfg.APIKey
	}
	if cfg.WebDir == "" {
		cfg.WebDir = "./web/dist"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24
	}

	return nil
}

// NormalizeDomain strips the noise off a domain: lowercase, no trailing dots.
func NormalizeDomain(d string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
}

// EffectiveAdminURL returns the configured admin panel URL or empty if not set.
func (c *Config) EffectiveAdminURL() string { return c.AdminURL }

// EffectiveDoHURL returns the configured DoH URL or empty if not set.
func (c *Config) EffectiveDoHURL() string { return c.DoHURL }

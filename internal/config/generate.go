package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GenerateConfigs writes a ready-to-edit config tree into a directory: a
// config.yaml plus the policy text files under data/. Usage:
// kairo generate config [dir]
func GenerateConfigs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kairo generate config [dir]")
	}
	if args[0] != "config" {
		return fmt.Errorf("unknown generate kind %q (only 'config' is supported)", args[0])
	}

	dir := "."
	if len(args) > 1 {
		dir = args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}

	files := map[string]string{
		"config.yaml":                        "",
		filepath.Join("data", "domains.txt"): "# Restricted domains, one per line. Subdomains are covered automatically.\n",
		filepath.Join("data", "domain.txt"):  "# Domains whose addresses should be allowlisted. Run 'kairo gen-ips' after editing.\n",
		filepath.Join("data", "allowed.txt"): "# Allowlisted client IPs, one per line. Managed via /api/allow and the generator.\n",
	}
	key, err := newAPIKey()
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}
	files["config.yaml"] = string(defaultConfigYAML(key))
	for name, content := range files {
		path := filepath.Join(dir, name)
		if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
			return fmt.Errorf("write %s: %w", path, werr)
		}
	}

	fmt.Printf("wrote %s and the policy files under %s\n", filepath.Join(dir, "config.yaml"), dataDir)
	fmt.Println("edit config.yaml (host, vps_ip, acme.email), then run: kairo run")
	return nil
}

// defaultConfigYAML builds the sample configuration with the given API key. It
// mirrors what ships in the repo so generated and example configs stay in sync.
func defaultConfigYAML(apiKey string) []byte {
	cfg := Config{
		Host:        "dns.example.com",
		HostBackend: "",
		AdminURL:    "https://dns.example.com/",
		DoHURL:      "https://dns.example.com/dns-query",
		VPSIP:       "1.2.3.4",
		APIKey:      apiKey,
		Upstream:    []string{"1.1.1.1:53", "8.8.8.8:53"},
		Listen: ListenConfig{
			DNS:     ":53",
			DoT:     ":853",
			HTTPS:   ":443",
			HTTP:    "127.0.0.1:8080",
			Metrics: "127.0.0.1:9090",
		},
		TLS:     TLSConfig{Cert: "/etc/letsencrypt/live/dns.example.com/fullchain.pem", Key: "/etc/letsencrypt/live/dns.example.com/privkey.pem"},
		ACME:    ACMEConfig{Email: "", Storage: "data/certs", Directory: "", RenewBeforeDays: 30, HTTPListen: ":80"},
		DataDir: "data",
		TTL:     300,
		Rate: RateConfig{
			DNS:      200,
			DNSBurst: 400,
			API:      20,
			APIBurst: 40,
		},
		IPSource: IPSource{DomainsFile: "domain.txt", Interval: 300},
	}
	b, _ := yaml.Marshal(cfg)
	return b
}

// newAPIKey returns a random 32-byte secret hex-encoded (64 chars), drawn from
// the operating system's crypto-secure random source.
func newAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

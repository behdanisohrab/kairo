package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// generateConfigs writes a ready-to-edit config tree into a directory. First
// time setup is boring enough; the point is you get valid files to tweak
// instead of a blank page. Usage: kairo --generate config [dir]
func generateConfigs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kairo --generate config [dir]")
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
		"config.json":                        string(defaultConfigJSON()),
		filepath.Join("data", "domains.txt"): "# Restricted domains, one per line. Subdomains are covered automatically.\n",
		filepath.Join("data", "domain.txt"):  "# Domains whose addresses should be allowlisted. Run 'kairo -gen-ips' after editing.\n",
		filepath.Join("data", "allowed.txt"): "# Allowlisted client IPs, one per line. Managed via /api/allow and the generator.\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	fmt.Printf("wrote %s and the policy files under %s\n", filepath.Join(dir, "config.json"), dataDir)
	fmt.Println("edit config.json (host, vps_ip, api_key, tls paths), then run: kairo -config config.json")
	return nil
}

// defaultConfigJSON builds the sample configuration. It mirrors what ships in
// the repo so generated and example configs stay in sync.
func defaultConfigJSON() []byte {
	cfg := Config{
		Host:        "dns.example.com",
		HostBackend: "",
		VPSIP:       "1.2.3.4",
		APIKey:      "REPLACE_WITH_A_STRONG_SECRET",
		Upstream:    []string{"1.1.1.1:53", "8.8.8.8:53"},
		IPSource:    IPSource{DomainsFile: "domain.txt", Interval: 300},
		Listen: ListenConfig{
			DNS:   ":53",
			DoT:   ":853",
			HTTPS: ":443",
			HTTP:  "127.0.0.1:8080",
		},
		TLS:     TLSConfig{Cert: "/etc/letsencrypt/live/dns.example.com/fullchain.pem", Key: "/etc/letsencrypt/live/dns.example.com/privkey.pem"},
		DataDir: "data",
		TTL:     300,
		Rate: RateConfig{
			DNS:      200,
			DNSBurst: 400,
			API:      20,
			APIBurst: 40,
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

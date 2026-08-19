package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeConfig(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func readConfigMap(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	m := make(map[string]interface{})
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return m
}

func TestMigrateAddsDefaults(t *testing.T) {
	// A config from an old release: no proxy_protocol, no rate section, and a
	// listen section that only knows about dns.
	old := `vps_ip: "1.2.3.4"
api_key: "secret"
upstream_dns:
  - 1.1.1.1
listen:
  dns: "127.0.0.1:53"
data_dir: "data"
`
	path := writeConfig(t, old)
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	m := readConfigMap(t, path)

	if v, ok := m["proxy_protocol"]; !ok || v != false {
		t.Errorf("proxy_protocol = %v, want false", v)
	}
	if _, ok := m["rate"]; !ok {
		t.Errorf("rate section missing after migrate")
	}
	if v, ok := m["vps_ip"]; !ok || v != "1.2.3.4" {
		t.Errorf("vps_ip = %v, want preserved 1.2.3.4", v)
	}
	if v, ok := m["api_key"]; !ok || v != "secret" {
		t.Errorf("api_key = %v, want preserved", v)
	}

	listen, ok := m["listen"].(map[string]interface{})
	if !ok {
		t.Fatalf("listen not an object: %v", m["listen"])
	}
	for _, key := range []string{"dot", "https", "http"} {
		if _, ok := listen[key]; !ok {
			t.Errorf("listen.%s missing after migrate", key)
		}
	}
	if v, ok := listen["dns"]; !ok || v != "127.0.0.1:53" {
		t.Errorf("listen.dns = %v, want preserved 127.0.0.1:53", v)
	}
}

func TestMigrateNeverAddsIdentityKeys(t *testing.T) {
	path := writeConfig(t, `vps_ip: "1.2.3.4"
api_key: "secret"
upstream_dns:
  - 1.1.1.1
`)
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	m := readConfigMap(t, path)
	for _, key := range []string{"host", "host_backend"} {
		if _, ok := m[key]; ok {
			t.Errorf("migrate added identity key %q, want it left alone", key)
		}
	}
}

func TestMigratePreservesUnknownKeys(t *testing.T) {
	path := writeConfig(t, `vps_ip: "1.2.3.4"
api_key: "secret"
upstream_dns:
  - 1.1.1.1
extra_setting: hello
`)
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	m := readConfigMap(t, path)
	if v, ok := m["extra_setting"]; !ok || v != "hello" {
		t.Errorf("unknown key extra_setting lost after migrate: %v", m["extra_setting"])
	}
}

func TestMigrateAlreadyUpToDate(t *testing.T) {
	path := writeConfig(t, `vps_ip: "1.2.3.4"
api_key: "secret"
upstream_dns:
  - 1.1.1.1
`)
	if err := Migrate(path); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := Migrate(path); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("no-op migrate rewrote the file")
	}
}

func TestMigrateRejectsInvalid(t *testing.T) {
	// Empty api_key must not survive migration; the file must be left alone.
	path := writeConfig(t, `vps_ip: "1.2.3.4"
api_key: ""
upstream_dns:
  - 1.1.1.1
`)
	if err := Migrate(path); err == nil {
		t.Fatalf("Migrate accepted an invalid config")
	}
	if m := readConfigMap(t, path); m["api_key"] != "" {
		t.Errorf("invalid config was written out: %v", m)
	}
}

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
		if got := NormalizeDomain(in); got != want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

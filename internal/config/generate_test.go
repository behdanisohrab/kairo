package config

import (
	"strings"
	"testing"
)

func TestNewAPIKey(t *testing.T) {
	key, err := newAPIKey()
	if err != nil {
		t.Fatalf("newAPIKey: %v", err)
	}
	if len(key) != 64 {
		t.Errorf("key length = %d, want 64 (32 random bytes hex-encoded)", len(key))
	}
	if strings.ContainsAny(key, "\n \t") {
		t.Errorf("key contains whitespace: %q", key)
	}

	other, err := newAPIKey()
	if err != nil {
		t.Fatalf("newAPIKey: %v", err)
	}
	if key == other {
		t.Errorf("two generated keys are identical; expected random values")
	}
}

func TestGenerateConfigsWritesSecureKey(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateConfigs([]string{"config", dir}); err != nil {
		t.Fatalf("GenerateConfigs: %v", err)
	}

	cfg, err := Load(dir + "/config.yaml")
	if err != nil {
		t.Fatalf("Load generated config: %v", err)
	}
	if len(cfg.APIKey) != 64 {
		t.Errorf("generated api_key length = %d, want 64", len(cfg.APIKey))
	}
	if cfg.APIKey == "REPLACE_WITH_A_STRONG_SECRET" {
		t.Errorf("api_key still the placeholder; expected a generated secret")
	}
}

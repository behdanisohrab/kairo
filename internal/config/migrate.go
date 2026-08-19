package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"kairo/internal/fileutil"
)

// Migrate brings an existing YAML config file up to the current schema by
// adding any settings it is missing, with their defaults, and writing the file
// back in place. User values are never overwritten and unknown keys are kept,
// so a config from an older release survives an upgrade intact. Run it after
// updating the binary; it is a no-op when the config is already current.
func Migrate(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	existing := make(map[string]interface{})
	if err := yaml.Unmarshal(b, &existing); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	defaults, err := migrationDefaults()
	if err != nil {
		return err
	}

	added := missingPaths(defaults, existing, "")
	if len(added) == 0 {
		fmt.Printf("migrate: %s is already up to date\n", path)
		return nil
	}

	for key, def := range defaults {
		mergeInto(existing, key, def)
	}

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		return fmt.Errorf("parse migrated config: %w", err)
	}
	if err := NormalizeAndValidate(&cfg); err != nil {
		return fmt.Errorf("migrated config is invalid: %w", err)
	}

	if err := fileutil.AtomicWrite(path, out); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	sort.Strings(added)
	fmt.Printf("migrate: %s updated (%d setting(s) added): %s\n",
		path, len(added), strings.Join(added, ", "))
	return nil
}

// migrationDefaults is the current schema expressed as YAML maps, minus the
// identity settings (host, host_backend, vps_ip, api_key) that are user data
// and never worth stamping into a file that does not have them.
func migrationDefaults() (map[string]interface{}, error) {
	b, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("encode defaults: %w", err)
	}
	m := make(map[string]interface{})
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse defaults: %w", err)
	}
	for _, k := range []string{"host", "host_backend", "vps_ip", "api_key", "policy", "ip_source"} {
		delete(m, k)
	}
	return m, nil
}

// mergeInto fills one default into dst: nested objects merge field by field,
// anything else only when the key is absent, so existing values always win.
func mergeInto(dst map[string]interface{}, key string, def interface{}) {
	if dm, ok := dst[key].(map[string]interface{}); ok {
		if sdm, ok := def.(map[string]interface{}); ok {
			for k, v := range sdm {
				mergeInto(dm, k, v)
			}
		}
		return
	}
	if _, ok := dst[key]; !ok {
		dst[key] = def
	}
}

// missingPaths lists the dotted paths under defaults that existing is missing,
// so the user can see exactly what an upgrade is going to add.
func missingPaths(defaults, existing map[string]interface{}, prefix string) []string {
	var out []string
	for key, def := range defaults {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		ev, ok := existing[key]
		if !ok {
			out = append(out, path)
			continue
		}
		dm, ok1 := def.(map[string]interface{})
		em, ok2 := ev.(map[string]interface{})
		if ok1 && ok2 {
			out = append(out, missingPaths(dm, em, path)...)
		}
	}
	return out
}

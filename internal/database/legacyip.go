package database

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"kairo/internal/fileutil"
)

const (
	// LegacyAllowedFilename is the 0.2.x-era flat file that held the client
	// allowlist before IPs moved into the database. It is imported once on
	// startup and never read afterwards.
	LegacyAllowedFilename = "allowed.txt"
	// LegacyAllowedArchiveName is where the imported file is preserved, so
	// operators keep a visible record of what was migrated.
	LegacyAllowedArchiveName = "allowed.txt.legacy"
)

// legacyAllowedArchiveHeader is prepended to the archived copy of the old
// allowlist file.
const legacyAllowedArchiveHeader = `# LEGACY FILE (Kairo 0.2.x) - no longer read by Kairo.
#
# Since v0.3 the client allowlist lives in the database (kairo.db,
# table user_allowed_ips) and is managed from the web panel or via
# POST/DELETE /api/allow. Every IP below was imported into the admin
# account's allowlist at startup. This copy is kept for reference only;
# it is safe to delete.
`

// MigrateLegacyAllowedFile performs the one-time 0.2.x -> 0.3.x data
// migration for the client allowlist. It imports every valid, non-loopback
// IP from <dataDir>/allowed.txt into the admin user's per-user allowlist
// (skipping IPs that any user already has), then renames the file to
// <dataDir>/allowed.txt.legacy with an explanatory header, retiring it as a
// 0.2x-era relic. Missing file means nothing to do; re-running is a no-op,
// so a crash between import and rename heals on the next startup.
func (db *DB) MigrateLegacyAllowedFile(dataDir string, adminUserID int) (int, error) {
	if dataDir == "" {
		dataDir = "."
	}
	src := filepath.Join(dataDir, LegacyAllowedFilename)
	raw, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read legacy %s: %w", LegacyAllowedFilename, err)
	}

	imported, err := db.importLegacyIPs(raw, adminUserID)
	if err != nil {
		return imported, err
	}

	archive := filepath.Join(dataDir, LegacyAllowedArchiveName)
	body := append([]byte(legacyAllowedArchiveHeader), raw...)
	if !bytes.HasSuffix(body, []byte("\n")) {
		body = append(body, '\n')
	}
	if err := fileutil.AtomicWrite(archive, body); err != nil {
		return imported, fmt.Errorf("write %s: %w", LegacyAllowedArchiveName, err)
	}
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return imported, fmt.Errorf("remove legacy %s: %w", LegacyAllowedFilename, err)
	}

	if imported > 0 {
		slog.Info("legacy allowlist migrated", "file", src, "imported", imported, "owner_user_id", adminUserID, "archived_as", archive)
	} else {
		slog.Info("legacy allowlist retired", "file", src, "archived_as", archive)
	}
	return imported, nil
}

// importLegacyIPs parses the raw contents of the old allowlist file and
// inserts every IP that no user owns yet under adminUserID.
func (db *DB) importLegacyIPs(raw []byte, adminUserID int) (int, error) {
	imported := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ip := net.ParseIP(line)
		if ip == nil || ip.IsLoopback() {
			slog.Warn("legacy allowlist: skipping unusable entry", "entry", line)
			continue
		}
		key := ip.String()

		owned, err := db.IsIPAllowlistedAny(key)
		if err != nil {
			return imported, fmt.Errorf("check legacy ip %s: %w", key, err)
		}
		if owned {
			continue
		}
		added, err := db.AddUserIPIfAbsent(adminUserID, key)
		if err != nil {
			return imported, err
		}
		if added {
			imported++
		}
	}
	return imported, nil
}

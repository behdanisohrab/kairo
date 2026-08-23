package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestDBWithAdmin(t *testing.T) (*DB, *User) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "kairo.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	admin, err := db.CreateUser("admin", "secret-password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	return db, admin
}

func TestMigrateLegacyAllowedFile(t *testing.T) {
	db, admin := newTestDBWithAdmin(t)
	dir := t.TempDir()

	content := "# 0.2x allowlist\n" +
		"203.0.113.5\n" + // valid, must import
		"\n" + // blank
		"999.9.9.9\n" + // invalid, skip
		"127.0.0.1\n" + // loopback v4, skip
		"::1\n" + // loopback v6, skip
		"203.0.113.5\n" + // duplicate in file, import once
		"2001:db8::7\n" + // valid IPv6, must import
		" 198.51.100.44  \n" // padded whitespace, must import
	if err := os.WriteFile(filepath.Join(dir, LegacyAllowedFilename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	imported, err := db.MigrateLegacyAllowedFile(dir, admin.ID)
	if err != nil {
		t.Fatalf("MigrateLegacyAllowedFile: %v", err)
	}
	if imported != 3 {
		t.Errorf("imported = %d, want 3 (203.0.113.5, 2001:db8::7, 198.51.100.44)", imported)
	}

	rows, err := db.ListUserIPs(admin.ID)
	if err != nil {
		t.Fatalf("ListUserIPs: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.IP] = true
	}
	for _, want := range []string{"203.0.113.5", "2001:db8::7", "198.51.100.44"} {
		if !got[want] {
			t.Errorf("admin rows missing %q, got %v", want, got)
		}
	}
	if len(got) != len(rows) || len(rows) != 3 {
		t.Errorf("admin rows = %v, want exactly the 3 migrated IPs", rows)
	}

	// The legacy file is retired and archived with an explanatory header.
	if _, err := os.Stat(filepath.Join(dir, LegacyAllowedFilename)); !os.IsNotExist(err) {
		t.Errorf("%s must be gone after migration (stat err = %v)", LegacyAllowedFilename, err)
	}
	archived, err := os.ReadFile(filepath.Join(dir, LegacyAllowedArchiveName))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if !strings.Contains(string(archived), "LEGACY") || !strings.Contains(string(archived), "203.0.113.5") {
		t.Errorf("archive must keep a legacy header plus original content, got:\n%s", archived)
	}
}

func TestMigrateLegacyAllowedFileIsIdempotent(t *testing.T) {
	db, admin := newTestDBWithAdmin(t)
	dir := t.TempDir()
	path := filepath.Join(dir, LegacyAllowedFilename)
	if err := os.WriteFile(path, []byte("203.0.113.5\n198.51.100.6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := db.MigrateLegacyAllowedFile(dir, admin.ID); err != nil || n != 2 {
		t.Fatalf("first run: imported = %d, err = %v, want 2 nil", n, err)
	}

	// Simulate a crash between import and rename: the file reappears.
	if err := os.WriteFile(path, []byte("203.0.113.5\n198.51.100.6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := db.MigrateLegacyAllowedFile(dir, admin.ID)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if n != 0 {
		t.Errorf("second run imported = %d, want 0 (already owned by admin)", n)
	}
	count, err := db.CountUserIPs(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("admin row count = %d, want 2 (no duplicates)", count)
	}
}

func TestMigrateLegacyAllowedFileSkipsIPsOwnedByOthers(t *testing.T) {
	db, admin := newTestDBWithAdmin(t)
	dir := t.TempDir()

	bob, err := db.CreateUser("bob", "secret-password", "user")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := db.AddUserIPIfAbsent(bob.ID, "203.0.113.9"); err != nil || !ok {
		t.Fatalf("seed bob ip: ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, LegacyAllowedFilename), []byte("203.0.113.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := db.MigrateLegacyAllowedFile(dir, admin.ID)
	if err != nil {
		t.Fatalf("MigrateLegacyAllowedFile: %v", err)
	}
	if n != 0 {
		t.Errorf("imported = %d, want 0 (IP already owned by bob)", n)
	}
	ok, _ := db.IsIPAllowlistedForUser(admin.ID, "203.0.113.9")
	if ok {
		t.Error("migration must not duplicate another user's IP onto admin")
	}
	// The union still contains it, so routing keeps working for everyone.
	all, _ := db.DistinctAllowedIPs()
	if len(all) != 1 || all[0] != "203.0.113.9" {
		t.Errorf("DistinctAllowedIPs = %v, want [203.0.113.9]", all)
	}
}

func TestMigrateLegacyAllowedFileWithoutFileIsNoOp(t *testing.T) {
	db, admin := newTestDBWithAdmin(t)
	n, err := db.MigrateLegacyAllowedFile(t.TempDir(), admin.ID)
	if err != nil {
		t.Fatalf("MigrateLegacyAllowedFile: %v", err)
	}
	if n != 0 {
		t.Errorf("imported = %d, want 0", n)
	}
}

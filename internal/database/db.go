package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type User struct {
	ID           int
	Username     string
	PasswordHash string
	APIKey       string
	Role         string
	RateLimit    int
	IpLimit      int
	CreatedAt    time.Time
	LastLogin    *time.Time
}

type UserAllowedIP struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Username  string    `json:"username,omitempty"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string
	UserID    int
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type DomainRequest struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Username  string    `json:"username"`
	Domain    string    `json:"domain"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			api_key       TEXT UNIQUE NOT NULL,
			role          TEXT NOT NULL DEFAULT 'user',
			rate_limit    INTEGER NOT NULL DEFAULT 100,
			ip_limit      INTEGER NOT NULL DEFAULT 3,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login    DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			ip         TEXT NOT NULL,
			user_agent TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS domain_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			domain TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, domain)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_domain_requests_status ON domain_requests(status)`,
		`CREATE INDEX IF NOT EXISTS idx_domain_requests_user ON domain_requests(user_id)`,
		`CREATE TABLE IF NOT EXISTS user_allowed_ips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			ip TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, ip)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_allowed_ips_user ON user_allowed_ips(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_allowed_ips_ip ON user_allowed_ips(ip)`,
		`CREATE TABLE IF NOT EXISTS connection_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ip         TEXT NOT NULL,
			user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
			domain     TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_created ON connection_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_user ON connection_logs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_ip ON connection_logs(ip)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	// Legacy device tracking (0.3.x and earlier): drop the devices table and
	// the device-keyed connection_logs so the new ip-keyed schema takes over.
	var legacyDevices int
	if err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'devices'`,
	).Scan(&legacyDevices); err != nil {
		return fmt.Errorf("check legacy devices table: %w", err)
	}
	if legacyDevices > 0 {
		slog.Info("migrating away legacy device tracking")
		if _, err := db.conn.Exec(`DROP TABLE IF EXISTS connection_logs`); err != nil {
			return fmt.Errorf("drop legacy connection_logs: %w", err)
		}
		if _, err := db.conn.Exec(`DROP TABLE IF EXISTS devices`); err != nil {
			return fmt.Errorf("drop legacy devices: %w", err)
		}
		if _, err := db.conn.Exec(`CREATE TABLE IF NOT EXISTS connection_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ip         TEXT NOT NULL,
			user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
			domain     TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
			return fmt.Errorf("create connection_logs: %w", err)
		}
		if _, err := db.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_connection_logs_created ON connection_logs(created_at)`); err != nil {
			return fmt.Errorf("index connection_logs: %w", err)
		}
	}

	// Add ip_limit column if missing (migration for existing DBs)
	if _, err := db.conn.Exec(`ALTER TABLE users ADD COLUMN ip_limit INTEGER NOT NULL DEFAULT 3`); err != nil {
		slog.Debug("ip_limit migration: column may already exist", "err", err)
	}

	slog.Info("database migrated")
	return nil
}

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
	CreatedAt    time.Time
	LastLogin    *time.Time
}

type Session struct {
	ID        string
	UserID    int
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Device struct {
	ID         int       `json:"id"`
	IP         string    `json:"ip"`
	JA3Hash    string    `json:"ja3_hash"`
	UserAgent  string    `json:"user_agent"`
	DeviceType string    `json:"device_type"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

type ConnectionLog struct {
	ID        int        `json:"id"`
	DeviceID  int        `json:"device_id"`
	UserID    *int       `json:"user_id"`
	Domain    string     `json:"domain"`
	CreatedAt time.Time  `json:"created_at"`
}

type DomainRequest struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Username  string    `json:"username"`
	Domain    string    `json:"domain"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type UserWithDevices struct {
	User
	Devices []Device
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
		`CREATE TABLE IF NOT EXISTS devices (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ip         TEXT NOT NULL,
			ja3_hash   TEXT NOT NULL,
			user_agent TEXT,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen  DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(ip, ja3_hash)
		)`,
		`CREATE TABLE IF NOT EXISTS connection_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			user_id    INTEGER,
			domain     TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(device_id, domain)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_ip ON devices(ip)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_user ON connection_logs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_device ON connection_logs(device_id)`,
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
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	// Add device_type column if missing (migration for existing DBs)
	if _, err := db.conn.Exec(`ALTER TABLE devices ADD COLUMN device_type TEXT NOT NULL DEFAULT ''`); err != nil {
		// SQLite doesn't error if column already exists, but we catch just in case
		slog.Debug("device_type migration: column may already exist", "err", err)
	}

	slog.Info("database migrated")
	return nil
}

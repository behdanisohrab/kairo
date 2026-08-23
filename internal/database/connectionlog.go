package database

import (
	"database/sql"
	"fmt"
	"time"
)

// ConnectionLog is one tunnelled SNI connection, attributed to the account
// owning the source IP when possible.
type ConnectionLog struct {
	ID        int       `json:"id"`
	IP        string    `json:"ip"`
	UserID    *int      `json:"user_id"`
	Username  string    `json:"username,omitempty"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// LogConnection records one tunnelled connection. One row per connection:
// traffic analytics need volume, not last-seen updates.
func (db *DB) LogConnection(ip string, userID *int, domain string) error {
	_, err := db.conn.Exec(
		`INSERT INTO connection_logs (ip, user_id, domain) VALUES (?, ?, ?)`,
		ip, userID, domain,
	)
	return err
}

// GetUserIDByAllowedIP resolves the account that allowlisted the given
// source IP. When several accounts share an IP the oldest mapping wins, so
// attribution stays stable.
func (db *DB) GetUserIDByAllowedIP(ip string) (int, bool, error) {
	var userID int
	err := db.conn.QueryRow(
		`SELECT user_id FROM user_allowed_ips WHERE ip = ? ORDER BY id LIMIT 1`, ip,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("resolve ip owner: %w", err)
	}
	return userID, true, nil
}

// TrafficBucket is one hour of connection volume, UTC-stamped as
// YYYY-MM-DDTHH:00:00Z. Hours without connections are absent.
type TrafficBucket struct {
	Bucket string `json:"bucket"`
	Count  int64  `json:"count"`
}

// NameCount is a generic "top N" row (domain or username plus hits).
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

func hoursClause(hours int) string {
	if hours <= 0 {
		hours = 24
	}
	return fmt.Sprintf("-%d hours", hours)
}

// TrafficBuckets returns per-hour connection counts over the window.
func (db *DB) TrafficBuckets(hours int) ([]TrafficBucket, error) {
	rows, err := db.conn.Query(
		`SELECT strftime('%Y-%m-%dT%H:00:00Z', created_at) AS bucket, COUNT(*)
		 FROM connection_logs
		 WHERE created_at >= datetime('now', ?)
		 GROUP BY bucket ORDER BY bucket`, hoursClause(hours),
	)
	if err != nil {
		return nil, fmt.Errorf("traffic buckets: %w", err)
	}
	defer rows.Close()
	var out []TrafficBucket
	for rows.Next() {
		var b TrafficBucket
		if err := rows.Scan(&b.Bucket, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// TopDomains returns the most-tunnelled domains over the window.
func (db *DB) TopDomains(hours, limit int) ([]NameCount, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.conn.Query(
		`SELECT domain, COUNT(*) AS n
		 FROM connection_logs
		 WHERE created_at >= datetime('now', ?)
		 GROUP BY domain ORDER BY n DESC LIMIT ?`, hoursClause(hours), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top domains: %w", err)
	}
	defer rows.Close()
	var out []NameCount
	for rows.Next() {
		var nc NameCount
		if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, err
		}
		out = append(out, nc)
	}
	return out, rows.Err()
}

// TopUsers returns the most active accounts over the window. Rows whose
// source IP no user owns are grouped under "unattributed".
func (db *DB) TopUsers(hours, limit int) ([]NameCount, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.conn.Query(
		`SELECT COALESCE(u.username, 'unattributed') AS name, COUNT(*) AS n
		 FROM connection_logs cl
		 LEFT JOIN users u ON u.id = cl.user_id
		 WHERE cl.created_at >= datetime('now', ?)
		 GROUP BY name ORDER BY n DESC LIMIT ?`, hoursClause(hours), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top users: %w", err)
	}
	defer rows.Close()
	var out []NameCount
	for rows.Next() {
		var nc NameCount
		if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, err
		}
		out = append(out, nc)
	}
	return out, rows.Err()
}

// TrafficTotals returns total connections and unique client IPs in-window.
func (db *DB) TrafficTotals(hours int) (connections int64, uniqueIPs int64, err error) {
	err = db.conn.QueryRow(
		`SELECT COUNT(*), COUNT(DISTINCT ip)
		 FROM connection_logs
		 WHERE created_at >= datetime('now', ?)`, hoursClause(hours),
	).Scan(&connections, &uniqueIPs)
	return connections, uniqueIPs, err
}

// UserTrafficTotals returns one account's connection count and distinct
// domains over the window.
func (db *DB) UserTrafficTotals(hours, userID int) (connections int64, uniqueDomains int64, err error) {
	err = db.conn.QueryRow(
		`SELECT COUNT(*), COUNT(DISTINCT domain)
		 FROM connection_logs
		 WHERE user_id = ? AND created_at >= datetime('now', ?)`,
		userID, hoursClause(hours),
	).Scan(&connections, &uniqueDomains)
	return connections, uniqueDomains, err
}

// RecentConnections returns the newest connections with usernames attached.
func (db *DB) RecentConnections(limit int) ([]ConnectionLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.conn.Query(
		`SELECT cl.id, cl.ip, cl.user_id, COALESCE(u.username, ''), cl.domain, cl.created_at
		 FROM connection_logs cl
		 LEFT JOIN users u ON u.id = cl.user_id
		 ORDER BY cl.id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recent connections: %w", err)
	}
	defer rows.Close()
	var out []ConnectionLog
	for rows.Next() {
		var l ConnectionLog
		if err := rows.Scan(&l.ID, &l.IP, &l.UserID, &l.Username, &l.Domain, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetConnectionLogsByUser returns a user's newest connections.
func (db *DB) GetConnectionLogsByUser(userID, limit int) ([]ConnectionLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.conn.Query(
		`SELECT id, ip, user_id, domain, created_at
		 FROM connection_logs
		 WHERE user_id = ?
		 ORDER BY id DESC LIMIT ?`, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("connection logs by user: %w", err)
	}
	defer rows.Close()
	var out []ConnectionLog
	for rows.Next() {
		var l ConnectionLog
		if err := rows.Scan(&l.ID, &l.IP, &l.UserID, &l.Domain, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CountConnectionLogs reports the all-time row count (health checks).
func (db *DB) CountConnectionLogs(out *int) error {
	return db.conn.QueryRow(`SELECT COUNT(*) FROM connection_logs`).Scan(out)
}

// PruneConnectionLogs deletes rows older than the given number of days.
func (db *DB) PruneConnectionLogs(keepDays int) (int64, error) {
	res, err := db.conn.Exec(
		`DELETE FROM connection_logs WHERE created_at < datetime('now', ?)`,
		fmt.Sprintf("-%d days", keepDays),
	)
	if err != nil {
		return 0, fmt.Errorf("prune connection logs: %w", err)
	}
	n, err := res.RowsAffected()
	return n, err
}

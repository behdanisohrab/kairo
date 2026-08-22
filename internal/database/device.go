package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ClassifyDevice returns a human-readable device type from a User-Agent string.
func ClassifyDevice(userAgent string) string {
	ua := strings.ToLower(userAgent)

	switch {
	case strings.Contains(ua, "bot") || strings.Contains(ua, "crawler") || strings.Contains(ua, "spider") || strings.Contains(ua, "curl") || strings.Contains(ua, "wget"):
		return "Bot"
	case strings.Contains(ua, "android"):
		return "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod") || strings.Contains(ua, "macintosh") && strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		return "iOS"
	case strings.Contains(ua, "windows"):
		return "Desktop"
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macos") || strings.Contains(ua, "darwin"):
		return "Desktop"
	case strings.Contains(ua, "linux") && !strings.Contains(ua, "android"):
		return "Desktop"
	case strings.Contains(ua, "tablet") || strings.Contains(ua, "kindle") || strings.Contains(ua, "playbook"):
		return "Tablet"
	case userAgent == "":
		return "Unknown"
	default:
		return "Other"
	}
}

func (db *DB) UpsertDevice(ip, ja3Hash, userAgent string) (*Device, error) {
	now := time.Now()
	deviceType := ClassifyDevice(userAgent)

	_, err := db.conn.Exec(
		`INSERT INTO devices (ip, ja3_hash, user_agent, device_type, first_seen, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(ip, ja3_hash) DO UPDATE SET last_seen = ?, user_agent = COALESCE(?, user_agent), device_type = COALESCE(NULLIF(?, ''), device_type)`,
		ip, ja3Hash, userAgent, deviceType, now, now, now, nullIfEmpty(userAgent), deviceType,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert device: %w", err)
	}

	var d Device
	err = db.conn.QueryRow(
		`SELECT id, ip, ja3_hash, user_agent, device_type, first_seen, last_seen
		 FROM devices WHERE ip = ? AND ja3_hash = ?`, ip, ja3Hash,
	).Scan(&d.ID, &d.IP, &d.JA3Hash, &d.UserAgent, &d.DeviceType, &d.FirstSeen, &d.LastSeen)
	if err != nil {
		return nil, fmt.Errorf("get device after upsert: %w", err)
	}
	return &d, nil
}

func (db *DB) UpsertConnectionLog(deviceID int, userID *int, domain string) error {
	_, err := db.conn.Exec(
		`INSERT INTO connection_logs (device_id, user_id, domain, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(device_id, domain) DO UPDATE SET created_at = ?`,
		deviceID, userID, domain, time.Now(), time.Now(),
	)
	return err
}

func (db *DB) GetDeviceByID(id int) (*Device, error) {
	var d Device
	err := db.conn.QueryRow(
		`SELECT id, ip, ja3_hash, user_agent, device_type, first_seen, last_seen
		 FROM devices WHERE id = ?`, id,
	).Scan(&d.ID, &d.IP, &d.JA3Hash, &d.UserAgent, &d.DeviceType, &d.FirstSeen, &d.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	return &d, nil
}

func (db *DB) GetDevicesByUser(userID int) ([]Device, error) {
	rows, err := db.conn.Query(
		`SELECT DISTINCT d.id, d.ip, d.ja3_hash, d.user_agent, d.device_type, d.first_seen, d.last_seen
		 FROM devices d
		 INNER JOIN connection_logs cl ON d.id = cl.device_id
		 WHERE cl.user_id = ?
		 ORDER BY d.last_seen DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get devices by user: %w", err)
	}
	defer rows.Close()

	return scanDevices(rows)
}

func (db *DB) GetAllDevices() ([]Device, error) {
	rows, err := db.conn.Query(
		`SELECT id, ip, ja3_hash, user_agent, device_type, first_seen, last_seen
		 FROM devices ORDER BY last_seen DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all devices: %w", err)
	}
	defer rows.Close()

	return scanDevices(rows)
}

func (db *DB) GetAllUsersWithDevices() ([]UserWithDevices, error) {
	users, err := db.ListUsers()
	if err != nil {
		return nil, err
	}

	var result []UserWithDevices
	for _, u := range users {
		devices, err := db.GetDevicesByUser(u.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, UserWithDevices{User: u, Devices: devices})
	}
	return result, nil
}

func (db *DB) GetConnectionLogsByDevice(deviceID int) ([]ConnectionLog, error) {
	rows, err := db.conn.Query(
		`SELECT id, device_id, user_id, domain, created_at
		 FROM connection_logs WHERE device_id = ?
		 ORDER BY created_at DESC`, deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get connection logs: %w", err)
	}
	defer rows.Close()

	var logs []ConnectionLog
	for rows.Next() {
		var l ConnectionLog
		if err := rows.Scan(&l.ID, &l.DeviceID, &l.UserID, &l.Domain, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan connection log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (db *DB) GetConnectionLogsByUser(userID int) ([]ConnectionLog, error) {
	rows, err := db.conn.Query(
		`SELECT cl.id, cl.device_id, cl.user_id, cl.domain, cl.created_at
		 FROM connection_logs cl
		 WHERE cl.user_id = ?
		 ORDER BY cl.created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get connection logs by user: %w", err)
	}
	defer rows.Close()

	var logs []ConnectionLog
	for rows.Next() {
		var l ConnectionLog
		if err := rows.Scan(&l.ID, &l.DeviceID, &l.UserID, &l.Domain, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan connection log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (db *DB) FindUserByIP(ip string) (*User, error) {
	var u User
	err := db.conn.QueryRow(
		`SELECT u.id, u.username, u.password_hash, u.api_key, u.role, u.rate_limit, u.created_at, u.last_login
		 FROM users u
		 INNER JOIN connection_logs cl ON u.id = cl.user_id
		 INNER JOIN devices d ON cl.device_id = d.id
		 WHERE d.ip = ?
		 ORDER BY cl.created_at DESC LIMIT 1`, ip,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.Role, &u.RateLimit, &u.CreatedAt, &u.LastLogin)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by ip: %w", err)
	}
	return &u, nil
}

func (db *DB) CountConnectionLogs(out *int) error {
	return db.conn.QueryRow(`SELECT COUNT(*) FROM connection_logs`).Scan(out)
}

func (db *DB) GetRecentConnectionLogs(limit int) ([]ConnectionLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.conn.Query(
		`SELECT id, device_id, user_id, domain, created_at FROM connection_logs ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent logs: %w", err)
	}
	defer rows.Close()
	var logs []ConnectionLog
	for rows.Next() {
		var l ConnectionLog
		if err := rows.Scan(&l.ID, &l.DeviceID, &l.UserID, &l.Domain, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func scanDevices(rows *sql.Rows) ([]Device, error) {
	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.IP, &d.JA3Hash, &d.UserAgent, &d.DeviceType, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

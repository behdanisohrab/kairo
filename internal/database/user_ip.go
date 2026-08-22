package database

import (
	"database/sql"
	"fmt"
	"time"
)

func (db *DB) ListUserIPs(userID int) ([]UserAllowedIP, error) {
	rows, err := db.conn.Query(`SELECT id, user_id, ip, created_at FROM user_allowed_ips WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserAllowedIP
	for rows.Next() {
		var r UserAllowedIP
		if err := rows.Scan(&r.ID, &r.UserID, &r.IP, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) ListAllUserIPs() ([]UserAllowedIP, error) {
	rows, err := db.conn.Query(`
		SELECT uai.id, uai.user_id, u.username, uai.ip, uai.created_at
		FROM user_allowed_ips uai JOIN users u ON u.id = uai.user_id
		ORDER BY uai.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserAllowedIP
	for rows.Next() {
		var r UserAllowedIP
		if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.IP, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) CountUserIPs(userID int) (int, error) {
	var c int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM user_allowed_ips WHERE user_id = ?`, userID).Scan(&c)
	return c, err
}

func (db *DB) IsIPAllowlistedForUser(userID int, ip string) (bool, error) {
	var c int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM user_allowed_ips WHERE user_id = ? AND ip = ?`, userID, ip).Scan(&c)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}

func (db *DB) IsIPAllowlistedAny(ip string) (bool, error) {
	var c int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM user_allowed_ips WHERE ip = ?`, ip).Scan(&c)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}

func (db *DB) AddUserIP(userID int, ip string) error {
	_, err := db.conn.Exec(`INSERT INTO user_allowed_ips (user_id, ip) VALUES (?, ?)`, userID, ip)
	if err != nil {
		return fmt.Errorf("add user ip: %w", err)
	}
	return nil
}

func (db *DB) RemoveUserIP(userID int, ip string) (bool, error) {
	res, err := db.conn.Exec(`DELETE FROM user_allowed_ips WHERE user_id = ? AND ip = ?`, userID, ip)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (db *DB) RemoveUserIPAny(ip string) (bool, error) {
	res, err := db.conn.Exec(`DELETE FROM user_allowed_ips WHERE ip = ?`, ip)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (db *DB) GetUserIPByID(id int) (*UserAllowedIP, error) {
	var r UserAllowedIP
	err := db.conn.QueryRow(`SELECT id, user_id, ip, created_at FROM user_allowed_ips WHERE id = ?`, id).Scan(&r.ID, &r.UserID, &r.IP, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

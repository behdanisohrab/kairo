package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (db *DB) CreateSession(userID int, ip, userAgent string, ttl time.Duration) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	now := time.Now()
	expires := now.Add(ttl)

	_, err = db.conn.Exec(
		`INSERT INTO sessions (id, user_id, ip, user_agent, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, ip, userAgent, now, expires,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Session{
		ID:        id,
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		CreatedAt: now,
		ExpiresAt: expires,
	}, nil
}

func (db *DB) GetSession(id string) (*Session, error) {
	s := &Session{}
	err := db.conn.QueryRow(
		`SELECT id, user_id, ip, user_agent, created_at, expires_at
		 FROM sessions WHERE id = ? AND expires_at > ?`, id, time.Now(),
	).Scan(&s.ID, &s.UserID, &s.IP, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return s, nil
}

func (db *DB) DeleteSession(id string) error {
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (db *DB) DeleteUserSessions(userID int) error {
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (db *DB) ExpireOldSessions() error {
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now())
	return err
}

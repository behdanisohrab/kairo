package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.conn.QueryRow(
		`SELECT id, username, password_hash, api_key, role, rate_limit, created_at, last_login
		 FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.Role, &u.RateLimit, &u.CreatedAt, &u.LastLogin)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByID(id int) (*User, error) {
	u := &User{}
	err := db.conn.QueryRow(
		`SELECT id, username, password_hash, api_key, role, rate_limit, created_at, last_login
		 FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.Role, &u.RateLimit, &u.CreatedAt, &u.LastLogin)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByAPIKey(apiKey string) (*User, error) {
	u := &User{}
	err := db.conn.QueryRow(
		`SELECT id, username, password_hash, api_key, role, rate_limit, created_at, last_login
		 FROM users WHERE api_key = ?`, apiKey,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.Role, &u.RateLimit, &u.CreatedAt, &u.LastLogin)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by api key: %w", err)
	}
	return u, nil
}

func (db *DB) CreateUser(username, password, role string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	if role == "" {
		role = "user"
	}

	result, err := db.conn.Exec(
		`INSERT INTO users (username, password_hash, api_key, role) VALUES (?, ?, ?, ?)`,
		username, hash, apiKey, role,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	id, _ := result.LastInsertId()
	return db.GetUserByID(int(id))
}

func (db *DB) UpdateUserAPIKey(userID int) (string, error) {
	apiKey, err := generateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}

	_, err = db.conn.Exec(`UPDATE users SET api_key = ? WHERE id = ?`, apiKey, userID)
	if err != nil {
		return "", fmt.Errorf("update api key: %w", err)
	}
	return apiKey, nil
}

func (db *DB) UpdateUserLastLogin(userID int) error {
	now := time.Now()
	_, err := db.conn.Exec(`UPDATE users SET last_login = ? WHERE id = ?`, now, userID)
	return err
}

func (db *DB) DeleteUser(id int) error {
	_, err := db.conn.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (db *DB) DeleteUserAtomic(id int) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM domain_requests WHERE user_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) ListUsers() ([]User, error) {
	rows, err := db.conn.Query(
		`SELECT id, username, password_hash, api_key, role, rate_limit, created_at, last_login
		 FROM users ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.Role, &u.RateLimit, &u.CreatedAt, &u.LastLogin); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (db *DB) AdminExists() (bool, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count > 0, err
}

func (db *DB) EnsureAdmin(username, password string) error {
	exists, err := db.AdminExists()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = db.CreateUser(username, password, "admin")
	return err
}

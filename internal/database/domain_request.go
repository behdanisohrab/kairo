package database

import (
	"database/sql"
	"fmt"
	"strings"
)

func (db *DB) CreateDomainRequest(userID int, domain string) (*DomainRequest, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	res, err := db.conn.Exec(`INSERT INTO domain_requests (user_id, domain, status) VALUES (?, ?, 'pending')`, userID, domain)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("you already requested this domain")
		}
		return nil, fmt.Errorf("create request: %w", err)
	}
	id, _ := res.LastInsertId()
	return db.GetDomainRequest(int(id))
}

func (db *DB) GetDomainRequest(id int) (*DomainRequest, error) {
	dr := &DomainRequest{}
	err := db.conn.QueryRow(
		`SELECT dr.id, dr.user_id, u.username, dr.domain, dr.status, dr.created_at
		 FROM domain_requests dr JOIN users u ON u.id = dr.user_id WHERE dr.id = ?`, id,
	).Scan(&dr.ID, &dr.UserID, &dr.Username, &dr.Domain, &dr.Status, &dr.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return dr, nil
}

func (db *DB) ListDomainRequests() ([]DomainRequest, error) {
	rows, err := db.conn.Query(
		`SELECT dr.id, dr.user_id, u.username, dr.domain, dr.status, dr.created_at
		 FROM domain_requests dr JOIN users u ON u.id = dr.user_id ORDER BY dr.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DomainRequest
	for rows.Next() {
		var dr DomainRequest
		if err := rows.Scan(&dr.ID, &dr.UserID, &dr.Username, &dr.Domain, &dr.Status, &dr.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, dr)
	}
	return out, rows.Err()
}

func (db *DB) UpdateDomainRequestStatus(id int, status string) error {
	_, err := db.conn.Exec(`UPDATE domain_requests SET status = ? WHERE id = ?`, status, id)
	return err
}

func (db *DB) DeleteDomainRequest(id int) error {
	_, err := db.conn.Exec(`DELETE FROM domain_requests WHERE id = ?`, id)
	return err
}

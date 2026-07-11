package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// SetHostKey stores a permanent Host GUI unlock key for a bound device.
func (db *DB) SetHostKey(deviceID, codeHash, codePlain string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || codeHash == "" {
		return fmt.Errorf("deviceId and host key hash required")
	}
	res, err := db.sql.Exec(
		`UPDATE agent_bindings SET host_key_hash=?, host_key_plain=? WHERE device_id=?`,
		codeHash, strings.TrimSpace(codePlain), deviceID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("binding not found")
	}
	return nil
}

// GetHostKeyHash returns the bcrypt hash for verify (empty if none).
func (db *DB) GetHostKeyHash(deviceID string) (string, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", fmt.Errorf("deviceId required")
	}
	var hash sql.NullString
	err := db.sql.QueryRow(
		`SELECT COALESCE(host_key_hash,'') FROM agent_bindings WHERE device_id=?`, deviceID,
	).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("binding not found")
	}
	if err != nil {
		return "", err
	}
	return hash.String, nil
}

// GetHostKeyPlain returns the plaintext key for tech/admin copy.
func (db *DB) GetHostKeyPlain(deviceID string) (string, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", fmt.Errorf("deviceId required")
	}
	var plain sql.NullString
	err := db.sql.QueryRow(
		`SELECT COALESCE(host_key_plain,'') FROM agent_bindings WHERE device_id=?`, deviceID,
	).Scan(&plain)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("binding not found")
	}
	if err != nil {
		return "", err
	}
	return plain.String, nil
}

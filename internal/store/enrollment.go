package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const StatusRedeemed = "redeemed"

type EnrollmentCode struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenantId"`
	TenantName string     `json:"tenantName,omitempty"`
	Label      string     `json:"label"`
	Status     string     `json:"status"`
	Code       string     `json:"code,omitempty"` // plaintext kept for admin/tech copy
	DeviceID   string     `json:"deviceId,omitempty"`
	RedeemedAt *time.Time `json:"redeemedAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	CodeHash   string     `json:"-"`
}

func (db *DB) CreateEnrollment(tenantID, label, codeHash, codePlain string, expiresAt *time.Time) (EnrollmentCode, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" || codeHash == "" {
		return EnrollmentCode{}, fmt.Errorf("tenantId and code hash required")
	}
	if _, err := db.GetTenant(tenantID); err != nil {
		return EnrollmentCode{}, err
	}
	e := EnrollmentCode{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Label:     strings.TrimSpace(label),
		Status:    StatusPending,
		Code:      strings.TrimSpace(codePlain),
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
		CodeHash:  codeHash,
	}
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := db.sql.Exec(
		`INSERT INTO enrollment_codes(id, tenant_id, code_hash, code_plain, label, status, device_id, redeemed_at, expires_at, created_at)
		 VALUES(?,?,?,?,?,?,NULL,NULL,?,?)`,
		e.ID, e.TenantID, e.CodeHash, e.Code, e.Label, e.Status, exp, e.CreatedAt.Format(time.RFC3339Nano),
	)
	return e, err
}

func (db *DB) ListEnrollments(tenantID string) ([]EnrollmentCode, error) {
	rows, err := db.sql.Query(`
SELECT e.id, e.tenant_id, e.label, e.status, e.device_id, e.redeemed_at, e.expires_at, e.created_at, t.name, COALESCE(e.code_plain,'')
FROM enrollment_codes e JOIN tenants t ON t.id=e.tenant_id
WHERE e.tenant_id=? ORDER BY e.created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEnrollments(rows)
}

func (db *DB) ListPendingEnrollments() ([]EnrollmentCode, error) {
	rows, err := db.sql.Query(`
SELECT e.id, e.tenant_id, e.label, e.status, e.device_id, e.redeemed_at, e.expires_at, e.created_at, t.name, e.code_hash
FROM enrollment_codes e JOIN tenants t ON t.id=e.tenant_id
WHERE e.status=? AND t.status=?`, StatusPending, StatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnrollmentCode
	for rows.Next() {
		var e EnrollmentCode
		var device, redeemed, expires, created sql.NullString
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Label, &e.Status, &device, &redeemed, &expires, &created, &e.TenantName, &e.CodeHash); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		if device.Valid {
			e.DeviceID = device.String
		}
		if redeemed.Valid {
			t, _ := time.Parse(time.RFC3339Nano, redeemed.String)
			e.RedeemedAt = &t
		}
		if expires.Valid {
			t, _ := time.Parse(time.RFC3339Nano, expires.String)
			e.ExpiresAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanEnrollments(rows *sql.Rows) ([]EnrollmentCode, error) {
	var out []EnrollmentCode
	for rows.Next() {
		var e EnrollmentCode
		var device, redeemed, expires, created sql.NullString
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Label, &e.Status, &device, &redeemed, &expires, &created, &e.TenantName, &e.Code); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		if device.Valid {
			e.DeviceID = device.String
		}
		if redeemed.Valid {
			t, _ := time.Parse(time.RFC3339Nano, redeemed.String)
			e.RedeemedAt = &t
		}
		if expires.Valid {
			t, _ := time.Parse(time.RFC3339Nano, expires.String)
			e.ExpiresAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RedeemEnrollment marks a pending enrollment as used by deviceID (one-time).
func (db *DB) RedeemEnrollment(id, deviceID, hostname string) (EnrollmentCode, error) {
	deviceID = strings.TrimSpace(deviceID)
	if id == "" || deviceID == "" {
		return EnrollmentCode{}, fmt.Errorf("id and deviceId required")
	}
	e, err := db.getEnrollment(id)
	if err != nil {
		return EnrollmentCode{}, err
	}
	if e.Status != StatusPending {
		return EnrollmentCode{}, fmt.Errorf("enrollment already used or revoked")
	}
	if e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt) {
		return EnrollmentCode{}, fmt.Errorf("enrollment expired")
	}
	now := time.Now().UTC()
	res, err := db.sql.Exec(
		`UPDATE enrollment_codes SET status=?, device_id=?, redeemed_at=? WHERE id=? AND status=?`,
		StatusRedeemed, deviceID, now.Format(time.RFC3339Nano), id, StatusPending,
	)
	if err != nil {
		return EnrollmentCode{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return EnrollmentCode{}, fmt.Errorf("enrollment already used")
	}
	if err := db.UpsertAgentBinding(deviceID, e.TenantID, hostname); err != nil {
		return EnrollmentCode{}, err
	}
	e.Status = StatusRedeemed
	e.DeviceID = deviceID
	e.RedeemedAt = &now
	return e, nil
}

func (db *DB) getEnrollment(id string) (EnrollmentCode, error) {
	row := db.sql.QueryRow(`
SELECT e.id, e.tenant_id, e.label, e.status, e.device_id, e.redeemed_at, e.expires_at, e.created_at, t.name, e.code_hash
FROM enrollment_codes e JOIN tenants t ON t.id=e.tenant_id WHERE e.id=?`, id)
	var e EnrollmentCode
	var device, redeemed, expires, created sql.NullString
	err := row.Scan(&e.ID, &e.TenantID, &e.Label, &e.Status, &device, &redeemed, &expires, &created, &e.TenantName, &e.CodeHash)
	if err == sql.ErrNoRows {
		return EnrollmentCode{}, fmt.Errorf("enrollment not found")
	}
	if err != nil {
		return EnrollmentCode{}, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	if device.Valid {
		e.DeviceID = device.String
	}
	if redeemed.Valid {
		t, _ := time.Parse(time.RFC3339Nano, redeemed.String)
		e.RedeemedAt = &t
	}
	if expires.Valid {
		t, _ := time.Parse(time.RFC3339Nano, expires.String)
		e.ExpiresAt = &t
	}
	return e, nil
}

func (db *DB) RevokeEnrollment(id string) error {
	res, err := db.sql.Exec(`UPDATE enrollment_codes SET status=? WHERE id=? AND status=?`, StatusRevoked, id, StatusPending)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("enrollment not found or not pending")
	}
	return nil
}

package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusPending   = "pending"
	StatusRevoked   = "revoked"
	RoleTech        = "tech"
)

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type AccessAccount struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenantId"`
	Label       string     `json:"label"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	RedeemedAt  *time.Time `json:"redeemedAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CodeHash    string     `json:"-"`
	TenantName  string     `json:"tenantName,omitempty"`
}

type AgentBinding struct {
	DeviceID  string    `json:"deviceId"`
	TenantID  string    `json:"tenantId"`
	Hostname  string    `json:"hostname,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetConnMaxLifetime(0)
	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func (db *DB) migrate() error {
	_, err := db.sql.Exec(`
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS tenants (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS access_accounts (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  label TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL,
  code_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  redeemed_at TEXT,
  expires_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_access_tenant ON access_accounts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_access_status ON access_accounts(status);
CREATE TABLE IF NOT EXISTS agent_bindings (
  device_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  hostname TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_tenant ON agent_bindings(tenant_id);
`)
	return err
}

func (db *DB) CreateTenant(name string) (Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tenant{}, fmt.Errorf("tenant name required")
	}
	t := Tenant{
		ID:        uuid.NewString(),
		Name:      name,
		Status:    StatusActive,
		CreatedAt: time.Now().UTC(),
	}
	_, err := db.sql.Exec(
		`INSERT INTO tenants(id, name, status, created_at) VALUES(?,?,?,?)`,
		t.ID, t.Name, t.Status, t.CreatedAt.Format(time.RFC3339Nano),
	)
	return t, err
}

func (db *DB) ListTenants() ([]Tenant, error) {
	rows, err := db.sql.Query(`SELECT id, name, status, created_at FROM tenants ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		var created string
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &created); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (db *DB) GetTenant(id string) (Tenant, error) {
	var t Tenant
	var created string
	err := db.sql.QueryRow(
		`SELECT id, name, status, created_at FROM tenants WHERE id=?`, id,
	).Scan(&t.ID, &t.Name, &t.Status, &created)
	if err == sql.ErrNoRows {
		return Tenant{}, fmt.Errorf("tenant not found")
	}
	if err != nil {
		return Tenant{}, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return t, nil
}

func (db *DB) CreateAccessAccount(tenantID, label, codeHash string, expiresAt *time.Time) (AccessAccount, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" || codeHash == "" {
		return AccessAccount{}, fmt.Errorf("tenantId and code hash required")
	}
	if _, err := db.GetTenant(tenantID); err != nil {
		return AccessAccount{}, err
	}
	a := AccessAccount{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Label:     strings.TrimSpace(label),
		Role:      RoleTech,
		Status:    StatusPending,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
		CodeHash:  codeHash,
	}
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := db.sql.Exec(
		`INSERT INTO access_accounts(id, tenant_id, label, role, code_hash, status, redeemed_at, expires_at, created_at)
		 VALUES(?,?,?,?,?,?,NULL,?,?)`,
		a.ID, a.TenantID, a.Label, a.Role, a.CodeHash, a.Status, exp, a.CreatedAt.Format(time.RFC3339Nano),
	)
	return a, err
}

func (db *DB) ListAccessAccounts(tenantID string) ([]AccessAccount, error) {
	rows, err := db.sql.Query(`
SELECT a.id, a.tenant_id, a.label, a.role, a.status, a.redeemed_at, a.expires_at, a.created_at, t.name
FROM access_accounts a JOIN tenants t ON t.id=a.tenant_id
WHERE a.tenant_id=? ORDER BY a.created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func (db *DB) ListAllAccessAccounts() ([]AccessAccount, error) {
	rows, err := db.sql.Query(`
SELECT a.id, a.tenant_id, a.label, a.role, a.status, a.redeemed_at, a.expires_at, a.created_at, t.name
FROM access_accounts a JOIN tenants t ON t.id=a.tenant_id
ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func scanAccounts(rows *sql.Rows) ([]AccessAccount, error) {
	var out []AccessAccount
	for rows.Next() {
		var a AccessAccount
		var redeemed, expires, created sql.NullString
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Label, &a.Role, &a.Status, &redeemed, &expires, &created, &a.TenantName); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		if redeemed.Valid {
			t, _ := time.Parse(time.RFC3339Nano, redeemed.String)
			a.RedeemedAt = &t
		}
		if expires.Valid {
			t, _ := time.Parse(time.RFC3339Nano, expires.String)
			a.ExpiresAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) GetAccessAccount(id string) (AccessAccount, error) {
	row := db.sql.QueryRow(`
SELECT a.id, a.tenant_id, a.label, a.role, a.code_hash, a.status, a.redeemed_at, a.expires_at, a.created_at, t.name
FROM access_accounts a JOIN tenants t ON t.id=a.tenant_id WHERE a.id=?`, id)
	var a AccessAccount
	var redeemed, expires, created sql.NullString
	err := row.Scan(&a.ID, &a.TenantID, &a.Label, &a.Role, &a.CodeHash, &a.Status, &redeemed, &expires, &created, &a.TenantName)
	if err == sql.ErrNoRows {
		return AccessAccount{}, fmt.Errorf("account not found")
	}
	if err != nil {
		return AccessAccount{}, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	if redeemed.Valid {
		t, _ := time.Parse(time.RFC3339Nano, redeemed.String)
		a.RedeemedAt = &t
	}
	if expires.Valid {
		t, _ := time.Parse(time.RFC3339Nano, expires.String)
		a.ExpiresAt = &t
	}
	return a, nil
}

// FindAccessByCodeHashLookup walks pending/active accounts; caller verifies bcrypt.
// Kept small: POC scale. Index by hash prefix later if needed.
func (db *DB) ListRedeemableAccounts() ([]AccessAccount, error) {
	rows, err := db.sql.Query(`
SELECT a.id, a.tenant_id, a.label, a.role, a.code_hash, a.status, a.redeemed_at, a.expires_at, a.created_at, t.name
FROM access_accounts a JOIN tenants t ON t.id=a.tenant_id
WHERE a.status IN (?,?) AND t.status=?`, StatusPending, StatusActive, StatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessAccount
	for rows.Next() {
		var a AccessAccount
		var redeemed, expires, created sql.NullString
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Label, &a.Role, &a.CodeHash, &a.Status, &redeemed, &expires, &created, &a.TenantName); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		if redeemed.Valid {
			t, _ := time.Parse(time.RFC3339Nano, redeemed.String)
			a.RedeemedAt = &t
		}
		if expires.Valid {
			t, _ := time.Parse(time.RFC3339Nano, expires.String)
			a.ExpiresAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) MarkAccessRedeemed(id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.sql.Exec(
		`UPDATE access_accounts SET status=?, redeemed_at=COALESCE(redeemed_at, ?) WHERE id=? AND status IN (?,?)`,
		StatusActive, now, id, StatusPending, StatusActive,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not redeemable")
	}
	return nil
}

func (db *DB) RevokeAccessAccount(id string) error {
	res, err := db.sql.Exec(`UPDATE access_accounts SET status=? WHERE id=?`, StatusRevoked, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

func (db *DB) UpsertAgentBinding(deviceID, tenantID, hostname string) error {
	deviceID = strings.TrimSpace(deviceID)
	tenantID = strings.TrimSpace(tenantID)
	if deviceID == "" || tenantID == "" {
		return fmt.Errorf("deviceId and tenantId required")
	}
	if _, err := db.GetTenant(tenantID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.sql.Exec(`
INSERT INTO agent_bindings(device_id, tenant_id, hostname, updated_at) VALUES(?,?,?,?)
ON CONFLICT(device_id) DO UPDATE SET tenant_id=excluded.tenant_id, hostname=excluded.hostname, updated_at=excluded.updated_at`,
		deviceID, tenantID, strings.TrimSpace(hostname), now,
	)
	return err
}

func (db *DB) GetAgentBinding(deviceID string) (AgentBinding, error) {
	var b AgentBinding
	var updated string
	err := db.sql.QueryRow(
		`SELECT device_id, tenant_id, hostname, updated_at FROM agent_bindings WHERE device_id=?`, deviceID,
	).Scan(&b.DeviceID, &b.TenantID, &b.Hostname, &updated)
	if err == sql.ErrNoRows {
		return AgentBinding{}, fmt.Errorf("binding not found")
	}
	if err != nil {
		return AgentBinding{}, err
	}
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return b, nil
}

func (db *DB) ListAgentBindings() ([]AgentBinding, error) {
	rows, err := db.sql.Query(`SELECT device_id, tenant_id, hostname, updated_at FROM agent_bindings ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentBinding
	for rows.Next() {
		var b AgentBinding
		var updated string
		if err := rows.Scan(&b.DeviceID, &b.TenantID, &b.Hostname, &updated); err != nil {
			return nil, err
		}
		b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (db *DB) ListAgentBindingsByTenant(tenantID string) ([]AgentBinding, error) {
	rows, err := db.sql.Query(
		`SELECT device_id, tenant_id, hostname, updated_at FROM agent_bindings WHERE tenant_id=? ORDER BY hostname`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentBinding
	for rows.Next() {
		var b AgentBinding
		var updated string
		if err := rows.Scan(&b.DeviceID, &b.TenantID, &b.Hostname, &updated); err != nil {
			return nil, err
		}
		b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, b)
	}
	return out, rows.Err()
}

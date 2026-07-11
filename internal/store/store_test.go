package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTenantAccessAndBinding(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "connect.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ten, err := db.CreateTenant("Acme")
	if err != nil {
		t.Fatal(err)
	}
	acc, err := db.CreateAccessAccount(ten.ID, "Alex", "hash-demo", "DEMO-CODE-PLAIN", nil)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Status != StatusPending {
		t.Fatalf("status=%s", acc.Status)
	}
	if err := db.MarkAccessRedeemed(acc.ID); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetAccessAccount(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusActive || got.RedeemedAt == nil {
		t.Fatalf("redeem failed: %+v", got)
	}
	if err := db.UpsertAgentBinding("dev-1", ten.ID, "shivudu"); err != nil {
		t.Fatal(err)
	}
	b, err := db.GetAgentBinding("dev-1")
	if err != nil || b.TenantID != ten.ID {
		t.Fatalf("binding: %+v %v", b, err)
	}
	list, err := db.ListAgentBindingsByTenant(ten.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list by tenant: %v %d", err, len(list))
	}
	exp := time.Now().Add(time.Hour)
	_, err = db.CreateAccessAccount(ten.ID, "Temp", "hash-2", "TEMP-CODE", &exp)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeAccessAccount(acc.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollmentRedeemOnce(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "connect.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ten, err := db.CreateTenant("EnrollCo")
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(24 * time.Hour)
	e, err := db.CreateEnrollment(ten.ID, "desk", "hash-enr", "ENR-TEST-CODE", &exp)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != StatusPending {
		t.Fatalf("status=%s", e.Status)
	}
	rec, err := db.RedeemEnrollment(e.ID, "dev-enroll-1", "pc1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusRedeemed || rec.DeviceID != "dev-enroll-1" {
		t.Fatalf("redeem: %+v", rec)
	}
	b, err := db.GetAgentBinding("dev-enroll-1")
	if err != nil || b.TenantID != ten.ID {
		t.Fatalf("binding: %+v %v", b, err)
	}
	if _, err := db.RedeemEnrollment(e.ID, "dev-enroll-2", "pc2"); err == nil {
		t.Fatal("expected second redeem to fail")
	}
	pending, err := db.ListPendingEnrollments()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pending {
		if p.ID == e.ID {
			t.Fatal("redeemed enrollment still pending")
		}
	}
}

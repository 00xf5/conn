package session

import (
	"testing"
	"time"
)

func TestCreateReplacesPriorTicketForDevice(t *testing.T) {
	store := NewStore()
	first, err := store.Create("dev-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("dev-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if first.Code == second.Code {
		t.Fatalf("expected new code, got same %s", first.Code)
	}
	if _, ok := store.Get(first.Code); ok {
		t.Fatalf("old ticket %s should be gone", first.Code)
	}
	if got, ok := store.Get(second.Code); !ok || got.Code != second.Code {
		t.Fatalf("new ticket missing")
	}
	list := store.List()
	if len(list) != 1 {
		t.Fatalf("list len=%d want 1", len(list))
	}
}

func TestListPurgesExpired(t *testing.T) {
	store := NewStore()
	sess, err := store.Create("dev-2", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	list := store.List()
	if len(list) != 0 {
		t.Fatalf("list len=%d want 0 after expiry", len(list))
	}
	if _, ok := store.Get(sess.Code); ok {
		t.Fatalf("expired ticket still gettable")
	}
}

func TestDeleteRemovesTicket(t *testing.T) {
	store := NewStore()
	sess, err := store.Create("dev-3", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	store.Delete(sess.Code)
	if len(store.List()) != 0 {
		t.Fatalf("expected empty list after delete")
	}
}

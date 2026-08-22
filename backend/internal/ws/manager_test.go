package ws

import "testing"

func TestUnregisterOnlyRemovesCurrentConnection(t *testing.T) {
	manager := NewManager()

	manager.Register(&Client{ConnectionID: "old", PlayerID: 1})
	manager.Register(&Client{ConnectionID: "new", PlayerID: 1})

	if manager.Unregister(1, "old") {
		t.Fatal("old connection must not unregister current connection")
	}
	if manager.Count() != 1 {
		t.Fatalf("expected one current connection, got %d", manager.Count())
	}

	if !manager.Unregister(1, "new") {
		t.Fatal("current connection should be unregistered")
	}
	if manager.Count() != 0 {
		t.Fatalf("expected no connections, got %d", manager.Count())
	}
}

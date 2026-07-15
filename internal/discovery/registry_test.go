package discovery

import (
	"testing"
	"time"

	"mesh-cu/internal/types"
)

func TestPeerRegistry(t *testing.T) {
	registry := NewPeerRegistry()

	p1 := types.Peer{ID: "peer1", Name: "Peer 1", IP: "1.2.3.4", Port: 8080}
	p2 := types.Peer{ID: "peer2", Name: "Peer 2", IP: "1.2.3.5", Port: 8081}

	registry.Update(p1)
	registry.Update(p2)

	peers := registry.GetActivePeers()
	if len(peers) != 2 {
		t.Errorf("Ожидалось 2 пира, получено %d", len(peers))
	}
}

func TestPeerRegistryCleanup(t *testing.T) {
	registry := NewPeerRegistry()

	registry.mu.Lock()
	registry.peers["old"] = types.RegisteredPeer{
		Peer:     types.Peer{ID: "old", Name: "Old Peer"},
		LastSeen: time.Now().Add(-60 * time.Second),
	}
	registry.mu.Unlock()

	registry.cleanup()

	peers := registry.GetActivePeers()
	if len(peers) != 0 {
		t.Errorf("Ожидалось 0 пиров после очистки, получено %d", len(peers))
	}
}

package discovery

import (
	"sync"
	"time"

	"landrop/internal/types"
)

// PeerRegistry хранит список активных узлов в сети.
type PeerRegistry struct {
	mu          sync.RWMutex
	peers       map[string]types.RegisteredPeer
	onPeerLeave func(peerID string)
}

// NewPeerRegistry создаёт реестр и запускает фоновую очистку.
func NewPeerRegistry() *PeerRegistry {
	r := &PeerRegistry{
		peers: make(map[string]types.RegisteredPeer),
	}
	go r.cleanupLoop()
	return r
}

// SetOnPeerLeave устанавливает callback при выходе пира из сети.
func (r *PeerRegistry) SetOnPeerLeave(fn func(peerID string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onPeerLeave = fn
}

// Update добавляет или обновляет информацию об узле.
func (r *PeerRegistry) Update(peer types.Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[peer.ID] = types.RegisteredPeer{
		Peer:     peer,
		LastSeen: time.Now(),
	}
}

// GetActivePeers возвращает список активных узлов.
func (r *PeerRegistry) GetActivePeers() []types.Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	active := make([]types.Peer, 0, len(r.peers))
	for _, p := range r.peers {
		active = append(active, p.Peer)
	}
	return active
}

func (r *PeerRegistry) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		r.cleanup()
	}
}

func (r *PeerRegistry) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, p := range r.peers {
		if now.Sub(p.LastSeen) > 30*time.Second {
			delete(r.peers, id)
			if r.onPeerLeave != nil {
				go r.onPeerLeave(id)
			}
		}
	}
}

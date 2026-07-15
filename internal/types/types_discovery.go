package types

import "time"

// Peer — информация об узле в сети.
type Peer struct {
	ID   string
	IP   string
	Port int
	Name string
}

// PeerInfo — информация о пире для ре-анонса.
type PeerInfo struct {
	ID   string
	IP   string
	Port int
	Name string
}

// RegisteredPeer — пир с отметкой времени последней активности.
type RegisteredPeer struct {
	Peer
	LastSeen time.Time
}

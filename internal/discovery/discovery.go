package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"landrop/internal/types"
)

const (
	DiscoveryPort = 9999
	DiscoveryMsg  = "LANDROP-HELLO"
	BroadcastIPv4 = "255.255.255.255"
)

// DiscoveryService — UDP-обнаружение пиров в локальной сети.
type DiscoveryService struct {
	Name string
	Port int
}

// NewDiscoveryService создаёт сервис обнаружения.
func NewDiscoveryService(name string, port int) *DiscoveryService {
	return &DiscoveryService{
		Name: name,
		Port: port,
	}
}

// UpdateName изменяет имя узла (при коллизии имён).
func (s *DiscoveryService) UpdateName(name string) {
	s.Name = name
}

// Start запускает broadcast и listen в фоне.
func (s *DiscoveryService) Start(ctx context.Context, peerChan chan<- types.Peer) {
	go s.listen(ctx, peerChan)
	go s.broadcast(ctx)

	log.Printf("[Discovery] Service started for %s (Broadcasting port: %d)", s.Name, s.Port)
	<-ctx.Done()
}

func (s *DiscoveryService) broadcast(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", BroadcastIPv4, DiscoveryPort))
	if err != nil {
		log.Printf("[Broadcast Error] Failed to resolve address: %v", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("[Broadcast Error] Failed to dial: %v", err)
		return
	}
	defer conn.Close()

	msg := fmt.Sprintf("%s:%s:%d", DiscoveryMsg, s.Name, s.Port)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := conn.Write([]byte(msg))
			if err != nil {
				log.Printf("[Broadcast Write Error] %v", err)
			}
		}
	}
}

func (s *DiscoveryService) listen(ctx context.Context, peerChan chan<- types.Peer) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				setSocketOptions(fd)
			})
		},
	}

	addr := fmt.Sprintf(":%d", DiscoveryPort)
	pc, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		log.Printf("[Listen Error] %v", err)
		return
	}
	defer pc.Close()

	conn := pc.(*net.UDPConn)

	localIPs := make(map[string]bool)
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				localIPs[ipnet.IP.String()] = true
			}
		}
	}
	localIPs["127.0.0.1"] = true

	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			data := string(buf[:n])
			parts := strings.Split(data, ":")

			if len(parts) == 3 && parts[0] == DiscoveryMsg {
				peerName := parts[1]

				peerPort, parseErr := strconv.Atoi(parts[2])
				if parseErr != nil {
					log.Printf("[Discovery Error] Invalid port from %s: %v", peerName, parseErr)
					continue
				}

				if localIPs[remoteAddr.IP.String()] && peerPort == s.Port {
					continue
				}

				peerChan <- types.Peer{
					ID:   peerName,
					IP:   remoteAddr.IP.String(),
					Port: peerPort,
					Name: peerName,
				}
			}
		}
	}
}

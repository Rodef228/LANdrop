package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	DiscoveryPort = 9999
	DiscoveryMsg  = "CU-BRIDGE-HELLO"
	// Выделяем специальный IP для нашей группы.
	// 239.x.x.x - это стандартный диапазон для локального Multicast
	MulticastIPv4 = "239.0.0.99"
)

type Peer struct {
	ID   string
	IP   string
	Port int
	Name string
}

type DiscoveryService struct {
	Name string
	Port int // Порт для TCP соединений (файлы/чат)
}

func NewDiscoveryService(name string, port int) *DiscoveryService {
	return &DiscoveryService{
		Name: name,
		Port: port,
	}
}

func (s *DiscoveryService) Start(ctx context.Context, peerChan chan<- Peer) {
	go s.listen(ctx, peerChan)
	go s.broadcast(ctx)

	log.Printf("[Discovery] Service started for %s (Broadcasting port: %d)", s.Name, s.Port)
	<-ctx.Done()
}

func (s *DiscoveryService) broadcast(ctx context.Context) {
	// Теперь стучимся на Multicast-адрес
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", MulticastIPv4, DiscoveryPort))
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

func (s *DiscoveryService) listen(ctx context.Context, peerChan chan<- Peer) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", MulticastIPv4, DiscoveryPort))
	if err != nil {
		log.Printf("[Listen Error] %v", err)
		return
	}

	// ВАЖНО: Используем ListenMulticastUDP вместо ListenUDP!
	// Это говорит Windows: "Разреши нескольким моим терминалам слушать этот порт"
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		log.Printf("[Listen Error] %v", err)
		return
	}
	defer conn.Close()

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

				if peerName == s.Name {
					continue
				}

				peerPort, parseErr := strconv.Atoi(parts[2])
				if parseErr != nil {
					log.Printf("[Discovery Error] Invalid port from %s: %v", peerName, parseErr)
					continue
				}

				peerChan <- Peer{
					ID:   peerName,
					IP:   remoteAddr.IP.String(), // IP адрес соседа
					Port: peerPort,               // TCP порт соседа для передачи файлов
					Name: peerName,
				}
			}
		}
	}
}

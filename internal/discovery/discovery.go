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
)

const (
	DiscoveryPort = 9999
	DiscoveryMsg  = "CU-BRIDGE-HELLO"
	// Для совместимости с Windows используем broadcast адрес
	BroadcastIPv4 = "255.255.255.255"
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

// UpdateName позволяет изменить имя узла после запуска (например, при коллизии имён).
func (s *DiscoveryService) UpdateName(name string) {
	s.Name = name
}

func (s *DiscoveryService) Start(ctx context.Context, peerChan chan<- Peer) {
	go s.listen(ctx, peerChan)
	go s.broadcast(ctx)

	log.Printf("[Discovery] Service started for %s (Broadcasting port: %d)", s.Name, s.Port)
	<-ctx.Done()
}

func (s *DiscoveryService) broadcast(ctx context.Context) {
	// Используем Broadcast вместо Multicast для лучшей совместимости с Windows
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

func (s *DiscoveryService) listen(ctx context.Context, peerChan chan<- Peer) {
	// Используем ListenConfig с Control функцией, чтобы установить SO_REUSEADDR
	// на сыром сокете ДО вызова bind(). Это работает на Windows и позволяет
	// нескольким процессам слушать один UDP порт 9999 одновременно.
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// SO_REUSEADDR = 2 (0x0004) на Windows — разрешаем переиспользование порта
				syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, 0x0004, 1)
				// SO_BROADCAST = 1 (0x0020) — разрешаем broadcast на сокете
				syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, 0x0020, 1)
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

	// Приводим net.PacketConn к *net.UDPConn для ReadFromUDP
	conn := pc.(*net.UDPConn)

	// Получаем список своих IP-адресов, чтобы отфильтровать собственные broadcast-сообщения
	// (они приходят с реального IP, а не с 127.0.0.1)
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

				// Фильтруем только себя: если IP наш (любой локальный интерфейс)
				// и Port совпадает с нашим TCP портом — это наше собственное
				// broadcast-сообщение, пропускаем.
				// По имени НЕ фильтруем, чтобы ноды с одинаковым именем видели друг друга.
				if localIPs[remoteAddr.IP.String()] && peerPort == s.Port {
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

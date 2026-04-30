package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

const (
	DiscoveryPort = 9999
	DiscoveryMsg  = "CU-BRIDGE-HELLO"
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

// Start запускает и вещание, и прослушивание
func (s *DiscoveryService) Start(ctx context.Context, peerChan chan<- Peer) {
	// 1. Запускаем слушателя (кто в сети?)
	go s.listen(ctx, peerChan)

	// 2. Запускаем вещателя (я тут!)
	go s.broadcast(ctx)

	log.Printf("[Discovery] Service started for %s", s.Name)
	<-ctx.Done()
}

func (s *DiscoveryService) broadcast(ctx context.Context) {
	// Подключаемся к адресу вещания
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", DiscoveryPort))
	if err != nil {
		log.Printf("[Broadcast Error] %v", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("[Broadcast Error] %v", err)
		return
	}
	defer conn.Close()

	// Формируем сообщение: ТИП:ИМЯ:ПОРТ
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
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", DiscoveryPort))
	if err != nil {
		log.Printf("[Listen Error] %v", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
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
			// Устанавливаем таймаут, чтобы не блокироваться вечно и проверять ctx.Done
			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			data := string(buf[:n])
			parts := strings.Split(data, ":")

			// Проверяем, наше ли это сообщение
			if len(parts) == 3 && parts[0] == DiscoveryMsg {
				peerName := parts[1]
				// Не добавляем самих себя
				if peerName == s.Name {
					continue
				}

				peerChan <- Peer{
					ID:   peerName,
					IP:   remoteAddr.IP.String(),
					Port: s.Port, // В будущем будем парсить порт из сообщения
					Name: peerName,
				}
			}
		}
	}
}

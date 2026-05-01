package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"mesh-cu/internal/protocol"
)

type MessageHandler func(conn net.Conn, senderID string, messageType protocol.MessageType, payload map[string]interface{}) error

type Server struct {
	Port     int
	NodeID   string
	Handler  MessageHandler
	listener net.Listener
}

func NewServer(nodeID string, port int, handler MessageHandler) *Server {
	return &Server{
		NodeID:  nodeID,
		Port:    port,
		Handler: handler,
	}
}

func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server on port %d: %w", s.Port, err)
	}
	s.listener = listener
	log.Printf("[Network] TCP Server started on port %d", s.Port)

	go func() {
		defer s.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("[Network] TCP Server shutting down...")
				return
			default:
				conn, err := s.listener.Accept()
				if err != nil {
					if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
						continue // Таймаут, пробуем снова, чтобы не блокировать select
					}
					// Игнорируем ошибки при закрытии листенера через context
					select {
					case <-ctx.Done():
						return
					default:
						log.Printf("[Network Error] Failed to accept connection: %v", err)
					}
					continue
				}
				go s.handleConnection(conn)
			}
		}
	}()
	return nil
}

func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Network] Accepted connection from %s", remoteAddr)

	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				break // Соединение простаивает
			}
			if err.Error() != "EOF" {
				log.Printf("[Network Error] Reading from %s: %v", remoteAddr, err)
			}
			break
		}

		if n > 0 {
			header, payload, err := protocol.Decode(buf[:n])
			if err != nil {
				log.Printf("[Network Error] Failed to decode message from %s: %v. Closing connection.", remoteAddr, err)
				break // Close connection on decoding error
			}

			if s.Handler != nil {
				err = s.Handler(conn, remoteAddr, header.MessageType, payload)
				if err != nil {
					log.Printf("[Network Error] Handler failed for %s: %v", remoteAddr, err)
				}
			}
		}
	}
	log.Printf("[Network] Connection from %s closed.", remoteAddr)
}

func SendMessage(peerIP string, peerPort int, message []byte) error {
	addr := net.JoinHostPort(peerIP, fmt.Sprintf("%d", peerPort))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(message)
	if err != nil {
		return fmt.Errorf("failed to send message to %s: %w", addr, err)
	}

	return nil
}

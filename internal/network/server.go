package network

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"mesh-cu/internal/helpers"
	"mesh-cu/internal/types"
)

// Server — TCP-сервер для приёма сообщений от пиров.
type Server struct {
	Port     int
	NodeID   string
	Handler  types.MessageHandler
	listener net.Listener
}

// NewServer создаёт TCP-сервер.
func NewServer(nodeID string, port int, handler types.MessageHandler) *Server {
	return &Server{
		NodeID:  nodeID,
		Port:    port,
		Handler: handler,
	}
}

// Start запускает сервер на указанном порту (0 = динамический).
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server on port %d: %w", s.Port, err)
	}
	s.listener = listener

	actualPort := s.listener.Addr().(*net.TCPAddr).Port
	s.Port = actualPort

	log.Printf("[Network] TCP Server started on port %d", s.Port)

	go func() {
		defer s.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := s.listener.Accept()
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
						continue
					}
				}
				go s.handleConnection(conn)
			}
		}
	}()
	return nil
}

// Stop останавливает сервер.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	data, err := io.ReadAll(conn)
	if err != nil {
		return
	}

	if len(data) > 0 {
		header, payload, err := helpers.Decode(data)
		if err != nil {
			log.Printf("[Network Error] Failed to decode message: %v", err)
			return
		}

		if s.Handler != nil {
			senderName, _ := payload["sender_name"].(string)
			if senderName == "" {
				senderName = header.SenderID
			}

			err = s.Handler(conn, header.SenderID, senderName, header.MessageType, payload)
			if err != nil {
				log.Printf("[Network Error] Handler failed: %v", err)
			}
		}
	}
}

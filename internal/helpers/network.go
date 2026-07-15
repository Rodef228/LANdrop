package helpers

import (
	"fmt"
	"net"
)

// SendMessage отправляет данные пиру по TCP.
func SendMessage(peerIP string, peerPort int, message []byte) error {
	addr := net.JoinHostPort(peerIP, fmt.Sprintf("%d", peerPort))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(message)
	return err
}

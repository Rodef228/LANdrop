package types

import "net"

// MessageHandler — обработчик входящих сообщений TCP-сервера.
type MessageHandler func(conn net.Conn, senderID string, name string, messageType MessageType, payload map[string]interface{}) error

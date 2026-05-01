package protocol

import (
	"encoding/json"
	"fmt"
)

type MessageType string

const (
	TypePing        MessageType = "PING"
	TypePong        MessageType = "PONG"
	TypeChat        MessageType = "CHAT"
	TypeFileRequest MessageType = "FILE_REQ"
	TypeFileChunk   MessageType = "FILE_CHUNK"
	TypeFileAck     MessageType = "FILE_ACK"
	TypeGroupCreate MessageType = "GROUP_CREATE"
)

type Header struct {
	MessageType MessageType `json:"type"`
	SenderID    string      `json:"sender_id"`
	SenderName  string      `json:"sender_name"`
	RecipientID string      `json:"recipient_id"`
}

type PingMessage struct {
	Header
	Timestamp int64 `json:"timestamp"`
}

type ChatMessage struct {
	Message string `json:"message"`
}

func Encode(header Header, payload interface{}) ([]byte, error) {
	temp := struct {
		Header
		Payload interface{} `json:"payload"`
	}{
		Header:  header,
		Payload: payload,
	}
	return json.Marshal(temp)
}

func Decode(data []byte) (Header, map[string]interface{}, error) {
	var temp struct {
		Header
		Payload json.RawMessage `json:"payload"`
	}
	err := json.Unmarshal(data, &temp)
	if err != nil {
		return Header{}, nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	var payloadMap map[string]interface{}
	// Безопасная проверка RawMessage
	if len(temp.Payload) > 0 && string(temp.Payload) != "null" {
		err = json.Unmarshal(temp.Payload, &payloadMap)
		if err != nil {
			return temp.Header, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
	}

	return temp.Header, payloadMap, nil
}

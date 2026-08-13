package protocol

import (
	"encoding/json"
	"fmt"
)

// MessageType — тип сообщения.
type MessageType string

const (
	TypePing         MessageType = "PING"
	TypePong         MessageType = "PONG"
	TypeChat         MessageType = "CHAT"
	TypeFileRequest  MessageType = "FILE_REQ"
	TypeFileAck      MessageType = "FILE_ACK"
	TypeFileAnnounce MessageType = "FILE_ANN"
	TypeChunkRequest MessageType = "CHUNK_REQ"
	TypeFileChunk    MessageType = "FILE_CHUNK"
	TypeCatalog      MessageType = "CATALOG"
)

// Header — заголовок сообщения.
type Header struct {
	MessageType MessageType `json:"type"`
	SenderID    string      `json:"sender_id"`
	SenderName  string      `json:"sender_name"`
	RecipientID string      `json:"recipient_id"`
}

// PingMessage — служебный ping.
type PingMessage struct {
	Header
	Timestamp int64 `json:"timestamp"`
}

// ChatMessage — сообщение чата.
type ChatMessage struct {
	Message string `json:"message"`
}

// FileAnnouncePayload — анонс файла.
type FileAnnouncePayload struct {
	FileID      string   `json:"file_id"`
	FileName    string   `json:"file_name"`
	FileSize    int64    `json:"file_size"`
	TotalChunks uint32   `json:"total_chunks"`
	Chunks      []uint32 `json:"chunks"`
}

// ChunkRequestPayload — запрос чанка.
type ChunkRequestPayload struct {
	FileID     string `json:"file_id"`
	ChunkIndex uint32 `json:"chunk_index"`
}

// ChunkPayload — данные чанка.
type ChunkPayload struct {
	FileID     string `json:"file_id"`
	ChunkIndex uint32 `json:"chunk_index"`
	Data       []byte `json:"data"`
}

// PeerInfo — информация о пире.
type PeerInfo struct {
	ID   string
	IP   string
	Port int
	Name string
}

// CatalogEntry — запись в каталоге файлов.
type CatalogEntry struct {
	FileID      string              `json:"file_id"`
	FileName    string              `json:"file_name"`
	FileSize    int64               `json:"file_size"`
	TotalChunks uint32              `json:"total_chunks"`
	Owners      map[string][]uint32 `json:"owners"`
}

// CatalogPayload — полезная нагрузка TypeCatalog.
type CatalogPayload struct {
	Entries []CatalogEntry `json:"entries"`
}

// Encode упаковывает заголовок и payload в JSON.
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

// Decode разбирает JSON-сообщение.
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
	if len(temp.Payload) > 0 && string(temp.Payload) != "null" {
		err = json.Unmarshal(temp.Payload, &payloadMap)
		if err != nil {
			return temp.Header, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
	}

	return temp.Header, payloadMap, nil
}

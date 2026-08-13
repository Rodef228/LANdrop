package types

// MessageType — тип сообщения в протоколе.
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

// PingMessage — служебное сообщение ping.
type PingMessage struct {
	Header
	Timestamp int64 `json:"timestamp"`
}

// ChatMessage — текстовое сообщение чата.
type ChatMessage struct {
	Message string `json:"message"`
}

// FileAnnouncePayload — анонс файла пиром.
type FileAnnouncePayload struct {
	FileID      string   `json:"file_id"`
	FileName    string   `json:"file_name"`
	FileSize    int64    `json:"file_size"`
	TotalChunks uint32   `json:"total_chunks"`
	Chunks      []uint32 `json:"chunks"`
}

// ChunkRequestPayload — запрос конкретного чанка.
type ChunkRequestPayload struct {
	FileID     string `json:"file_id"`
	ChunkIndex uint32 `json:"chunk_index"`
}

// ChunkPayload — данные одного чанка.
type ChunkPayload struct {
	FileID     string `json:"file_id"`
	ChunkIndex uint32 `json:"chunk_index"`
	Data       []byte `json:"data"`
}

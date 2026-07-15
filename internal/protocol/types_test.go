package protocol

import (
	"encoding/json"
	"testing"
)

func TestEncodeDecode_ChatMessage(t *testing.T) {
	header := Header{
		MessageType: TypeChat,
		SenderID:    "alice",
		SenderName:  "Alice",
		RecipientID: "",
	}
	payload := map[string]interface{}{
		"message":     "Hello, world!",
		"sender_name": "Alice",
	}

	data, err := Encode(header, payload)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decodedHeader, decodedPayload, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decodedHeader.MessageType != TypeChat {
		t.Errorf("Expected MessageType=CHAT, got %s", decodedHeader.MessageType)
	}
	if decodedHeader.SenderID != "alice" {
		t.Errorf("Expected SenderID=alice, got %s", decodedHeader.SenderID)
	}
	if decodedHeader.SenderName != "Alice" {
		t.Errorf("Expected SenderName=Alice, got %s", decodedHeader.SenderName)
	}

	msg, ok := decodedPayload["message"].(string)
	if !ok || msg != "Hello, world!" {
		t.Errorf("Expected message='Hello, world!', got %v", decodedPayload["message"])
	}
}

func TestEncodeDecode_FileAnnounce(t *testing.T) {
	header := Header{
		MessageType: TypeFileAnnounce,
		SenderID:    "bob",
		SenderName:  "Bob",
	}
	payload := FileAnnouncePayload{
		FileID:      "abc123",
		FileName:    "photo.jpg",
		FileSize:    1024,
		TotalChunks: 16,
		Chunks:      []uint32{0, 1, 2, 3, 4, 5},
	}

	data, err := Encode(header, payload)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decodedHeader, decodedPayload, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decodedHeader.MessageType != TypeFileAnnounce {
		t.Errorf("Expected MessageType=FILE_ANN, got %s", decodedHeader.MessageType)
	}

	// Check payload fields
	if fID, ok := decodedPayload["file_id"].(string); !ok || fID != "abc123" {
		t.Errorf("Expected file_id=abc123, got %v", decodedPayload["file_id"])
	}
	if fName, ok := decodedPayload["file_name"].(string); !ok || fName != "photo.jpg" {
		t.Errorf("Expected file_name=photo.jpg, got %v", decodedPayload["file_name"])

	}
	if fs, ok := decodedPayload["file_size"].(float64); !ok || int64(fs) != 1024 {
		t.Errorf("Expected file_size=1024, got %v", decodedPayload["file_size"])
	}
	if tc, ok := decodedPayload["total_chunks"].(float64); !ok || uint32(tc) != 16 {
		t.Errorf("Expected total_chunks=16, got %v", decodedPayload["total_chunks"])
	}
}

func TestEncodeDecode_CatalogPayload(t *testing.T) {
	header := Header{
		MessageType: TypeCatalog,
		SenderID:    "alice",
		SenderName:  "Alice",
	}
	payload := CatalogPayload{
		Entries: []CatalogEntry{
			{
				FileID:      "file1",
				FileName:    "photo.jpg",
				FileSize:    1000,
				TotalChunks: 10,
				Owners: map[string][]uint32{
					"alice": {0, 1, 2},
					"bob":   {3, 4, 5},
				},
			},
			{
				FileID:      "file2",
				FileName:    "doc.pdf",
				FileSize:    2000,
				TotalChunks: 20,
				Owners: map[string][]uint32{
					"bob": {0, 1, 2, 3},
				},
			},
		},
	}

	data, err := Encode(header, payload)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decodedHeader, decodedPayload, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decodedHeader.MessageType != TypeCatalog {
		t.Errorf("Expected MessageType=CATALOG, got %s", decodedHeader.MessageType)
	}

	// Parse entries from decoded payload
	entriesRaw, ok := decodedPayload["entries"].([]interface{})
	if !ok {
		t.Fatal("Expected entries in decoded payload")
	}
	if len(entriesRaw) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entriesRaw))
	}

	entry1 := entriesRaw[0].(map[string]interface{})
	if entry1["file_id"] != "file1" {
		t.Errorf("Expected entry[0].file_id=file1, got %v", entry1["file_id"])
	}
	if entry1["file_name"] != "photo.jpg" {
		t.Errorf("Expected entry[0].file_name=photo.jpg, got %v", entry1["file_name"])
	}

	owners1 := entry1["owners"].(map[string]interface{})
	if len(owners1) != 2 {
		t.Errorf("Expected 2 owners for file1, got %d", len(owners1))
	}
}

func TestEncodeDecode_EmptyPayload(t *testing.T) {
	header := Header{
		MessageType: TypePing,
		SenderID:    "node1",
		SenderName:  "Node1",
	}

	data, err := Encode(header, nil)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decodedHeader, decodedPayload, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decodedHeader.MessageType != TypePing {
		t.Errorf("Expected MessageType=PING, got %s", decodedHeader.MessageType)
	}
	if decodedPayload != nil {
		t.Errorf("Expected nil payload for PING, got %v", decodedPayload)
	}
}

func TestEncodeDecode_ChunkPayload(t *testing.T) {
	header := Header{
		MessageType: TypeFileChunk,
		SenderID:    "alice",
		SenderName:  "Alice",
	}
	payload := ChunkPayload{
		FileID:     "abc123",
		ChunkIndex: 5,
		Data:       []byte("hello-world-chunk-data"),
	}

	data, err := Encode(header, payload)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decodedHeader, decodedPayload, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decodedHeader.MessageType != TypeFileChunk {
		t.Errorf("Expected MessageType=FILE_CHUNK, got %s", decodedHeader.MessageType)
	}
	if fID, ok := decodedPayload["file_id"].(string); !ok || fID != "abc123" {
		t.Errorf("Expected file_id=abc123, got %v", decodedPayload["file_id"])
	}
	if ci, ok := decodedPayload["chunk_index"].(float64); !ok || uint32(ci) != 5 {
		t.Errorf("Expected chunk_index=5, got %v", decodedPayload["chunk_index"])
	}
}

func TestDecode_InvalidData(t *testing.T) {
	_, _, err := Decode([]byte("invalid json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestDecode_EmptyData(t *testing.T) {
	_, _, err := Decode([]byte{})
	if err == nil {
		t.Error("Expected error for empty data")
	}
}

func TestMessageTypeConstants(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{TypePing, "PING"},
		{TypePong, "PONG"},
		{TypeChat, "CHAT"},
		{TypeFileRequest, "FILE_REQ"},
		{TypeFileAck, "FILE_ACK"},
		{TypeFileAnnounce, "FILE_ANN"},
		{TypeChunkRequest, "CHUNK_REQ"},
		{TypeFileChunk, "FILE_CHUNK"},
		{TypeCatalog, "CATALOG"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.expected {
			t.Errorf("Expected %s=%s, got %s", tt.msgType, tt.expected, string(tt.msgType))
		}
	}
}

func TestCatalogEntry_JSONRoundTrip(t *testing.T) {
	original := CatalogEntry{
		FileID:      "file1",
		FileName:    "photo.jpg",
		FileSize:    1000,
		TotalChunks: 10,
		Owners: map[string][]uint32{
			"alice": {0, 1, 2},
			"bob":   {3, 4, 5},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded CatalogEntry
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.FileID != original.FileID {
		t.Errorf("Expected FileID=%s, got %s", original.FileID, decoded.FileID)
	}
	if decoded.FileName != original.FileName {
		t.Errorf("Expected FileName=%s, got %s", original.FileName, decoded.FileName)
	}
	if decoded.FileSize != original.FileSize {
		t.Errorf("Expected FileSize=%d, got %d", original.FileSize, decoded.FileSize)
	}
	if decoded.TotalChunks != original.TotalChunks {
		t.Errorf("Expected TotalChunks=%d, got %d", original.TotalChunks, decoded.TotalChunks)
	}
	if len(decoded.Owners) != 2 {
		t.Errorf("Expected 2 owners, got %d", len(decoded.Owners))
	}
}

func TestCatalogPayload_JSONRoundTrip(t *testing.T) {
	original := CatalogPayload{
		Entries: []CatalogEntry{
			{
				FileID:      "file1",
				FileName:    "photo.jpg",
				FileSize:    1000,
				TotalChunks: 10,
				Owners: map[string][]uint32{
					"alice": {0, 1},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded CatalogPayload
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(decoded.Entries))
	}
	if decoded.Entries[0].FileID != "file1" {
		t.Errorf("Expected FileID=file1, got %s", decoded.Entries[0].FileID)
	}
}

func TestPeerInfo_Struct(t *testing.T) {
	pi := PeerInfo{
		ID:   "alice",
		IP:   "10.0.0.1",
		Port: 9000,
		Name: "Alice",
	}

	if pi.ID != "alice" {
		t.Errorf("Expected ID=alice, got %s", pi.ID)
	}
	if pi.IP != "10.0.0.1" {
		t.Errorf("Expected IP=10.0.0.1, got %s", pi.IP)
	}
	if pi.Port != 9000 {
		t.Errorf("Expected Port=9000, got %d", pi.Port)
	}
	if pi.Name != "Alice" {
		t.Errorf("Expected Name=Alice, got %s", pi.Name)
	}
}

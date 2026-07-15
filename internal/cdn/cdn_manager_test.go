package cdn

import (
	"testing"

	"mesh-cu/internal/protocol"
)

// newMockCDNManager creates a CDNManager with a real FileManager in a temp dir.
func newMockCDNManager(t *testing.T, nodeID string) *CDNManager {
	t.Helper()
	fm, err := NewFileManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create temp FileManager: %v", err)
	}
	return &CDNManager{
		fm:        fm,
		Files:     make(map[string]*FileInfo),
		PeerFiles: make(map[string]map[string][]uint32),
		NodeID:    nodeID,
	}
}

func TestNewCDNManager(t *testing.T) {
	fm, err := NewFileManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cm := NewCDNManager("test-node", fm)
	if cm == nil {
		t.Fatal("NewCDNManager returned nil")
	}
	if cm.NodeID != "test-node" {
		t.Errorf("Expected NodeID=test-node, got %s", cm.NodeID)
	}
	if len(cm.Files) != 0 {
		t.Errorf("Expected empty Files map, got %d", len(cm.Files))
	}
	if len(cm.PeerFiles) != 0 {
		t.Errorf("Expected empty PeerFiles map, got %d", len(cm.PeerFiles))
	}
}

func TestHandleAnnounce_NewFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	payload := protocol.FileAnnouncePayload{
		FileID:      "file1",
		FileName:    "photo.jpg",
		FileSize:    1000,
		TotalChunks: 10,
		Chunks:      []uint32{0, 1, 2},
	}

	cm.HandleAnnounce(payload, "alice")

	fi := cm.GetFileInfo("file1")
	if fi == nil {
		t.Fatal("Expected FileInfo for file1")
	}
	if fi.Name != "photo.jpg" {
		t.Errorf("Expected Name=photo.jpg, got %s", fi.Name)
	}
	if fi.Size != 1000 {
		t.Errorf("Expected Size=1000, got %d", fi.Size)
	}
	if fi.TotalChunks != 10 {
		t.Errorf("Expected TotalChunks=10, got %d", fi.TotalChunks)
	}

	// Check that alice's chunks are stored in PeerFiles
	peers, ok := cm.PeerFiles["file1"]
	if !ok {
		t.Fatal("Expected PeerFiles entry for file1")
	}
	chunks, ok := peers["alice"]
	if !ok {
		t.Fatal("Expected alice in PeerFiles[file1]")
	}
	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks for alice, got %d", len(chunks))
	}
}

func TestHandleAnnounce_EmptyChunksMeansAll(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	payload := protocol.FileAnnouncePayload{
		FileID:      "file1",
		FileName:    "doc.pdf",
		FileSize:    5000,
		TotalChunks: 8,
		Chunks:      nil, // empty — means all
	}

	cm.HandleAnnounce(payload, "alice")

	peers, ok := cm.PeerFiles["file1"]
	if !ok {
		t.Fatal("Expected PeerFiles entry for file1")
	}
	chunks, ok := peers["alice"]
	if !ok {
		t.Fatal("Expected alice in PeerFiles[file1]")
	}
	if len(chunks) != 8 {
		t.Errorf("Expected 8 chunks (all), got %d", len(chunks))
	}
	for i := uint32(0); i < 8; i++ {
		if chunks[i] != i {
			t.Errorf("Expected chunks[%d]=%d, got %d", i, i, chunks[i])
		}
	}
}

func TestHandleAnnounce_SecondPeer(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{3, 4, 5},
	}, "bob")

	peers := cm.PeerFiles["file1"]
	if len(peers) != 2 {
		t.Errorf("Expected 2 peers for file1, got %d", len(peers))
	}
	if len(peers["alice"]) != 3 {
		t.Errorf("Expected 3 chunks for alice, got %d", len(peers["alice"]))
	}
	if len(peers["bob"]) != 3 {
		t.Errorf("Expected 3 chunks for bob, got %d", len(peers["bob"]))
	}
}

func TestHandleAnnounce_UpdatesExistingPeer(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2, 3, 4, 5},
	}, "alice")

	peers := cm.PeerFiles["file1"]
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer, got %d", len(peers))
	}
	if len(peers["alice"]) != 6 {
		t.Errorf("Expected 6 chunks for alice, got %d", len(peers["alice"]))
	}
}

func TestHandleAnnounce_DoesNotOverwriteExistingFileInfo(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	// First announce creates FileInfo
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")

	// Second announce with different metadata — FileInfo should NOT be overwritten
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "hacked.jpg", FileSize: 9999, TotalChunks: 99,
		Chunks: []uint32{3, 4},
	}, "bob")

	fi := cm.GetFileInfo("file1")
	if fi.Name != "photo.jpg" {
		t.Errorf("Expected FileInfo.Name to remain photo.jpg, got %s", fi.Name)
	}
	if fi.Size != 1000 {
		t.Errorf("Expected FileInfo.Size to remain 1000, got %d", fi.Size)
	}
}

func TestGetChunkOwners(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")
	cm.HandleAnnounce(protocol.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{2, 3, 4},
	}, "bob")

	// Chunk 0: only alice
	owners := cm.GetChunkOwners("file1", 0)
	if len(owners) != 1 || owners[0] != "alice" {
		t.Errorf("Expected owners=[alice] for chunk 0, got %v", owners)
	}

	// Chunk 2: both alice and bob
	owners = cm.GetChunkOwners("file1", 2)
	if len(owners) != 2 {
		t.Errorf("Expected 2 owners for chunk 2, got %v", owners)
	}

	// Chunk 9: nobody
	owners = cm.GetChunkOwners("file1", 9)
	if len(owners) != 0 {
		t.Errorf("Expected 0 owners for chunk 9, got %v", owners)
	}
}

func TestGetChunkOwners_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	owners := cm.GetChunkOwners("nonexistent", 0)
	if owners != nil {
		t.Errorf("Expected nil for unknown file, got %v", owners)
	}
}

func TestRegisterLocalFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	fi := cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	if fi == nil {
		t.Fatal("RegisterLocalFile returned nil")
	}
	if fi.ID != "file1" {
		t.Errorf("Expected ID=file1, got %s", fi.ID)
	}
	if fi.Name != "photo.jpg" {
		t.Errorf("Expected Name=photo.jpg, got %s", fi.Name)
	}
	if fi.Size != 1000 {
		t.Errorf("Expected Size=1000, got %d", fi.Size)
	}
	if fi.OriginalPath != "/path/to/photo.jpg" {
		t.Errorf("Expected OriginalPath=/path/to/photo.jpg, got %s", fi.OriginalPath)
	}

	// TotalChunks should be ceil(1000 / 65536) = 1
	if fi.TotalChunks != 1 {
		t.Errorf("Expected TotalChunks=1 for 1000 byte file, got %d", fi.TotalChunks)
	}

	// Should be stored in Files map
	stored := cm.GetFileInfo("file1")
	if stored == nil {
		t.Fatal("Expected file1 in Files map")
	}
}

func TestRegisterLocalFile_LargeFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	// 200 KB file
	fi := cm.RegisterLocalFile("file1", "large.bin", 200*1024, "/path/to/large.bin")

	expectedChunks := uint32((200*1024 + ChunkSize - 1) / ChunkSize)
	if fi.TotalChunks != expectedChunks {
		t.Errorf("Expected TotalChunks=%d for 200KB file, got %d", expectedChunks, fi.TotalChunks)
	}

	// All chunks should be owned
	for i := uint32(0); i < fi.TotalChunks; i++ {
		if !fi.OwnedChunks[i] {
			t.Errorf("Expected chunk %d to be owned", i)
		}
	}
}

func TestGetOwnedChunks(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	chunks := cm.GetOwnedChunks("file1")
	if len(chunks) != 1 {
		t.Errorf("Expected 1 owned chunk, got %d", len(chunks))
	}
	if chunks[0] != 0 {
		t.Errorf("Expected owned chunk index 0, got %d", chunks[0])
	}
}

func TestGetOwnedChunks_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	chunks := cm.GetOwnedChunks("nonexistent")
	if chunks != nil {
		t.Errorf("Expected nil for unknown file, got %v", chunks)
	}
}

func TestGetFileInfo_ReturnsCopy(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	fi1 := cm.GetFileInfo("file1")
	fi2 := cm.GetFileInfo("file1")

	// Modify fi1
	fi1.Name = "hacked.jpg"
	fi1.OwnedChunks[0] = false

	// fi2 should be unchanged
	if fi2.Name != "photo.jpg" {
		t.Errorf("Original was modified! Expected Name=photo.jpg, got %s", fi2.Name)
	}
	if !fi2.OwnedChunks[0] {
		t.Error("Original was modified! Expected OwnedChunks[0]=true")
	}
}

func TestGetFileInfo_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	fi := cm.GetFileInfo("nonexistent")
	if fi != nil {
		t.Errorf("Expected nil for unknown file, got %v", fi)
	}
}

func TestMarkChunkOwned(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	// Mark additional chunks (even though file is small, we can still mark)
	cm.MarkChunkOwned("file1", 5)
	cm.MarkChunkOwned("file1", 10)

	fi := cm.GetFileInfo("file1")
	if !fi.OwnedChunks[5] {
		t.Error("Expected chunk 5 to be owned after MarkChunkOwned")
	}
	if !fi.OwnedChunks[10] {
		t.Error("Expected chunk 10 to be owned after MarkChunkOwned")
	}
}

func TestMarkChunkOwned_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	// Should not panic
	cm.MarkChunkOwned("nonexistent", 0)
}

func TestUpdateNodeName(t *testing.T) {
	cm := newMockCDNManager(t, "old-name")
	cm.UpdateNodeName("new-name")
	if cm.NodeID != "new-name" {
		t.Errorf("Expected NodeID=new-name, got %s", cm.NodeID)
	}
}

func TestReAnnounce_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	called := false
	sendFn := func(peerIP string, peerPort int, data []byte) error {
		called = true
		return nil
	}

	// Should not call sendFn for unknown file
	cm.ReAnnounce("nonexistent", nil, sendFn)
	if called {
		t.Error("Expected sendFn NOT to be called for unknown file")
	}
}

func TestReAnnounce_SendsToOthers(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	var sentTo []string
	sendFn := func(peerIP string, peerPort int, data []byte) error {
		sentTo = append(sentTo, peerIP)
		return nil
	}

	peers := []protocol.PeerInfo{
		{ID: "me", IP: "127.0.0.1", Port: 9000, Name: "me"},      // should be skipped
		{ID: "alice", IP: "10.0.0.1", Port: 9001, Name: "alice"}, // should receive
		{ID: "bob", IP: "10.0.0.2", Port: 9002, Name: "bob"},     // should receive
	}

	cm.ReAnnounce("file1", peers, sendFn)

	if len(sentTo) != 2 {
		t.Errorf("Expected 2 sends, got %d: %v", len(sentTo), sentTo)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			cm.HandleAnnounce(protocol.FileAnnouncePayload{
				FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
				Chunks: []uint32{0, 1, 2},
			}, "alice")
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			cm.GetChunkOwners("file1", 0)
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			cm.MarkChunkOwned("file1", uint32(i%10))
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			cm.GetFileInfo("file1")
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}
}

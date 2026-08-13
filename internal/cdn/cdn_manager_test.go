package cdn

import (
	"testing"

	"landrop/internal/helpers"
	"landrop/internal/types"
)

func newMockCDNManager(t *testing.T, nodeID string) *CDNManager {
	t.Helper()
	fm, err := helpers.NewFileManager(t.TempDir())
	if err != nil {
		t.Fatalf("Ошибка создания временного FileManager: %v", err)
	}
	return &CDNManager{
		fm:        fm,
		Files:     make(map[string]*types.FileInfo),
		PeerFiles: make(map[string]map[string][]uint32),
		NodeID:    nodeID,
	}
}

func TestNewCDNManager(t *testing.T) {
	fm, err := helpers.NewFileManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cm := NewCDNManager("test-node", fm)
	if cm == nil {
		t.Fatal("NewCDNManager вернул nil")
	}
	if cm.NodeID != "test-node" {
		t.Errorf("Ожидалось NodeID=test-node, получено %s", cm.NodeID)
	}
	if len(cm.Files) != 0 {
		t.Errorf("Ожидался пустой Files, получено %d", len(cm.Files))
	}
	if len(cm.PeerFiles) != 0 {
		t.Errorf("Ожидался пустой PeerFiles, получено %d", len(cm.PeerFiles))
	}
}

func TestHandleAnnounce_NewFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	payload := types.FileAnnouncePayload{
		FileID:      "file1",
		FileName:    "photo.jpg",
		FileSize:    1000,
		TotalChunks: 10,
		Chunks:      []uint32{0, 1, 2},
	}

	cm.HandleAnnounce(payload, "alice")

	fi := cm.GetFileInfo("file1")
	if fi == nil {
		t.Fatal("Ожидался FileInfo для file1")
	}
	if fi.Name != "photo.jpg" {
		t.Errorf("Ожидалось Name=photo.jpg, получено %s", fi.Name)
	}
	if fi.Size != 1000 {
		t.Errorf("Ожидалось Size=1000, получено %d", fi.Size)
	}
	if fi.TotalChunks != 10 {
		t.Errorf("Ожидалось TotalChunks=10, получено %d", fi.TotalChunks)
	}

	peers, ok := cm.PeerFiles["file1"]
	if !ok {
		t.Fatal("Ожидался PeerFiles для file1")
	}
	chunks, ok := peers["alice"]
	if !ok {
		t.Fatal("Ожидалась alice в PeerFiles[file1]")
	}
	if len(chunks) != 3 {
		t.Errorf("Ожидалось 3 чанка для alice, получено %d", len(chunks))
	}
}

func TestHandleAnnounce_EmptyChunksMeansAll(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	payload := types.FileAnnouncePayload{
		FileID:      "file1",
		FileName:    "doc.pdf",
		FileSize:    5000,
		TotalChunks: 8,
		Chunks:      nil,
	}

	cm.HandleAnnounce(payload, "alice")

	peers, ok := cm.PeerFiles["file1"]
	if !ok {
		t.Fatal("Ожидался PeerFiles для file1")
	}
	chunks, ok := peers["alice"]
	if !ok {
		t.Fatal("Ожидалась alice в PeerFiles[file1]")
	}
	if len(chunks) != 8 {
		t.Errorf("Ожидалось 8 чанков (все), получено %d", len(chunks))
	}
	for i := uint32(0); i < 8; i++ {
		if chunks[i] != i {
			t.Errorf("Ожидалось chunks[%d]=%d, получено %d", i, i, chunks[i])
		}
	}
}

func TestHandleAnnounce_SecondPeer(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{3, 4, 5},
	}, "bob")

	peers := cm.PeerFiles["file1"]
	if len(peers) != 2 {
		t.Errorf("Ожидалось 2 пира для file1, получено %d", len(peers))
	}
	if len(peers["alice"]) != 3 {
		t.Errorf("Ожидалось 3 чанка для alice, получено %d", len(peers["alice"]))
	}
	if len(peers["bob"]) != 3 {
		t.Errorf("Ожидалось 3 чанка для bob, получено %d", len(peers["bob"]))
	}
}

func TestHandleAnnounce_UpdatesExistingPeer(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2, 3, 4, 5},
	}, "alice")

	peers := cm.PeerFiles["file1"]
	if len(peers) != 1 {
		t.Errorf("Ожидался 1 пир, получено %d", len(peers))
	}
	if len(peers["alice"]) != 6 {
		t.Errorf("Ожидалось 6 чанков для alice, получено %d", len(peers["alice"]))
	}
}

func TestHandleAnnounce_DoesNotOverwriteExistingFileInfo(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")

	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "hacked.jpg", FileSize: 9999, TotalChunks: 99,
		Chunks: []uint32{3, 4},
	}, "bob")

	fi := cm.GetFileInfo("file1")
	if fi.Name != "photo.jpg" {
		t.Errorf("Ожидалось, что FileInfo.Name останется photo.jpg, получено %s", fi.Name)
	}
	if fi.Size != 1000 {
		t.Errorf("Ожидалось, что FileInfo.Size останется 1000, получено %d", fi.Size)
	}
}

func TestGetChunkOwners(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{0, 1, 2},
	}, "alice")
	cm.HandleAnnounce(types.FileAnnouncePayload{
		FileID: "file1", FileName: "photo.jpg", FileSize: 1000, TotalChunks: 10,
		Chunks: []uint32{2, 3, 4},
	}, "bob")

	owners := cm.GetChunkOwners("file1", 0)
	if len(owners) != 1 || owners[0] != "alice" {
		t.Errorf("Ожидалось owners=[alice] для чанка 0, получено %v", owners)
	}

	owners = cm.GetChunkOwners("file1", 2)
	if len(owners) != 2 {
		t.Errorf("Ожидалось 2 владельца для чанка 2, получено %v", owners)
	}

	owners = cm.GetChunkOwners("file1", 9)
	if len(owners) != 0 {
		t.Errorf("Ожидалось 0 владельцев для чанка 9, получено %v", owners)
	}
}

func TestGetChunkOwners_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	owners := cm.GetChunkOwners("nonexistent", 0)
	if owners != nil {
		t.Errorf("Ожидался nil для неизвестного файла, получено %v", owners)
	}
}

func TestRegisterLocalFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	fi := cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	if fi == nil {
		t.Fatal("RegisterLocalFile вернул nil")
	}
	if fi.ID != "file1" {
		t.Errorf("Ожидалось ID=file1, получено %s", fi.ID)
	}
	if fi.Name != "photo.jpg" {
		t.Errorf("Ожидалось Name=photo.jpg, получено %s", fi.Name)
	}
	if fi.Size != 1000 {
		t.Errorf("Ожидалось Size=1000, получено %d", fi.Size)
	}
	if fi.OriginalPath != "/path/to/photo.jpg" {
		t.Errorf("Ожидалось OriginalPath=/path/to/photo.jpg, получено %s", fi.OriginalPath)
	}
	if fi.TotalChunks != 1 {
		t.Errorf("Ожидалось TotalChunks=1 для 1000 байт, получено %d", fi.TotalChunks)
	}

	stored := cm.GetFileInfo("file1")
	if stored == nil {
		t.Fatal("Ожидался file1 в Files")
	}
}

func TestRegisterLocalFile_LargeFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	fi := cm.RegisterLocalFile("file1", "large.bin", 200*1024, "/path/to/large.bin")

	expectedChunks := uint32((200*1024 + helpers.ChunkSize - 1) / helpers.ChunkSize)
	if fi.TotalChunks != expectedChunks {
		t.Errorf("Ожидалось TotalChunks=%d для 200KB, получено %d", expectedChunks, fi.TotalChunks)
	}

	for i := uint32(0); i < fi.TotalChunks; i++ {
		if !fi.OwnedChunks[i] {
			t.Errorf("Ожидалось, что чанк %d принадлежит нам", i)
		}
	}
}

func TestGetOwnedChunks(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	chunks := cm.GetOwnedChunks("file1")
	if len(chunks) != 1 {
		t.Errorf("Ожидался 1 чанк, получено %d", len(chunks))
	}
	if chunks[0] != 0 {
		t.Errorf("Ожидался индекс чанка 0, получено %d", chunks[0])
	}
}

func TestGetOwnedChunks_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	chunks := cm.GetOwnedChunks("nonexistent")
	if chunks != nil {
		t.Errorf("Ожидался nil для неизвестного файла, получено %v", chunks)
	}
}

func TestGetFileInfo_ReturnsCopy(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	fi1 := cm.GetFileInfo("file1")
	fi2 := cm.GetFileInfo("file1")

	fi1.Name = "hacked.jpg"
	fi1.OwnedChunks[0] = false

	if fi2.Name != "photo.jpg" {
		t.Errorf("Оригинал изменился! Ожидалось Name=photo.jpg, получено %s", fi2.Name)
	}
	if !fi2.OwnedChunks[0] {
		t.Error("Оригинал изменился! Ожидалось OwnedChunks[0]=true")
	}
}

func TestGetFileInfo_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	fi := cm.GetFileInfo("nonexistent")
	if fi != nil {
		t.Errorf("Ожидался nil для неизвестного файла, получено %v", fi)
	}
}

func TestMarkChunkOwned(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.RegisterLocalFile("file1", "photo.jpg", 1000, "/path/to/photo.jpg")

	cm.MarkChunkOwned("file1", 5)
	cm.MarkChunkOwned("file1", 10)

	fi := cm.GetFileInfo("file1")
	if !fi.OwnedChunks[5] {
		t.Error("Ожидалось, что чанк 5 принадлежит нам после MarkChunkOwned")
	}
	if !fi.OwnedChunks[10] {
		t.Error("Ожидалось, что чанк 10 принадлежит нам после MarkChunkOwned")
	}
}

func TestMarkChunkOwned_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	cm.MarkChunkOwned("nonexistent", 0)
}

func TestUpdateNodeName(t *testing.T) {
	cm := newMockCDNManager(t, "old-name")
	cm.UpdateNodeName("new-name")
	if cm.NodeID != "new-name" {
		t.Errorf("Ожидалось NodeID=new-name, получено %s", cm.NodeID)
	}
}

func TestReAnnounce_UnknownFile(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	called := false
	sendFn := func(peerIP string, peerPort int, data []byte) error {
		called = true
		return nil
	}

	cm.ReAnnounce("nonexistent", nil, sendFn)
	if called {
		t.Error("Ожидалось, что sendFn НЕ будет вызван для неизвестного файла")
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

	peers := []types.PeerInfo{
		{ID: "me", IP: "127.0.0.1", Port: 9000, Name: "me"},
		{ID: "alice", IP: "10.0.0.1", Port: 9001, Name: "alice"},
		{ID: "bob", IP: "10.0.0.2", Port: 9002, Name: "bob"},
	}

	cm.ReAnnounce("file1", peers, sendFn)

	if len(sentTo) != 2 {
		t.Errorf("Ожидалось 2 отправки, получено %d: %v", len(sentTo), sentTo)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cm := newMockCDNManager(t, "me")
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			cm.HandleAnnounce(types.FileAnnouncePayload{
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

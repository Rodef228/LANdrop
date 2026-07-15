package catalog

import (
	"testing"

	"mesh-cu/internal/types"
)

func TestNewCatalog(t *testing.T) {
	c := NewCatalog()
	if c == nil {
		t.Fatal("NewCatalog() вернул nil")
	}
	entries := c.GetEntries()
	if len(entries) != 0 {
		t.Errorf("Ожидался пустой каталог, получено %d записей", len(entries))
	}
}

func TestAddOrUpdate_NewFile(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидалась запись для file1, получен nil")
	}
	if entry.FileName != "photo.jpg" {
		t.Errorf("Ожидалось FileName=photo.jpg, получено %s", entry.FileName)
	}
	if entry.FileSize != 1000 {
		t.Errorf("Ожидалось FileSize=1000, получено %d", entry.FileSize)
	}
	if entry.TotalChunks != 10 {
		t.Errorf("Ожидалось TotalChunks=10, получено %d", entry.TotalChunks)
	}
	if len(entry.Owners) != 1 {
		t.Errorf("Ожидался 1 владелец, получено %d", len(entry.Owners))
	}
	chunks, ok := entry.Owners["alice"]
	if !ok {
		t.Fatal("Ожидалась alice в списке владельцев")
	}
	if len(chunks) != 3 {
		t.Errorf("Ожидалось 3 чанка для alice, получено %d", len(chunks))
	}
}

func TestAddOrUpdate_EmptyChunksMeansAll(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "doc.pdf", 5000, 8, nil)

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидалась запись для file1")
	}
	chunks, ok := entry.Owners["alice"]
	if !ok {
		t.Fatal("Ожидалась alice в списке владельцев")
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

func TestAddOrUpdate_SecondPeer(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})
	c.AddOrUpdate("file1", "bob", "photo.jpg", 1000, 10, []uint32{3, 4, 5})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидалась запись для file1")
	}
	if len(entry.Owners) != 2 {
		t.Errorf("Ожидалось 2 владельца, получено %d", len(entry.Owners))
	}
	if len(entry.Owners["alice"]) != 3 {
		t.Errorf("Ожидалось 3 чанка для alice, получено %d", len(entry.Owners["alice"]))
	}
	if len(entry.Owners["bob"]) != 3 {
		t.Errorf("Ожидалось 3 чанка для bob, получено %d", len(entry.Owners["bob"]))
	}
}

func TestAddOrUpdate_UpdateExistingPeer(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2, 3, 4, 5})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидалась запись для file1")
	}
	if len(entry.Owners) != 1 {
		t.Errorf("Ожидался 1 владелец, получено %d", len(entry.Owners))
	}
	if len(entry.Owners["alice"]) != 6 {
		t.Errorf("Ожидалось 6 чанков для alice, получено %d", len(entry.Owners["alice"]))
	}
}

func TestAddOrUpdate_MultipleFiles(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file2", "bob", "doc.pdf", 2000, 20, []uint32{0, 1, 2})

	entries := c.GetEntries()
	if len(entries) != 2 {
		t.Errorf("Ожидалось 2 записи, получено %d", len(entries))
	}

	e1 := c.GetEntry("file1")
	if e1 == nil || e1.FileName != "photo.jpg" {
		t.Errorf("Ожидалось file1=photo.jpg, получено %v", e1)
	}
	e2 := c.GetEntry("file2")
	if e2 == nil || e2.FileName != "doc.pdf" {
		t.Errorf("Ожидалось file2=doc.pdf, получено %v", e2)
	}
}

func TestRemovePeer_SingleFile(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	affected := c.RemovePeer("alice")
	if len(affected) != 1 || affected[0] != "file1" {
		t.Errorf("Ожидалось affected=[file1], получено %v", affected)
	}

	entry := c.GetEntry("file1")
	if entry != nil {
		t.Errorf("Ожидалось удаление записи, получено %v", entry)
	}
}

func TestRemovePeer_MultipleFiles(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file2", "alice", "doc.pdf", 2000, 20, []uint32{0, 1, 2})
	c.AddOrUpdate("file3", "bob", "notes.txt", 500, 5, []uint32{0})

	affected := c.RemovePeer("alice")
	if len(affected) != 2 {
		t.Errorf("Ожидалось 2 затронутых файла, получено %d: %v", len(affected), affected)
	}

	if c.GetEntry("file1") != nil {
		t.Error("Ожидалось удаление file1")
	}
	if c.GetEntry("file2") != nil {
		t.Error("Ожидалось удаление file2")
	}
	if c.GetEntry("file3") == nil {
		t.Error("Ожидалось, что file3 останется")
	}
}

func TestRemovePeer_OnlyFromOneFile(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file1", "bob", "photo.jpg", 1000, 10, []uint32{2, 3})

	affected := c.RemovePeer("alice")
	if len(affected) != 1 || affected[0] != "file1" {
		t.Errorf("Ожидалось affected=[file1], получено %v", affected)
	}

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидалось, что file1 останется")
	}
	if len(entry.Owners) != 1 {
		t.Errorf("Ожидался 1 оставшийся владелец, получено %d", len(entry.Owners))
	}
	if _, ok := entry.Owners["bob"]; !ok {
		t.Error("Ожидалось, что bob останется владельцем")
	}
}

func TestRemovePeer_NonExistent(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0})

	affected := c.RemovePeer("nonexistent")
	if len(affected) != 0 {
		t.Errorf("Ожидался пустой список, получено %v", affected)
	}

	if c.GetEntry("file1") == nil {
		t.Error("Ожидалось, что file1 останется")
	}
}

func TestGetEntries_ReturnsCopy(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	entries := c.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Ожидалась 1 запись, получено %d", len(entries))
	}

	entries[0].FileName = "hacked.jpg"
	entries[0].Owners["alice"] = []uint32{99}

	original := c.GetEntry("file1")
	if original == nil {
		t.Fatal("Ожидалась оригинальная запись")
	}
	if original.FileName != "photo.jpg" {
		t.Errorf("Оригинал изменился! Ожидалось photo.jpg, получено %s", original.FileName)
	}
	if original.Owners["alice"][0] != 0 {
		t.Errorf("Оригинальные чанки изменились! Ожидалось [0 1 2], получено %v", original.Owners["alice"])
	}
}

func TestGetEntry_NonExistent(t *testing.T) {
	c := NewCatalog()
	entry := c.GetEntry("nonexistent")
	if entry != nil {
		t.Errorf("Ожидался nil для несуществующего файла, получено %v", entry)
	}
}

func TestGetEntry_ReturnsCopy(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидалась запись")
	}

	entry.FileName = "hacked.jpg"
	entry.Owners["alice"] = []uint32{99}

	entry2 := c.GetEntry("file1")
	if entry2.FileName != "photo.jpg" {
		t.Errorf("Оригинал изменился! Ожидалось photo.jpg, получено %s", entry2.FileName)
	}
}

func TestMerge_NewEntries(t *testing.T) {
	c := NewCatalog()
	entries := []types.CatalogEntry{
		{
			FileID:      "file1",
			FileName:    "photo.jpg",
			FileSize:    1000,
			TotalChunks: 10,
			Owners: map[string][]uint32{
				"alice": {0, 1, 2},
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
	}

	c.Merge(entries)

	if len(c.GetEntries()) != 2 {
		t.Errorf("Ожидалось 2 записи после merge, получено %d", len(c.GetEntries()))
	}
	if c.GetEntry("file1") == nil {
		t.Error("Ожидался file1 после merge")
	}
	if c.GetEntry("file2") == nil {
		t.Error("Ожидался file2 после merge")
	}
}

func TestMerge_UpdatesExisting(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})

	entries := []types.CatalogEntry{
		{
			FileID:      "file1",
			FileName:    "photo.jpg",
			FileSize:    1000,
			TotalChunks: 10,
			Owners: map[string][]uint32{
				"bob": {2, 3, 4},
			},
		},
	}
	c.Merge(entries)

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Ожидался file1")
	}
	if len(entry.Owners) != 2 {
		t.Errorf("Ожидалось 2 владельца после merge, получено %d", len(entry.Owners))
	}
	if _, ok := entry.Owners["alice"]; !ok {
		t.Error("Ожидалось, что alice останется владельцем")
	}
	if _, ok := entry.Owners["bob"]; !ok {
		t.Error("Ожидалось, что bob появится как владелец")
	}
}

func TestMerge_EmptyOwnersSkipped(t *testing.T) {
	c := NewCatalog()
	entries := []types.CatalogEntry{
		{
			FileID:      "file1",
			FileName:    "photo.jpg",
			FileSize:    1000,
			TotalChunks: 10,
			Owners:      map[string][]uint32{},
		},
	}
	c.Merge(entries)

	if c.GetEntry("file1") != nil {
		t.Error("Ожидалось, что file1 НЕ добавится (пустые владельцы)")
	}
}

func TestMerge_OverwritesMetadata(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "old_name.jpg", 100, 5, []uint32{0})

	entries := []types.CatalogEntry{
		{
			FileID:      "file1",
			FileName:    "new_name.jpg",
			FileSize:    200,
			TotalChunks: 10,
			Owners: map[string][]uint32{
				"alice": {0, 1},
			},
		},
	}
	c.Merge(entries)

	entry := c.GetEntry("file1")
	if entry.FileName != "new_name.jpg" {
		t.Errorf("Ожидалось FileName=new_name.jpg, получено %s", entry.FileName)
	}
	if entry.FileSize != 200 {
		t.Errorf("Ожидалось FileSize=200, получено %d", entry.FileSize)
	}
	if entry.TotalChunks != 10 {
		t.Errorf("Ожидалось TotalChunks=10, получено %d", entry.TotalChunks)
	}
}

func TestHasPeer(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file2", "bob", "doc.pdf", 2000, 20, []uint32{0, 1, 2})

	if !c.HasPeer("alice") {
		t.Error("Ожидалось HasPeer(alice)=true")
	}
	if !c.HasPeer("bob") {
		t.Error("Ожидалось HasPeer(bob)=true")
	}
	if c.HasPeer("charlie") {
		t.Error("Ожидалось HasPeer(charlie)=false")
	}
}

func TestHasPeer_AfterRemove(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.RemovePeer("alice")

	if c.HasPeer("alice") {
		t.Error("Ожидалось HasPeer(alice)=false после RemovePeer")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewCatalog()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			c.AddOrUpdate("file1", "bob", "photo.jpg", 1000, 10, []uint32{3, 4, 5})
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			c.GetEntries()
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			c.RemovePeer("alice")
			c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Error("Ожидалось, что file1 существует после конкурентного доступа")
	}
}

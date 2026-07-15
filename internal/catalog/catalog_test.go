package catalog

import (
	"mesh-cu/internal/protocol"
	"testing"
)

func TestNewCatalog(t *testing.T) {
	c := NewCatalog()
	if c == nil {
		t.Fatal("NewCatalog() returned nil")
	}
	entries := c.GetEntries()
	if len(entries) != 0 {
		t.Errorf("Expected empty catalog, got %d entries", len(entries))
	}
}

func TestAddOrUpdate_NewFile(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Expected entry for file1, got nil")
	}
	if entry.FileName != "photo.jpg" {
		t.Errorf("Expected FileName=photo.jpg, got %s", entry.FileName)
	}
	if entry.FileSize != 1000 {
		t.Errorf("Expected FileSize=1000, got %d", entry.FileSize)
	}
	if entry.TotalChunks != 10 {
		t.Errorf("Expected TotalChunks=10, got %d", entry.TotalChunks)
	}
	if len(entry.Owners) != 1 {
		t.Errorf("Expected 1 owner, got %d", len(entry.Owners))
	}
	chunks, ok := entry.Owners["alice"]
	if !ok {
		t.Fatal("Expected alice in owners")
	}
	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks for alice, got %d", len(chunks))
	}
}

func TestAddOrUpdate_EmptyChunksMeansAll(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "doc.pdf", 5000, 8, nil)

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Expected entry for file1")
	}
	chunks, ok := entry.Owners["alice"]
	if !ok {
		t.Fatal("Expected alice in owners")
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

func TestAddOrUpdate_SecondPeer(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})
	c.AddOrUpdate("file1", "bob", "photo.jpg", 1000, 10, []uint32{3, 4, 5})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Expected entry for file1")
	}
	if len(entry.Owners) != 2 {
		t.Errorf("Expected 2 owners, got %d", len(entry.Owners))
	}
	if len(entry.Owners["alice"]) != 3 {
		t.Errorf("Expected 3 chunks for alice, got %d", len(entry.Owners["alice"]))
	}
	if len(entry.Owners["bob"]) != 3 {
		t.Errorf("Expected 3 chunks for bob, got %d", len(entry.Owners["bob"]))
	}
}

func TestAddOrUpdate_UpdateExistingPeer(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})
	// Alice now has more chunks
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2, 3, 4, 5})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Expected entry for file1")
	}
	if len(entry.Owners) != 1 {
		t.Errorf("Expected 1 owner, got %d", len(entry.Owners))
	}
	if len(entry.Owners["alice"]) != 6 {
		t.Errorf("Expected 6 chunks for alice, got %d", len(entry.Owners["alice"]))
	}
}

func TestAddOrUpdate_MultipleFiles(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file2", "bob", "doc.pdf", 2000, 20, []uint32{0, 1, 2})

	entries := c.GetEntries()
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
	}

	e1 := c.GetEntry("file1")
	if e1 == nil || e1.FileName != "photo.jpg" {
		t.Errorf("Expected file1=photo.jpg, got %v", e1)
	}
	e2 := c.GetEntry("file2")
	if e2 == nil || e2.FileName != "doc.pdf" {
		t.Errorf("Expected file2=doc.pdf, got %v", e2)
	}
}

func TestRemovePeer_SingleFile(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	affected := c.RemovePeer("alice")
	if len(affected) != 1 || affected[0] != "file1" {
		t.Errorf("Expected affected=[file1], got %v", affected)
	}

	// Entry should be deleted (no owners left)
	entry := c.GetEntry("file1")
	if entry != nil {
		t.Errorf("Expected entry to be deleted, got %v", entry)
	}
}

func TestRemovePeer_MultipleFiles(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file2", "alice", "doc.pdf", 2000, 20, []uint32{0, 1, 2})
	c.AddOrUpdate("file3", "bob", "notes.txt", 500, 5, []uint32{0})

	affected := c.RemovePeer("alice")
	if len(affected) != 2 {
		t.Errorf("Expected 2 affected files, got %d: %v", len(affected), affected)
	}

	// file1 and file2 should be gone (only alice owned them)
	if c.GetEntry("file1") != nil {
		t.Error("Expected file1 to be deleted")
	}
	if c.GetEntry("file2") != nil {
		t.Error("Expected file2 to be deleted")
	}
	// file3 should still exist (bob owns it)
	if c.GetEntry("file3") == nil {
		t.Error("Expected file3 to still exist")
	}
}

func TestRemovePeer_OnlyFromOneFile(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file1", "bob", "photo.jpg", 1000, 10, []uint32{2, 3})

	affected := c.RemovePeer("alice")
	if len(affected) != 1 || affected[0] != "file1" {
		t.Errorf("Expected affected=[file1], got %v", affected)
	}

	// file1 should still exist (bob still owns chunks)
	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Expected file1 to still exist")
	}
	if len(entry.Owners) != 1 {
		t.Errorf("Expected 1 owner left, got %d", len(entry.Owners))
	}
	if _, ok := entry.Owners["bob"]; !ok {
		t.Error("Expected bob to still be an owner")
	}
}

func TestRemovePeer_NonExistent(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0})

	affected := c.RemovePeer("nonexistent")
	if len(affected) != 0 {
		t.Errorf("Expected empty affected list, got %v", affected)
	}

	// file1 should still exist
	if c.GetEntry("file1") == nil {
		t.Error("Expected file1 to still exist")
	}
}

func TestGetEntries_ReturnsCopy(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	entries := c.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	// Modify the returned entry
	entries[0].FileName = "hacked.jpg"
	entries[0].Owners["alice"] = []uint32{99}

	// Original should be unchanged
	original := c.GetEntry("file1")
	if original == nil {
		t.Fatal("Expected original entry")
	}
	if original.FileName != "photo.jpg" {
		t.Errorf("Original FileName was modified! Expected photo.jpg, got %s", original.FileName)
	}
	if original.Owners["alice"][0] != 0 {
		t.Errorf("Original chunks were modified! Expected [0 1 2], got %v", original.Owners["alice"])
	}
}

func TestGetEntry_NonExistent(t *testing.T) {
	c := NewCatalog()
	entry := c.GetEntry("nonexistent")
	if entry != nil {
		t.Errorf("Expected nil for nonexistent file, got %v", entry)
	}
}

func TestGetEntry_ReturnsCopy(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1, 2})

	entry := c.GetEntry("file1")
	if entry == nil {
		t.Fatal("Expected entry")
	}

	// Modify the returned entry
	entry.FileName = "hacked.jpg"
	entry.Owners["alice"] = []uint32{99}

	// Get again — should be unchanged
	entry2 := c.GetEntry("file1")
	if entry2.FileName != "photo.jpg" {
		t.Errorf("Original FileName was modified! Expected photo.jpg, got %s", entry2.FileName)
	}
}

func TestMerge_NewEntries(t *testing.T) {
	c := NewCatalog()
	entries := []protocol.CatalogEntry{
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
		t.Errorf("Expected 2 entries after merge, got %d", len(c.GetEntries()))
	}
	if c.GetEntry("file1") == nil {
		t.Error("Expected file1 after merge")
	}
	if c.GetEntry("file2") == nil {
		t.Error("Expected file2 after merge")
	}
}

func TestMerge_UpdatesExisting(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})

	// Merge with updated info from bob
	entries := []protocol.CatalogEntry{
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
		t.Fatal("Expected file1")
	}
	if len(entry.Owners) != 2 {
		t.Errorf("Expected 2 owners after merge, got %d", len(entry.Owners))
	}
	if _, ok := entry.Owners["alice"]; !ok {
		t.Error("Expected alice to still be an owner")
	}
	if _, ok := entry.Owners["bob"]; !ok {
		t.Error("Expected bob to be an owner after merge")
	}
}

func TestMerge_EmptyOwnersSkipped(t *testing.T) {
	c := NewCatalog()
	entries := []protocol.CatalogEntry{
		{
			FileID:      "file1",
			FileName:    "photo.jpg",
			FileSize:    1000,
			TotalChunks: 10,
			Owners:      map[string][]uint32{}, // empty owners
		},
	}
	c.Merge(entries)

	if c.GetEntry("file1") != nil {
		t.Error("Expected file1 to NOT be added (empty owners)")
	}
}

func TestMerge_OverwritesMetadata(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "old_name.jpg", 100, 5, []uint32{0})

	// Merge with updated metadata
	entries := []protocol.CatalogEntry{
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
		t.Errorf("Expected FileName=new_name.jpg, got %s", entry.FileName)
	}
	if entry.FileSize != 200 {
		t.Errorf("Expected FileSize=200, got %d", entry.FileSize)
	}
	if entry.TotalChunks != 10 {
		t.Errorf("Expected TotalChunks=10, got %d", entry.TotalChunks)
	}
}

func TestHasPeer(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.AddOrUpdate("file2", "bob", "doc.pdf", 2000, 20, []uint32{0, 1, 2})

	if !c.HasPeer("alice") {
		t.Error("Expected HasPeer(alice)=true")
	}
	if !c.HasPeer("bob") {
		t.Error("Expected HasPeer(bob)=true")
	}
	if c.HasPeer("charlie") {
		t.Error("Expected HasPeer(charlie)=false")
	}
}

func TestHasPeer_AfterRemove(t *testing.T) {
	c := NewCatalog()
	c.AddOrUpdate("file1", "alice", "photo.jpg", 1000, 10, []uint32{0, 1})
	c.RemovePeer("alice")

	if c.HasPeer("alice") {
		t.Error("Expected HasPeer(alice)=false after RemovePeer")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewCatalog()
	done := make(chan bool)

	// Concurrent writes
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

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Should not panic or deadlock
	entry := c.GetEntry("file1")
	if entry == nil {
		t.Error("Expected file1 to exist after concurrent access")
	}
}

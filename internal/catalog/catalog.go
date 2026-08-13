package catalog

import (
	"sync"

	"landrop/internal/types"
)

// Catalog — распределённый in-memory каталог файлов.
// Хранит информацию о том, у каких пиров какие файлы и чанки есть.
type Catalog struct {
	mu      sync.RWMutex
	entries map[string]*types.CatalogEntry
}

// NewCatalog создаёт пустой каталог.
func NewCatalog() *Catalog {
	return &Catalog{
		entries: make(map[string]*types.CatalogEntry),
	}
}

// AddOrUpdate добавляет или обновляет информацию о файле от пира.
func (c *Catalog) AddOrUpdate(fileID, peerID string, fileName string, fileSize int64, totalChunks uint32, chunks []uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[fileID]
	if !exists {
		entry = &types.CatalogEntry{
			FileID:      fileID,
			FileName:    fileName,
			FileSize:    fileSize,
			TotalChunks: totalChunks,
			Owners:      make(map[string][]uint32),
		}
		c.entries[fileID] = entry
	} else {
		entry.FileName = fileName
		entry.FileSize = fileSize
		entry.TotalChunks = totalChunks
	}

	if len(chunks) > 0 {
		entry.Owners[peerID] = chunks
	} else {
		allChunks := make([]uint32, totalChunks)
		for i := uint32(0); i < totalChunks; i++ {
			allChunks[i] = i
		}
		entry.Owners[peerID] = allChunks
	}
}

// RemovePeer удаляет все записи о пире. Возвращает список затронутых fileID.
func (c *Catalog) RemovePeer(peerID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var affectedFiles []string
	for fileID, entry := range c.entries {
		if _, exists := entry.Owners[peerID]; exists {
			delete(entry.Owners, peerID)
			affectedFiles = append(affectedFiles, fileID)
		}
		if len(entry.Owners) == 0 {
			delete(c.entries, fileID)
		}
	}
	return affectedFiles
}

// GetEntries возвращает копию всех записей каталога.
func (c *Catalog) GetEntries() []types.CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]types.CatalogEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		ownersCopy := make(map[string][]uint32, len(entry.Owners))
		for peerID, chunks := range entry.Owners {
			chunksCopy := make([]uint32, len(chunks))
			copy(chunksCopy, chunks)
			ownersCopy[peerID] = chunksCopy
		}
		result = append(result, types.CatalogEntry{
			FileID:      entry.FileID,
			FileName:    entry.FileName,
			FileSize:    entry.FileSize,
			TotalChunks: entry.TotalChunks,
			Owners:      ownersCopy,
		})
	}
	return result
}

// GetEntry возвращает копию записи о файле (или nil).
func (c *Catalog) GetEntry(fileID string) *types.CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[fileID]
	if !exists {
		return nil
	}

	ownersCopy := make(map[string][]uint32, len(entry.Owners))
	for peerID, chunks := range entry.Owners {
		chunksCopy := make([]uint32, len(chunks))
		copy(chunksCopy, chunks)
		ownersCopy[peerID] = chunksCopy
	}
	return &types.CatalogEntry{
		FileID:      entry.FileID,
		FileName:    entry.FileName,
		FileSize:    entry.FileSize,
		TotalChunks: entry.TotalChunks,
		Owners:      ownersCopy,
	}
}

// Merge объединяет полученный от пира каталог с нашим.
func (c *Catalog) Merge(entries []types.CatalogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range entries {
		if len(entry.Owners) == 0 {
			continue
		}

		existing, exists := c.entries[entry.FileID]
		if !exists {
			ownersCopy := make(map[string][]uint32, len(entry.Owners))
			for peerID, chunks := range entry.Owners {
				chunksCopy := make([]uint32, len(chunks))
				copy(chunksCopy, chunks)
				ownersCopy[peerID] = chunksCopy
			}
			c.entries[entry.FileID] = &types.CatalogEntry{
				FileID:      entry.FileID,
				FileName:    entry.FileName,
				FileSize:    entry.FileSize,
				TotalChunks: entry.TotalChunks,
				Owners:      ownersCopy,
			}
		} else {
			existing.FileName = entry.FileName
			existing.FileSize = entry.FileSize
			existing.TotalChunks = entry.TotalChunks

			for peerID, chunks := range entry.Owners {
				chunksCopy := make([]uint32, len(chunks))
				copy(chunksCopy, chunks)
				existing.Owners[peerID] = chunksCopy
			}
		}
	}
}

// HasPeer проверяет, есть ли пир среди владельцев хотя бы одного файла.
func (c *Catalog) HasPeer(peerID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, entry := range c.entries {
		if _, exists := entry.Owners[peerID]; exists {
			return true
		}
	}
	return false
}

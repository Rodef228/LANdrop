package catalog

import (
	"sync"

	"mesh-cu/internal/protocol"
)

// Catalog — распределённый in-memory каталог файлов, известных в сети.
// Хранит информацию о том, у каких пиров какие файлы и чанки есть.
// Все данные только в памяти, ничего не сохраняется на диск.
type Catalog struct {
	mu sync.RWMutex
	// fileID -> CatalogEntry
	entries map[string]*protocol.CatalogEntry
}

// NewCatalog создаёт новый пустой каталог.
func NewCatalog() *Catalog {
	return &Catalog{
		entries: make(map[string]*protocol.CatalogEntry),
	}
}

// AddOrUpdate добавляет или обновляет информацию о файле от конкретного пира.
// Если файл уже есть — добавляет/обновляет чанки для этого пира.
// Если пир уже есть — обновляет список его чанков.
func (c *Catalog) AddOrUpdate(fileID, peerID string, fileName string, fileSize int64, totalChunks uint32, chunks []uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[fileID]
	if !exists {
		entry = &protocol.CatalogEntry{
			FileID:      fileID,
			FileName:    fileName,
			FileSize:    fileSize,
			TotalChunks: totalChunks,
			Owners:      make(map[string][]uint32),
		}
		c.entries[fileID] = entry
	} else {
		// Обновляем метаданные (имя, размер) на случай если они изменились
		entry.FileName = fileName
		entry.FileSize = fileSize
		entry.TotalChunks = totalChunks
	}

	// Сохраняем чанки пира
	if len(chunks) > 0 {
		entry.Owners[peerID] = chunks
	} else {
		// Если список чанков пуст — считаем что пир владеет всеми
		allChunks := make([]uint32, totalChunks)
		for i := uint32(0); i < totalChunks; i++ {
			allChunks[i] = i
		}
		entry.Owners[peerID] = allChunks
	}
}

// RemovePeer удаляет все записи о пире из каталога.
// Возвращает список fileID, которые были затронуты (для оповещения).
func (c *Catalog) RemovePeer(peerID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var affectedFiles []string
	for fileID, entry := range c.entries {
		if _, exists := entry.Owners[peerID]; exists {
			delete(entry.Owners, peerID)
			affectedFiles = append(affectedFiles, fileID)
		}
		// Если у файла больше нет владельцев — удаляем запись целиком
		if len(entry.Owners) == 0 {
			delete(c.entries, fileID)
		}
	}
	return affectedFiles
}

// GetEntries возвращает копию всех записей каталога для отправки пиру.
func (c *Catalog) GetEntries() []protocol.CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]protocol.CatalogEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		// Создаём глубокую копию
		ownersCopy := make(map[string][]uint32, len(entry.Owners))
		for peerID, chunks := range entry.Owners {
			chunksCopy := make([]uint32, len(chunks))
			copy(chunksCopy, chunks)
			ownersCopy[peerID] = chunksCopy
		}
		result = append(result, protocol.CatalogEntry{
			FileID:      entry.FileID,
			FileName:    entry.FileName,
			FileSize:    entry.FileSize,
			TotalChunks: entry.TotalChunks,
			Owners:      ownersCopy,
		})
	}
	return result
}

// GetEntry возвращает копию записи о конкретном файле (или nil).
func (c *Catalog) GetEntry(fileID string) *protocol.CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[fileID]
	if !exists {
		return nil
	}

	// Глубокая копия
	ownersCopy := make(map[string][]uint32, len(entry.Owners))
	for peerID, chunks := range entry.Owners {
		chunksCopy := make([]uint32, len(chunks))
		copy(chunksCopy, chunks)
		ownersCopy[peerID] = chunksCopy
	}
	return &protocol.CatalogEntry{
		FileID:      entry.FileID,
		FileName:    entry.FileName,
		FileSize:    entry.FileSize,
		TotalChunks: entry.TotalChunks,
		Owners:      ownersCopy,
	}
}

// Merge объединяет полученный от пира каталог с нашим.
// entries, у которых нет владельцев, не добавляются.
func (c *Catalog) Merge(entries []protocol.CatalogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range entries {
		if len(entry.Owners) == 0 {
			continue
		}

		existing, exists := c.entries[entry.FileID]
		if !exists {
			// Новая запись — копируем целиком
			ownersCopy := make(map[string][]uint32, len(entry.Owners))
			for peerID, chunks := range entry.Owners {
				chunksCopy := make([]uint32, len(chunks))
				copy(chunksCopy, chunks)
				ownersCopy[peerID] = chunksCopy
			}
			c.entries[entry.FileID] = &protocol.CatalogEntry{
				FileID:      entry.FileID,
				FileName:    entry.FileName,
				FileSize:    entry.FileSize,
				TotalChunks: entry.TotalChunks,
				Owners:      ownersCopy,
			}
		} else {
			// Обновляем метаданные
			existing.FileName = entry.FileName
			existing.FileSize = entry.FileSize
			existing.TotalChunks = entry.TotalChunks

			// Добавляем/обновляем владельцев
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

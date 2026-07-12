package cdn

import (
	"sync"

	"mesh-cu/internal/protocol"
)

type FileInfo struct {
	ID           string
	Name         string
	OriginalPath string
	Size         int64
	TotalChunks  uint32
	OwnedChunks  map[uint32]bool
}

type CDNManager struct {
	fm        *FileManager
	mu        sync.RWMutex
	Files     map[string]*FileInfo           // My files (including partially downloaded)
	PeerFiles map[string]map[string][]uint32 // fileID -> peerID -> chunkIndices
	NodeID    string
}

func NewCDNManager(nodeID string, fm *FileManager) *CDNManager {
	return &CDNManager{
		fm:        fm,
		Files:     make(map[string]*FileInfo),
		PeerFiles: make(map[string]map[string][]uint32),
		NodeID:    nodeID,
	}
}

// HandleAnnounce обрабатывает анонс файла от пира.
// Создаёт FileInfo если его ещё нет, и сохраняет какие чанки есть у отправителя.
// ВАЖНО: не вызывать при уже захваченном Lock()!
func (cm *CDNManager) HandleAnnounce(payload protocol.FileAnnouncePayload, senderID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Создаём FileInfo если его ещё нет (первый раз слышим об этом файле)
	if _, exists := cm.Files[payload.FileID]; !exists {
		// Если список чанков пустой — считаем что пир владеет всеми чанками
		owned := make(map[uint32]bool)
		if len(payload.Chunks) == 0 {
			for i := uint32(0); i < payload.TotalChunks; i++ {
				owned[i] = true
			}
		} else {
			for _, idx := range payload.Chunks {
				owned[idx] = true
			}
		}

		cm.Files[payload.FileID] = &FileInfo{
			ID:          payload.FileID,
			Name:        payload.FileName,
			Size:        payload.FileSize,
			TotalChunks: payload.TotalChunks,
			OwnedChunks: owned,
		}
	}

	// Сохраняем какие чанки есть у отправителя
	if _, ok := cm.PeerFiles[payload.FileID]; !ok {
		cm.PeerFiles[payload.FileID] = make(map[string][]uint32)
	}

	if len(payload.Chunks) > 0 {
		cm.PeerFiles[payload.FileID][senderID] = payload.Chunks
	} else {
		// Если список чанков не передан — считаем что у пира есть все
		allChunks := make([]uint32, payload.TotalChunks)
		for i := uint32(0); i < payload.TotalChunks; i++ {
			allChunks[i] = i
		}
		cm.PeerFiles[payload.FileID][senderID] = allChunks
	}
}

// GetChunkOwners возвращает список пиров, у которых есть указанный чанк.
func (cm *CDNManager) GetChunkOwners(fileID string, chunkIndex uint32) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var owners []string
	if peers, ok := cm.PeerFiles[fileID]; ok {
		for peerID, chunks := range peers {
			for _, idx := range chunks {
				if idx == chunkIndex {
					owners = append(owners, peerID)
					break
				}
			}
		}
	}
	return owners
}

// RegisterLocalFile регистрирует локальный файл (которым мы владеем полностью).
func (cm *CDNManager) RegisterLocalFile(fileID, name string, size int64, originalPath string) *FileInfo {
	totalChunks := uint32((size + ChunkSize - 1) / ChunkSize)
	owned := make(map[uint32]bool)
	for i := uint32(0); i < totalChunks; i++ {
		owned[i] = true
	}

	fi := &FileInfo{
		ID:           fileID,
		Name:         name,
		OriginalPath: originalPath,
		Size:         size,
		TotalChunks:  totalChunks,
		OwnedChunks:  owned,
	}

	cm.mu.Lock()
	cm.Files[fileID] = fi
	cm.mu.Unlock()

	return fi
}

// GetOwnedChunks возвращает список индексов чанков, которыми мы владеем.
func (cm *CDNManager) GetOwnedChunks(fileID string) []uint32 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	fi, ok := cm.Files[fileID]
	if !ok {
		return nil
	}

	var chunks []uint32
	for idx, owned := range fi.OwnedChunks {
		if owned {
			chunks = append(chunks, idx)
		}
	}
	return chunks
}

// GetFileInfo возвращает копию информации о файле (или nil если файл неизвестен).
func (cm *CDNManager) GetFileInfo(fileID string) *FileInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	fi, ok := cm.Files[fileID]
	if !ok {
		return nil
	}

	// Возвращаем копию
	info := &FileInfo{
		ID:           fi.ID,
		Name:         fi.Name,
		OriginalPath: fi.OriginalPath,
		Size:         fi.Size,
		TotalChunks:  fi.TotalChunks,
		OwnedChunks:  make(map[uint32]bool),
	}
	for k, v := range fi.OwnedChunks {
		info.OwnedChunks[k] = v
	}
	return info
}

// MarkChunkOwned отмечает чанк как принадлежащий нам (после скачивания).
func (cm *CDNManager) MarkChunkOwned(fileID string, chunkIdx uint32) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	fi, ok := cm.Files[fileID]
	if !ok {
		return
	}
	if fi.OwnedChunks == nil {
		fi.OwnedChunks = make(map[uint32]bool)
	}
	fi.OwnedChunks[chunkIdx] = true
}

// ReAnnounce отправляет анонс файла всем активным пирам с обновлённым списком чанков.
func (cm *CDNManager) ReAnnounce(fileID string, registryPeers []protocol.PeerInfo, sendFn func(peerIP string, peerPort int, data []byte) error) {
	cm.mu.RLock()
	fi, ok := cm.Files[fileID]
	if !ok {
		cm.mu.RUnlock()
		return
	}
	cm.mu.RUnlock()

	chunks := cm.GetOwnedChunks(fileID)

	header := protocol.Header{
		MessageType: protocol.TypeFileAnnounce,
		SenderID:    cm.NodeID,
		SenderName:  cm.NodeID,
	}

	payload := protocol.FileAnnouncePayload{
		FileID:      fileID,
		FileName:    fi.Name,
		FileSize:    fi.Size,
		TotalChunks: fi.TotalChunks,
		Chunks:      chunks,
	}

	encoded, err := protocol.Encode(header, payload)
	if err != nil {
		return
	}

	for _, p := range registryPeers {
		if p.ID != cm.NodeID {
			sendFn(p.IP, p.Port, encoded)
		}
	}
}

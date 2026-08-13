package cdn

import (
	"sync"

	"landrop/internal/helpers"
	"landrop/internal/types"
)

// CDNManager — управляет файлами: регистрация, анонсы, отслеживание владельцев чанков.
type CDNManager struct {
	fm        *helpers.FileManager
	mu        sync.RWMutex
	Files     map[string]*types.FileInfo
	PeerFiles map[string]map[string][]uint32
	NodeID    string
}

// NewCDNManager создаёт CDNManager.
func NewCDNManager(nodeID string, fm *helpers.FileManager) *CDNManager {
	return &CDNManager{
		fm:        fm,
		Files:     make(map[string]*types.FileInfo),
		PeerFiles: make(map[string]map[string][]uint32),
		NodeID:    nodeID,
	}
}

// HandleAnnounce обрабатывает анонс файла от пира.
func (cm *CDNManager) HandleAnnounce(payload types.FileAnnouncePayload, senderID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.Files[payload.FileID]; !exists {
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

		cm.Files[payload.FileID] = &types.FileInfo{
			ID:          payload.FileID,
			Name:        payload.FileName,
			Size:        payload.FileSize,
			TotalChunks: payload.TotalChunks,
			OwnedChunks: owned,
		}
	}

	if _, ok := cm.PeerFiles[payload.FileID]; !ok {
		cm.PeerFiles[payload.FileID] = make(map[string][]uint32)
	}

	if len(payload.Chunks) > 0 {
		cm.PeerFiles[payload.FileID][senderID] = payload.Chunks
	} else {
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

// RegisterLocalFile регистрирует локальный файл.
func (cm *CDNManager) RegisterLocalFile(fileID, name string, size int64, originalPath string) *types.FileInfo {
	totalChunks := uint32((size + helpers.ChunkSize - 1) / helpers.ChunkSize)
	owned := make(map[uint32]bool)
	for i := uint32(0); i < totalChunks; i++ {
		owned[i] = true
	}

	fi := &types.FileInfo{
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

// GetOwnedChunks возвращает индексы чанков, которыми мы владеем.
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

// GetFileInfo возвращает копию FileInfo (или nil).
func (cm *CDNManager) GetFileInfo(fileID string) *types.FileInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	fi, ok := cm.Files[fileID]
	if !ok {
		return nil
	}

	info := &types.FileInfo{
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

// MarkChunkOwned отмечает чанк как принадлежащий нам.
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

// ReAnnounce отправляет анонс файла всем активным пирам.
func (cm *CDNManager) ReAnnounce(fileID string, registryPeers []types.PeerInfo, sendFn func(peerIP string, peerPort int, data []byte) error) {
	cm.mu.RLock()
	fi, ok := cm.Files[fileID]
	if !ok {
		cm.mu.RUnlock()
		return
	}
	cm.mu.RUnlock()

	chunks := cm.GetOwnedChunks(fileID)

	header := types.Header{
		MessageType: types.TypeFileAnnounce,
		SenderID:    cm.NodeID,
		SenderName:  cm.NodeID,
	}

	payload := types.FileAnnouncePayload{
		FileID:      fileID,
		FileName:    fi.Name,
		FileSize:    fi.Size,
		TotalChunks: fi.TotalChunks,
		Chunks:      chunks,
	}

	encoded, err := helpers.Encode(header, payload)
	if err != nil {
		return
	}

	for _, p := range registryPeers {
		if p.ID != cm.NodeID {
			sendFn(p.IP, p.Port, encoded)
		}
	}
}

// UpdateNodeName обновляет имя узла.
func (cm *CDNManager) UpdateNodeName(newName string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.NodeID = newName
}

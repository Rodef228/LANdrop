package types

// FileInfo — метаинформация о файле: какие чанки есть у нас.
type FileInfo struct {
	ID           string
	Name         string
	OriginalPath string
	Size         int64
	TotalChunks  uint32
	OwnedChunks  map[uint32]bool
}

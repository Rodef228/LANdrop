package types

import "sync"

// DownloadTask — отслеживает процесс скачивания одного файла.
type DownloadTask struct {
	Mu             sync.Mutex
	FileID         string
	FileName       string
	TotalChunks    uint32
	ReceivedChunks uint32
	Done           bool
}

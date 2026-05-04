package cdn

import (
	"fmt"
	"os"
	"path/filepath"
)

const ChunkSize = 1024 * 1024 // 1 MB

type FileManager struct {
	StoragePath string
}

func NewFileManager(storagePath string) (*FileManager, error) {
	absPath, err := filepath.Abs(storagePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, err
	}
	return &FileManager{StoragePath: absPath}, nil
}

func (fm *FileManager) ReadChunk(fileName string, index uint32) ([]byte, error) {
	path := filepath.Join(fm.StoragePath, fileName)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	offset := int64(index) * ChunkSize
	data := make([]byte, ChunkSize)
	n, err := file.ReadAt(data, offset)
	if err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("failed to read chunk: %w", err)
	}
	return data[:n], nil
}

func (fm *FileManager) WriteChunk(fileName string, index uint32, data []byte) error {
	path := filepath.Join(fm.StoragePath, fileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for writing: %w", err)
	}
	defer file.Close()

	offset := int64(index) * ChunkSize
	_, err = file.WriteAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	return nil
}

func (fm *FileManager) GetFileSize(fileName string) (int64, error) {
	path := filepath.Join(fm.StoragePath, fileName)
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

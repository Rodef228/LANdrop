package types

// CatalogEntry — запись в распределённом каталоге файлов.
type CatalogEntry struct {
	FileID      string              `json:"file_id"`
	FileName    string              `json:"file_name"`
	FileSize    int64               `json:"file_size"`
	TotalChunks uint32              `json:"total_chunks"`
	Owners      map[string][]uint32 `json:"owners"`
}

// CatalogPayload — полезная нагрузка для TypeCatalog.
type CatalogPayload struct {
	Entries []CatalogEntry `json:"entries"`
}

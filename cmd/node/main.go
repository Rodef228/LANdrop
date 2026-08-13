package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"landrop/internal/catalog"
	"landrop/internal/cdn"
	"landrop/internal/discovery"
	"landrop/internal/helpers"
	"landrop/internal/network"
	"landrop/internal/types"
)

func main() {
	name := flag.String("name", "", "Имя узла")
	port := flag.Int("port", 0, "TCP порт")
	storageDir := flag.String("storage", "./storage", "Каталог для хранения файлов")
	flag.Parse()

	if *name == "" {
		if envName := os.Getenv("NODE_NAME"); envName != "" {
			*name = envName
		}
	}

	if *port == 0 {
		if envPort := os.Getenv("NODE_PORT"); envPort != "" {
			if p, err := strconv.Atoi(envPort); err == nil {
				*port = p
			}
		}
	}

	if *storageDir == "./storage" {
		if envStorage := os.Getenv("NODE_STORAGE"); envStorage != "" {
			*storageDir = envStorage
		}
	}

	if *name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			*name = fmt.Sprintf("node-%d", os.Getpid())
		} else {
			*name = fmt.Sprintf("node-%s-%d", hostname, os.Getpid())
		}
	}

	fm, err := helpers.NewFileManager(*storageDir)
	if err != nil {
		log.Fatalf("Ошибка инициализации FileManager: %v", err)
	}
	cdnMgr := cdn.NewCDNManager(*name, fm)
	registry := discovery.NewPeerRegistry()
	fileCatalog := catalog.NewCatalog()

	fmt.Printf("Добро пожаловать в LANdrop, %s!\n", *name)
	fmt.Println("Ожидание пиров и сообщений...")
	fmt.Println("-----------------------------------------")

	downloads := make(map[string]*types.DownloadTask)
	var dlMu sync.Mutex

	requestChunkFromPeer := func(fileID string, chunkIdx uint32, peerIP string, peerPort int) {
		header := types.Header{
			MessageType: types.TypeFileRequest,
			SenderID:    *name,
			SenderName:  *name,
		}
		payload := map[string]interface{}{
			"file_id":     fileID,
			"chunk_index": float64(chunkIdx),
		}
		encoded, err := helpers.Encode(header, payload)
		if err != nil {
			log.Printf("[ERROR] Ошибка кодирования запроса чанка: %v", err)
			return
		}
		helpers.SendMessage(peerIP, peerPort, encoded)
	}

	requestChunk := func(fileID string, chunkIdx uint32) {
		owners := cdnMgr.GetChunkOwners(fileID, chunkIdx)
		if len(owners) == 0 {
			activePeers := registry.GetActivePeers()
			for _, peer := range activePeers {
				if peer.ID != *name {
					requestChunkFromPeer(fileID, chunkIdx, peer.IP, peer.Port)
					return
				}
			}
			log.Printf("[ERROR] Нет пиров для запроса чанка %d файла %s", chunkIdx, fileID)
			return
		}
		ownerID := owners[0]
		activePeers := registry.GetActivePeers()
		for _, peer := range activePeers {
			if peer.ID == ownerID {
				requestChunkFromPeer(fileID, chunkIdx, peer.IP, peer.Port)
				return
			}
		}
		for _, peer := range activePeers {
			if peer.ID != *name {
				requestChunkFromPeer(fileID, chunkIdx, peer.IP, peer.Port)
				return
			}
		}
	}

	startDownload := func(fileID string, totalChunks uint32) {
		for chunkIdx := uint32(0); chunkIdx < totalChunks; chunkIdx++ {
			requestChunk(fileID, chunkIdx)
		}
	}

	sendCatalogToPeer := func(peerIP string, peerPort int) {
		entries := fileCatalog.GetEntries()
		if len(entries) == 0 {
			return
		}
		header := types.Header{
			MessageType: types.TypeCatalog,
			SenderID:    *name,
			SenderName:  *name,
		}
		payload := types.CatalogPayload{Entries: entries}
		encoded, err := helpers.Encode(header, payload)
		if err != nil {
			log.Printf("[ERROR] Ошибка кодирования каталога: %v", err)
			return
		}
		helpers.SendMessage(peerIP, peerPort, encoded)
	}

	broadcastCatalog := func() {
		entries := fileCatalog.GetEntries()
		if len(entries) == 0 {
			return
		}
		header := types.Header{
			MessageType: types.TypeCatalog,
			SenderID:    *name,
			SenderName:  *name,
		}
		payload := types.CatalogPayload{Entries: entries}
		encoded, err := helpers.Encode(header, payload)
		if err != nil {
			log.Printf("[ERROR] Ошибка кодирования каталога: %v", err)
			return
		}
		for _, peer := range registry.GetActivePeers() {
			if peer.ID != *name {
				helpers.SendMessage(peer.IP, peer.Port, encoded)
			}
		}
	}

	handleIncomingMessage := func(conn net.Conn, senderID string, senderName string, messageType types.MessageType, payload map[string]interface{}) error {
		switch messageType {
		case types.TypeChat:
			message, ok := payload["message"].(string)
			if !ok {
				log.Printf("[Handler Error] Неверный формат чат-сообщения от %s", senderID)
				return fmt.Errorf("invalid chat message format")
			}
			fmt.Printf("\r%s: %s\nyou:  ", senderName, message)

		case types.TypeFileAnnounce:
			fID, _ := payload["file_id"].(string)
			fName, _ := payload["file_name"].(string)

			if fID == "" {
				fID, _ = payload["FileID"].(string)
			}
			if fName == "" {
				fName, _ = payload["FileName"].(string)
			}

			var p types.FileAnnouncePayload
			p.FileID = fID
			p.FileName = fName
			if size, ok := payload["file_size"].(float64); ok {
				p.FileSize = int64(size)
			}
			if tc, ok := payload["total_chunks"].(float64); ok {
				p.TotalChunks = uint32(tc)
			} else {
				p.TotalChunks = uint32((p.FileSize + helpers.ChunkSize - 1) / helpers.ChunkSize)
			}

			if chunksRaw, ok := payload["chunks"].([]interface{}); ok {
				for _, c := range chunksRaw {
					if ci, ok := c.(float64); ok {
						p.Chunks = append(p.Chunks, uint32(ci))
					}
				}
			}

			cdnMgr.HandleAnnounce(p, senderID)
			fileCatalog.AddOrUpdate(fID, senderID, fName, p.FileSize, p.TotalChunks, p.Chunks)

			fmt.Printf("\r[CDN]: Пир %s имеет файл: %s (ID: %s, %d чанков)\nyou:  ", senderName, fName, fID, p.TotalChunks)

		case types.TypeFileRequest:
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64)

			fi := cdnMgr.GetFileInfo(fileID)
			if fi == nil {
				return nil
			}

			if !fi.OwnedChunks[uint32(idx)] {
				return nil
			}

			data, err := fm.ReadChunk(fi.Name, uint32(idx))
			if err != nil || len(data) == 0 {
				log.Printf("[ERROR] Чанк %d не найден для файла %s", int(idx), fi.Name)
				return nil
			}

			respHeader := types.Header{
				MessageType: types.TypeFileChunk,
				SenderID:    *name,
				SenderName:  *name,
			}

			respPayload := map[string]interface{}{
				"file_id":     fileID,
				"chunk_index": idx,
				"data":        data,
			}

			encoded, _ := helpers.Encode(respHeader, respPayload)

			for _, peer := range registry.GetActivePeers() {
				if peer.ID == senderID {
					helpers.SendMessage(peer.IP, peer.Port, encoded)
					break
				}
			}

		case types.TypeFileChunk:
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64)

			var data []byte
			if strData, ok := payload["data"].(string); ok {
				data, _ = base64.StdEncoding.DecodeString(strData)
			} else if byteData, ok := payload["data"].([]byte); ok {
				data = byteData
			} else {
				log.Printf("[ERROR] Поле 'data' отсутствует или неверного типа: %T", payload["data"])
			}

			if len(data) == 0 {
				return nil
			}

			fi := cdnMgr.GetFileInfo(fileID)
			if fi == nil {
				log.Printf("[ERROR] Неизвестный file ID: %s", fileID)
				return nil
			}

			err := fm.WriteChunk(fi.Name, uint32(idx), data)
			if err != nil {
				log.Printf("[ERROR] Ошибка записи чанка %d файла %s: %v", int(idx), fi.Name, err)
				return nil
			}

			cdnMgr.MarkChunkOwned(fileID, uint32(idx))

			dlMu.Lock()
			task, hasTask := downloads[fileID]
			if hasTask {
				task.ReceivedChunks++
				done := task.ReceivedChunks >= task.TotalChunks
				dlMu.Unlock()

				fmt.Printf("\r[CDN]: Получен чанк %d/%d для %s от %s\nyou:  ", int(idx)+1, task.TotalChunks, task.FileName, senderName)

				if done {
					fmt.Printf("\n[CDN]: Файл %s скачан полностью!\nyou: ", task.FileName)
					activePeers := registry.GetActivePeers()
					peerInfos := make([]types.PeerInfo, 0, len(activePeers))
					for _, p := range activePeers {
						peerInfos = append(peerInfos, types.PeerInfo{
							ID:   p.ID,
							IP:   p.IP,
							Port: p.Port,
							Name: p.Name,
						})
					}
					cdnMgr.ReAnnounce(fileID, peerInfos, helpers.SendMessage)

					fi := cdnMgr.GetFileInfo(fileID)
					if fi != nil {
						allChunks := make([]uint32, fi.TotalChunks)
						for i := uint32(0); i < fi.TotalChunks; i++ {
							allChunks[i] = i
						}
						fileCatalog.AddOrUpdate(fileID, *name, fi.Name, fi.Size, fi.TotalChunks, allChunks)
						broadcastCatalog()
					}
				}
			} else {
				dlMu.Unlock()
			}

		case types.TypeCatalog:
			if entriesRaw, ok := payload["entries"].([]interface{}); ok {
				var entries []types.CatalogEntry
				for _, eRaw := range entriesRaw {
					if eMap, ok := eRaw.(map[string]interface{}); ok {
						entry := types.CatalogEntry{
							Owners: make(map[string][]uint32),
						}
						entry.FileID, _ = eMap["file_id"].(string)
						entry.FileName, _ = eMap["file_name"].(string)
						if fs, ok := eMap["file_size"].(float64); ok {
							entry.FileSize = int64(fs)
						}
						if tc, ok := eMap["total_chunks"].(float64); ok {
							entry.TotalChunks = uint32(tc)
						}
						if ownersRaw, ok := eMap["owners"].(map[string]interface{}); ok {
							for peerID, chunksRaw := range ownersRaw {
								if chunksList, ok := chunksRaw.([]interface{}); ok {
									var chunks []uint32
									for _, c := range chunksList {
										if ci, ok := c.(float64); ok {
											chunks = append(chunks, uint32(ci))
										}
									}
									entry.Owners[peerID] = chunks
								}
							}
						}
						entries = append(entries, entry)
					}
				}
				if len(entries) > 0 {
					fileCatalog.Merge(entries)
					fmt.Printf("\r[CATALOG]: Получен каталог от %s (%d файлов)\nyou: ", senderName, len(entries))
				}
			}

		case types.TypePing:
		case types.TypePong:
		default:
			log.Printf("[Handler] Неизвестный тип сообщения %s от %s", messageType, senderID)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := network.NewServer(*name, *port, handleIncomingMessage)
	if err := server.Start(ctx); err != nil {
		log.Fatalf("[Network] Ошибка запуска сервера: %v", err)
	}

	*port = server.Port

	discService := discovery.NewDiscoveryService(*name, *port)

	peerChan := make(chan types.Peer)

	go discService.Start(ctx, peerChan)

	time.Sleep(1 * time.Second)

	registry.SetOnPeerLeave(func(peerID string) {
		affected := fileCatalog.RemovePeer(peerID)
		if len(affected) > 0 {
			fmt.Printf("\r[SYSTEM]: Пир %s вышел, удалён из %d файла(ов)\nyou: ", peerID, len(affected))
			broadcastCatalog()
		}
	})

	go func() {
		knownPeers := make(map[string]bool)
		for peer := range peerChan {
			if peer.Name == *name && (peer.IP != "127.0.0.1" || peer.Port != *port) {
				fmt.Println("\n-----------------------------------------------------")
				log.Printf("[КРИТИЧЕСКАЯ ОШИБКА] Имя '%s' уже занято (%s:%d)!", *name, peer.IP, peer.Port)
				fmt.Println("Перезапустите приложение с другим именем.")
				fmt.Println("-----------------------------------------------------")
				os.Exit(1)
			}

			peerKey := fmt.Sprintf("%s:%d", peer.IP, peer.Port)
			if !knownPeers[peerKey] {
				fmt.Printf("\r[SYSTEM]: Новый пир: %s (%s:%d)\nyou: ", peer.Name, peer.IP, peer.Port)
				knownPeers[peerKey] = true
				sendCatalogToPeer(peer.IP, peer.Port)
			}

			registry.Update(peer)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("you: ")
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 {
				fmt.Print("you: ")
				continue
			}
			if strings.HasPrefix(line, "/") {
				parts := strings.Fields(line)
				if parts[0] == "/announce" {
					if len(parts) < 2 {
						fmt.Printf("\r[ERROR]: Использование: /announce <путь> [id]\nyou: ")
						continue
					}
					path := parts[1]
					var fID string
					if len(parts) > 2 {
						fID = parts[2]
					} else {
						fID = fmt.Sprintf("%x", time.Now().UnixNano())[:6]
					}
					info, err := os.Stat(path)
					if err != nil {
						fmt.Printf("\r[ERROR]: Файл не найден: %s\nyou: ", path)
						continue
					}

					fi := cdnMgr.RegisterLocalFile(fID, info.Name(), info.Size(), path)

					srcData, err := os.ReadFile(path)
					if err == nil {
						totalChunks := uint32((len(srcData) + helpers.ChunkSize - 1) / helpers.ChunkSize)
						for i := uint32(0); i < totalChunks; i++ {
							start := int64(i) * helpers.ChunkSize
							end := start + helpers.ChunkSize
							if end > int64(len(srcData)) {
								end = int64(len(srcData))
							}
							chunkData := srcData[start:end]
							fm.WriteChunk(info.Name(), i, chunkData)
						}
					}

					chunks := make([]uint32, fi.TotalChunks)
					for i := uint32(0); i < fi.TotalChunks; i++ {
						chunks[i] = i
					}

					header := types.Header{
						MessageType: types.TypeFileAnnounce,
						SenderID:    *name,
						SenderName:  *name,
					}

					payload := map[string]interface{}{
						"file_id":      fID,
						"file_name":    info.Name(),
						"file_size":    info.Size(),
						"total_chunks": fi.TotalChunks,
						"chunks":       chunks,
					}

					encoded, err := helpers.Encode(header, payload)
					if err != nil {
						fmt.Printf("\r[ERROR]: Ошибка кодирования: %v\nyou: ", err)
						continue
					}

					activePeers := registry.GetActivePeers()
					count := 0
					for _, peer := range activePeers {
						if peer.ID != *name {
							helpers.SendMessage(peer.IP, peer.Port, encoded)
							count++
						}
					}

					fileCatalog.AddOrUpdate(fID, *name, info.Name(), info.Size(), fi.TotalChunks, chunks)
					broadcastCatalog()

					fmt.Printf("\r[SYSTEM]: Анонсирован файл %s (ID: %s) для %d пиров\nyou: ", path, fID, count)
				} else if parts[0] == "/download" && len(parts) > 1 {
					fileID := parts[1]

					fi := cdnMgr.GetFileInfo(fileID)
					if fi == nil {
						fmt.Printf("\r[ERROR]: File ID %s неизвестен. Дождитесь анонса.\nyou: ", fileID)
						continue
					}

					neededChunks := fi.TotalChunks
					if neededChunks == 0 {
						fmt.Printf("\r[ERROR]: Файл %s не содержит чанков\nyou: ", fileID)
						continue
					}

					task := &types.DownloadTask{
						FileID:         fileID,
						FileName:       fi.Name,
						TotalChunks:    neededChunks,
						ReceivedChunks: 0,
					}

					dlMu.Lock()
					downloads[fileID] = task
					dlMu.Unlock()

					fmt.Printf("\r[SYSTEM]: Начало скачивания %s (%d чанков)...\nyou: ", fi.Name, neededChunks)

					startDownload(fileID, neededChunks)

				} else if parts[0] == "/files" {
					entries := fileCatalog.GetEntries()
					if len(entries) == 0 {
						fmt.Printf("\r[SYSTEM]: Каталог пуст.\nyou: ")
					} else {
						fmt.Printf("\r[SYSTEM]: Известные файлы (%d):\n", len(entries))
						for _, e := range entries {
							ownerCount := len(e.Owners)
							fmt.Printf("  - %s (ID: %s, размер: %d байт, чанков: %d, владельцев: %d)\n",
								e.FileName, e.FileID, e.FileSize, e.TotalChunks, ownerCount)
							for peerID, chunks := range e.Owners {
								fmt.Printf("      [%s] имеет %d чанк(ов)\n", peerID, len(chunks))
							}
						}
						fmt.Print("you: ")
					}

				} else {
					fmt.Printf("\r[ERROR]: Неизвестная команда: %s\nyou: ", parts[0])
				}

				fmt.Print("you: ")
				continue
			}

			header := types.Header{
				MessageType: types.TypeChat,
				SenderID:    *name,
				RecipientID: "",
			}
			chatPayload := map[string]interface{}{
				"message":     line,
				"sender_name": *name,
			}
			encodedMsg, err := helpers.Encode(header, chatPayload)
			if err != nil {
				log.Printf("[Encoder Error] Ошибка кодирования сообщения: %v", err)
				fmt.Print("you: ")
				continue
			}

			activePeers := registry.GetActivePeers()
			if len(activePeers) == 0 {
				fmt.Println("\r[SYSTEM]: Нет активных пиров для отправки.")
			}
			for _, peer := range activePeers {
				if peer.ID == *name {
					continue
				}
				err := helpers.SendMessage(peer.IP, peer.Port, encodedMsg)
				if err != nil {
					log.Printf("[Network Error] Ошибка отправки %s (%s:%d): %v", peer.Name, peer.IP, peer.Port, err)
				}
			}
			fmt.Print("you: ")
		}
		if err := scanner.Err(); err != nil {
			log.Printf("[UI Error] Ошибка чтения stdin: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("Завершение работы...")
	cancel()
	server.Stop()
}

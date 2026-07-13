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

	"mesh-cu/internal/catalog"
	"mesh-cu/internal/cdn"
	"mesh-cu/internal/discovery"
	"mesh-cu/internal/network"
	"mesh-cu/internal/protocol"
)

// downloadTask отслеживает процесс скачивания одного файла.
type downloadTask struct {
	mu             sync.Mutex
	FileID         string
	FileName       string
	TotalChunks    uint32
	receivedChunks uint32 // сколько чанков уже получили
	done           bool
}

func main() {
	name := flag.String("name", "", "Name of the node")
	port := flag.Int("port", 0, "TCP port to listen on")
	storageDir := flag.String("storage", "./storage", "Directory to store files")
	flag.Parse()

	// Если флаг пустой, проверяем переменную окружения для Андроида
	if *name == "" {
		if envName := os.Getenv("NODE_NAME"); envName != "" {
			*name = envName
		}
	}

	// Если порт остался дефолтным, проверяем окружение
	if *port == 0 {
		if envPort := os.Getenv("NODE_PORT"); envPort != "" {
			if p, err := strconv.Atoi(envPort); err == nil {
				*port = p
			}
		}
	}

	// Если папка дефолтная, проверяем окружение
	if *storageDir == "./storage" {
		if envStorage := os.Getenv("NODE_STORAGE"); envStorage != "" {
			*storageDir = envStorage
		}
	}

	// Твоя родная генерация уникального имени (сработает, если и флаг, и ENV пустые)
	if *name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			*name = fmt.Sprintf("node-%d", os.Getpid())
		} else {
			*name = fmt.Sprintf("node-%s-%d", hostname, os.Getpid())
		}
	}

	fm, err := cdn.NewFileManager(*storageDir)
	if err != nil {
		log.Fatalf("Failed to initialize FileManager: %v", err)
	}
	cdnMgr := cdn.NewCDNManager(*name, fm)
	registry := discovery.NewPeerRegistry()
	fileCatalog := catalog.NewCatalog()

	// Welcome Message
	fmt.Printf("Welcome to Mesh-CU Messenger, %s!\n", *name)
	fmt.Println("Listening for peers and messages...")
	fmt.Println("Type your message and press Enter to send.")
	fmt.Println("-----------------------------------------")

	fmt.Printf("Starting node %s on port %d...\n", *name, *port)

	// Карта активных задач скачивания: fileID -> *downloadTask
	downloads := make(map[string]*downloadTask)
	var dlMu sync.Mutex

	// Вспомогательная функция: отправить запрос на конкретный чанк конкретному пиру
	requestChunkFromPeer := func(fileID string, chunkIdx uint32, peerIP string, peerPort int) {
		header := protocol.Header{
			MessageType: protocol.TypeFileRequest,
			SenderID:    *name,
			SenderName:  *name,
		}
		payload := map[string]interface{}{
			"file_id":     fileID,
			"chunk_index": float64(chunkIdx),
		}
		encoded, err := protocol.Encode(header, payload)
		if err != nil {
			log.Printf("[ERROR] Failed to encode chunk request: %v", err)
			return
		}
		network.SendMessage(peerIP, peerPort, encoded)
	}

	// Вспомогательная функция: найти пира для чанка и запросить у него
	requestChunk := func(fileID string, chunkIdx uint32) {
		owners := cdnMgr.GetChunkOwners(fileID, chunkIdx)
		if len(owners) == 0 {
			// Если владельцев нет — пробуем запросить у всех активных пиров
			activePeers := registry.GetActivePeers()
			for _, peer := range activePeers {
				if peer.ID != *name {
					requestChunkFromPeer(fileID, chunkIdx, peer.IP, peer.Port)
					return
				}
			}
			log.Printf("[ERROR] No peers to request chunk %d for file %s", chunkIdx, fileID)
			return
		}
		// Берём первого владельца (можно улучшить: выбирать по нагрузке)
		ownerID := owners[0]
		activePeers := registry.GetActivePeers()
		for _, peer := range activePeers {
			if peer.ID == ownerID {
				requestChunkFromPeer(fileID, chunkIdx, peer.IP, peer.Port)
				return
			}
		}
		// Если владелец не найден в активных — шлём всем
		for _, peer := range activePeers {
			if peer.ID != *name {
				requestChunkFromPeer(fileID, chunkIdx, peer.IP, peer.Port)
				return
			}
		}
	}

	// Вспомогательная функция: запустить скачивание всех чанков (параллельно)
	startDownload := func(fileID string, totalChunks uint32) {
		// Отправляем запросы на ВСЕ чанки сразу — параллельно
		for chunkIdx := uint32(0); chunkIdx < totalChunks; chunkIdx++ {
			requestChunk(fileID, chunkIdx)
		}
	}

	// Вспомогательная функция: отправить наш каталог конкретному пиру
	sendCatalogToPeer := func(peerIP string, peerPort int) {
		entries := fileCatalog.GetEntries()
		if len(entries) == 0 {
			return // нечего отправлять
		}
		header := protocol.Header{
			MessageType: protocol.TypeCatalog,
			SenderID:    *name,
			SenderName:  *name,
		}
		payload := protocol.CatalogPayload{Entries: entries}
		encoded, err := protocol.Encode(header, payload)
		if err != nil {
			log.Printf("[ERROR] Failed to encode catalog: %v", err)
			return
		}
		network.SendMessage(peerIP, peerPort, encoded)
	}

	// Вспомогательная функция: разослать наш каталог всем активным пирам
	broadcastCatalog := func() {
		entries := fileCatalog.GetEntries()
		if len(entries) == 0 {
			return
		}
		header := protocol.Header{
			MessageType: protocol.TypeCatalog,
			SenderID:    *name,
			SenderName:  *name,
		}
		payload := protocol.CatalogPayload{Entries: entries}
		encoded, err := protocol.Encode(header, payload)
		if err != nil {
			log.Printf("[ERROR] Failed to encode catalog: %v", err)
			return
		}
		for _, peer := range registry.GetActivePeers() {
			if peer.ID != *name {
				network.SendMessage(peer.IP, peer.Port, encoded)
			}
		}
	}

	// Handler для входящих сообщений
	handleIncomingMessage := func(conn net.Conn, senderID string, senderName string, messageType protocol.MessageType, payload map[string]interface{}) error {
		switch messageType {
		case protocol.TypeChat:
			message, ok := payload["message"].(string)
			if !ok {
				log.Printf("[Handler Error] Invalid chat message format from %s", senderID)
				return fmt.Errorf("invalid chat message format")
			}
			fmt.Printf("\r%s: %s\nyou:  ", senderName, message)

		case protocol.TypeFileAnnounce:
			// Извлекаем данные из мапы
			fID, _ := payload["file_id"].(string)
			fName, _ := payload["file_name"].(string)

			if fID == "" {
				fID, _ = payload["FileID"].(string)
			}
			if fName == "" {
				fName, _ = payload["FileName"].(string)
			}

			var p protocol.FileAnnouncePayload
			p.FileID = fID
			p.FileName = fName
			if size, ok := payload["file_size"].(float64); ok {
				p.FileSize = int64(size)
			}
			if tc, ok := payload["total_chunks"].(float64); ok {
				p.TotalChunks = uint32(tc)
			} else {
				p.TotalChunks = uint32((p.FileSize + cdn.ChunkSize - 1) / cdn.ChunkSize)
			}

			// Извлекаем список чанков, если передан
			if chunksRaw, ok := payload["chunks"].([]interface{}); ok {
				for _, c := range chunksRaw {
					if ci, ok := c.(float64); ok {
						p.Chunks = append(p.Chunks, uint32(ci))
					}
				}
			}

			// HandleAnnounce сам создаст FileInfo и сохранит владельца чанков
			cdnMgr.HandleAnnounce(p, senderID)

			// Обновляем каталог
			fileCatalog.AddOrUpdate(fID, senderID, fName, p.FileSize, p.TotalChunks, p.Chunks)

			fmt.Printf("\r[CDN]: Peer %s has file: %s (ID: %s, %d chunks)\nyou:  ", senderName, fName, fID, p.TotalChunks)

		case protocol.TypeFileRequest:
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64)

			// Получаем информацию о файле, чтобы узнать оригинальное имя
			fi := cdnMgr.GetFileInfo(fileID)
			if fi == nil {
				return nil
			}

			// Проверяем, владеем ли мы этим чанком
			if !fi.OwnedChunks[uint32(idx)] {
				return nil
			}

			// Читаем чанк из хранилища по оригинальному имени файла
			data, err := fm.ReadChunk(fi.Name, uint32(idx))
			if err != nil || len(data) == 0 {
				log.Printf("[ERROR] Chunk %d not found for file %s", int(idx), fi.Name)
				return nil
			}

			respHeader := protocol.Header{
				MessageType: protocol.TypeFileChunk,
				SenderID:    *name,
				SenderName:  *name,
			}

			respPayload := map[string]interface{}{
				"file_id":     fileID,
				"chunk_index": idx,
				"data":        data,
			}

			encoded, _ := protocol.Encode(respHeader, respPayload)

			// Отправляем конкретно тому, кто просил
			for _, peer := range registry.GetActivePeers() {
				if peer.ID == senderID {
					network.SendMessage(peer.IP, peer.Port, encoded)
					break
				}
			}

		case protocol.TypeFileChunk:
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64)

			var data []byte
			if strData, ok := payload["data"].(string); ok {
				data, _ = base64.StdEncoding.DecodeString(strData)
			} else if byteData, ok := payload["data"].([]byte); ok {
				data = byteData
			} else {
				log.Printf("[ERROR] Payload 'data' is missing or not a string! Type is: %T", payload["data"])
			}

			if len(data) == 0 {
				return nil
			}

			// Получаем оригинальное имя файла
			fi := cdnMgr.GetFileInfo(fileID)
			if fi == nil {
				log.Printf("[ERROR] Unknown file ID: %s", fileID)
				return nil
			}

			// Записываем чанк на диск под оригинальным именем
			err := fm.WriteChunk(fi.Name, uint32(idx), data)
			if err != nil {
				log.Printf("[ERROR] Failed to write chunk %d for file %s: %v", int(idx), fi.Name, err)
				return nil
			}

			// Отмечаем чанк как наш
			cdnMgr.MarkChunkOwned(fileID, uint32(idx))

			dlMu.Lock()
			task, hasTask := downloads[fileID]
			if hasTask {
				task.receivedChunks++
				done := task.receivedChunks >= task.TotalChunks
				dlMu.Unlock()

				fmt.Printf("\r[CDN]: Received chunk %d/%d for %s from %s\nyou:  ", int(idx)+1, task.TotalChunks, task.FileName, senderName)

				if done {
					fmt.Printf("\n[CDN]: File %s download complete!\nyou: ", task.FileName)
					// Ре-анонсируем файл
					activePeers := registry.GetActivePeers()
					peerInfos := make([]protocol.PeerInfo, 0, len(activePeers))
					for _, p := range activePeers {
						peerInfos = append(peerInfos, protocol.PeerInfo{
							ID:   p.ID,
							IP:   p.IP,
							Port: p.Port,
							Name: p.Name,
						})
					}
					cdnMgr.ReAnnounce(fileID, peerInfos, network.SendMessage)

					// Обновляем каталог: теперь мы тоже владеем всеми чанками
					fi := cdnMgr.GetFileInfo(fileID)
					if fi != nil {
						allChunks := make([]uint32, fi.TotalChunks)
						for i := uint32(0); i < fi.TotalChunks; i++ {
							allChunks[i] = i
						}
						fileCatalog.AddOrUpdate(fileID, *name, fi.Name, fi.Size, fi.TotalChunks, allChunks)
						// Рассылаем обновлённый каталог всем
						broadcastCatalog()
					}
				}
			} else {
				dlMu.Unlock()
			}

		case protocol.TypeCatalog:
			// Получен каталог от пира — сливаем с нашим
			if entriesRaw, ok := payload["entries"].([]interface{}); ok {
				var entries []protocol.CatalogEntry
				for _, eRaw := range entriesRaw {
					if eMap, ok := eRaw.(map[string]interface{}); ok {
						entry := protocol.CatalogEntry{
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
					fmt.Printf("\r[CATALOG]: Received catalog from %s (%d files)\nyou: ", senderName, len(entries))
				}
			}

		case protocol.TypePing:
		case protocol.TypePong:
		default:
			log.Printf("[Handler] Received unknown message type %s from %s", messageType, senderID)
		}
		return nil
	}

	// 0. Создаем контекст для управления жизненным циклом ноды
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Сначала запускаем TCP сервер, чтобы он занял динамический порт
	server := network.NewServer(*name, *port, handleIncomingMessage)
	if err := server.Start(ctx); err != nil {
		log.Fatalf("[Network] Failed to start server: %v", err)
	}

	// 2. Вытаскиваем реальный порт, который выдала ОС
	*port = server.Port // или server.ServerPort, в зависимости от твоей структуры

	// 3. Создаем сервис обнаружения и передаем ему УЖЕ РЕАЛЬНЫЙ порт
	discService := discovery.NewDiscoveryService(*name, *port)

	// Канал для найденных пиров
	peerChan := make(chan discovery.Peer)

	// 4. Запускаем сервис обнаружения (вещание + прослушивание) в фоне
	go discService.Start(ctx, peerChan)

	// Периодически выводим список активных узлов для наглядности (можно убрать в финальной версии)
	// go func() {
	// 	ticker := time.NewTicker(5 * time.Second)
	// 	defer ticker.Stop()
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case <-ticker.C:
	// 			active := registry.GetActivePeers()
	// 			// This part is mostly for debugging. In a real messenger, you might not want to spam this.
	// 			if len(active) > 0 {
	// 				// fmt.Printf("\n--- Active Peers (%d) ---\n", len(active))
	// 				// for _, p := range active {
	// 				// 	fmt.Printf("- %s (%s:%d)\n", p.Name, p.IP, p.Port)
	// 				// }
	// 				// fmt.Println("------------------------")
	// 			}
	// 		}
	// 	}
	// }()

	time.Sleep(1 * time.Second)

	// Устанавливаем callback на выход пира из сети
	registry.SetOnPeerLeave(func(peerID string) {
		// Удаляем файлы пира из каталога
		affected := fileCatalog.RemovePeer(peerID)
		if len(affected) > 0 {
			fmt.Printf("\r[SYSTEM]: Peer %s left, removed from %d file(s)\nyou: ", peerID, len(affected))
			// Рассылаем обновлённый каталог всем оставшимся
			broadcastCatalog()
		}
	})

	// Чтение найденных пиров из Discovery
	go func() {
		knownPeers := make(map[string]bool)
		for peer := range peerChan {
			// Если имя совпадает с нашим, но IP или Порт отличаются — это клон!
			if peer.Name == *name && (peer.IP != "127.0.0.1" || peer.Port != *port) {
				fmt.Println("\n-----------------------------------------------------")
				log.Printf("[КРИТИЧЕСКАЯ ОШИБКА] Имя '%s' уже занято другим участником в сети (%s:%d)!", *name, peer.IP, peer.Port)
				fmt.Println("Пожалуйста, перезапустите приложение с другим именем.")
				fmt.Println("-----------------------------------------------------")

				// Красиво завершаем работу текущего процесса
				os.Exit(1)
			}

			// Сообщаем о новом пире только один раз
			peerKey := fmt.Sprintf("%s:%d", peer.IP, peer.Port)
			if !knownPeers[peerKey] {
				fmt.Printf("\r[SYSTEM]: New peer joined: %s (%s:%d)\nyou: ", peer.Name, peer.IP, peer.Port)
				knownPeers[peerKey] = true

				// Отправляем новому пиру наш текущий каталог файлов
				sendCatalogToPeer(peer.IP, peer.Port)
			}

			// Если всё ок, добавляем пира в реестр
			registry.Update(peer)
		}
	}()

	// Цикл чтения из консоли и рассылки сообщений
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
						fmt.Printf("\r[ERROR]: Usage: /announce <path> [optional_id]\nyou: ")
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
						fmt.Printf("\r[ERROR]: File not found: %s\nyou: ", path)
						continue
					}

					// Регистрируем локальный файл
					fi := cdnMgr.RegisterLocalFile(fID, info.Name(), info.Size(), path)

					// Копируем файл в хранилище под именем fileID для раздачи по чанкам
					// (если это не тот же путь)
					// Читаем исходный файл и раскладываем по чанкам в хранилище
					srcData, err := os.ReadFile(path)
					if err == nil {
						// Записываем чанками в хранилище
						totalChunks := uint32((len(srcData) + cdn.ChunkSize - 1) / cdn.ChunkSize)
						for i := uint32(0); i < totalChunks; i++ {
							start := int64(i) * cdn.ChunkSize
							end := start + cdn.ChunkSize
							if end > int64(len(srcData)) {
								end = int64(len(srcData))
							}
							chunkData := srcData[start:end]
							fm.WriteChunk(info.Name(), i, chunkData)
						}
					}

					// Формируем анонс со списком всех чанков
					chunks := make([]uint32, fi.TotalChunks)
					for i := uint32(0); i < fi.TotalChunks; i++ {
						chunks[i] = i
					}

					header := protocol.Header{
						MessageType: protocol.TypeFileAnnounce,
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

					encoded, err := protocol.Encode(header, payload)
					if err != nil {
						fmt.Printf("\r[ERROR]: Failed to encode: %v\nyou: ", err)
						continue
					}

					// Рассылаем всем активным пирам
					activePeers := registry.GetActivePeers()
					count := 0
					for _, peer := range activePeers {
						if peer.ID != *name {
							network.SendMessage(peer.IP, peer.Port, encoded)
							count++
						}
					}

					// Обновляем каталог: мы владеем всеми чанками
					fileCatalog.AddOrUpdate(fID, *name, info.Name(), info.Size(), fi.TotalChunks, chunks)
					// Рассылаем обновлённый каталог всем
					broadcastCatalog()

					fmt.Printf("\r[SYSTEM]: Announced file %s (ID: %s) to %d peers\nyou: ", path, fID, count)
				} else if parts[0] == "/download" && len(parts) > 1 {
					fileID := parts[1]

					// Получаем информацию о файле
					fi := cdnMgr.GetFileInfo(fileID)
					if fi == nil {
						fmt.Printf("\r[ERROR]: File ID %s unknown. Wait for announce.\nyou: ", fileID)
						continue
					}

					// Определяем какие чанки нам ещё нужно скачать
					neededChunks := fi.TotalChunks
					if neededChunks == 0 {
						fmt.Printf("\r[ERROR]: File %s has no chunks\nyou: ", fileID)
						continue
					}

					// Создаём задачу скачивания
					task := &downloadTask{
						FileID:         fileID,
						FileName:       fi.Name,
						TotalChunks:    neededChunks,
						receivedChunks: 0,
					}

					dlMu.Lock()
					downloads[fileID] = task
					dlMu.Unlock()

					fmt.Printf("\r[SYSTEM]: Starting download %s (%d chunks)...\nyou: ", fi.Name, neededChunks)

					// Запрашиваем ВСЕ чанки сразу — параллельно
					startDownload(fileID, neededChunks)

				} else if parts[0] == "/files" {
					// Показать список известных файлов из каталога
					entries := fileCatalog.GetEntries()
					if len(entries) == 0 {
						fmt.Printf("\r[SYSTEM]: No files in catalog.\nyou: ")
					} else {
						fmt.Printf("\r[SYSTEM]: Known files (%d):\n", len(entries))
						for _, e := range entries {
							ownerCount := len(e.Owners)
							fmt.Printf("  - %s (ID: %s, size: %d bytes, chunks: %d, owners: %d)\n",
								e.FileName, e.FileID, e.FileSize, e.TotalChunks, ownerCount)
							for peerID, chunks := range e.Owners {
								fmt.Printf("      [%s] has %d chunk(s)\n", peerID, len(chunks))
							}
						}
						fmt.Print("you: ")
					}

				} else {
					fmt.Printf("\r[ERROR]: Unknown command: %s\nyou: ", parts[0])
				}

				fmt.Print("you: ")
				continue
			}

			// Упаковываем сообщение
			header := protocol.Header{
				MessageType: protocol.TypeChat,
				SenderID:    *name,
				RecipientID: "",
			}
			chatPayload := map[string]interface{}{
				"message":     line,
				"sender_name": *name,
			}
			encodedMsg, err := protocol.Encode(header, chatPayload)
			if err != nil {
				log.Printf("[Encoder Error] Failed to encode message: %v", err)
				fmt.Print("you: ")
				continue
			}

			// Рассылаем всем активным пирам
			activePeers := registry.GetActivePeers()
			if len(activePeers) == 0 {
				fmt.Println("\r[SYSTEM]: No active peers to send message to.")
			}
			for _, peer := range activePeers {
				if peer.ID == *name {
					continue
				}
				err := network.SendMessage(peer.IP, peer.Port, encodedMsg)
				if err != nil {
					log.Printf("[Network Error] Failed to send message to %s (%s:%d): %v", peer.Name, peer.IP, peer.Port, err)
				}
			}
			fmt.Print("you: ")
		}
		if err := scanner.Err(); err != nil {
			log.Printf("[UI Error] Reading standard input: %v", err)
		}
	}()

	// Ждем сигнала прерывания (Ctrl+C)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	cancel()
	server.Stop()
}

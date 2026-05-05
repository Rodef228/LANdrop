package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mesh-cu/internal/cdn"
	"mesh-cu/internal/discovery"
	"mesh-cu/internal/network"
	"mesh-cu/internal/protocol"
)

func main() {
	name := flag.String("name", fmt.Sprintf("node-%d", os.Getpid()), "Name of the node")
	port := flag.Int("port", 8080, "TCP port to listen on")
	storageDir := flag.String("storage", "./storage", "Directory to store files")
	flag.Parse()

	fm, err := cdn.NewFileManager(*storageDir)
	if err != nil {
		log.Fatalf("Failed to initialize FileManager: %v", err)
	}
	cdnMgr := cdn.NewCDNManager(*name, fm)
	registry := discovery.NewPeerRegistry()

	// Welcome Message
	fmt.Printf("Welcome to Mesh-CU Messenger, %s!\n", *name)
	fmt.Println("Listening for peers and messages...")
	fmt.Println("Type your message and press Enter to send.")
	fmt.Println("-----------------------------------------")

	fmt.Printf("Starting node %s on port %d...\n", *name, *port)

	// Handler для входящих сообщений
	handleIncomingMessage := func(conn net.Conn, senderID string, name string, messageType protocol.MessageType, payload map[string]interface{}) error {
		switch messageType {
		case protocol.TypeChat:
			message, ok := payload["message"].(string)
			if !ok {
				log.Printf("[Handler Error] Invalid chat message format from %s", senderID)
				return fmt.Errorf("invalid chat message format")
			}
			fmt.Printf("\r%s: %s\nyou:  ", name, message)
		case protocol.TypeFileAnnounce:
			// 1. Извлекаем данные из мапы
			fID, _ := payload["file_id"].(string) // Если ID это строка, используй .(string)
			fName, _ := payload["file_name"].(string)

			if fID == "" {
				fID, _ = payload["FileID"].(string)
			}
			if fName == "" {
				fName, _ = payload["FileName"].(string)
			}
			// 2. Чтобы cdnMgr «увидел» файл, нужно собрать структуру и вызвать HandleAnnounce
			// Превращаем мапу обратно в типизированную нагрузку для CDN[cite: 1, 5]
			var p protocol.FileAnnouncePayload
			p.FileID = fID
			p.FileName = fName
			// Получаем остальные поля (размер и чанки), если они есть в payload
			if size, ok := payload["file_size"].(float64); ok {
				p.FileSize = int64(size)
			}

			// 3. ПЕРЕДАЕМ В МЕНЕДЖЕР (теперь fID используется внутри, ошибка уйдет)[cite: 2]
			cdnMgr.HandleAnnounce(p, senderID)

			cdnMgr.Lock() // Важно: используй Lock, а не RLock
			if _, exists := cdnMgr.Files[fID]; !exists {
				cdnMgr.Files[fID] = &cdn.FileInfo{
					ID:          fID,
					Name:        fName,
					Size:        p.FileSize,
					TotalChunks: uint32((p.FileSize + 1023) / 1024), // Рассчитываем чанки (допустим, по 1КБ)
				}
			}
			cdnMgr.Unlock()

			fmt.Printf("\r[CDN]: Peer %s has file: %s (ID: %s)\nyou:  ", name, fName, fID)
		case protocol.TypeFileRequest:
			// У нас просят чанк
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64) // JSON числа в Go — это float64

			fi, ok := cdnMgr.Files[fileID]
			if ok {
				data, _ := fm.ReadChunk(fi.Name, uint32(idx))
				header := protocol.Header{MessageType: protocol.TypeFileChunk, SenderID: senderID, SenderName: name}
				respPayload := map[string]interface{}{
					"file_id":     fileID,
					"chunk_index": idx,
					"data":        data,
				}
				encoded, _ := protocol.Encode(header, respPayload)
				fmt.Printf("Sending announce to %d peers\n", len(registry.GetActivePeers()))
				for _, peer := range registry.GetActivePeers() {
					if peer.ID == senderID {
						network.SendMessage(peer.IP, peer.Port, encoded)
						break
					}
				}
			}

		case protocol.TypeFileChunk:
			// Нам прилетел кусок файла
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64)
			data, _ := payload["data"].([]byte)

			fi, ok := cdnMgr.Files[fileID]
			if ok {
				fm.WriteChunk(fi.Name, uint32(idx), data)
				cdnMgr.Lock()
				fi.OwnedChunks[uint32(idx)] = true
				cdnMgr.Unlock()
				fmt.Printf("\r[CDN]: Received chunk %d for %s\nyou:  ", int(idx), fi.Name)
			}
			// Внутри case protocol.TypeFileChunk в handleIncomingMessage
			if uint32(idx)+1 < uint32(fi.TotalChunks) {
				// Формируем такой же запрос (TypeFileRequest), но для idx + 1
				// И отправляем его обратно senderID
			} else {
				fmt.Printf("\n[CDN]: File %s download complete!\nyou: ", fi.Name)
			}
		case protocol.TypePing:
			// Можно обработать пинг, если нужно, но для мессенджера пока игнорируем
		case protocol.TypePong:
			// Можно обработать понг, если нужно
		default:
			log.Printf("[Handler] Received unknown message type %s from %s", messageType, senderID)
		}
		return nil
	}

	// // Инициализируем реестр пиров
	// registry := discovery.NewPeerRegistry()

	// Создаем сервис обнаружения
	discService := discovery.NewDiscoveryService(*name, *port)

	// Канал для найденных пиров
	peerChan := make(chan discovery.Peer)
	ctx, cancel := context.WithCancel(context.Background())

	// Запускаем сервис обнаружения (вещание + прослушивание) в фоне
	go discService.Start(ctx, peerChan)

	// Запускаем TCP сервер для сообщений
	server := network.NewServer(*name, *port, handleIncomingMessage)
	go func() {
		if err := server.Start(ctx); err != nil {
			log.Fatalf("[Network] Failed to start server: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	// Слушаем найденных соседей и обновляем реестр с уведомлением
	go func() {
		knownPeers := make(map[string]string) // Track known peers to announce new ones
		for {
			select {
			case <-ctx.Done():
				return
			case peer, ok := <-peerChan:
				if !ok {
					return
				}
				// Анонс нового пира
				if _, exists := knownPeers[peer.ID]; !exists {
					fmt.Printf("\r[SYSTEM]: New peer joined: %s (%s:%d)\nyou: ", peer.Name, peer.IP, peer.Port)
					knownPeers[peer.ID] = peer.Name
				}
				registry.Update(peer)

			case <-time.After(2 * time.Second):
				// Проверка на "пропавших" без отдельной горутины снаружи
				activePeers := registry.GetActivePeers()
				activeMap := make(map[string]bool)
				for _, p := range activePeers {
					activeMap[p.ID] = true
				}

				for id, name := range knownPeers {
					if !activeMap[id] {
						fmt.Printf("\r[SYSTEM]: %s has left the network\nyou:  ", name)
						delete(knownPeers, id)
					}
				}
			}
		}

	}()

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
				if parts[0] == "/announce" && len(parts) > 1 {
					path, fID := parts[1], parts[2]
					if len(parts) > 2 {
						fID = parts[2]
					} else {
						// Простейшая генерация короткого ID на базе времени
						fID = fmt.Sprintf("%x", time.Now().UnixNano())[:6]
					}
					info, err := os.Stat(path)
					if err != nil {
						fmt.Printf("\r[ERROR]: File not found: %s\nyou: ", path)
						continue
					}

					// 1. Регистрируем файл в локальном менеджере (чтобы мы знали, что раздаем)
					fi := cdnMgr.RegisterLocalFile(fID, info.Name(), info.Size())

					// 2. Создаем заголовок сообщения
					header := protocol.Header{
						MessageType: protocol.TypeFileAnnounce, // Должно быть "FILE_ANN" из твоего types.go
						SenderID:    *name,
						SenderName:  *name,
					}

					// 3. Формируем полезную нагрузку (важно: ключи должны совпадать с обработчиком!)
					payload := map[string]interface{}{
						"file_id":      fID,
						"file_name":    info.Name(),
						"file_size":    float64(info.Size()),
						"total_chunks": float64(fi.TotalChunks),
					}

					// 4. Кодируем в JSON/байты
					encoded, err := protocol.Encode(header, payload)
					if err != nil {
						fmt.Printf("\r[ERROR]: Failed to encode: %v\nyou: ", err)
						continue
					}

					// 5. РАССЫЛКА: отправляем всем, кого нашли через Discovery
					activePeers := registry.GetActivePeers()
					count := 0
					for _, peer := range activePeers {
						if peer.ID != *name {
							network.SendMessage(peer.IP, peer.Port, encoded)
							count++
						}
					}

					fmt.Printf("\r[SYSTEM]: Announced file %s to %d peers\nyou: ", path, count)
				}
				if parts[0] == "/download" && len(parts) > 1 {
					fileID := parts[1]

					// 1. Проверяем, знаем ли мы вообще про такой файл
					cdnMgr.RLock()
					fi, exists := cdnMgr.Files[fileID]
					cdnMgr.RUnlock()

					if !exists {
						fmt.Printf("\r[ERROR]: File ID %s unknown. Wait for announce.\nyou: ", fileID)
						continue
					}

					fmt.Printf("\r[SYSTEM]: Starting download for %s...\nyou: ", fi.Name)

					// 2. Формируем запрос на первый чанк (0)
					header := protocol.Header{
						MessageType: protocol.TypeFileRequest,
						SenderID:    *name,
						SenderName:  *name,
					}

					payload := map[string]interface{}{
						"file_id":     fileID,
						"chunk_index": float64(0),
					}

					encoded, _ := protocol.Encode(header, payload)

					// 3. Отправляем запрос ВСЕМ активным пирам.
					// Тот, у кого есть файл, ответит чанком. Это проще и надежнее для теста.
					activePeers := registry.GetActivePeers()
					for _, peer := range activePeers {
						if peer.ID != *name {
							network.SendMessage(peer.IP, peer.Port, encoded)
						}
					}
					fmt.Printf("\r[CDN]: Requested chunk 0 from network\nyou: ")
				}
				fmt.Print("you: ")
				continue
			}

			// fmt.Printf("you: %s\n", line)
			// Упаковываем сообщение
			header := protocol.Header{
				MessageType: protocol.TypeChat,
				SenderID:    *name,
				RecipientID: "", // Для широковещания, можно оставить пустым или "ALL"
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
				if peer.ID == *name { // Не отправляем сообщение самому себе
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
	cancel()      // Отменяем контекст, чтобы остановить горутины
	server.Stop() // Останавливаем TCP сервер
}

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
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"mesh-cu/internal/api"
	"mesh-cu/internal/cdn"
	"mesh-cu/internal/db"
	"mesh-cu/internal/discovery"
	"mesh-cu/internal/network"
	"mesh-cu/internal/protocol"
)

func getDirectChatID(peer1, peer2 string) string {
	peers := []string{peer1, peer2}
	sort.Strings(peers)
	return "direct:" + peers[0] + ":" + peers[1]
}

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
	// enableAnsiSupport()

	db.InitDB(*name)

	var currentChatID atomic.Value
	currentChatID.Store("ALL")

	// Welcome Message
	fmt.Printf("Welcome to Mesh-CU Messenger, %s!\n", *name)
	fmt.Println("Listening for peers and messages...")
	fmt.Println("Type your message and press Enter to send.")
	fmt.Println("Type /help for available commands.")
	fmt.Println("-----------------------------------------")

	fmt.Printf("Starting node %s on port %d...\n", *name, *port)

	// Handler для входящих сообщений
	handleIncomingMessage := func(conn net.Conn, header protocol.Header, payload map[string]interface{}) error {
		senderID := header.SenderID
		senderName, _ := payload["sender_name"].(string)
		if senderName == "" {
			senderName = senderID
		}

		switch header.MessageType {
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

			var totalChunks uint32
			if tc, ok := payload["total_chunks"].(float64); ok {
				totalChunks = uint32(tc)
			} else {
				totalChunks = uint32((p.FileSize + cdn.ChunkSize - 1) / cdn.ChunkSize)
			}

			// 3. ПЕРЕДАЕМ В МЕНЕДЖЕР (теперь fID используется внутри, ошибка уйдет)[cite: 2]
			cdnMgr.HandleAnnounce(p, senderID)

			cdnMgr.Lock() // Важно: используй Lock, а не RLock
			if _, exists := cdnMgr.Files[fID]; !exists {
				cdnMgr.Files[fID] = &cdn.FileInfo{
					ID:          fID,
					Name:        fName,
					Size:        p.FileSize,
					TotalChunks: totalChunks,
					OwnedChunks: make(map[uint32]bool),
				}
			}
			cdnMgr.Unlock()

			fmt.Printf("\r[CDN]: Peer %s has file: %s (ID: %s)\nyou:  ", name, fName, fID)
		case protocol.TypeFileRequest:
			fileID, _ := payload["file_id"].(string)
			idx, _ := payload["chunk_index"].(float64)

			fi, ok := cdnMgr.Files[fileID]
			if ok {
				var data []byte
				var err error
				if fi.OriginalPath != "" {
					data, err = fm.ReadChunkFromPath(fi.OriginalPath, uint32(idx))
				} else {
					data, err = fm.ReadChunk(fi.Name, uint32(idx))
				}

				if err != nil || len(data) == 0 {
					log.Printf("[ERROR] Chunk %d not found for file %s", int(idx), fileID)
					return nil
				}

				// ВАЖНО: SenderID должен быть ВАШИМ (Kamil)
				respHeader := protocol.Header{
					MessageType: protocol.TypeFileChunk,
					SenderID:    *name, // Используйте переменную имени текущего узла
					SenderName:  *name,
				}

				respPayload := map[string]interface{}{
					"file_id":     fileID,
					"chunk_index": idx,
					"data":        data,
				}

				encoded, _ := protocol.Encode(respHeader, respPayload)

				// Отправляем конкретно тому, кто просил (senderID)
				for _, peer := range registry.GetActivePeers() {
					if peer.ID == senderID {
						fmt.Printf("\r[DEBUG]: Sending chunk %v to %s at %s:%d\n", idx, peer.ID, peer.IP, peer.Port)

						network.SendMessage(peer.IP, peer.Port, encoded)
						// log.Printf("[CDN] Sent chunk %d to %s", int(idx), senderID)
						break
					}
				}
			}

		case protocol.TypeFileChunk:
			// Нам прилетел кусок файла
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
			// ignored
		case protocol.TypePong:
			// ignored
		default:
			log.Printf("[Handler] Received unknown message type %s from %s", header.MessageType, senderID)
		}
		return nil
	}

	// // Инициализируем реестр пиров
	registry = discovery.NewPeerRegistry()

	// Создаем сервис обнаружения
	discService := discovery.NewDiscoveryService(*name, *port)
	peerChan := make(chan discovery.Peer)
	ctx, cancel := context.WithCancel(context.Background())

	go discService.Start(ctx, peerChan)

	server := network.NewServer(*name, *port, handleIncomingMessage)
	go func() {
		if err := server.Start(ctx); err != nil {
			log.Fatalf("[Network] Failed to start server: %v", err)
		}
	}()

	// Start WebSocket API for Flutter web interface
	wsAPI := api.NewWSAPI(*name, *port, registry)
	wsAPI.Start(ctx, 8765) // WS API port

	time.Sleep(1 * time.Second)

	go func() {
		knownPeers := make(map[string]string)
		for {
			select {
			case <-ctx.Done():
				return
			case peer, ok := <-peerChan:
				if !ok {
					return
				}
				if _, exists := knownPeers[peer.ID]; !exists {
					fmt.Printf("\r[SYSTEM]: New peer joined: %s (%s:%d)\nyou: ", peer.Name, peer.IP, peer.Port)
					knownPeers[peer.ID] = peer.Name
				}
				registry.Update(peer)

			case <-time.After(2 * time.Second):
				activePeers := registry.GetActivePeers()
				activeMap := make(map[string]bool)
				for _, p := range activePeers {
					activeMap[p.ID] = true
				}

				for id, pName := range knownPeers {
					if !activeMap[id] {
						fmt.Printf("\r[SYSTEM]: %s has left the network\nyou:  ", pName)
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
			line := strings.TrimSpace(scanner.Text())
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
						// Простейшая генерация короткого ID на базе времени
						fID = fmt.Sprintf("%x", time.Now().UnixNano())[:6]
					}
					info, err := os.Stat(path)
					if err != nil {
						fmt.Printf("\r[ERROR]: File not found: %s\nyou: ", path)
						continue
					}

					// 1. Регистрируем файл в локальном менеджере (чтобы мы знали, что раздаем)
					fi := cdnMgr.RegisterLocalFile(fID, info.Name(), info.Size(), path)

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
						"file_size":    info.Size(), // Передаем чистые байты (int64)
						"total_chunks": fi.TotalChunks,
					}

					// log.Printf("Size of file %d, and name %s, and total chunks %d", info.Size(), info.Name(), fi.TotalChunks)
					// log.Printf("Size: %.2f MB", float64(info.Size())/1000000)

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

					cdnMgr.RLock()
					fi, exists := cdnMgr.Files[fileID]
					cdnMgr.RUnlock()

					if !exists {
						fmt.Printf("\r[ERROR]: File ID %s unknown. Wait for announce.\nyou: ", fileID)
						continue
					}
					fmt.Printf("\r[SYSTEM]: Starting download for %s (%d chunks)...\nyou: ", fi.Name, fi.TotalChunks)

					activePeers := registry.GetActivePeers()
					if len(activePeers) == 0 {
						fmt.Printf("\r[ERROR]: No active peers to request chunks from.\nyou: ")
						continue
					}

					for chunkIdx := uint32(0); chunkIdx < fi.TotalChunks; chunkIdx++ {
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
							fmt.Printf("\r[ERROR]: Failed to encode chunk %d request: %v\nyou: ", chunkIdx, err)
							continue
						}

						for _, peer := range activePeers {
							if peer.ID != *name {
								network.SendMessage(peer.IP, peer.Port, encoded)
							}
						}
					}
					fmt.Printf("\r[CDN]: Requested all %d chunks from network\nyou: ", fi.TotalChunks)
				}

				fmt.Print("you: ")
				continue
			}

			if strings.HasPrefix(line, "/") {
				handleCommand(line, *name, &currentChatID, registry)
				fmt.Print("you: ")
				continue
			}

			cid := currentChatID.Load().(string)

			var recipientID string
			if cid == "ALL" {
				recipientID = "ALL"
			} else {
				var chat db.Chat
				if err := db.DB.First(&chat, "id = ?", cid).Error; err == nil {
					if !chat.IsGroup {
						// Direct chat. Recipient is the other peer
						parts := strings.Split(chat.ID, ":") // direct:A:B
						if len(parts) == 3 {
							if parts[1] == *name {
								recipientID = parts[2]
							} else {
								recipientID = parts[1]
							}
						}
					} else {
						// Group chat
						recipientID = cid
					}
				}
			}

			header := protocol.Header{
				MessageType: protocol.TypeChat,
				SenderID:    *name,
				RecipientID: recipientID,
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

			// Store my own message in DB
			db.DB.Create(&db.Message{
				ChatID:     cid,
				SenderID:   *name,
				SenderName: *name,
				Content:    line,
				Timestamp:  time.Now().Unix(),
				IsRead:     true, // I read my own messages
			})

			// Send to network
			activePeers := registry.GetActivePeers()
			var chat db.Chat
			if cid != "ALL" {
				db.DB.First(&chat, "id = ?", cid)
			}

			for _, peer := range activePeers {
				if peer.ID == *name {
					continue
				}

				shouldSend := false
				if cid == "ALL" {
					shouldSend = true
				} else if !chat.IsGroup {
					// direct chat, send only if it's the other guy
					if peer.ID == recipientID || peer.Name == recipientID {
						shouldSend = true
					}
				} else {
					// group chat, send only to participants
					participants := strings.Split(chat.Participants, ",")
					for _, p := range participants {
						if p == peer.ID || p == peer.Name {
							shouldSend = true
							break
						}
					}
				}

				if shouldSend {
					err := network.SendMessage(peer.IP, peer.Port, encodedMsg)
					if err != nil {
						log.Printf("[Network Error] Failed to send message to %s: %v", peer.Name, err)
					}
				}
			}
			fmt.Print("you: ")
		}
		if err := scanner.Err(); err != nil {
			log.Printf("[UI Error] Reading standard input: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	cancel()
	server.Stop()
}

func handleCommand(line, myID string, currentChatID *atomic.Value, registry *discovery.PeerRegistry) {
	if line == "/help" {
		fmt.Println("--- Commands ---")
		fmt.Println("/chat [name] - Open a chat/group with [name]")
		fmt.Println("/new messages - Show chats with unread messages")
		fmt.Println("/all - Show all chats")
		fmt.Println("/create -name [group name] -parts [nick1, nick2, ...] - Create a group")
		return
	}

	if strings.HasPrefix(line, "/chat ") {
		target := strings.TrimPrefix(line, "/chat ")
		var chat db.Chat

		if err := db.DB.First(&chat, "name = ?", target).Error; err != nil {
			// Not found by name, check if it's an active peer
			found := false
			for _, p := range registry.GetActivePeers() {
				if p.Name == target {
					chatID := getDirectChatID(myID, p.ID)
					db.DB.Create(&db.Chat{ID: chatID, Name: p.Name, IsGroup: false, Participants: chatID})
					currentChatID.Store(chatID)
					found = true
					break
				}
			}
			if !found {
				// Try falling back to ID if they typed ID
				if err := db.DB.First(&chat, "id = ?", target).Error; err == nil {
					currentChatID.Store(chat.ID)
				} else {
					fmt.Printf("[SYSTEM]: Chat or peer '%s' not found.\n", target)
					return
				}
			}
		} else {
			currentChatID.Store(chat.ID)
		}

		cid := currentChatID.Load().(string)
		fmt.Printf("--- Chat: %s ---\n", target)
		var messages []db.Message
		db.DB.Where("chat_id = ?", cid).Order("timestamp asc").Find(&messages)
		for _, m := range messages {
			fmt.Printf("[%s]: %s\n", m.SenderName, m.Content)
		}
		db.DB.Model(&db.Message{}).Where("chat_id = ?", cid).Update("is_read", true)

	} else if line == "/new messages" {
		type Result struct {
			ChatID string
			Name   string
			Count  int
		}
		var results []Result
		db.DB.Raw(`SELECT chats.id as chat_id, chats.name, COUNT(messages.id) as count 
            FROM chats JOIN messages ON chats.id = messages.chat_id 
            WHERE messages.is_read = 0 GROUP BY chats.id`).Scan(&results)

		fmt.Println("--- Unread Messages ---")
		if len(results) == 0 {
			fmt.Println("No new messages.")
		} else {
			for _, r := range results {
				if r.Count > 0 {
					fmt.Printf("- %s: %d\n", r.Name, r.Count)
				}
			}
		}
	} else if line == "/all" {
		type Result struct {
			ChatID string
			Name   string
			Count  int
		}
		var results []Result
		db.DB.Raw(`SELECT chats.id as chat_id, chats.name, SUM(CASE WHEN messages.is_read = 0 THEN 1 ELSE 0 END) as count 
            FROM chats LEFT JOIN messages ON chats.id = messages.chat_id
            GROUP BY chats.id`).Scan(&results)

		fmt.Println("--- All Chats ---")
		for _, r := range results {
			if r.Count > 0 {
				fmt.Printf("- %s (%d unread)\n", r.Name, r.Count)
			} else {
				fmt.Printf("- %s\n", r.Name)
			}
		}
	} else if strings.HasPrefix(line, "/create ") {
		namePart := ""
		partsPart := ""

		if strings.Contains(line, "-name ") && strings.Contains(line, "-parts ") {
			nIdx := strings.Index(line, "-name ")
			pIdx := strings.Index(line, "-parts ")
			if nIdx < pIdx {
				namePart = strings.TrimSpace(line[nIdx+6 : pIdx])
				partsPart = strings.TrimSpace(line[pIdx+7:])
			} else {
				partsPart = strings.TrimSpace(line[pIdx+7 : nIdx])
				namePart = strings.TrimSpace(line[nIdx+6:])
			}
		}

		namePart = strings.Trim(namePart, "[]")
		partsPart = strings.Trim(partsPart, "[]")

		if namePart == "" || partsPart == "" {
			fmt.Println("[SYSTEM]: Invalid syntax. Use /create -name [group name] -parts [nick1, nick2, ...]")
			return
		}

		partsArray := strings.Split(partsPart, ",")
		var participants []string
		participants = append(participants, myID)
		for _, p := range partsArray {
			p = strings.TrimSpace(p)
			if p != "" && p != myID {
				participants = append(participants, p)
			}
		}

		sort.Strings(participants)
		groupID := "group:" + strings.Join(participants, ":")

		// Create group
		var existing db.Chat
		if err := db.DB.First(&existing, "id = ?", groupID).Error; err != nil {
			db.DB.Create(&db.Chat{ID: groupID, Name: namePart, IsGroup: true, Participants: strings.Join(participants, ",")})
			fmt.Printf("[SYSTEM]: Group '%s' created.\n", namePart)
		} else {
			fmt.Printf("[SYSTEM]: Group '%s' already exists.\n", existing.Name)
		}

		// Broadcast group creation to active participant peers
		header := protocol.Header{
			MessageType: protocol.TypeChat,
			SenderID:    myID,
			RecipientID: groupID,
		}
		payload := map[string]interface{}{
			"group_id":     groupID,
			"group_name":   namePart,
			"participants": strings.Join(participants, ","),
		}
		encodedMsg, _ := protocol.Encode(header, payload)

		for _, peer := range registry.GetActivePeers() {
			if peer.ID == myID {
				continue
			}
			// send to those in the participants list
			for _, p := range participants {
				if p == peer.Name || p == peer.ID {
					network.SendMessage(peer.IP, peer.Port, encodedMsg)
					break
				}
			}
		}
		currentChatID.Store(groupID)
	} else {
		fmt.Println("[SYSTEM]: Unknown command.")
	}
}

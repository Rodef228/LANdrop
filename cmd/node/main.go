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
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

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
	flag.Parse()

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

			// Determine ChatID
			var chatID string
			if header.RecipientID == "ALL" || header.RecipientID == "" {
				chatID = "ALL"
			} else if header.RecipientID == *name {
				// Direct message to me
				chatID = getDirectChatID(*name, senderID)
				var chat db.Chat
				if db.DB.First(&chat, "id = ?", chatID).Error != nil {
					db.DB.Create(&db.Chat{ID: chatID, Name: senderName, IsGroup: false, Participants: chatID})
				}
			} else {
				// Sent to a group ID
				chatID = header.RecipientID
			}

			cid := currentChatID.Load().(string)
			isRead := (cid == chatID)

			db.DB.Create(&db.Message{
				ChatID:     chatID,
				SenderID:   senderID,
				SenderName: senderName,
				Content:    message,
				Timestamp:  time.Now().Unix(),
				IsRead:     isRead,
			})

			if isRead {
				fmt.Printf("\r%s: %s\nyou:  ", senderName, message)
			} else {
				var cName string
				if chatID == "ALL" {
					cName = "Global Chat"
				} else {
					var c db.Chat
					if db.DB.First(&c, "id = ?", chatID).Error == nil {
						cName = c.Name
					} else {
						cName = chatID
					}
				}
				fmt.Printf("\r[New message from %s in %s]\nyou:  ", senderName, cName)
			}

		case protocol.TypeGroupCreate:
			groupID, _ := payload["group_id"].(string)
			groupName, _ := payload["group_name"].(string)
			partsStr, _ := payload["participants"].(string)

			if groupID != "" {
				var chat db.Chat
				if db.DB.First(&chat, "id = ?", groupID).Error != nil {
					db.DB.Create(&db.Chat{ID: groupID, Name: groupName, IsGroup: true, Participants: partsStr})
					fmt.Printf("\r[SYSTEM]: You were added to group '%s'\nyou:  ", groupName)
				}
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

	registry := discovery.NewPeerRegistry()
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
			MessageType: protocol.TypeGroupCreate,
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

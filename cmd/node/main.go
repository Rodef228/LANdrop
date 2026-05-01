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
	"syscall"
	"time"

	"mesh-cu/internal/discovery"
	"mesh-cu/internal/network"
	"mesh-cu/internal/protocol"
)

func main() {
	name := flag.String("name", fmt.Sprintf("node-%d", os.Getpid()), "Name of the node")
	port := flag.Int("port", 8080, "TCP port to listen on")
	flag.Parse()

	// Welcome Message
	fmt.Printf("Welcome to Mesh-CU Messenger, %s!\n", *name)
	fmt.Println("Listening for peers and messages...")
	fmt.Println("Type your message and press Enter to send.")
	fmt.Println("-----------------------------------------")

	log.Printf("Starting node %s on port %d...", *name, *port)

	// Handler для входящих сообщений
	handleIncomingMessage := func(conn net.Conn, senderID string, name string, messageType protocol.MessageType, payload map[string]interface{}) error {
		switch messageType {
		case protocol.TypeChat:
			message, ok := payload["message"].(string)
			if !ok {
				log.Printf("[Handler Error] Invalid chat message format from %s", senderID)
				return fmt.Errorf("invalid chat message format")
			}
			fmt.Printf("\r%s: %s\n> ", name, message)
		case protocol.TypePing:
			// Можно обработать пинг, если нужно, но для мессенджера пока игнорируем
		case protocol.TypePong:
			// Можно обработать понг, если нужно
		default:
			log.Printf("[Handler] Received unknown message type %s from %s", messageType, senderID)
		}
		return nil
	}

	// Инициализируем реестр пиров
	registry := discovery.NewPeerRegistry()

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
					fmt.Printf("\r[SYSTEM]: New peer joined: %s (%s:%d)\n> ", peer.Name, peer.IP, peer.Port)
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
						fmt.Printf("\r[SYSTEM]: %s has left the network\n> ", name)
						delete(knownPeers, id)
					}
				}
			}
		}

	}()

	// Периодически выводим список активных узлов для наглядности (можно убрать в финальной версии)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				active := registry.GetActivePeers()
				// This part is mostly for debugging. In a real messenger, you might not want to spam this.
				if len(active) > 0 {
					// fmt.Printf("\n--- Active Peers (%d) ---\n", len(active))
					// for _, p := range active {
					// 	fmt.Printf("- %s (%s:%d)\n", p.Name, p.IP, p.Port)
					// }
					// fmt.Println("------------------------")
				}
			}
		}
	}()

	// Цикл чтения из консоли и рассылки сообщений
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 {
				fmt.Print("> ")
				continue
			}

			// Упаковываем сообщение
			header := protocol.Header{
				MessageType: protocol.TypeChat,
				SenderID:    *name,
				RecipientID: "", // Для широковещания, можно оставить пустым или "ALL"
			}
			chatPayload := protocol.ChatMessage{Message: line}
			encodedMsg, err := protocol.Encode(header, chatPayload)
			if err != nil {
				log.Printf("[Encoder Error] Failed to encode message: %v", err)
				fmt.Print("> ")
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
			fmt.Print("> ")
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

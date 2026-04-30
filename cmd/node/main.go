package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mesh-cu/internal/discovery"
)

func main() {
	name := flag.String("name", fmt.Sprintf("node-%d", os.Getpid()), "Name of the node") // Делаем имя уникальным по PID
	port := flag.Int("port", 8080, "TCP port to listen on")                              // Этот порт пока не используется, но нужен для анонса
	flag.Parse()

	log.Printf("Starting node %s on port %d...", *name, *port)

	// Создаем сервис обнаружения
	discService := discovery.NewDiscoveryService(*name, *port)

	// Канал для найденных пиров
	peerChan := make(chan discovery.Peer)
	ctx, cancel := context.WithCancel(context.Background())

	// Запускаем сервис обнаружения (вещание + прослушивание) в фоне
	go discService.Start(ctx, peerChan)

	// Слушаем найденных соседей
	go func() {
		knownPeers := make(map[string]bool)
		for peer := range peerChan {
			if !knownPeers[peer.ID] {
				fmt.Printf("[Discovery] Found new peer: %s at %s:%d", peer.Name, peer.IP, peer.Port)
				knownPeers[peer.ID] = true
			}
		}
	}()

	// Ждем сигнала прерывания (Ctrl+C)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	cancel() // Отменяем контекст, чтобы остановить горутины
}

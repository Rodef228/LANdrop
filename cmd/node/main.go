package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mesh-cu/internal/discovery"
)

func main() {
	name := flag.String("name", fmt.Sprintf("node-%d", os.Getpid()), "Name of the node")
	port := flag.Int("port", 8080, "TCP port to listen on")
	flag.Parse()

	log.Printf("Starting node %s on port %d...", *name, *port)

	// Инициализируем реестр пиров
	registry := discovery.NewPeerRegistry()

	// Создаем сервис обнаружения
	discService := discovery.NewDiscoveryService(*name, *port)

	// Канал для найденных пиров
	peerChan := make(chan discovery.Peer)
	ctx, cancel := context.WithCancel(context.Background())

	// Запускаем сервис обнаружения (вещание + прослушивание) в фоне
	go discService.Start(ctx, peerChan)

	// Слушаем найденных соседей и обновляем реестр
	go func() {
		for peer := range peerChan {
			registry.Update(peer)
		}
	}()

	// Периодически выводим список активных узлов для наглядности
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				active := registry.GetActivePeers()
				if len(active) > 0 {
					fmt.Printf("\n--- Active Peers (%d) ---\n", len(active))
					for _, p := range active {
						fmt.Printf("- %s (%s:%d)\n", p.Name, p.IP, p.Port)
					}
					fmt.Println("------------------------")
				}
			}
		}
	}()

	// Ждем сигнала прерывания (Ctrl+C)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	cancel()
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"mesh-cu/internal/db"
	"mesh-cu/internal/discovery"
	"mesh-cu/internal/network"
	"mesh-cu/internal/protocol"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Для разработки разрешаем все origins
	},
}

type WSMessage struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	Timestamp int64                  `json:"timestamp,omitempty"`
}

type Client struct {
	conn   *websocket.Conn
	nodeID string
	send   chan []byte
	mu     sync.Mutex
}

type WSAPI struct {
	NodeID       string
	Port         int
	Registry     *discovery.PeerRegistry
	MessageChan  chan db.Message
	Clients      map[*Client]bool
	ClientsMu    sync.RWMutex
	GetChatFunc  func(string) []db.Message
	SendMsgFunc  func(string, string, string) error
	CreateGroupFunc func(string, []string) error
}

func NewWSAPI(nodeID string, port int, registry *discovery.PeerRegistry) *WSAPI {
	return &WSAPI{
		NodeID:      nodeID,
		Port:        port,
		Registry:    registry,
		MessageChan: make(chan db.Message, 100),
		Clients:     make(map[*Client]bool),
	}
}

func (api *WSAPI) Start(ctx context.Context, httpPort int) {
	http.HandleFunc("/ws", api.handleWebSocket)
	http.HandleFunc("/api/chats", api.handleGetChats)
	http.HandleFunc("/api/messages/", api.handleGetMessages)
	http.HandleFunc("/api/peers", api.handleGetPeers)
	
	addr := fmt.Sprintf(":%d", httpPort)
	log.Printf("[WS API] Starting WebSocket API server on %s", addr)
	
	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("[WS API Error] %v", err)
		}
	}()
	
	// Broadcast messages to all connected clients
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-api.MessageChan:
				api.broadcastMessage(msg)
			}
		}
	}()
}

func (api *WSAPI) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:   conn,
		nodeID: api.NodeID,
		send:   make(chan []byte, 256),
	}

	api.ClientsMu.Lock()
	api.Clients[client] = true
	api.ClientsMu.Unlock()

	// Send initial data
	api.sendInitialData(client)

	go client.writer(api)
	go client.reader(api)
}

func (c *Client) writer(api *WSAPI) {
	defer func() {
		api.ClientsMu.Lock()
		delete(api.Clients, c)
		api.ClientsMu.Unlock()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.mu.Lock()
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			c.mu.Unlock()
			if err != nil {
				return
			}
		case <-time.After(30 * time.Second):
			// Send ping to keep connection alive
			c.mu.Lock()
			c.conn.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()
		}
	}
}

func (c *Client) reader(api *WSAPI) {
	defer func() {
		api.ClientsMu.Lock()
		delete(api.Clients, c)
		api.ClientsMu.Unlock()
		c.conn.Close()
		close(c.send)
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			continue
		}

		switch wsMsg.Type {
		case "send_message":
			api.handleSendMessage(c, wsMsg.Payload)
		case "get_messages":
			api.handleGetMessagesWS(c, wsMsg.Payload)
		case "get_chats":
			api.handleGetChatsWS(c)
		case "get_peers":
			api.handleGetPeersWS(c)
		case "create_group":
			api.handleCreateGroup(c, wsMsg.Payload)
		}
	}
}

func (api *WSAPI) sendInitialData(client *Client) {
	// Send current peer list
	peers := api.Registry.GetActivePeers()
	peerList := make([]map[string]interface{}, len(peers))
	for i, p := range peers {
		peerList[i] = map[string]interface{}{
			"id":   p.ID,
			"name": p.Name,
			"ip":   p.IP,
			"port": p.Port,
		}
	}
	
	client.send <- mustMarshal(WSMessage{
		Type:    "peers",
		Payload: map[string]interface{}{"peers": peerList},
	})

	// Send chats list
	api.handleGetChatsWS(client)
}

func (api *WSAPI) handleSendMessage(client *Client, payload map[string]interface{}) {
	chatID, _ := payload["chat_id"].(string)
	content, _ := payload["content"].(string)
	recipientID, _ := payload["recipient_id"].(string)

	if chatID == "" || content == "" {
		client.send <- mustMarshal(WSMessage{
			Type:    "error",
			Payload: map[string]interface{}{"message": "Invalid message data"},
		})
		return
	}

	// Store message in DB
	msg := db.Message{
		ChatID:     chatID,
		SenderID:   api.NodeID,
		SenderName: api.NodeID,
		Content:    content,
		Timestamp:  time.Now().Unix(),
		IsRead:     true,
	}
	db.DB.Create(&msg)

	// Send to network if we have a send function
	if api.SendMsgFunc != nil {
		api.SendMsgFunc(chatID, content, recipientID)
	}

	// Broadcast to all clients
	api.broadcastMessage(msg)
}

func (api *WSAPI) handleGetMessagesWS(client *Client, payload map[string]interface{}) {
	chatID, _ := payload["chat_id"].(string)
	if chatID == "" {
		return
	}

	var messages []db.Message
	db.DB.Where("chat_id = ?", chatID).Order("timestamp asc").Find(&messages)

	msgList := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		msgList[i] = map[string]interface{}{
			"id":          m.ID,
			"chat_id":     m.ChatID,
			"sender_id":   m.SenderID,
			"sender_name": m.SenderName,
			"content":     m.Content,
			"timestamp":   m.Timestamp,
			"is_read":     m.IsRead,
		}
	}

	client.send <- mustMarshal(WSMessage{
		Type:    "messages",
		Payload: map[string]interface{}{"chat_id": chatID, "messages": msgList},
	})
}

func (api *WSAPI) handleGetChatsWS(client *Client) {
	type ChatInfo struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		IsGroup      bool   `json:"is_group"`
		Participants string `json:"participants"`
		UnreadCount  int    `json:"unread_count"`
		LastMessage  string `json:"last_message,omitempty"`
		LastTime     int64  `json:"last_time,omitempty"`
	}

	var chats []ChatInfo
	db.DB.Raw(`SELECT chats.id, chats.name, chats.is_group, chats.participants,
              SUM(CASE WHEN messages.is_read = 0 THEN 1 ELSE 0 END) as unread_count,
              (SELECT content FROM messages WHERE messages.chat_id = chats.id ORDER BY timestamp DESC LIMIT 1) as last_message,
              (SELECT timestamp FROM messages WHERE messages.chat_id = chats.id ORDER BY timestamp DESC LIMIT 1) as last_time
              FROM chats LEFT JOIN messages ON chats.id = messages.chat_id
              GROUP BY chats.id`).Scan(&chats)

	client.send <- mustMarshal(WSMessage{
		Type:    "chats",
		Payload: map[string]interface{}{"chats": chats},
	})
}

func (api *WSAPI) handleGetPeersWS(client *Client) {
	peers := api.Registry.GetActivePeers()
	peerList := make([]map[string]interface{}, len(peers))
	for i, p := range peers {
		peerList[i] = map[string]interface{}{
			"id":   p.ID,
			"name": p.Name,
			"ip":   p.IP,
			"port": p.Port,
		}
	}

	client.send <- mustMarshal(WSMessage{
		Type:    "peers",
		Payload: map[string]interface{}{"peers": peerList},
	})
}

func (api *WSAPI) handleCreateGroup(client *Client, payload map[string]interface{}) {
	groupName, _ := payload["name"].(string)
	participantsRaw, _ := payload["participants"].([]interface{})
	
	if groupName == "" || len(participantsRaw) == 0 {
		client.send <- mustMarshal(WSMessage{
			Type:    "error",
			Payload: map[string]interface{}{"message": "Invalid group data"},
		})
		return
	}

	var participants []string
	participants = append(participants, api.NodeID)
	for _, p := range participantsRaw {
		if name, ok := p.(string); ok && name != api.NodeID {
			participants = append(participants, name)
		}
	}

	// Create group logic here
	// For now just send confirmation
	client.send <- mustMarshal(WSMessage{
		Type:    "group_created",
		Payload: map[string]interface{}{"name": groupName, "participants": participants},
	})
}

func (api *WSAPI) broadcastMessage(msg db.Message) {
	msgData := map[string]interface{}{
		"id":          msg.ID,
		"chat_id":     msg.ChatID,
		"sender_id":   msg.SenderID,
		"sender_name": msg.SenderName,
		"content":     msg.Content,
		"timestamp":   msg.Timestamp,
		"is_read":     msg.IsRead,
	}

	api.ClientsMu.RLock()
	defer api.ClientsMu.RUnlock()

	for client := range api.Clients {
		select {
		case client.send <- mustMarshal(WSMessage{
			Type:    "new_message",
			Payload: msgData,
		}):
		default:
			// Client buffer full, skip
		}
	}
}

func (api *WSAPI) handleGetChats(w http.ResponseWriter, r *http.Request) {
	type ChatInfo struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		IsGroup      bool   `json:"is_group"`
		Participants string `json:"participants"`
		UnreadCount  int    `json:"unread_count"`
	}

	var chats []ChatInfo
	db.DB.Raw(`SELECT chats.id, chats.name, chats.is_group, chats.participants,
              SUM(CASE WHEN messages.is_read = 0 THEN 1 ELSE 0 END) as unread_count
              FROM chats LEFT JOIN messages ON chats.id = messages.chat_id
              GROUP BY chats.id`).Scan(&chats)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chats)
}

func (api *WSAPI) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	chatID := r.URL.Path[len("/api/messages/"):]
	if chatID == "" {
		http.Error(w, "Chat ID required", http.StatusBadRequest)
		return
	}

	var messages []db.Message
	db.DB.Where("chat_id = ?", chatID).Order("timestamp asc").Find(&messages)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (api *WSAPI) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	peers := api.Registry.GetActivePeers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

package main

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type serverMessage struct {
	Type    string     `json:"type"`
	Room    *roomState `json:"room,omitempty"`
	Message string     `json:"message,omitempty"`
}

type client struct {
	roomCode      string
	participantID string
	conn          *websocket.Conn
	writeMu       sync.Mutex
}

func (c *client) send(message serverMessage) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = wsjson.Write(ctx, c.conn, message)
}

type hub struct {
	mu      sync.RWMutex
	clients map[string]map[*client]struct{}
}

func newHub() *hub {
	return &hub{clients: make(map[string]map[*client]struct{})}
}

func (h *hub) add(roomCode, participantID string, conn *websocket.Conn) *client {
	c := &client{roomCode: roomCode, participantID: participantID, conn: conn}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[roomCode] == nil {
		h.clients[roomCode] = make(map[*client]struct{})
	}
	h.clients[roomCode][c] = struct{}{}
	return c
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[c.roomCode], c)
	if len(h.clients[c.roomCode]) == 0 {
		delete(h.clients, c.roomCode)
	}
}

func (h *hub) broadcast(roomCode string, message serverMessage) {
	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients[roomCode]))
	for c := range h.clients[roomCode] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		go c.send(message)
	}
}

func (h *hub) onlineParticipants(roomCode string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[string]bool)
	participants := []string{}
	for c := range h.clients[roomCode] {
		if !seen[c.participantID] {
			seen[c.participantID] = true
			participants = append(participants, c.participantID)
		}
	}
	return participants
}

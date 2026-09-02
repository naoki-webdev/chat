package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	connection *websocket.Conn
	channelID  string
	user       User
	server     *server
	hub        *hub
	send       chan []byte
	done       chan struct{}
	closeOnce  sync.Once
}

type websocketCommand struct {
	Type      string `json:"type"`
	ChannelID string `json:"channel_id,omitempty"`
	Presence  string `json:"presence,omitempty"`
}

type hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

const (
	pongWait   = 60 * time.Second
	writeWait  = 10 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

var realtimeRepositoryTimeout = 5 * time.Second

func newHub() *hub { return &hub{clients: make(map[*client]struct{})} }

func (h *hub) add(client *client) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

func (h *hub) remove(client *client) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
}

func (h *hub) broadcast(channelID string, payload []byte, memberIDs map[string]struct{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if channelID != "*" && client.channelID != "*" && client.channelID != channelID {
			continue
		}
		if channelID != "*" {
			if _, ok := memberIDs[client.user.ID]; !ok {
				continue
			}
		}
		select {
		case <-client.done:
			continue
		default:
		}
		select {
		case <-client.done:
		case client.send <- payload:
		default:
			log.Printf("disconnecting slow client after realtime buffer overflow on channel %s", channelID)
			client.shutdown()
		}
	}
}

func (c *client) readPump() {
	defer func() {
		c.hub.remove(c)
		c.shutdown()
	}()
	c.connection.SetReadLimit(maxWebSocketFrameBytes)
	_ = c.connection.SetReadDeadline(time.Now().Add(pongWait))
	c.connection.SetPongHandler(func(string) error { return c.connection.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, payload, err := c.connection.ReadMessage()
		if err != nil {
			return
		}
		var command websocketCommand
		if json.Unmarshal(payload, &command) != nil {
			continue
		}
		c.handleCommand(command)
	}
}

func (c *client) shutdown() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.connection != nil {
			_ = c.connection.Close()
		}
	})
}

func (c *client) handleCommand(command websocketCommand) {
	if command.Type != "typing.started" && command.Type != "typing.stopped" && command.Type != "presence.changed" {
		return
	}
	channelID := strings.TrimSpace(command.ChannelID)
	if command.Type == "presence.changed" {
		if command.Presence != "online" && command.Presence != "away" && command.Presence != "offline" {
			return
		}
		c.server.broadcast(realtimeEvent{Type: command.Type, ChannelID: "*", ActorID: c.user.ID, ActorName: c.user.Name, ActorHandle: c.user.Handle, ActorInitials: c.user.Initials, ActorColor: c.user.Color, Presence: command.Presence})
		return
	}
	if channelID == "" || channelID == "*" {
		channelID = c.channelID
	}
	if channelID == "*" {
		return
	}
	if c.channelID != "*" && c.channelID != channelID {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), realtimeRepositoryTimeout)
	defer cancel()
	member, err := c.server.repository.IsChannelMember(ctx, c.user.ID, channelID)
	if err != nil || !member {
		return
	}
	c.server.broadcast(realtimeEvent{Type: command.Type, ChannelID: channelID, ActorID: c.user.ID, ActorName: c.user.Name, ActorHandle: c.user.Handle, ActorInitials: c.user.Initials, ActorColor: c.user.Color})
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer func() {
		c.hub.remove(c)
		c.shutdown()
	}()
	for {
		select {
		case <-c.done:
			return
		case message, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.connection.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := c.connection.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.connection.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := c.connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

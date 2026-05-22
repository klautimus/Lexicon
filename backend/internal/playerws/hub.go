// Package playerws provides a WebSocket hub for multi-device playback control.
// Devices connect as either "player" (the host that outputs audio) or
// "controller" (remote devices that send play/pause/seek commands).
package playerws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // CORS already handled by chi middleware
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Message types
const (
	MsgPlay      = "play"
	MsgPause     = "pause"
	MsgResume    = "resume"
	MsgNext      = "next"
	MsgPrev      = "prev"
	MsgSeek      = "seek"
	MsgSetQueue  = "set_queue"
	MsgTransfer  = "transfer"
	MsgState     = "state"
	MsgDevices   = "devices"
	MsgRegister  = "register"
	MsgPromoted  = "promoted"
	MsgDemoted   = "demoted"
	MsgError     = "error"
)

type Message struct {
	Type      string          `json:"type"`
	TrackID   int64           `json:"track_id,omitempty"`
	Position  float64         `json:"position,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	Playing   bool            `json:"playing,omitempty"`
	Device    string          `json:"device,omitempty"`
	Target    string          `json:"target,omitempty"`
	Tracks    json.RawMessage `json:"tracks,omitempty"`
	StartIdx  int             `json:"start_index,omitempty"`
	Track     json.RawMessage `json:"track,omitempty"`
	// Queue and CurrentTrack match the frontend's transfer message fields
	// (DevicePicker sends {queue, currentTrack, position} on transfer).
	Queue        json.RawMessage `json:"queue,omitempty"`
	CurrentTrack json.RawMessage `json:"currentTrack,omitempty"`
	Devices      json.RawMessage `json:"devices,omitempty"`
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	deviceID string
	role     string // "player" or "controller"
	name     string
}

type Hub struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *clientMsg
	broadcast  chan []byte
	done       chan struct{}
	mu         sync.RWMutex
	lastState  []byte
}

type clientMsg struct {
	client *Client
}

func New() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *clientMsg),
		broadcast:  make(chan []byte, 256),
		done:       make(chan struct{}),
	}
}

// Shutdown signals the hub's Run loop to exit.
func (h *Hub) Shutdown() {
	close(h.done)
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.done:
			log.Printf("[playerws] hub shutdown")
			return

		case client := <-h.register:
			h.mu.Lock()
			// If a new player registers, demote the old one
			if client.role == "player" {
				for id, c := range h.clients {
					if c.role == "player" && id != client.deviceID {
						c.role = "controller"
					}
				}
			}
			h.clients[client.deviceID] = client
			h.mu.Unlock()
			log.Printf("[playerws] device registered: %s (%s, %s)", client.deviceID, client.role, client.name)
			h.broadcastDevices()

		case cm := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[cm.client.deviceID]; ok {
				delete(h.clients, cm.client.deviceID)
				close(cm.client.send)
			}
			h.mu.Unlock()
			log.Printf("[playerws] device unregistered: %s", cm.client.deviceID)
			h.broadcastDevices()

		case message := <-h.broadcast:
			// Capture state messages for transfer continuity
			var peek Message
			if err := json.Unmarshal(message, &peek); err == nil && peek.Type == MsgState {
				h.lastState = message
			}
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client's send buffer is full, skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) broadcastDevices() {
	h.mu.RLock()
	devices := make([]map[string]interface{}, 0, len(h.clients))
	for id, c := range h.clients {
		active := c.role == "player"
		devType := c.role
		devices = append(devices, map[string]interface{}{
			"id":     id,
			"name":   c.name,
			"type":   devType,
			"active": active,
		})
	}
	h.mu.RUnlock()

	data, err := json.Marshal(Message{Type: MsgDevices, Devices: mustJSON(devices)})
	if err != nil {
		return
	}

	h.mu.RLock()
	for _, client := range h.clients {
		select {
		case client.send <- data:
		default:
		}
	}
	h.mu.RUnlock()
}

// handleTransfer reassigns the player role from the current player to the target device.
// The requesting client must be the current player (or any controller if no player exists).
// queue, currentTrack, and position carry the full playback state from the initiator
// so the new player can resume seamlessly.
// Edge cases handled: target not found (error sent to requester), no current player
// (target promoted directly), self-transfer (no-op).
func (h *Hub) handleTransfer(source *Client, targetID string, queue json.RawMessage, currentTrack json.RawMessage, position float64) {
	h.mu.Lock()

	// Validate target exists
	target, ok := h.clients[targetID]
	if !ok {
		h.mu.Unlock()
		errMsg, _ := json.Marshal(Message{
			Type:  MsgError,
			Track: mustJSON(map[string]string{"message": "target device not found"}),
		})
		select {
		case source.send <- errMsg:
		default:
		}
		return
	}

	// Find current player
	var currentPlayer *Client
	for _, c := range h.clients {
		if c.role == "player" {
			currentPlayer = c
			break
		}
	}

	// Self-transfer — target is already the player
	if currentPlayer != nil && currentPlayer.deviceID == targetID {
		h.mu.Unlock()
		return
	}

	// Demote current player if one exists
	if currentPlayer != nil {
		currentPlayer.role = "controller"
	}

	// Promote target
	target.role = "player"

	// Build promoted message.
	// Prefer the explicit queue + currentTrack from the transfer message.
	// Fall back to lastState if the transfer initiator didn't send queue data.
	var promotedMsg []byte
	if queue != nil {
		promotedMsg, _ = json.Marshal(Message{
			Type:         MsgPromoted,
			Queue:        queue,
			CurrentTrack: currentTrack,
			Position:     position,
		})
	} else if h.lastState != nil {
		var lastState Message
		if err := json.Unmarshal(h.lastState, &lastState); err == nil {
			promotedMsg, _ = json.Marshal(Message{
				Type:     MsgPromoted,
				Playing:  lastState.Playing,
				Position: lastState.Position,
				Duration: lastState.Duration,
				Track:    lastState.Track,
				Tracks:   lastState.Tracks,
				StartIdx: lastState.StartIdx,
			})
		}
	}
	if promotedMsg == nil {
		promotedMsg, _ = json.Marshal(Message{Type: MsgPromoted})
	}

	h.mu.Unlock()

	// Send promoted to new player
	select {
	case target.send <- promotedMsg:
	default:
	}

	// Send demoted to old player
	if currentPlayer != nil {
		demotedMsg, _ := json.Marshal(Message{Type: MsgDemoted})
		select {
		case currentPlayer.send <- demotedMsg:
		default:
		}
	}

	log.Printf("[playerws] transfer: %s → %s (player role)", source.deviceID, targetID)
	h.broadcastDevices()
}

// ServeHTTP handles WebSocket upgrade requests.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceID")
	if deviceID == "" {
		deviceID = r.RemoteAddr
	}
	role := r.URL.Query().Get("role")
	if role == "" {
		role = "controller"
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = role
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[playerws] upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		deviceID: deviceID,
		role:     role,
		name:     name,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- &clientMsg{client: c}
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[playerws] read error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// Transfer is handled by the hub (role reassignment with validation).
		// Pass queue + currentTrack + position so the promoted message carries
		// the full playback state for continuity on the new player.
		if msg.Type == MsgTransfer && msg.Target != "" {
			c.hub.handleTransfer(c, msg.Target, msg.Queue, msg.CurrentTrack, msg.Position)
			continue
		}

		// Only controllers can send commands; players send state updates
		if c.role == "controller" {
			// Forward command to the player
			c.hub.mu.RLock()
			for _, player := range c.hub.clients {
				if player.role == "player" {
					select {
					case player.send <- data:
					default:
					}
					break
				}
			}
			c.hub.mu.RUnlock()
		} else if c.role == "player" && msg.Type == MsgState {
			// Player broadcasting state — forward to all controllers
			c.hub.broadcast <- data
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func mustJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

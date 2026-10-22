package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var wsClients = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "plantorq_api_websocket_clients",
	Help: "Browsers currently connected to the live event stream.",
})

// Event is a message pushed to browsers.
type Event struct {
	Type string `json:"type"` // "reading" | "alert" | "gateway"
	Data any    `json:"data"`
}

type wsClient struct {
	tenant string
	send   chan []byte
}

// Hub fans out events to the WebSocket clients of each tenant (Observer
// pattern). A slow client gets its oldest messages dropped rather than
// slowing down everyone else: a dashboard only cares about recent values.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*wsClient]struct{}
	log     *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{clients: map[string]map[*wsClient]struct{}{}, log: log}
}

// Broadcast sends ev to every client of tenant.
func (h *Hub) Broadcast(tenant string, ev Event) {
	msg, err := json.Marshal(ev)
	if err != nil {
		h.log.Error("encode event", "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[tenant] {
		select {
		case c.send <- msg:
		default: // client too slow: drop this message for it
		}
	}
}

func (h *Hub) add(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.tenant] == nil {
		h.clients[c.tenant] = map[*wsClient]struct{}{}
	}
	h.clients[c.tenant][c] = struct{}{}
	wsClients.Inc()
}

func (h *Hub) remove(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[c.tenant], c)
	close(c.send)
	wsClients.Dec()
}

// Clients returns how many browsers of tenant are connected.
func (h *Hub) Clients(tenant string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[tenant])
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = pongWait * 9 / 10
)

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: s.checkOrigin}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the HTTP error
	}
	c := &wsClient{tenant: principalFrom(r.Context()).TenantSlug, send: make(chan []byte, 64)}
	s.Hub.add(c)
	go readPump(conn, func() { s.Hub.remove(c) })
	writePump(conn, c.send)
}

// checkOrigin accepts same-origin requests plus the configured origins,
// preventing other websites from opening sockets with a user's token.
func (s *Server) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host || slices.Contains(s.Origins, origin)
}

// readPump consumes control frames (pongs, close) until the connection ends.
func readPump(conn *websocket.Conn, onClose func()) {
	defer onClose()
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// writePump is the only goroutine writing to conn, as gorilla requires.
func writePump(conn *websocket.Conn, send <-chan []byte) {
	ticker := time.NewTicker(pingPeriod)
	defer func() { ticker.Stop(); conn.Close() }()
	for {
		select {
		case msg, ok := <-send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

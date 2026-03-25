package main

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
		// Only accept connections from the extension (chrome-extension:// scheme)
		origin := r.Header.Get("Origin")
		return origin == "" || len(origin) > 17 && origin[:17] == "chrome-extension:"
	},
}

// Action is a single DOM interaction step for interact requests.
type Action struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"`
	Ms       int    `json:"ms,omitempty"`
}

// ExtRequest is sent from proxy-server to extension.
type ExtRequest struct {
	RequestID string            `json:"request_id"`
	Type      string            `json:"type"` // "fetch" or "interact"
	URL       string            `json:"url"`
	Method    string            `json:"method,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      interface{}       `json:"body,omitempty"`
	Actions   []Action          `json:"actions,omitempty"`
}

// ExtResponse is received from extension.
type ExtResponse struct {
	RequestID string            `json:"request_id"`
	Status    int               `json:"status,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	Title     string            `json:"title,omitempty"`
	PageURL   string            `json:"page_url,omitempty"`
	Result    interface{}       `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
	Message   string            `json:"message,omitempty"`
}

// Hub manages the single WebSocket connection from the extension.
type Hub struct {
	mu      sync.Mutex
	conn    *websocket.Conn
	pending map[string]chan ExtResponse
}

func NewHub() *Hub {
	return &Hub{pending: make(map[string]chan ExtResponse)}
}

func (h *Hub) IsConnected() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conn != nil
}

// ServeWS upgrades the HTTP connection and handles the extension.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	if h.conn != nil {
		h.conn.Close() // replace old connection
	}
	h.conn = conn
	h.mu.Unlock()

	log.Println("extension connected")
	defer func() {
		h.mu.Lock()
		if h.conn == conn {
			h.conn = nil
		}
		h.mu.Unlock()
		conn.Close()
		log.Println("extension disconnected")
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var resp ExtResponse
		if err := json.Unmarshal(msg, &resp); err != nil || resp.RequestID == "" {
			continue
		}
		h.mu.Lock()
		ch, ok := h.pending[resp.RequestID]
		if ok {
			delete(h.pending, resp.RequestID)
		}
		h.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

// Send forwards a request to the extension and waits for the response.
func (h *Hub) Send(req ExtRequest, timeout time.Duration) (ExtResponse, error) {
	ch := make(chan ExtResponse, 1)

	h.mu.Lock()
	if h.conn == nil {
		h.mu.Unlock()
		return ExtResponse{Error: "no_extension"}, nil
	}
	h.pending[req.RequestID] = ch
	err := h.conn.WriteJSON(req)
	h.mu.Unlock()

	if err != nil {
		h.mu.Lock()
		delete(h.pending, req.RequestID)
		h.mu.Unlock()
		return ExtResponse{Error: "no_extension"}, nil
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		h.mu.Lock()
		delete(h.pending, req.RequestID)
		h.mu.Unlock()
		return ExtResponse{Error: "timeout"}, nil
	}
}

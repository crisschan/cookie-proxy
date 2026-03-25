package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Server wires together HTTP routes and the Hub.
type Server struct {
	hub        *Hub
	mux        *http.ServeMux
	OnActivity func()
}

func NewServer(hub *Hub) *Server {
	s := &Server{
		hub:        hub,
		mux:        http.NewServeMux(),
		OnActivity: func() {},
	}
	s.mux.HandleFunc("/ping",   s.handlePing)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/fetch",  s.handleFetch)
	s.mux.HandleFunc("/action", s.handleAction)
	s.mux.HandleFunc("/ws",     s.handleWS)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.OnActivity()
	// Only accept local connections
	w.Header().Set("Access-Control-Allow-Origin", "*")
	s.mux.ServeHTTP(w, r)
}

// GET /ping
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jsonResp(w, map[string]interface{}{
		"status":             "ok",
		"version":            Version,
		"name":               "Cookie Proxy",
		"extension_connected": s.hub.IsConnected(),
	})
}

// GET /status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jsonResp(w, map[string]interface{}{
		"version":            Version,
		"extension_connected": s.hub.IsConnected(),
	})
}

// POST /fetch
func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    interface{}       `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}
	if req.URL == "" {
		jsonResp(w, map[string]string{"error": "bad_request", "message": "url is required"})
		return
	}
	if req.Method == "" {
		req.Method = "GET"
	}

	// SSRF check
	if err := CheckURL(req.URL); err != nil {
		jsonResp(w, map[string]string{"error": "ssrf_blocked", "message": err.Error()})
		return
	}

	if !s.hub.IsConnected() {
		jsonResp(w, map[string]string{"error": "no_extension", "message": "Chrome extension not connected"})
		return
	}

	extReq := ExtRequest{
		RequestID: uuid.NewString(),
		Type:      "fetch",
		URL:       req.URL,
		Method:    req.Method,
		Headers:   req.Headers,
		Body:      req.Body,
	}

	resp, err := s.hub.Send(extReq, 35*time.Second)
	if err != nil {
		jsonResp(w, map[string]string{"error": "internal", "message": err.Error()})
		return
	}

	if resp.Error != "" {
		jsonResp(w, map[string]string{"error": resp.Error, "message": resp.Message})
		return
	}

	jsonResp(w, map[string]interface{}{
		"status":  resp.Status,
		"headers": resp.Headers,
		"body":    resp.Body,
	})
}

// POST /action
func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL     string   `json:"url"`
		Actions []Action `json:"actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}
	if err := CheckURL(req.URL); err != nil {
		jsonResp(w, map[string]string{"error": "ssrf_blocked", "message": err.Error()})
		return
	}

	if !s.hub.IsConnected() {
		jsonResp(w, map[string]string{"error": "no_extension", "message": "Chrome extension not connected"})
		return
	}

	extReq := ExtRequest{
		RequestID: uuid.NewString(),
		Type:      "interact",
		URL:       req.URL,
		Actions:   req.Actions,
	}

	resp, err := s.hub.Send(extReq, 60*time.Second)
	if err != nil {
		jsonResp(w, map[string]string{"error": "internal", "message": err.Error()})
		return
	}

	if resp.Error != "" {
		jsonResp(w, map[string]string{"error": resp.Error, "message": resp.Message})
		return
	}

	jsonResp(w, map[string]interface{}{
		"title":  resp.Title,
		"url":    resp.PageURL,
		"result": resp.Result,
	})
}

// GET /ws  (WebSocket upgrade)
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	s.hub.ServeWS(w, r)
}

func jsonResp(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

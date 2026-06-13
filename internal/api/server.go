// Package api exposes the local HTTP+WebSocket API that the GUI and TUI connect
// to. The daemon hosts this; all frontends are thin clients over it.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rufatronics/velkrogo/internal/audit"
	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/orchestrator"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/scheduler"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // localhost only
}

// Server is the local daemon HTTP+WS server.
type Server struct {
	addr      string
	engine    *orchestrator.Engine
	db        *memory.DB
	alog      *audit.Log
	store     *provider.Store
	sched     *scheduler.Scheduler
	listener  net.Listener
	clients   map[*websocket.Conn]struct{}
	clientsMu sync.Mutex
	events    <-chan orchestrator.Event
}

// New creates a Server. addr is typically "127.0.0.1:7477".
func New(addr string, engine *orchestrator.Engine, db *memory.DB, alog *audit.Log,
	store *provider.Store, sched *scheduler.Scheduler, events <-chan orchestrator.Event) *Server {
	return &Server{addr: addr, engine: engine, db: db, alog: alog,
		store: store, sched: sched, clients: map[*websocket.Conn]struct{}{}, events: events}
}

// Start binds the listener and launches the server. Returns the actual address.
func (s *Server) Start(ctx context.Context) (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	s.listener = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/providers", s.handleProviders)
	mux.HandleFunc("/api/providers/test", s.handleProviderTest)
	mux.HandleFunc("/api/jobs", s.handleJobs)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	go s.broadcastEvents(ctx)
	return ln.Addr().String(), nil
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// --- WebSocket ---

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.clientsMu.Lock()
	s.clients[conn] = struct{}{}
	s.clientsMu.Unlock()
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		conn.Close()
	}()
	// Read until disconnect.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (s *Server) broadcast(msg any) {
	b, _ := json.Marshal(msg)
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for conn := range s.clients {
		conn.WriteMessage(websocket.TextMessage, b)
	}
}

func (s *Server) broadcastEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-s.events:
			if !ok {
				return
			}
			s.broadcast(map[string]any{"type": "event", "event": ev})
		}
	}
}

// --- REST endpoints ---

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Input     string `json:"input"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go func() {
		if err := s.engine.Run(r.Context(), req.Input); err != nil {
			s.broadcast(map[string]any{"type": "error", "message": err.Error()})
		}
	}()
	writeJSON(w, map[string]string{"status": "running"})
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.store.List())
	case http.MethodPost:
		var e provider.Entry
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.Add(e); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		_ = s.store.Remove(id)
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var e provider.Entry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := provider.TestConnection(ctx, e); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs, err := s.db.ListJobs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, jobs)
	case http.MethodPost:
		var j memory.Job
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if j.ID == "" {
			j.ID = fmt.Sprintf("job-%d", time.Now().UnixMilli())
		}
		if err := s.sched.AddJob(j); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "ok", "id": j.ID})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		_ = s.db.DeleteJob(id)
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.alog.Recent(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, entries)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.db.ListSessions(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sessions)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

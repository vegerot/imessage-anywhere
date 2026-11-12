package main

// The HTTP layer: routes, the source-address filter, and (later) SSE. It talks only to
// the Engine client.

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/netip"
	"strconv"
)

const conversationListLimit = 100

type server struct {
	engine *Engine
	mux    *http.ServeMux
}

func NewServer(engine *Engine) http.Handler {
	s := &server{engine: engine, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /{$}", servePage)
	s.mux.HandleFunc("GET /api/conversations", s.listConversations)
	s.mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)
	return s
}

// tailnet is Tailscale's address range (CGNAT, 100.64.0.0/10). Through the Pi stand-in
// node, tailnet callers keep their own 100.x address.
var tailnet = netip.MustParsePrefix("100.64.0.0/10")

// ServeHTTP refuses every caller except my tailnet devices and the Host Mac itself.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	addr := addrPort.Addr().Unmap()
	if err != nil || !(tailnet.Contains(addr) || addr.IsLoopback()) {
		http.Error(w, "only tailnet devices may connect", http.StatusForbidden)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *server) listConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := s.engine.Conversations(r.Context(), conversationListLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, conversations)
}

//go:embed index.html
var page []byte

func servePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

const historyLimit = 100

// listMessages serves a Conversation's last Messages, oldest first. ?limit=1 gives the
// list its last-Message preview.
func (s *server) listMessages(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad Conversation id", http.StatusBadRequest)
		return
	}
	limit := historyLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > historyLimit {
			http.Error(w, "limit must be 1 to 100", http.StatusBadRequest)
			return
		}
	}
	messages, err := s.engine.Messages(r.Context(), id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, messages)
}

package main

// The HTTP layer: routes, the source-address filter, and (later) SSE. It talks only to
// the Engine client.

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const conversationListLimit = 100

type server struct {
	engine  *Engine
	handler http.Handler
}

func NewServer(engine *Engine) http.Handler {
	s := &server{engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", servePage)
	mux.HandleFunc("GET /conversations/{id}", serveConversationPage)
	mux.HandleFunc("GET /api/conversations", s.listConversations)
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.sendMessage)
	mux.HandleFunc("GET /api/events", s.streamEvents)
	// Any website open on a tailnet device could otherwise submit a form here and send
	// Messages as me. This refuses cross-origin POSTs using the browser's own headers.
	s.handler = http.NewCrossOriginProtection().Handler(mux)
	return s
}

// tailnet is Tailscale's address range (CGNAT, 100.64.0.0/10). Through the Pi stand-in
// node, tailnet callers keep their own 100.x address.
var tailnet = netip.MustParsePrefix("100.64.0.0/10")

// allowedHost refuses DNS rebinding: a page on another site whose name rebinds to the Host
// Mac's address would otherwise pass the source filter and the cross-origin check. Its
// requests carry that site's name; mine carry an IP address, localhost, or a tailnet name.
func allowedHost(hostPort string) bool {
	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return true
	}
	return host == "localhost" || host == "work-macbook" || strings.HasSuffix(host, ".ts.net")
}

// ServeHTTP refuses every caller except my tailnet devices and the Host Mac itself.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	addr := addrPort.Addr().Unmap()
	if err != nil || !(tailnet.Contains(addr) || addr.IsLoopback()) {
		http.Error(w, "only tailnet devices may connect", http.StatusForbidden)
		return
	}
	if !allowedHost(r.Host) {
		http.Error(w, "unknown host name", http.StatusForbidden)
		return
	}
	s.handler.ServeHTTP(w, r)
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

// serveConversationPage serves the same page at a Conversation's clean path, so a reload
// or a bookmark opens it directly; the page reads the path.
func serveConversationPage(w http.ResponseWriter, r *http.Request) {
	if _, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err != nil {
		http.NotFound(w, r)
		return
	}
	servePage(w, r)
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

// heartbeatInterval keeps traffic flowing so the page can tell a live stream from one
// that died silently (the Host Mac sleeps every night; a receiving browser never notices).
const heartbeatInterval = 25 * time.Second

// streamEvents is a Server-Sent Events stream of new Messages. Each browser connection has
// its own imsg subscription, starting after its Last-Event-ID (sent automatically when
// EventSource reconnects) or ?since= (for the page's own reconnects), else at the tail.
func (s *server) streamEvents(w http.ResponseWriter, r *http.Request) {
	cursor := int64(0)
	if v := cmp.Or(r.Header.Get("Last-Event-ID"), r.URL.Query().Get("since")); v != "" {
		var err error
		if cursor, err = strconv.ParseInt(v, 10, 64); err != nil || cursor < 0 {
			http.Error(w, "bad cursor", http.StatusBadRequest)
			return
		}
	}
	sub, err := s.engine.Subscribe(r.Context(), cursor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer s.engine.Unsubscribe(sub)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	flusher := http.NewResponseController(w)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			// A named event, not an SSE comment: comments never reach the page's JavaScript.
			fmt.Fprint(w, "event: heartbeat\ndata: {}\n\n")
		case m, ok := <-sub.Messages:
			if !ok {
				return // imsg ended the subscription; the browser reconnects from its cursor
			}
			data, err := json.Marshal(m)
			if err != nil {
				continue
			}
			// Edits and unsends reuse an older row ID; only new rows move the cursor, so a
			// reconnect never replays from too far back.
			if m.ID > cursor {
				cursor = m.ID
				fmt.Fprintf(w, "id: %d\n", m.ID)
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
		}
		flusher.Flush()
	}
}

// sendMessage sends a form's text into a Conversation and reports whether it's confirmed.
func (s *server) sendMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad Conversation id", http.StatusBadRequest)
		return
	}
	text := r.FormValue("text")
	if strings.TrimSpace(text) == "" {
		http.Error(w, "nothing to send", http.StatusBadRequest)
		return
	}
	result, err := s.engine.Send(r.Context(), id, text)
	if err != nil {
		// e.g. the Automation permission (control Messages) is missing; the log says why.
		log.Printf("send to Conversation %d failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, result)
}

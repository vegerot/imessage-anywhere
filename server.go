package main

// The HTTP layer: routes, the source-address filter, and (later) SSE. It talks only to
// the Engine client.

import (
	"cmp"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
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
	mux.HandleFunc("GET /app.js", serveScript)
	// The web app manifest and its icons let a browser install the page as its own window.
	for _, name := range []string{"manifest.webmanifest", "icon.svg", "icon-192.png", "icon-512.png"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			if name == "manifest.webmanifest" {
				w.Header().Set("Content-Type", "application/manifest+json")
			}
			http.ServeFileFS(w, r, installFiles, name)
		})
	}
	mux.HandleFunc("GET /api/conversations", s.listConversations)
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.sendMessage)
	mux.HandleFunc("GET /api/conversations/{id}/messages/{message}/attachments/{index}", s.serveAttachment)
	mux.HandleFunc("GET /api/events", s.streamEvents)
	// Any website open on a tailnet device could otherwise submit a form here and send
	// Messages as me. This refuses cross-origin POSTs using the browser's own headers.
	s.handler = http.NewCrossOriginProtection().Handler(mux)
	return s
}

// tailnet is Tailscale's address range (CGNAT, 100.64.0.0/10). Through the Pi stand-in
// node, tailnet callers keep their own 100.x address.
var tailnet = netip.MustParsePrefix("100.64.0.0/10")

// isAllowedHost refuses DNS rebinding: a page on another site whose name rebinds to the Host
// Mac's address would otherwise pass the source filter and the cross-origin check. Its
// requests carry that site's name; mine carry an IP address, localhost, or a tailnet name.
func isAllowedHost(hostPort string) bool {
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
	if !isAllowedHost(r.Host) {
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

// The page's code is TypeScript (app.ts). This type-checks it and compiles it to app.js,
// which is generated, not committed: run `go generate ./...` before `go build` in a fresh checkout.
//go:generate bunx --package=typescript@7.0.2 tsc --project .

//go:embed index.html
var page []byte

//go:embed app.js
var script []byte

//go:embed manifest.webmanifest icon.svg icon-192.png icon-512.png
var installFiles embed.FS

func servePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func serveScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Write(script)
}

// pathID reads a numeric path value as the page writes it: plain digits, no sign, no leading
// zeros, so the Proxy and the page's router agree on which paths name a Conversation.
func pathID(r *http.Request, name string) (int64, bool) {
	value := r.PathValue(name)
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id >= 0 && strconv.FormatInt(id, 10) == value
}

// serveConversationPage serves the same page at a Conversation's clean path, so a reload
// or a bookmark opens it directly; the page reads the path.
func serveConversationPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := pathID(r, "id"); !ok {
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
	id, ok := pathID(r, "id")
	if !ok {
		http.Error(w, "bad Conversation id", http.StatusBadRequest)
		return
	}
	limit := historyLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		var err error
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
	id, ok := pathID(r, "id")
	if !ok {
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

// Content types by Apple UTI; mime_type from imsg is unreliable (empty in history reads).
var attachmentTypes = map[string]string{
	"public.jpeg":                        "image/jpeg",
	"public.png":                         "image/png",
	"com.compuserve.gif":                 "image/gif",
	"public.heic":                        "image/jpeg", // converted, like the audio/mp4 ones below
	"public.heif":                        "image/jpeg",
	"com.apple.m4a-audio":                "audio/mp4",
	"com.apple.coreaudio-format":         "audio/mp4",
	"org.3gpp.adaptive-multi-rate-audio": "audio/mp4",
	"public.mpeg-4":                      "video/mp4",
	"com.apple.quicktime-movie":          "video/quicktime",
	"com.adobe.pdf":                      "application/pdf",
}

// converters turn formats browsers can't show into ones they can, with tools macOS ships.
// Each takes the source and output paths. iPhone photos are HEIC (about two thirds of
// received images); voice messages are Opus in a .caf file, or AMR over SMS (measured
// 2026-11-16: afconvert takes 0.14 s for a 40 s voice message).
var converters = map[string]struct {
	ext     string
	command func(in, out string) *exec.Cmd
}{
	"public.heic":                        {".jpg", sips},
	"public.heif":                        {".jpg", sips},
	"com.apple.coreaudio-format":         {".m4a", afconvert},
	"org.3gpp.adaptive-multi-rate-audio": {".m4a", afconvert},
}

func sips(in, out string) *exec.Cmd {
	return exec.Command("sips", "-s", "format", "jpeg", in, "--out", out)
}

func afconvert(in, out string) *exec.Cmd {
	return exec.Command("afconvert", "-f", "m4af", "-d", "aac", in, out)
}

// serveAttachment serves a file imsg reports for a Message, if it's on the Host Mac's disk,
// converted when a browser couldn't show it.
func (s *server) serveAttachment(w http.ResponseWriter, r *http.Request) {
	conversationID, ok1 := pathID(r, "id")
	messageID, ok2 := pathID(r, "message")
	index, ok3 := pathID(r, "index")
	if !ok1 || !ok2 || !ok3 {
		http.Error(w, "bad attachment path", http.StatusBadRequest)
		return
	}
	file, err := s.engine.AttachmentFile(r.Context(), conversationID, messageID, int(index))
	if errors.Is(err, errNoAttachment) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	path := file.Path
	if converter, ok := converters[file.UTI]; ok {
		converted, err := convert(path, converter.ext, converter.command)
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer os.Remove(converted)
		path = converted
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	contentType := attachmentTypes[file.UTI]
	if contentType == "" {
		contentType = cmp.Or(mime.TypeByExtension(filepath.Ext(file.Name)), "application/octet-stream")
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
	http.ServeContent(w, r, file.Name, info.ModTime(), f) // handles Range, so videos can seek
}

// convert runs a converter on path and returns its output, a temporary file with ext.
func convert(path, ext string, command func(in, out string) *exec.Cmd) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	out, err := os.CreateTemp("", "imessage-anywhere-*"+ext)
	if err != nil {
		return "", err
	}
	out.Close()
	cmd := command(path, out.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(out.Name())
		return "", fmt.Errorf("%s: %v: %s", filepath.Base(cmd.Path), err, output)
	}
	return out.Name(), nil
}

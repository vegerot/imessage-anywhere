package main

// Tests exercise the one agreed seam: the whole Proxy, HTTP in and JSON-RPC out. The imsg
// child is replaced by this test binary re-running itself as a fake (Go's os/exec
// helper-process pattern) that replays a recorded, sanitized `imsg rpc` transcript.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const tailnetAddr = "100.100.7.7:51234"

func TestMain(m *testing.M) {
	if transcript := os.Getenv("IMSG_FAKE_TRANSCRIPT"); transcript != "" {
		runFakeImsg(transcript, os.Getenv("IMSG_FAKE_LOG"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeImsg answers JSON-RPC requests on stdin with the first recorded response for the
// same method. watch.subscribe replays the recorded Message notifications after the
// requested since_rowid (all of them when since_rowid is 0 or absent, as if they arrived
// live). Every request is appended to logPath; a method the transcript doesn't cover is
// logged as UNCOVERED, which fails the test.
func runFakeImsg(transcriptPath, logPath string) {
	if os.Getenv("IMSG_FAKE_EXIT_NOW") != "" {
		return
	}
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		panic(err)
	}
	keyByID := map[string]string{}
	recorded := map[string]json.RawMessage{} // request key (method, or method#chat_id) -> first recorded response line
	var notifications []json.RawMessage      // params.message of each "message" notification
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec struct{ Dir, Line string }
		must(json.Unmarshal([]byte(line), &rec))
		if rec.Dir != "in" && rec.Dir != "out" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Message json.RawMessage `json:"message"`
				ChatID  int64           `json:"chat_id"`
			} `json:"params"`
		}
		must(json.Unmarshal([]byte(rec.Line), &msg))
		switch {
		case rec.Dir == "in":
			keyByID[string(msg.ID)] = requestKey(msg.Method, msg.Params.ChatID)
		case msg.Method == "message":
			notifications = append(notifications, msg.Params.Message)
		case msg.ID != nil:
			key := keyByID[string(msg.ID)]
			for _, k := range []string{key, strings.Split(key, "#")[0]} {
				if _, seen := recorded[k]; !seen {
					recorded[k] = json.RawMessage(rec.Line)
				}
			}
		}
	}

	log, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	must(err)
	out := bufio.NewWriter(os.Stdout)
	reply := func(v any) {
		b, err := json.Marshal(v)
		must(err)
		out.Write(append(b, '\n'))
		out.Flush()
	}
	subscriptions := 0
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Fprintln(log, scanner.Text())
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				SinceRowID int64 `json:"since_rowid"`
				ChatID     int64 `json:"chat_id"`
			} `json:"params"`
		}
		must(json.Unmarshal(scanner.Bytes(), &req))
		switch req.Method {
		case "watch.subscribe":
			subscriptions++
			reply(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"subscription": subscriptions, "buffer_limit": 256}})
			for _, m := range notifications {
				var id struct{ ID int64 }
				must(json.Unmarshal(m, &id))
				if id.ID > req.Params.SinceRowID {
					reply(map[string]any{"jsonrpc": "2.0", "method": "message", "params": map[string]any{"subscription": subscriptions, "message": m}})
				}
			}
		case "watch.unsubscribe":
			reply(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"ok": true}})
		default:
			line, ok := recorded[requestKey(req.Method, req.Params.ChatID)]
			if !ok {
				line, ok = recorded[req.Method]
			}
			if !ok {
				fmt.Fprintln(log, "UNCOVERED", req.Method)
				reply(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "fake imsg: no recorded response for " + req.Method}})
				continue
			}
			var resp map[string]any
			must(json.Unmarshal(line, &resp))
			resp["id"] = req.ID
			reply(resp)
		}
	}
}

// requestKey lets the fake answer per Conversation when a transcript holds several.
func requestKey(method string, chatID int64) string {
	if chatID == 0 {
		return method
	}
	return fmt.Sprintf("%s#%d", method, chatID)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type proxyUnderTest struct {
	handler http.Handler
	engine  *Engine
	logPath string
}

// startProxy runs the real Proxy against a fake imsg replaying testdata/<transcript>.
func startProxy(t *testing.T, transcript string, extraEnv ...string) *proxyUnderTest {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "imsg-requests.log")
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "IMSG_FAKE_TRANSCRIPT=testdata/"+transcript, "IMSG_FAKE_LOG="+logPath)
	cmd.Env = append(cmd.Env, extraEnv...)
	engine, err := StartEngine(cmd)
	if err != nil {
		t.Fatal(err)
	}
	p := &proxyUnderTest{handler: NewServer(engine), engine: engine, logPath: logPath}
	t.Cleanup(func() {
		engine.Close()
		for _, line := range p.requestLines(t) {
			if strings.HasPrefix(line, "UNCOVERED") {
				t.Errorf("the Proxy sent a request the transcript doesn't cover: %s", line)
			}
		}
	})
	return p
}

func (p *proxyUnderTest) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	p.handler.ServeHTTP(rec, req)
	return rec
}

func (p *proxyUnderTest) get(t *testing.T, path, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	return p.do(t, req)
}

// requestLines returns every line the fake imsg received (plus UNCOVERED markers).
func (p *proxyUnderTest) requestLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(p.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestListsConversationsNewestFirstWithDisplayNames(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	got := decode[[]Conversation](t, p.get(t, "/api/conversations", tailnetAddr))

	if len(got) != 100 {
		t.Fatalf("got %d Conversations, want 100", len(got))
	}
	want := Conversation{ID: 242, Name: "Synthetic Name 0", Service: "iMessage", Unread: 1, LastMessageAt: "2026-11-12T08:27:59.322Z"}
	if got[0] != want {
		t.Errorf("first Conversation = %+v, want %+v", got[0], want)
	}
	byID := map[int64]Conversation{}
	for _, c := range got {
		byID[c.ID] = c
	}
	for _, tc := range []struct {
		id   int64
		name string
	}{
		{21, "Synthetic Name 5"},                       // named group
		{2093, "+1 (555) 010-0018, +1 (555) 010-0002"}, // unnamed group: its participants, formatted like Messages.app
		{1622, "+1 (555) 010-0003"},                    // unnamed SMS Conversation: its number, formatted
		{2267, "Synthetic Name 9"},                     // RCS Conversation with a display name
	} {
		if byID[tc.id].Name != tc.name {
			t.Errorf("Conversation %d name = %q, want %q", tc.id, byID[tc.id].Name, tc.name)
		}
	}
	if !byID[21].IsGroup || byID[242].IsGroup {
		t.Errorf("IsGroup wrong: 21=%v 242=%v", byID[21].IsGroup, byID[242].IsGroup)
	}
}

func TestOnlyTailnetAndLoopbackCallersAreServed(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	for _, tc := range []struct {
		remoteAddr string
		want       int
	}{
		{tailnetAddr, http.StatusOK},
		{"127.0.0.1:50000", http.StatusOK},
		{"[::1]:50000", http.StatusOK},
		{"10.70.166.20:50000", http.StatusForbidden}, // office network
		{"192.168.1.10:50000", http.StatusForbidden}, // home LAN, not through the tailnet
		{"100.128.0.1:50000", http.StatusForbidden},  // just outside 100.64.0.0/10
		{"172.16.1.1:50000", http.StatusForbidden},   // the WireGuard peer itself
	} {
		if got := p.get(t, "/api/conversations", tc.remoteAddr).Code; got != tc.want {
			t.Errorf("caller %s: status %d, want %d", tc.remoteAddr, got, tc.want)
		}
	}
}

func TestEngineSignalsDoneWhenImsgExits(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "IMSG_FAKE_TRANSCRIPT=testdata/imsg-0.21.0-01.jsonl", "IMSG_FAKE_EXIT_NOW=1")
	engine, err := StartEngine(cmd)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-engine.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done() didn't close after imsg exited")
	}
	if _, err := engine.Conversations(context.Background(), 1); err == nil {
		t.Error("a call after imsg exited succeeded; want an error")
	}
}

// The guarantee every other test leans on: a request the transcript doesn't cover is
// answered with an error and logged as UNCOVERED, which startProxy turns into a failure.
func TestFakeImsgFlagsUncoveredRequests(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "imsg-requests.log")
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "IMSG_FAKE_TRANSCRIPT=testdata/imsg-0.21.0-01.jsonl", "IMSG_FAKE_LOG="+logPath)
	engine, err := StartEngine(cmd)
	if err != nil {
		t.Fatal(err)
	}
	err = engine.call(context.Background(), "messages.search", map[string]any{"query": "x"}, nil)
	engine.Close()
	if err == nil {
		t.Error("an uncovered request succeeded; want an error")
	}
	log, _ := os.ReadFile(logPath)
	if !strings.Contains(string(log), "UNCOVERED messages.search") {
		t.Errorf("log doesn't flag the uncovered request:\n%s", log)
	}
}

// imsgRequests returns the params of every request the fake imsg received for method.
func (p *proxyUnderTest) imsgRequests(t *testing.T, method string) []map[string]any {
	t.Helper()
	var params []map[string]any
	for _, line := range p.requestLines(t) {
		var req struct {
			Method string
			Params map[string]any
		}
		if json.Unmarshal([]byte(line), &req) == nil && req.Method == method {
			params = append(params, req.Params)
		}
	}
	return params
}

func TestConversationMessagesAreOldestFirstAndDefaultToTheLast100(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	got := decode[[]Message](t, p.get(t, "/api/conversations/242/messages", tailnetAddr))
	p.get(t, "/api/conversations/242/messages?limit=1", tailnetAddr)

	var ids []int64
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	if want := []int64{183035, 183575, 183574, 183577, 183576}; fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("Message IDs = %v, want %v (oldest first)", ids, want)
	}
	last := got[len(got)-1]
	want := Message{ID: 183576, ConversationID: 242, FromMe: true, Sender: "person0@example.com", Text: "🧪 imessage-proxy test (P0 rpc)", CreatedAt: "2026-11-12T08:27:59.322Z"}
	if fmt.Sprintf("%+v", last) != fmt.Sprintf("%+v", want) {
		t.Errorf("newest Message = %+v\nwant %+v", last, want)
	}
	reqs := p.imsgRequests(t, "messages.history")
	if len(reqs) != 2 || reqs[0]["chat_id"] != 242.0 || reqs[0]["limit"] != 100.0 || reqs[1]["limit"] != 1.0 {
		t.Errorf("messages.history requests = %v, want chat_id 242 with limit 100, then limit 1", reqs)
	}
}

func messageByID(t *testing.T, messages []Message, id int64) Message {
	t.Helper()
	for _, m := range messages {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("Message %d not found", id)
	return Message{}
}

func TestGroupMessagesShowSendersAndTapbacks(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-02.jsonl")

	got := messageByID(t, decode[[]Message](t, p.get(t, "/api/conversations/21/messages", tailnetAddr)), 183536)

	if got.SenderName != "Synthetic Name 1" {
		t.Errorf("SenderName = %q, want the sender's Contact name", got.SenderName)
	}
	want := []Reaction{{Emoji: "❤️", From: "Synthetic Name 2"}, {Emoji: "❤️", From: "Synthetic Name 3"}}
	if fmt.Sprint(got.Reactions) != fmt.Sprint(want) {
		t.Errorf("Reactions = %+v, want %+v", got.Reactions, want)
	}
	if reqs := p.imsgRequests(t, "messages.history"); len(reqs) != 1 || reqs[0]["attachments"] != true {
		t.Errorf("messages.history requests = %v, want attachments: true (to label attachment-only Messages)", reqs)
	}
}

// Message types the page can't show yet get a placeholder instead of vanishing.
func TestUnsupportedMessagesGetPlaceholders(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-02.jsonl")
	group := decode[[]Message](t, p.get(t, "/api/conversations/21/messages", tailnetAddr))
	edits := decode[[]Message](t, p.get(t, "/api/conversations/1947/messages", tailnetAddr))
	polls := decode[[]Message](t, p.get(t, "/api/conversations/2093/messages", tailnetAddr))
	apps := decode[[]Message](t, p.get(t, "/api/conversations/47/messages", tailnetAddr))

	for _, tc := range []struct {
		name        string
		got         Message
		text        string
		placeholder string
	}{
		{"attachment-only photo", messageByID(t, group, 183536), "", "📷 Photo"},
		{"sticker", messageByID(t, edits, 183553), "", "Sticker"},
		{"poll vote", messageByID(t, polls, 178893), "", "📊 Poll vote"},
		{"Apple Cash", messageByID(t, apps, 183542), "Synthetic message 283", "🧩 Apple Cash"},
	} {
		if tc.got.Text != tc.text || tc.got.Placeholder != tc.placeholder {
			t.Errorf("%s: Text %q, Placeholder %q; want %q, %q", tc.name, tc.got.Text, tc.got.Placeholder, tc.text, tc.placeholder)
		}
	}
	if edited := messageByID(t, edits, 183568); !edited.Edited || edited.Text != "Synthetic message 93" {
		t.Errorf("edited Message = %+v, want Edited with its current text", edited)
	}
}

// A whitespace-only Message (seen in the wild: text "\n", no attachment or app) has
// nothing to show, so it's left out rather than drawn as an empty bubble.
func TestContentlessMessagesAreLeftOut(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-02.jsonl")

	for _, m := range decode[[]Message](t, p.get(t, "/api/conversations/1947/messages", tailnetAddr)) {
		if m.ID == 183562 {
			t.Errorf("whitespace-only Message 183562 was served: %+v", m)
		}
	}
}

type sseEvent struct{ ID, Event, Data string }

// readEvents opens the Proxy's event stream on a real HTTP server and returns the first n
// events (comments such as heartbeats are skipped), then disconnects.
func readEvents(t *testing.T, p *proxyUnderTest, path string, header http.Header, n int) []sseEvent {
	t.Helper()
	server := httptest.NewServer(p.handler) // loopback, which the source filter allows
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	var events []sseEvent
	var current sseEvent
	scanner := bufio.NewScanner(resp.Body)
	for len(events) < n && scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if current.Data != "" {
				events = append(events, current)
			}
			current = sseEvent{}
		case strings.HasPrefix(line, "id: "):
			current.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			current.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	return events
}

func TestLiveMessagesResumeFromLastEventID(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-p0.jsonl")

	events := readEvents(t, p, "/api/events", http.Header{"Last-Event-Id": {"183575"}}, 2)

	var ids []string
	for _, e := range events {
		ids = append(ids, e.ID)
		var m Message
		if err := json.Unmarshal([]byte(e.Data), &m); err != nil || m.ConversationID != 242 || fmt.Sprint(m.ID) != e.ID {
			t.Errorf("event %+v doesn't carry its Message (decoded %+v, err %v)", e, m, err)
		}
	}
	if want := []string{"183576", "183577"}; fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("event IDs = %v, want %v (the rows after Last-Event-ID, which are database row IDs)", ids, want)
	}
	subs := p.imsgRequests(t, "watch.subscribe")
	if len(subs) != 1 || subs[0]["since_rowid"] != 183575.0 || subs[0]["attachments"] != true {
		t.Errorf("watch.subscribe requests = %v, want one with since_rowid 183575 and attachments", subs)
	}
}

func TestLiveMessagesStartAtTheTailWithoutACursorAndUnsubscribeOnDisconnect(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-p0.jsonl")

	readEvents(t, p, "/api/events", nil, 1)

	subs := p.imsgRequests(t, "watch.subscribe")
	if len(subs) != 1 || subs[0]["since_rowid"] != nil {
		t.Errorf("watch.subscribe requests = %v, want one without since_rowid (start at the tail)", subs)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(p.imsgRequests(t, "watch.unsubscribe")) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := p.imsgRequests(t, "watch.unsubscribe"); len(got) != 1 || got[0]["subscription"] != 1.0 {
		t.Errorf("watch.unsubscribe requests = %v, want one for subscription 1 after the browser left", got)
	}
}

func TestLiveMessagesAcceptAnExplicitCursorForManualReconnects(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-p0.jsonl")

	events := readEvents(t, p, "/api/events?since=183576", nil, 1)

	if len(events) != 1 || events[0].ID != "183577" {
		t.Errorf("events = %+v, want only 183577", events)
	}
}

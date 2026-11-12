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
	"net/url"
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
	var notifications []map[string]any       // every recorded subscription notification, in order
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
		case msg.Method != "":
			var n map[string]any
			must(json.Unmarshal([]byte(rec.Line), &n))
			notifications = append(notifications, n)
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
			for _, n := range notifications {
				params := n["params"].(map[string]any)
				if m, ok := params["message"].(map[string]any); ok && int64(m["id"].(float64)) <= req.Params.SinceRowID {
					continue
				}
				params["subscription"] = subscriptions
				reply(n)
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
	req.Host = "100.121.37.3:7171"
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
	want := Message{ID: 183576, GUID: "00000000-0000-0000-0000-000000000005", ConversationID: 242, FromMe: true, Sender: "person0@example.com", Text: "🧪 imessage-proxy test (P0 rpc)", CreatedAt: "2026-11-12T08:27:59.322Z"}
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

func (p *proxyUnderTest) postForm(t *testing.T, path string, form url.Values, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.RemoteAddr = tailnetAddr
	req.Host = "100.121.37.3:7171"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		req.Header[k] = v
	}
	return p.do(t, req)
}

type sendReply struct {
	Status string `json:"status"`
	ID     int64  `json:"id"`
}

func TestSendingTextTargetsTheConversationOverAppleScript(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-p0.jsonl")

	got := decode[sendReply](t, p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"🧪 hi"}}, nil))

	if got != (sendReply{Status: "confirmed", ID: 183576}) {
		t.Errorf("reply = %+v, want confirmed with the outgoing row's ID", got)
	}
	sends := p.imsgRequests(t, "send")
	want := map[string]any{"chat_id": 242.0, "text": "🧪 hi", "transport": "applescript"}
	if len(sends) != 1 || fmt.Sprint(sends[0]) != fmt.Sprint(want) {
		t.Errorf("send requests = %v, want exactly %v", sends, want)
	}
}

func TestAnUnconfirmedSendIsReportedAndNeverRetried(t *testing.T) {
	p := startProxy(t, "synthetic-send-unconfirmed.jsonl")

	got := decode[sendReply](t, p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"🧪 hi"}}, nil))

	if got.Status != "unconfirmed" {
		t.Errorf("reply = %+v, want status unconfirmed", got)
	}
	if sends := p.imsgRequests(t, "send"); len(sends) != 1 {
		t.Errorf("imsg got %d send requests, want exactly 1 (an unconfirmed send may have gone out)", len(sends))
	}
}

func TestASendThatNeverStartedFails(t *testing.T) {
	p := startProxy(t, "synthetic-send-not-started.jsonl")

	if got := p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"🧪 hi"}}, nil).Code; got != http.StatusBadGateway {
		t.Errorf("status %d, want 502 (definitely not sent)", got)
	}
}

func TestEmptyTextIsNotSent(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-p0.jsonl")

	if got := p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"  \n "}}, nil).Code; got != http.StatusBadRequest {
		t.Errorf("status %d, want 400", got)
	}
	if sends := p.imsgRequests(t, "send"); len(sends) != 0 {
		t.Errorf("imsg got send requests %v, want none", sends)
	}
}

// Any website open on a tailnet device could otherwise submit a hidden form here and send
// Messages as me.
func TestCrossSiteSendsAreRefused(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-p0.jsonl")

	got := p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"🧪 hi"}}, http.Header{"Sec-Fetch-Site": {"cross-site"}})

	if got.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", got.Code)
	}
	if sends := p.imsgRequests(t, "send"); len(sends) != 0 {
		t.Errorf("imsg got send requests %v, want none", sends)
	}
}

func TestReadyWhenImsgCanReadTheMessagesDatabase(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	if err := p.engine.Ready(context.Background()); err != nil {
		t.Errorf("Ready() = %v, want nil", err)
	}
}

func TestNotReadyExplainsTheMissingFullDiskAccess(t *testing.T) {
	p := startProxy(t, "synthetic-status-no-disk-access.jsonl")

	err := p.engine.Ready(context.Background())

	if err == nil || !strings.Contains(err.Error(), "authorization denied") || !strings.Contains(err.Error(), "Full Disk Access") {
		t.Errorf("Ready() = %v, want imsg's reason and how to grant Full Disk Access", err)
	}
}

func TestLiveTapbacksPointAtTheirMessageAndContentlessRowsAreLeftOut(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-03.jsonl")

	byID := map[int64]Message{}
	for _, e := range readEvents(t, p, "/api/events?since=183535", nil, 9) {
		var m Message
		must(json.Unmarshal([]byte(e.Data), &m))
		byID[m.ID] = m
	}

	if target := byID[183536]; target.GUID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("photo 183536 GUID = %q, want its GUID so tapbacks can find it", target.GUID)
	}
	tapback := byID[183537]
	if tapback.ReactsTo != "00000000-0000-0000-0000-000000000001" || tapback.Tapback != "❤️" || tapback.SenderName != "Synthetic Name 1" || tapback.TapbackRemoved {
		t.Errorf("tapback 183537 = %+v, want ❤️ from Synthetic Name 1 on 183536's GUID", tapback)
	}
	if m, ok := byID[183562]; ok {
		t.Errorf("whitespace-only Message 183562 was streamed: %+v", m)
	}
	if subs := p.imsgRequests(t, "watch.subscribe"); len(subs) != 1 || subs[0]["include_reactions"] != true {
		t.Errorf("watch.subscribe requests = %v, want include_reactions: true", subs)
	}
}

func TestAFailedWatchEndsTheStreamSoTheBrowserReconnects(t *testing.T) {
	p := startProxy(t, "synthetic-watch-error.jsonl")

	start := time.Now()
	events := readEvents(t, p, "/api/events", nil, 1)

	if len(events) != 0 || time.Since(start) > 3*time.Second {
		t.Errorf("got %v after %v; want the stream to end promptly with no events", events, time.Since(start))
	}
}

// A page on another site whose DNS name rebinds to the Host Mac's address would otherwise
// pass both the source filter and the cross-origin check.
func TestRequestsForOtherHostNamesAreRefused(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	for _, tc := range []struct {
		host string
		want int
	}{
		{"evil.example:7171", http.StatusForbidden},
		{"100.121.37.3:7171", http.StatusOK},
		{"127.0.0.1:7171", http.StatusOK},
		{"localhost:7171", http.StatusOK},
		{"work-macbook:7171", http.StatusOK},
		{"work-macbook.tail1234.ts.net:7171", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
		req.RemoteAddr = tailnetAddr
		req.Host = tc.host
		if got := p.do(t, req).Code; got != tc.want {
			t.Errorf("Host %s: status %d, want %d", tc.host, got, tc.want)
		}
	}
}

func waitForDone(t *testing.T, e *Engine) bool {
	t.Helper()
	select {
	case <-e.Done():
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

// A send still in flight blocks imsg's send queue until the child restarts; the Proxy
// exits so launchd restarts it, and a send refused by the blocked queue never ran.
func TestABlockedSendQueueIsNotSentAndRestartsImsg(t *testing.T) {
	p := startProxy(t, "synthetic-send-lane-blocked.jsonl")

	if got := p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"🧪 hi"}}, nil).Code; got != http.StatusBadGateway {
		t.Errorf("status %d, want 502 (the blocked queue refused it before sending)", got)
	}
	if !waitForDone(t, p.engine) {
		t.Error("imsg wasn't restarted; its send queue stays blocked")
	}
}

func TestAStillInFlightSendIsUnconfirmedAndRestartsImsg(t *testing.T) {
	p := startProxy(t, "synthetic-send-still-in-flight.jsonl")

	if got := decode[sendReply](t, p.postForm(t, "/api/conversations/242/messages", url.Values{"text": {"🧪 hi"}}, nil)); got.Status != "unconfirmed" {
		t.Errorf("reply = %+v, want unconfirmed", got)
	}
	if !waitForDone(t, p.engine) {
		t.Error("imsg wasn't restarted; its send queue stays blocked")
	}
}

// Conversations have clean paths: the Proxy serves the same page at /conversations/{id},
// so a reload or a bookmark opens that Conversation directly.
func TestConversationPathsServeThePage(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	for _, path := range []string{"/", "/conversations/21"} {
		got := p.get(t, path, tailnetAddr)
		if got.Code != http.StatusOK || !strings.HasPrefix(got.Header().Get("Content-Type"), "text/html") || !strings.Contains(got.Body.String(), "<title>Messages</title>") {
			t.Errorf("GET %s: status %d, Content-Type %q; want the page", path, got.Code, got.Header().Get("Content-Type"))
		}
	}
	if got := p.get(t, "/conversations/not-a-number", tailnetAddr).Code; got != http.StatusNotFound {
		t.Errorf("GET /conversations/not-a-number: status %d, want 404", got)
	}
}

// The page's code is TypeScript (app.ts), compiled by `go generate` into app.js, which the
// Proxy embeds and serves as a module.
func TestThePageScriptIsServedAsJavaScript(t *testing.T) {
	p := startProxy(t, "imsg-0.21.0-01.jsonl")

	got := p.get(t, "/app.js", tailnetAddr)

	if got.Code != http.StatusOK || !strings.HasPrefix(got.Header().Get("Content-Type"), "text/javascript") || !strings.Contains(got.Body.String(), "EventSource") {
		t.Errorf("GET /app.js: status %d, Content-Type %q; want the page's JavaScript", got.Code, got.Header().Get("Content-Type"))
	}
	if page := p.get(t, "/", tailnetAddr).Body.String(); !strings.Contains(page, `<script type="module" src="/app.js"></script>`) {
		t.Error("the page doesn't load /app.js as a module")
	}
}

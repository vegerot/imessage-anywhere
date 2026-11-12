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
	methodByID := map[string]string{}
	recorded := map[string]json.RawMessage{} // method -> first recorded response line
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
			} `json:"params"`
		}
		must(json.Unmarshal([]byte(rec.Line), &msg))
		switch {
		case rec.Dir == "in":
			methodByID[string(msg.ID)] = msg.Method
		case msg.Method == "message":
			notifications = append(notifications, msg.Params.Message)
		case msg.ID != nil:
			method := methodByID[string(msg.ID)]
			if _, seen := recorded[method]; !seen {
				recorded[method] = json.RawMessage(rec.Line)
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
			line, ok := recorded[req.Method]
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

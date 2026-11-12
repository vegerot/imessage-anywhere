package main

// The Engine client: the only code that knows imsg exists. It owns one long-lived
// `imsg rpc` child, speaks its JSON-RPC over stdio, and turns imsg's JSON into the
// Proxy's own types (Conversation, Message).

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Engine struct {
	stdin io.WriteCloser
	done  chan struct{} // closed when the child exits

	mu      sync.Mutex
	nextID  int
	pending map[string]chan rpcResponse
}

type rpcResponse struct {
	Result json.RawMessage
	Error  *rpcError
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("imsg: %s (code %d, %v)", e.Message, e.Code, e.Data)
}

var errEngineGone = errors.New("imsg rpc child exited")

// StartEngine starts cmd (normally `imsg rpc`) and reads its output until it exits.
func StartEngine(cmd *exec.Cmd) (*Engine, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	e := &Engine{stdin: stdin, done: make(chan struct{}), pending: map[string]chan rpcResponse{}}
	go func() {
		e.readLoop(stdout)
		cmd.Wait()
		close(e.done)
	}()
	return e, nil
}

// Done is closed when the imsg child exits. The Proxy then exits too, and launchd
// restarts it; browsers resume on their own through Last-Event-ID.
func (e *Engine) Done() <-chan struct{} { return e.done }

// Close ends the child by closing its stdin, then waits for it to exit.
func (e *Engine) Close() {
	e.stdin.Close()
	<-e.done
}

func (e *Engine) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20) // one Conversation list can be large
	for scanner.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			fmt.Fprintf(os.Stderr, "engine: unparsable line from imsg: %v\n", err)
			continue
		}
		id, _ := strconv.Unquote(string(msg.ID))
		e.mu.Lock()
		ch := e.pending[id]
		delete(e.pending, id)
		e.mu.Unlock()
		if ch != nil {
			ch <- rpcResponse{Result: msg.Result, Error: msg.Error}
		}
	}
}

// call sends one JSON-RPC request and decodes its result into out.
func (e *Engine) call(ctx context.Context, method string, params any, out any) error {
	ch := make(chan rpcResponse, 1)
	e.mu.Lock()
	e.nextID++
	id := strconv.Itoa(e.nextID)
	e.pending[id] = ch
	line, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err == nil {
		_, err = e.stdin.Write(append(line, '\n'))
	}
	e.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(resp.Result, out)
	case <-ctx.Done():
		e.forget(id)
		return ctx.Err()
	case <-e.done:
		return errEngineGone
	}
}

func (e *Engine) forget(id string) {
	e.mu.Lock()
	delete(e.pending, id)
	e.mu.Unlock()
}

// Conversation is one 1:1 or group exchange, as the page sees it.
type Conversation struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Service       string `json:"service"`
	IsGroup       bool   `json:"isGroup"`
	Unread        int    `json:"unread"`
	LastMessageAt string `json:"lastMessageAt"`
}

type imsgChat struct {
	ID            int64    `json:"id"`
	DisplayName   string   `json:"display_name"`
	ContactName   string   `json:"contact_name"`
	Identifier    string   `json:"identifier"`
	Participants  []string `json:"participants"`
	Service       string   `json:"service"`
	IsGroup       bool     `json:"is_group"`
	UnreadCount   int      `json:"unread_count"`
	LastMessageAt string   `json:"last_message_at"`
}

// Conversations lists the newest Conversations first, like the Messages.app sidebar.
func (e *Engine) Conversations(ctx context.Context, limit int) ([]Conversation, error) {
	var result struct{ Chats []imsgChat }
	if err := e.call(ctx, "chats.list", map[string]any{"limit": limit}, &result); err != nil {
		return nil, err
	}
	conversations := make([]Conversation, 0, len(result.Chats))
	for _, c := range result.Chats {
		conversations = append(conversations, Conversation{
			ID:            c.ID,
			Name:          c.displayName(),
			Service:       c.Service,
			IsGroup:       c.IsGroup,
			Unread:        c.UnreadCount,
			LastMessageAt: c.LastMessageAt,
		})
	}
	return conversations, nil
}

// displayName follows Messages.app: the group's or Conversation's own name, else the
// contact's name, else (for a group) its participants, else the raw handle.
func (c imsgChat) displayName() string {
	switch {
	case c.DisplayName != "":
		return c.DisplayName
	case c.ContactName != "":
		return c.ContactName
	case c.IsGroup && len(c.Participants) > 0:
		names := make([]string, len(c.Participants))
		for i, p := range c.Participants {
			names[i] = formatHandle(p)
		}
		return strings.Join(names, ", ")
	default:
		return formatHandle(c.Identifier)
	}
}

// nanpNumber matches a +1 (North American) phone number, the only kind I see.
var nanpNumber = regexp.MustCompile(`^\+1(\d{3})(\d{3})(\d{4})$`)

// formatHandle writes +1 numbers the way Messages.app does, "+1 (555) 010-0003". Other
// handles (emails, short codes, other countries) pass through unchanged.
func formatHandle(handle string) string {
	return nanpNumber.ReplaceAllString(handle, "+1 ($1) $2-$3")
}

// Message is one item sent or received in a Conversation, as the page sees it.
type Message struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversationId"`
	FromMe         bool   `json:"fromMe"`
	Sender         string `json:"sender"`
	SenderName     string `json:"senderName,omitempty"`
	Text           string `json:"text"`
	CreatedAt      string `json:"createdAt"`
}

type imsgMessage struct {
	ID         int64  `json:"id"`
	ChatID     int64  `json:"chat_id"`
	IsFromMe   bool   `json:"is_from_me"`
	Sender     string `json:"sender"`
	SenderName string `json:"sender_name"`
	Text       string `json:"text"`
	CreatedAt  string `json:"created_at"`
}

func (m imsgMessage) toMessage() Message {
	return Message{
		ID:             m.ID,
		ConversationID: m.ChatID,
		FromMe:         m.IsFromMe,
		Sender:         m.Sender,
		SenderName:     m.SenderName,
		Text:           m.Text,
		CreatedAt:      m.CreatedAt,
	}
}

// Messages returns a Conversation's newest limit Messages, oldest first.
func (e *Engine) Messages(ctx context.Context, conversationID int64, limit int) ([]Message, error) {
	var result struct{ Messages []imsgMessage }
	params := map[string]any{"chat_id": conversationID, "limit": limit}
	if err := e.call(ctx, "messages.history", params, &result); err != nil {
		return nil, err
	}
	messages := make([]Message, len(result.Messages))
	for i, m := range result.Messages { // imsg returns newest first
		messages[len(messages)-1-i] = m.toMessage()
	}
	return messages, nil
}

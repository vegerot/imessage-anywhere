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
	"slices"
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
	ID             int64      `json:"id"`
	ConversationID int64      `json:"conversationId"`
	FromMe         bool       `json:"fromMe"`
	Sender         string     `json:"sender"`
	SenderName     string     `json:"senderName,omitempty"` // Contact name or formatted Handle; empty for my own
	Text           string     `json:"text"`
	CreatedAt      string     `json:"createdAt"`
	Edited         bool       `json:"edited,omitempty"`
	Reactions      []Reaction `json:"reactions,omitempty"`
	// Placeholder names what the page can't show yet (attachments, polls, app messages,
	// unsent Messages), so nothing vanishes silently.
	Placeholder string `json:"placeholder,omitempty"`
}

// Reaction is a tapback on a Message.
type Reaction struct {
	Emoji  string `json:"emoji"`
	From   string `json:"from"`
	FromMe bool   `json:"fromMe,omitempty"`
}

type imsgMessage struct {
	ID              int64  `json:"id"`
	ChatID          int64  `json:"chat_id"`
	IsFromMe        bool   `json:"is_from_me"`
	Sender          string `json:"sender"`
	SenderName      string `json:"sender_name"`
	Text            string `json:"text"`
	CreatedAt       string `json:"created_at"`
	EditedAt        string `json:"edited_at"`
	UnsentAt        string `json:"unsent_at"`
	BalloonBundleID string `json:"balloon_bundle_id"`
	Poll            *struct {
		Kind     string `json:"kind"`
		Question string `json:"question"`
	} `json:"poll"`
	Reactions []struct {
		Emoji      string `json:"emoji"`
		Sender     string `json:"sender"`
		SenderName string `json:"sender_name"`
		IsFromMe   bool   `json:"is_from_me"`
	} `json:"reactions"`
	Attachments []imsgAttachment `json:"attachments"`
}

type imsgAttachment struct {
	TransferName string `json:"transfer_name"`
	UTI          string `json:"uti"`
	IsSticker    bool   `json:"is_sticker"`
	Missing      bool   `json:"missing"`
}

// objectReplacement marks where an attachment or app balloon sits inside a Message's text.
const objectReplacement = "\ufffc"

func (m imsgMessage) toMessage() Message {
	msg := Message{
		ID:             m.ID,
		ConversationID: m.ChatID,
		FromMe:         m.IsFromMe,
		Sender:         m.Sender,
		Text:           strings.TrimSpace(strings.ReplaceAll(m.Text, objectReplacement, "")),
		CreatedAt:      m.CreatedAt,
		Edited:         m.EditedAt != "",
		Placeholder:    m.placeholder(),
	}
	if !m.IsFromMe {
		msg.SenderName = nameOr(m.SenderName, m.Sender)
	}
	for _, r := range m.Reactions {
		msg.Reactions = append(msg.Reactions, Reaction{Emoji: r.Emoji, From: nameOr(r.SenderName, r.Sender), FromMe: r.IsFromMe})
	}
	return msg
}

func nameOr(name, handle string) string {
	if name != "" {
		return name
	}
	return formatHandle(handle)
}

func (m imsgMessage) placeholder() string {
	switch {
	case m.UnsentAt != "":
		return "Unsent a message"
	case m.Poll != nil && m.Poll.Kind == "vote":
		return "📊 Poll vote"
	case m.Poll != nil && m.Poll.Question != "":
		return "📊 Poll: " + m.Poll.Question
	case m.Poll != nil:
		return "📊 Poll"
	case m.BalloonBundleID != "" && !strings.Contains(m.BalloonBundleID, "URLBalloonProvider"):
		return "🧩 " + appName(m.BalloonBundleID)
	case len(m.Attachments) > 0 && !strings.Contains(m.BalloonBundleID, "URLBalloonProvider"):
		label := attachmentLabel(m.Attachments[0])
		if extra := len(m.Attachments) - 1; extra > 0 {
			label += fmt.Sprintf(" (+%d)", extra)
		}
		return label
	default:
		return ""
	}
}

// appName names the iMessage apps seen in my Messages; others are just "App message".
func appName(bundleID string) string {
	for fragment, name := range map[string]string{
		"PeerPayment":  "Apple Cash",
		"gamepigeon":   "GamePigeon",
		"Handwriting":  "Handwriting",
		"DigitalTouch": "Digital Touch",
	} {
		if strings.Contains(bundleID, fragment) {
			return name
		}
	}
	return "App message"
}

func attachmentLabel(a imsgAttachment) string {
	switch {
	case a.IsSticker:
		return "Sticker"
	case strings.Contains(a.UTI, "image") || a.UTI == "public.heic" || a.UTI == "public.jpeg" || a.UTI == "public.png":
		return "📷 Photo"
	case strings.Contains(a.UTI, "movie") || strings.Contains(a.UTI, "video") || a.UTI == "public.mpeg-4":
		return "🎬 Video"
	case strings.Contains(a.UTI, "audio") || a.UTI == "com.apple.coreaudio-format":
		return "🎤 Audio"
	default:
		return "📎 " + a.TransferName
	}
}

// Messages returns a Conversation's newest limit Messages, oldest first.
func (e *Engine) Messages(ctx context.Context, conversationID int64, limit int) ([]Message, error) {
	var result struct{ Messages []imsgMessage }
	// attachments: true adds attachment metadata, so attachment-only Messages get a label.
	params := map[string]any{"chat_id": conversationID, "limit": limit, "attachments": true}
	if err := e.call(ctx, "messages.history", params, &result); err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(result.Messages))
	for _, m := range slices.Backward(result.Messages) { // imsg returns newest first
		if msg := m.toMessage(); msg.Text != "" || msg.Placeholder != "" || len(msg.Reactions) > 0 {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

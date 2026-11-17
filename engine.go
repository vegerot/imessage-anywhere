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
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	stdin io.WriteCloser
	done  chan struct{} // closed when the child exits

	mu            sync.Mutex
	nextID        int
	pending       map[string]pendingCall
	subscriptions map[int]chan Message
}

type pendingCall struct {
	response    chan rpcResponse
	isSubscribe bool // a watch.subscribe call: register its channel before reading on
}

type rpcResponse struct {
	Result       json.RawMessage
	Error        *rpcError
	Subscription *Subscription
}

// Subscription delivers new Messages until Unsubscribe, or until imsg drops it (overflow,
// exit); then Messages is closed and the browser reconnects from its cursor.
type Subscription struct {
	ID       int
	Messages <-chan Message
}

// subscriptionBuffer bounds how far one slow browser may fall behind before its stream is
// ended; it then reconnects from its cursor, so nothing is lost.
const subscriptionBuffer = 256

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func (e *rpcError) disposition() string {
	data, _ := e.Data.(map[string]any)
	d, _ := data["disposition"].(string)
	return d
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
	e := &Engine{stdin: stdin, done: make(chan struct{}), pending: map[string]pendingCall{}, subscriptions: map[int]chan Message{}}
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
	defer e.closeSubscriptions()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20) // one Conversation list can be large
	for scanner.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Subscription int         `json:"subscription"`
				Message      imsgMessage `json:"message"`
			} `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			fmt.Fprintf(os.Stderr, "engine: unparsable line from imsg: %v\n", err)
			continue
		}
		switch msg.Method {
		case "message":
			if m := msg.Params.Message.toMessage(); m.isVisible() {
				e.deliver(msg.Params.Subscription, m)
			}
			continue
		case "watch.overflow", "error": // both end that subscription; the browser reconnects
			e.dropSubscription(msg.Params.Subscription)
			continue
		}
		id, _ := strconv.Unquote(string(msg.ID))
		e.mu.Lock()
		call, ok := e.pending[id]
		delete(e.pending, id)
		resp := rpcResponse{Result: msg.Result, Error: msg.Error}
		if ok && call.isSubscribe && msg.Error == nil {
			var result struct{ Subscription int }
			if json.Unmarshal(msg.Result, &result) == nil {
				ch := make(chan Message, subscriptionBuffer)
				e.subscriptions[result.Subscription] = ch
				resp.Subscription = &Subscription{ID: result.Subscription, Messages: ch}
			}
		}
		e.mu.Unlock()
		if ok {
			call.response <- resp
		}
	}
}

func (e *Engine) deliver(subscription int, m Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch, ok := e.subscriptions[subscription]
	if !ok {
		return // already unsubscribed
	}
	select {
	case ch <- m:
	default: // that browser fell far behind: end its stream; it resumes from its cursor
		delete(e.subscriptions, subscription)
		close(ch)
	}
}

func (e *Engine) dropSubscription(subscription int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ch, ok := e.subscriptions[subscription]; ok {
		delete(e.subscriptions, subscription)
		close(ch)
	}
}

func (e *Engine) closeSubscriptions() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, ch := range e.subscriptions {
		delete(e.subscriptions, id)
		close(ch)
	}
}

// call sends one JSON-RPC request and decodes its result into out.
func (e *Engine) call(ctx context.Context, method string, params any, out any) error {
	resp, err := e.request(ctx, method, params, false)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(resp.Result, out)
}

func (e *Engine) request(ctx context.Context, method string, params any, isSubscribe bool) (rpcResponse, error) {
	call := pendingCall{response: make(chan rpcResponse, 1), isSubscribe: isSubscribe}
	e.mu.Lock()
	e.nextID++
	id := strconv.Itoa(e.nextID)
	e.pending[id] = call
	line, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err == nil {
		_, err = e.stdin.Write(append(line, '\n'))
	}
	e.mu.Unlock()
	if err != nil {
		return rpcResponse{}, err
	}
	select {
	case resp := <-call.response:
		if resp.Error != nil {
			return rpcResponse{}, resp.Error
		}
		return resp, nil
	case <-ctx.Done():
		e.forget(id)
		return rpcResponse{}, ctx.Err()
	case <-e.done:
		return rpcResponse{}, errEngineGone
	}
}

// Subscribe streams new Messages from all Conversations, starting after row sinceRowID,
// or at the current tail when sinceRowID is 0.
func (e *Engine) Subscribe(ctx context.Context, sinceRowID int64) (*Subscription, error) {
	params := map[string]any{"attachments": true, "include_reactions": true}
	if sinceRowID > 0 {
		params["since_rowid"] = sinceRowID
	}
	// Uncancellable: if the browser left mid-call, the caller still gets the subscription and
	// unsubscribes it; a cancelled call would leak it inside imsg (which allows only 64).
	resp, err := e.request(context.WithoutCancel(ctx), "watch.subscribe", params, true)
	if err != nil {
		return nil, err
	}
	if resp.Subscription == nil {
		return nil, fmt.Errorf("imsg: watch.subscribe returned no subscription: %s", resp.Result)
	}
	return resp.Subscription, nil
}

// Unsubscribe ends a Subscription; its Messages channel is closed.
func (e *Engine) Unsubscribe(s *Subscription) {
	e.dropSubscription(s.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.call(ctx, "watch.unsubscribe", map[string]any{"subscription": s.ID}, nil)
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
	LastMessageAt string   `json:"last_message_at"`
}

// Conversations lists the newest Conversations first, like the Messages.app sidebar.
func (e *Engine) Conversations(ctx context.Context, limit int) ([]Conversation, error) {
	var result struct{ Chats []imsgChat }
	if err := e.call(ctx, "chats.list", map[string]any{"limit": limit}, &result); err != nil {
		return nil, err
	}
	// imsg names a group's participants by Handle only. My one-to-one Conversations in the
	// same list carry Handle and Contact name together, so they name most participants
	// (13 of 14 on 2026-11-16); the rest stay numbers.
	names := map[string]string{}
	for _, c := range result.Chats {
		if !c.IsGroup && c.ContactName != "" {
			names[c.Identifier] = c.ContactName
		}
	}
	conversations := make([]Conversation, 0, len(result.Chats))
	for _, c := range result.Chats {
		conversations = append(conversations, Conversation{
			ID:            c.ID,
			Name:          c.displayName(names),
			Service:       c.Service,
			IsGroup:       c.IsGroup,
			LastMessageAt: c.LastMessageAt,
		})
	}
	return conversations, nil
}

// displayName follows Messages.app: the group's or Conversation's own name, else the
// contact's name, else (for a group) its participants, else the raw handle.
func (c imsgChat) displayName(names map[string]string) string {
	switch {
	case c.DisplayName != "":
		return c.DisplayName
	case c.ContactName != "":
		return c.ContactName
	case c.IsGroup && len(c.Participants) > 0:
		participants := make([]string, len(c.Participants))
		for i, p := range c.Participants {
			participants[i] = nameOr(names[p], p)
		}
		return strings.Join(participants, ", ")
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
	ID              int64  `json:"id"`
	GUID            string `json:"guid"`
	ConversationID  int64  `json:"conversationId"`
	IsFromMe        bool   `json:"isFromMe"`
	Sender          string `json:"sender"`
	SenderName      string `json:"senderName,omitempty"`      // Contact name or formatted Handle; empty for my own
	IsUnknownSender bool   `json:"isUnknownSender,omitempty"` // received from a Handle that isn't in my contacts
	Text            string `json:"text"`
	CreatedAt       string `json:"createdAt"`
	IsEdited        bool   `json:"isEdited,omitempty"`
	// IsUnread: received and not yet read on any of my Apple devices, per the Host Mac's database.
	IsUnread  bool       `json:"isUnread,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
	// Placeholder names what the page can't show yet (attachments, polls, app messages,
	// unsent Messages), so nothing vanishes silently.
	Placeholder string       `json:"placeholder,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	// A tapback arriving live is its own row pointing at the Message it reacts to.
	ReactsTo         string `json:"reactsTo,omitempty"`
	Tapback          string `json:"tapback,omitempty"`
	IsTapbackRemoved bool   `json:"isTapbackRemoved,omitempty"`
}

// Attachment is a file sent with a Message. IsMissing means it isn't on the Host Mac's disk:
// Messages.app downloads attachments only when asked (experiment, 2026-11-12).
type Attachment struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // image, video, audio, sticker, or file
	IsMissing bool   `json:"isMissing,omitempty"`
}

// visible says whether a Message has anything to show; whitespace-only rows (seen in the
// wild) are left out of history and the live stream alike.
func (m Message) isVisible() bool {
	return m.Text != "" || m.Placeholder != "" || len(m.Reactions) > 0 || m.ReactsTo != "" || len(m.Attachments) > 0
}

// Reaction is a tapback on a Message.
type Reaction struct {
	Emoji    string `json:"emoji"`
	From     string `json:"from"`
	IsFromMe bool   `json:"isFromMe,omitempty"`
}

type imsgMessage struct {
	ID              int64  `json:"id"`
	GUID            string `json:"guid"`
	ChatID          int64  `json:"chat_id"`
	IsFromMe        bool   `json:"is_from_me"`
	IsRead          *bool  `json:"is_read"` // received Messages only
	Sender          string `json:"sender"`
	SenderName      string `json:"sender_name"`
	Text            string `json:"text"`
	CreatedAt       string `json:"created_at"`
	EditedAt        string `json:"edited_at"`
	UnsentAt        string `json:"unsent_at"`
	BalloonBundleID string `json:"balloon_bundle_id"`
	IsReaction      bool   `json:"is_reaction"`
	ReactionEmoji   string `json:"reaction_emoji"`
	IsReactionAdd   bool   `json:"is_reaction_add"`
	ReactedToGUID   string `json:"reacted_to_guid"`
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
	OriginalPath string `json:"original_path"`
	TransferName string `json:"transfer_name"`
	UTI          string `json:"uti"`
	IsSticker    bool   `json:"is_sticker"`
	IsMissing    bool   `json:"missing"`
}

// objectReplacement marks where an attachment or app balloon sits inside a Message's text.
const objectReplacement = "\ufffc"

func (m imsgMessage) toMessage() Message {
	msg := Message{
		ID:             m.ID,
		GUID:           m.GUID,
		ConversationID: m.ChatID,
		IsFromMe:       m.IsFromMe,
		Sender:         m.Sender,
		Text:           strings.TrimSpace(strings.ReplaceAll(m.Text, objectReplacement, "")),
		CreatedAt:      m.CreatedAt,
		IsEdited:       m.EditedAt != "",
		Placeholder:    m.placeholder(),
	}
	if !m.IsFromMe {
		msg.SenderName = nameOr(m.SenderName, m.Sender)
		msg.IsUnknownSender = m.SenderName == ""
		msg.IsUnread = m.IsRead != nil && !*m.IsRead
	}
	if !m.isLinkPreview() {
		for _, a := range m.Attachments {
			msg.Attachments = append(msg.Attachments, Attachment{Name: a.TransferName, Kind: a.kind(), IsMissing: a.IsMissing})
		}
	}
	if m.IsReaction {
		msg.Text, msg.Placeholder = "", "" // its text is a summary like "Loved an image"
		msg.ReactsTo, msg.Tapback, msg.IsTapbackRemoved = m.ReactedToGUID, m.ReactionEmoji, !m.IsReactionAdd
		return msg
	}
	for _, r := range m.Reactions {
		msg.Reactions = append(msg.Reactions, Reaction{Emoji: r.Emoji, From: nameOr(r.SenderName, r.Sender), IsFromMe: r.IsFromMe})
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
	case m.BalloonBundleID != "" && !m.isLinkPreview():
		return "🧩 " + appName(m.BalloonBundleID)
	case len(m.Attachments) > 0 && !m.isLinkPreview():
		label := attachmentLabel(m.Attachments[0])
		if extra := len(m.Attachments) - 1; extra > 0 {
			label += fmt.Sprintf(" (+%d)", extra)
		}
		return label
	default:
		return ""
	}
}

// isLinkPreview: a link's preview card; its attachment is preview data, not a file to show.
func (m imsgMessage) isLinkPreview() bool {
	return strings.Contains(m.BalloonBundleID, "URLBalloonProvider")
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

func (a imsgAttachment) kind() string {
	switch {
	case a.IsSticker:
		return "sticker"
	case strings.Contains(a.UTI, "image") || a.UTI == "public.heic" || a.UTI == "public.jpeg" || a.UTI == "public.png":
		return "image"
	case strings.Contains(a.UTI, "movie") || strings.Contains(a.UTI, "video") || a.UTI == "public.mpeg-4":
		return "video"
	case strings.Contains(a.UTI, "audio") || a.UTI == "com.apple.coreaudio-format":
		return "audio"
	default:
		return "file"
	}
}

func attachmentLabel(a imsgAttachment) string {
	switch a.kind() {
	case "sticker":
		return "Sticker"
	case "image":
		return "📷 Photo"
	case "video":
		return "🎬 Video"
	case "audio":
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
		if msg := m.toMessage(); msg.isVisible() {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// SendResult says whether a sent Message is confirmed in the Messages database.
type SendResult struct {
	Status string `json:"status"` // "confirmed" or "unconfirmed"
	ID     int64  `json:"id,omitempty"`
}

// errNotSent means the Message certainly didn't go out, so sending again is safe.
var errNotSent = errors.New("not sent")

// imsg's "delivery outcome unknown" (may_have_completed, still_in_flight) and "mutation lane
// blocked" errors: the Message may have gone out.
const (
	imsgOutcomeUnknown = -32001
	imsgLaneBlocked    = -32004
)

// Send sends text into a Conversation through AppleScript (never imsg's SIP-off bridge).
// imsg confirms a send only after its row appears in the database, which catches the
// macOS 26+ "ghost row" bug. An unconfirmed send is reported, never retried: it may have
// been delivered.
func (e *Engine) Send(ctx context.Context, conversationID int64, text string) (SendResult, error) {
	var result struct{ ID int64 }
	params := map[string]any{"chat_id": conversationID, "text": text, "transport": "applescript"}
	// Once written to imsg, a send can't be taken back; a closed browser mustn't abandon it.
	err := e.call(context.WithoutCancel(ctx), "send", params, &result)
	var rpcErr *rpcError
	switch {
	case err == nil && result.ID != 0:
		return SendResult{Status: "confirmed", ID: result.ID}, nil
	case err == nil: // ok without a row: imsg couldn't read the database to verify
		return SendResult{Status: "unconfirmed"}, nil
	case errors.Is(err, errEngineGone):
		return SendResult{Status: "unconfirmed"}, nil
	case errors.As(err, &rpcErr) && rpcErr.Code == imsgLaneBlocked:
		// An earlier send is still in flight, so imsg refuses sends until its child restarts.
		// This one never ran. Exiting lets launchd restart everything.
		fmt.Fprintf(os.Stderr, "engine: imsg's send queue is blocked; restarting: %v\n", rpcErr)
		go e.Close()
		return SendResult{}, fmt.Errorf("%w: %v", errNotSent, rpcErr)
	case errors.As(err, &rpcErr) && rpcErr.Code == imsgOutcomeUnknown:
		fmt.Fprintf(os.Stderr, "engine: send to %d unconfirmed: %v\n", conversationID, rpcErr)
		if rpcErr.disposition() == "still_in_flight" {
			go e.Close() // imsg now blocks further sends until its child restarts
		}
		return SendResult{Status: "unconfirmed"}, nil
	default:
		return SendResult{}, fmt.Errorf("%w: %v", errNotSent, err)
	}
}

// Ready checks that imsg can read the Messages database. Without Full Disk Access it
// can't, and the error says which program needs the grant: the Proxy itself when launchd
// runs it, or the terminal app when it runs in a terminal.
func (e *Engine) Ready(ctx context.Context) error {
	var status struct {
		Version  string
		Database struct {
			Ready bool
			Error string
		}
	}
	if err := e.call(ctx, "status", map[string]any{}, &status); err != nil {
		return err
	}
	if status.Database.Ready {
		return nil
	}
	program, _ := os.Executable()
	return fmt.Errorf("imsg %s can't read the Messages database (%s). Grant Full Disk Access to %s "+
		"(or to the terminal app running it) in System Settings → Privacy & Security → Full Disk Access, then restart it",
		status.Version, status.Database.Error, program)
}

// AttachmentFile is an attachment's file on the Host Mac's disk.
type AttachmentFile struct {
	Path string
	UTI  string
	Name string
}

// errNoAttachment: imsg reports no such attachment for that Message, or it isn't on disk.
var errNoAttachment = errors.New("no such attachment on the Host Mac")

// AttachmentFile finds the file of a Message's index-th attachment, as imsg reports it. The
// browser names only (Conversation, Message, index), never a path.
func (e *Engine) AttachmentFile(ctx context.Context, conversationID, messageID int64, index int) (AttachmentFile, error) {
	var result struct{ Messages []imsgMessage }
	params := map[string]any{"since_rowid": messageID - 1, "chat_id": conversationID, "limit": 1, "attachments": true}
	if err := e.call(ctx, "messages.after", params, &result); err != nil {
		return AttachmentFile{}, err
	}
	if len(result.Messages) == 0 || result.Messages[0].ID != messageID || index < 0 || index >= len(result.Messages[0].Attachments) {
		return AttachmentFile{}, errNoAttachment
	}
	a := result.Messages[0].Attachments[index]
	if a.IsMissing {
		return AttachmentFile{}, errNoAttachment
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return AttachmentFile{}, err
	}
	path := a.OriginalPath // absolute, per imsg's docs; expand "~/" too, to be safe
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		path = filepath.Join(home, rest)
	}
	// imsg only reports paths in Messages' attachment store; anything else would be a bug.
	root := filepath.Join(home, "Library", "Messages", "Attachments")
	if rel, err := filepath.Rel(root, path); err != nil || strings.HasPrefix(rel, "..") {
		return AttachmentFile{}, fmt.Errorf("imsg reported an attachment outside %s: %s", root, path)
	}
	return AttachmentFile{Path: path, UTI: a.UTI, Name: a.TransferName}, nil
}

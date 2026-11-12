# Plan: iMessage Anywhere

## Goal

Let me use iMessage anywhere on my LAN or Tailnet. A web UI lets me send and receive iMessages from any machine there (GNU+Linux most often, Windows less often), as long as my Mac is online.

## Constraints and decisions

Vocabulary is in [GLOSSARY.md](GLOSSARY.md).

- 🍎 The Host Mac is the work Mac (`work-macbook`). The Proxy works only while it is online. Clients only need a browser.
- 🧪 Style experiments go rung by rung, covering only the UI that rung adds. Each saves screenshots and measured values (sizes, `oklch()` colors, fonts) to `style-notes/`. Test Messages sent to myself say "🧪 imessage-anywhere test".
- 🌐 Reach: tailnet only. The Host Mac sits on office Wi-Fi, so its LAN is the corporate network; the Proxy must never listen there. It reaches the tailnet through the Pi stand-in node ([`vpn/work-macbook-stand-in/`](../vpn/work-macbook-stand-in/README.md)).
- 🔓 SIP stays on, always. Start with public surfaces (reading the Messages database, AppleScript sending). Reverse engineering and private APIs are fair game later, as long as they work with SIP on, for features public surfaces lack (tapbacks, replies, typing, read receipts, edits). Each one is its own experiment after the basics work.
- 💬 Every Service (iMessage, SMS, RCS) is handled the same way; the UI shows which one.
- 📱 Clients: desktop browsers on GNU+Linux and Windows, and Chrome on my Android dev phone. Single user.
- 🔑 Only my tailnet devices may connect. The Proxy listens only on `172.16.1.2` (the WireGuard tunnel) and accepts only callers from `100.64.0.0/10`; tailnet membership is the login. No passwords or tokens. Add a login only if a device I don't control ever joins the tailnet.
- 🔐 Plain HTTP for now: WireGuard and Tailscale already encrypt the whole path. HTTPS waits until notifications or an installable Android web app need a secure context.
- 🛠️ Build our own Proxy and web page. Not BlueBubbles: its server is Electron. No Electron anywhere. See [ADR 0001](docs/adr/0001-build-own-proxy-not-bluebubbles.md).
- 🧰 The Mac-side engine is [`imsg`](https://github.com/openclaw/imsg) (MIT, source at `~/code/github.com/openclaw/imsg`), driven through one long-lived `imsg rpc` child (JSON-RPC over stdio) that the Proxy restarts if it exits. Every send passes `transport:"applescript"`, because the default `auto` would pick imsg's SIP-off bridge. imsg's own send check catches the "ghost row" bug.
- 🍺 Install imsg from its signed, notarized Homebrew release (`brew install steipete/tap/imsg`), not from source. The Proxy checks imsg's version at startup and refuses to run on an unexpected one, because imsg's JSON has no schema version.
- 🦫 The Proxy is written in Go: `net/http` and `embed` from the standard library serve the page.
- 📡 Live updates use Server-Sent Events (`EventSource`). Each event's ID is the Message's database row ID, so a dropped connection resumes from `Last-Event-ID` via imsg's cursor. Sending is a plain `<form>` POST.
- 🚀 Run the Proxy by hand from the terminal during P1–P6, then add a launchd agent before the week of daily use.
- 📄 The web page is one static HTML file with plain JavaScript and CSS: no framework, no build step. Reach for a framework only if the page starts to hurt.
- 🎨 Look and feel follows Apple's Messages.app: macOS at desktop widths, iOS at phone widths. Learn the style by experiment: computer use on Messages.app (resizing its window as needed) and on iPhone Mirroring, sending Messages to myself. Get closer incrementally, and use idiomatic platform HTML/CSS/JS (the `web-coding-style` skill): when a plain platform feature gets ~90% of Apple's look, take it and skip the custom last 10%.
- 🪜 Grow in small prototypes, simplest test first, toward v1.0.
- 📁 Scoped to `ai-conversations/imessage-anywhere/` as its own project root (2026-11-11).
- 🏷️ The project is named **iMessage Anywhere** (folder `imessage-anywhere/`, 2026-11-12), replacing "iMessage proxy". "Proxy" stays as the glossary term for the program on the Host Mac.
- 🗂️ Work is tracked here and in `progress.md`, not GitHub Issues: the GitHub repo is the shared conversation archive.

## v1.0 scope

- List Conversations and read their history.
- See new Messages live, without reloading.
- Send text into an existing Conversation (1:1 or group).
- View received images and attachments.
- Show contact names instead of raw numbers and emails.

Next after v1.0: start a new 1:1 Conversation; send attachments.

## Prototype ladder

Each rung is one commit plus one `progress.md` entry.

- **P0** (by hand, no code): install imsg from Homebrew and add an `imsg` wrapper to `~/.aliases` (in dotfiles) pointing agents at its source (`findrepo imsg`), like the `tailscale` one. Grant Full Disk Access, then list Conversations and send a Message to myself from the terminal. Biggest risk: corporate device management may block Full Disk Access.
- **P1**: the Proxy lists Conversations in a browser on `maxs-pc` over the tailnet (read-only). Also check the undocumented Full Disk Access risks: does the grant survive a `go build`, and is it attributed correctly under launchd?
- **P2**: open a Conversation and read its history.
- **P3**: see incoming Messages live.
- **P4**: send text, and confirm it actually landed in the Messages database (the macOS 26+ "ghost row" bug).
- **P5**: images and attachments.
- **P6**: contact names. That completes v1.0.

## Done when

v1.0 works through the tailnet from `maxs-pc` (Debian) and the Android phone, and I've used it daily for a week without falling back to the Mac or iPhone for Messages.

## Spec: iMessage Anywhere v1.0

Goal, constraints, scope, and the prototype ladder are above; this section adds only what they don't say.

### Problem Statement

My Messages live on my iPhone and the Host Mac. Most of my day is on GNU+Linux (and sometimes Windows or my Android dev phone), where I can't read or answer them without reaching for another device. Every Message I want to answer costs a context switch.

### Solution

A Proxy on the Host Mac serves one web page to any Client on my tailnet. In it I see my Conversations the way Messages.app shows them, read their history, watch new Messages arrive, send text, and view attachments, with contact names instead of raw numbers. On desktop it looks and behaves like Messages.app on macOS; at phone width, like Messages on iOS. It works while the Host Mac is online, and needs nothing installed on the Client but a browser.

### User Stories

**Reaching it**

1. As Max, I want to open the Proxy by typing one address into any browser on my tailnet, so that I need no app on the Client.
2. As Max, I want the same address to work from `maxs-pc` on Debian and Windows and from my Android phone, so that I don't remember per-device setups.
3. As Max, I want devices outside my tailnet to be refused, so that nobody on the office network or the internet can read my Messages.
4. As Max, I want the Proxy never to listen on the office network at all, so that a misconfigured filter can't expose it there.
5. As Max, I want no login screen, so that opening the page is instant; being on my tailnet is the login.
6. As Max, I want a clear "Host Mac is offline" state instead of a blank or hanging page, so that I know why nothing loads.
7. As Max, I want the page to recover on its own when the Host Mac comes back, so that I don't have to reload by hand.

**Conversations**

8. As Max, I want a list of my Conversations, newest activity first, so that I see what needs attention, like the Messages.app sidebar.
9. As Max, I want each Conversation in the list to show its name, the last Message's preview, and when it arrived, so that I can triage without opening it.
10. As Max, I want group Conversations to show their group name, or their participants when unnamed, so that I can tell groups apart.
11. As Max, I want each Conversation to show its Service (iMessage, SMS, RCS) as a small badge, so that I know how a reply will travel.
12. As Max, I want unread Conversations marked, so that I see what I haven't read yet.
13. As Max, I want the list to reorder when a new Message arrives, so that the newest activity stays on top without a reload.

**Reading**

14. As Max, I want to open a Conversation and see its recent history, so that I have the context to reply.
15. As Max, I want to scroll back for older Messages, loaded as I go, so that long Conversations open fast.
16. As Max, I want my Messages and other people's on opposite sides in bubbles, coloured like Messages.app (blue iMessage, green SMS/RCS), so that the page reads the way I'm used to.
17. As Max, I want each Message in a group Conversation labelled with its sender, so that I know who said what.
18. As Max, I want timestamps grouped the way Messages.app groups them, so that I can tell when things were said without clutter.
19. As Max, I want links in Messages to be clickable, so that I can open them on the Client.
20. As Max, I want emoji and non-Latin text to render correctly, so that nothing comes through garbled.
21. As Max, I want a placeholder for Message types the Proxy can't show yet (tapbacks, stickers, polls, edits), so that I know something is there instead of silently missing it.

**Live**

22. As Max, I want new incoming Messages to appear in the open Conversation within a few seconds, so that I can have a real-time exchange.
23. As Max, I want Messages I send from my iPhone or the Host Mac to also appear, so that the page always matches Messages.app.
24. As Max, I want nothing lost or duplicated when my Client's connection drops and comes back, so that I can trust the page after sleep or a network change.
25. As Max, I want several tabs or Clients open at once, all updating, so that I can leave the page open on every machine.

**Sending**

26. As Max, I want a text box at the bottom of an open Conversation, so that I can reply in place.
27. As Max, I want Enter to send and Shift+Enter to insert a new line on desktop, so that typing feels like Messages.app.
28. As Max, I want my sent Message to show as sending, then sent once it's confirmed in the Messages database, so that I know it really went.
29. As Max, I want a clear warning when a send can't be confirmed (the "ghost row" case), with no automatic retry, so that I neither lose a Message nor send it twice.
30. As Max, I want to send into group Conversations as well as 1:1, so that I can answer anything I can read.
31. As Max, I want my reply to go out over the Conversation's own Service, so that the Proxy never changes how the other person receives it.

**Attachments**

32. As Max, I want received images shown inline in the Conversation, so that I see photos without downloading them.
33. As Max, I want iPhone photos (HEIC) to display in Chrome, so that most received images aren't broken.
34. As Max, I want other attachments (video, audio, PDFs, files) shown as a named item I can open or download, so that I can still get at them.
35. As Max, I want attachments that live only in iCloud and aren't on the Host Mac to say so, so that a missing file isn't mistaken for a bug.

**Names**

36. As Max, I want contact names from the Host Mac's Contacts instead of phone numbers and emails, so that I recognise people.
37. As Max, I want the raw number or email shown when there's no contact, so that unknown senders are still identifiable.

**Look and feel**

38. As Max, I want the desktop layout to mirror Messages.app on macOS (sidebar plus Conversation pane), so that it feels familiar.
39. As Max, I want the phone layout to mirror Messages on iOS (list, then a full-screen Conversation with a back button), so that it fits my Android phone's screen.
40. As Max, I want the page to follow my system's light or dark mode, so that it matches the rest of my desktop.
41. As Max, I want it to stay usable with the keyboard and with larger text sizes, so that it's comfortable on every machine.

**Running it**

42. As Max, I want the Proxy to refuse to start against an unexpected imsg version, so that a silent JSON change can't corrupt what I see.
43. As Max, I want the Proxy to restart imsg by itself if imsg exits, so that a crash doesn't leave the page dead until I notice.
44. As Max, I want the Proxy to start on login and keep running via launchd, so that I don't have to babysit a terminal once v1.0 is done.
45. As Max, I want clear startup errors when Full Disk Access or Automation is missing, so that I can fix permissions without digging through logs.

### Implementation Decisions

- **Modules.** Three, each with one job:
  - *Engine client*: owns the single long-lived imsg child process, speaks its JSON-RPC, checks its version at start, restarts it when it exits, and fans its Message notifications out to subscribers. It is the only code that knows imsg exists.
  - *HTTP layer*: the routes below, the source-address filter, and SSE. It talks only to the Engine client.
  - *The page*: one static HTML file, embedded in the Proxy binary.
- **Engine-client interface** (in domain terms): list Conversations; Conversation history before a cursor; Messages after a database row ID; subscribe to new Messages from a row ID; send text to a Conversation; contact name for a handle. Sends always request imsg's AppleScript transport.
- **HTTP contract** (JSON unless noted):
  - the page itself;
  - the Conversation list;
  - one Conversation's history, paged backwards by cursor;
  - an SSE stream of new Messages, where each event's ID is the Message's database row ID, so the browser's own `Last-Event-ID` resumes it after a drop;
  - send text to a Conversation (a plain form POST), returning `confirmed` or `unconfirmed` (imsg's `may_have_completed`);
  - an attachment by ID, served by the Proxy from the Host Mac's disk, with HEIC converted to a format Chrome displays.
- **Identity.** A Conversation is addressed by imsg's chat ID inside the Proxy and the page. The GUID (`any;-;…` on macOS 26+) is used only for sending.
- **Access.** The listener binds only to the WireGuard tunnel address and rejects any source outside `100.64.0.0/10` before routing.
- **One Engine, many Clients.** All SSE subscribers share one imsg subscription; the Proxy keeps a short in-memory buffer of recent Messages to serve resumes, and falls back to imsg's "Messages after row ID" for longer gaps.
- **Offline behaviour.** Belongs to the page: the browser's built-in `EventSource` reconnect drives the offline/online state, with no custom retry loop.
- **Read state.** Viewing a Conversation in the page doesn't mark it read on the Host Mac (that needs imsg's SIP-off path).

### Testing Decisions

- **One seam: the whole Proxy, HTTP in and JSON-RPC out.** Tests call the real HTTP layer; the imsg child is replaced by a fake that speaks the same JSON-RPC over stdio. No interfaces exist just for testing beyond a configurable path to the imsg executable.
- **The fake replays recorded real transcripts.** P0 records real `imsg rpc` request/response pairs; personal content is replaced with synthetic Conversations before they're committed. A test fails if the Proxy sends a request the transcript doesn't cover.
- **A good test checks external behaviour only**: HTTP status, JSON, and SSE events a Client would see, never internal structure. Covered behaviour: listing and history paging; SSE event IDs and resume from `Last-Event-ID` without gaps or duplicates; multiple subscribers; send passes the AppleScript transport and reports unconfirmed sends honestly; foreign source addresses are refused; an unexpected imsg version is refused; the child is restarted after it exits; attachments are served and HEIC is converted.
- **Prior art**: Go's own `os/exec` tests use the helper-process pattern (the test binary re-runs itself as the fake child). The project has no code of its own yet.
- **Manual end-to-end check per rung**, recorded in `progress.md`: real imsg, real browsers on `maxs-pc` and the Android phone, a "🧪 imessage-anywhere test" Message, and a side-by-side style comparison saved under `style-notes/`.
- **No automated browser tests** for now: for one static page, they'd cost more than they catch. Revisit if the page grows past that.

### Out of Scope

- Starting a new Conversation and sending attachments (next after v1.0).
- Tapbacks, threaded replies, typing indicators, read receipts, edit and unsend, stickers, polls: later experiments with private APIs that work with SIP on.
- Marking Conversations read on the Host Mac.
- Browser notifications, an installable Android web app, and HTTPS.
- Search, Conversation management (delete, mute, pin, rename), and settings.
- Multiple users, logins, and access from outside my tailnet.
- Any Host Mac other than `work-macbook`; any Mac with SIP off.

### Further Notes

- The P0 transcripts double as the reference for what imsg 0.21.0 really returns; record them before writing the Engine client.
- The biggest unverified risks are Full Disk Access (corporate device management may block it; a grant may not survive a rebuild or hold under launchd) and HEIC display. P0 and P1 test them first.
- "Contact" isn't in `GLOSSARY.md` yet; add it when P6 settles what a contact name means for handles shared by several people.

## Work: iMessage Anywhere v1.0

Slices of the spec above, in dependency order. A slice is ready when every slice in its **Blocked by** line has a `✅ NN` entry in `progress.md`; the lowest-numbered ready slice goes first. Every slice also meets these shared criteria:

- Seam tests (the whole Proxy, HTTP in and JSON-RPC out, against a fake replaying recorded `imsg rpc` transcripts) cover the slice's behaviour; new transcripts are recorded on the Host Mac and sanitized before they're committed.
- A manual end-to-end check passes on `maxs-pc` and on the Android phone over the tailnet, using a "🧪 imessage-anywhere test" Message where sending is involved, and is recorded in `progress.md`.
- The UI the slice adds is compared side by side with Messages.app (macOS at desktop width, iOS at phone width) and the findings saved under `style-notes/`; it works at phone width and in light and dark mode.

### 01. Conversation list in a browser

**What to build:** From `maxs-pc`, opening the Host Mac's tailnet address in a browser shows my Conversations, newest activity first, each with its name, last-Message preview, time, Service badge, and unread mark. This is the walking skeleton: Go Proxy, Engine client, HTTP layer, embedded page, and the test seam.
**Blocked by:** None
**Acceptance criteria:**
- The Proxy listens only on the WireGuard tunnel address and refuses callers outside `100.64.0.0/10`; a seam test proves the refusal.
- The Engine client runs one long-lived `imsg rpc` child, refuses to start on an imsg version other than the pinned one, and restarts the child when it exits; seam tests prove both.
- The Conversation list matches what Messages.app shows for the same Conversations (spot-check of at least five, including an SMS one and a group).
- The replay fake fails a test when the Proxy sends a request its transcript doesn't cover.
- Rebuilding the Proxy with `go build` and running it again from the terminal still reads the Messages database (the Full Disk Access grant survives a rebuild), recorded in `progress.md`.

### 02. Read a Conversation

**What to build:** Choosing a Conversation shows its history the way Messages.app does: my Messages and others' in opposite bubbles coloured by Service, sender labels in groups, time grouping, clickable links, and a placeholder for each Message type the Proxy can't show yet. Older Messages load as I scroll back. At phone width the list and the Conversation are separate screens with a back button.
**Blocked by:** 01
**Acceptance criteria:**
- History pages backwards without gaps or repeats across page boundaries; a seam test proves it.
- A group Conversation labels each Message with its sender.
- Tapbacks, stickers, polls, and edited or unsent Messages show a placeholder, never vanish silently.
- Emoji and non-Latin text render correctly.

### 03. Live Messages

**What to build:** New Messages appear in the open Conversation within a few seconds and the Conversation list reorders, in every open tab and Client. When the Host Mac goes offline the page says so, and it recovers on its own when the Host Mac is back.
**Blocked by:** 02
**Acceptance criteria:**
- Live updates use Server-Sent Events whose event ID is the Message's database row ID; after a dropped connection the browser's `Last-Event-ID` resumes with no missed or duplicated Messages, proven by a seam test.
- Several simultaneous subscribers all receive each Message while the Proxy holds a single imsg subscription.
- After the imsg child restarts, its subscription resumes from the last delivered row ID.
- Messages sent from the iPhone or the Host Mac appear too.

### 04. Send text

**What to build:** A text box at the bottom of an open Conversation sends text into it, 1:1 or group, over that Conversation's own Service. The new bubble shows "sending", then "sent" once confirmed; a send that can't be confirmed shows a warning instead.
**Blocked by:** 03
**Acceptance criteria:**
- Every send asks imsg for the AppleScript transport; a seam test proves it.
- Confirmation comes from `send`'s own result, not from an immediate send-status check (which reports `pending` too early, per P0).
- An unconfirmed send (imsg's `may_have_completed`) shows a warning and is never retried automatically.
- The sent bubble and the live stream's copy of the same row, plus the received echo in a note-to-self Conversation, end up as one bubble each, never duplicates.
- On desktop, Enter sends and Shift+Enter inserts a new line.

### 05. Attachments

**What to build:** Received images show inline in the Conversation, including iPhone HEIC photos in Chrome. Other attachments show as a named item I can open or download. An attachment that lives only in iCloud says so.
**Blocked by:** 02
**Acceptance criteria:**
- The Proxy serves attachment files from the Host Mac only for attachments imsg reported, never an arbitrary path.
- File types come from `uti` and `transfer_name`, never `mime_type` (empty in history reads, per P0).
- A HEIC photo displays in Chrome on `maxs-pc` and on the Android phone.
- Attachments with the `missing` flag show an "in iCloud" placeholder.

### 06. Contact names

**What to build:** Contact names replace phone numbers and emails in the Conversation list, the Conversation header, and group sender labels; a handle with no contact still shows the raw number or email.
**Blocked by:** 02
**Acceptance criteria:**
- The permission path is decided and recorded in `plan.md`: macOS's Contacts permission, or reading the AddressBook database under Full Disk Access.
- **Contact** is defined in `GLOSSARY.md`.
- A seam test covers both a named handle and an unnamed one.

### 07. Always running

**What to build:** The Proxy starts at login and stays running without a terminal, and tells me clearly at startup when a permission it needs is missing.
**Blocked by:** 01
**Acceptance criteria:**
- A launchd agent starts the Proxy at login and restarts it if it exits.
- Started by launchd, the Proxy reads the Messages database and can send; which process macOS attributes Full Disk Access and Automation to is recorded in `progress.md`.
- With Full Disk Access or Automation missing, the Proxy logs which permission is missing and how to grant it.

### 08. v1.0 shakedown week

**What to build:** The "Done when" test: a week of daily use from `maxs-pc` and the Android phone without falling back to the Mac or iPhone for Messages.
**Blocked by:** 03, 04, 05, 06, 07
**Acceptance criteria:**
- A daily `progress.md` note for seven days, each naming any fallback to the Mac or iPhone and why.
- Each problem found becomes a new slice in this section.

# Plan: iMessage Anywhere

## Goal

Let me use iMessage anywhere on my LAN or Tailnet. A web UI lets me send and receive iMessages from any machine there (GNU+Linux most often, Windows less often), as long as my Mac is online.

## Constraints and decisions

Vocabulary is in [GLOSSARY.md](GLOSSARY.md).

- 🍎 The Host Mac is the work Mac (`work-macbook`). The Proxy works only while it is online. Clients only need a browser.
- 🌐 Reach: tailnet only. The Host Mac sits on office Wi-Fi, so its LAN is the corporate network; the Proxy must never listen there. It reaches the tailnet through the Pi stand-in node ([`vpn/work-macbook-stand-in/`](../vpn/work-macbook-stand-in/README.md)) over a WireGuard tunnel I start by hand.
- 🔑 Only my devices may connect. The Proxy listens only on `172.16.1.2` (the WireGuard tunnel) and `127.0.0.1` (loopback, for checks on the Host Mac itself), and accepts only callers from `100.64.0.0/10` or loopback; tailnet membership is the login. No passwords or tokens. Add a login only if a device I don't control ever joins the tailnet.
- 🔐 Plain HTTP for now: WireGuard and Tailscale already encrypt the whole path. HTTPS waits until notifications or an installable Android web app need a secure context.
- 🔓 SIP stays on, always. Start with public surfaces (reading the Messages database, AppleScript sending). Reverse engineering and private APIs are fair game later, as long as they work with SIP on, for features public surfaces lack (tapbacks, replies, typing, read receipts, edits). Each one is its own experiment after the basics work.
- 💬 Every Service (iMessage, SMS, RCS) is handled the same way; the UI shows which one.
- 📱 Clients: desktop browsers on GNU+Linux and Windows, and Chrome on my Android dev phone. Single user.
- 🛠️ Build our own Proxy and web page. Not BlueBubbles: its server is Electron. No Electron anywhere. See [ADR 0001](docs/adr/0001-build-own-proxy-not-bluebubbles.md).
- 🧰 The Mac-side engine is [`imsg`](https://github.com/openclaw/imsg) (MIT, source at `~/code/github.com/openclaw/imsg`), driven through one long-lived `imsg rpc` child (JSON-RPC over stdio). If the child exits, the Proxy exits too: launchd restarts the Proxy and browsers resume on their own. Every send targets the Conversation by `chat_id` and passes `transport:"applescript"`, because the default `auto` would pick imsg's SIP-off bridge. imsg's own send check catches the "ghost row" bug. The child runs with `SSH_CONNECTION` set, which makes imsg read contact names from the AddressBook database under Full Disk Access instead of asking for the Contacts permission (which a background process can't get a prompt for; experiment E3).
- 🍺 imsg comes from its signed, notarized Homebrew release (`brew install steipete/tap/imsg`) and is held at a known version with `brew pin imsg`, because its JSON has no schema version and it releases about weekly. Upgrade on purpose, then re-record a transcript.
- 🦫 The Proxy is written in Go, one `main` package: `net/http` and `embed` from the standard library serve the page.
- 🧭 Clean paths: a Conversation lives at `/conversations/<id>` (the Proxy serves the page there), switched in place with `pushState`/`popstate` and a same-document View Transition (iOS push/pop at phone width, a quick fade on desktop, none with reduced motion), focus moved to the Conversation's heading or back to its row. Replaced the `/#/` fragment routes (2026-11-12, Max leaned toward clean paths).
- 📡 Live updates use Server-Sent Events (`EventSource`). Each browser connection gets its own imsg `watch.subscribe` starting at its `Last-Event-ID`; each event's ID is the Message's database row ID, so a dropped connection resumes exactly where it stopped. Sending is a plain `<form>` POST.
- 🚀 Run the Proxy by hand from the terminal until ticket 07 adds a launchd agent.
- 📄 The web page is one static HTML file with hand-written CSS, and its code is TypeScript (`app.ts`): no framework, no bundler, no `package.json`. `go generate` runs a pinned `tsc` (TypeScript 7.0.2 via `npx`) that type-checks with `strict` plus every extra strictness option and emits `app.js` (`erasableSyntaxOnly`, so emitting only strips types; target `esnext`). `app.js` is committed so `go build` needs no JavaScript tools; the Proxy embeds and serves it at `/app.js` (2026-11-12, Max). Browser policy: every Client is a current Chrome or Firefox, so Baseline "newly available" features (e.g. `light-dark()`, CSS nesting, View Transitions) are used without fallbacks.
- 📎 Attachments in v1.0 (ticket 05, decided 2026-11-12 from experiments): the page shows attachments that are on the Host Mac's disk (images inline, HEIC converted to JPEG; video, audio, files) and labels the rest "not downloaded on the Mac". Messages.app on this Mac doesn't bring attachments down when a Conversation is viewed, and a manual ⬇︎ didn't either for the one (SMS) attachment tried, so making missing ones appear is out of v1.0.
- 🎨 Look and feel follows Apple's Messages.app: macOS at desktop widths, iOS at phone widths. Learn the style by experiment: computer use on Messages.app (resizing its window as needed) and on iPhone Mirroring, sending Messages to myself ("🧪 imessage-anywhere test"). Get closer incrementally, and use idiomatic platform HTML/CSS/JS (the `web-coding-style` skill): when a plain platform feature gets ~90% of Apple's look, take it and skip the custom last 10%. Measured values live in the CSS as custom properties with a comment naming their source; no separate style notes or screenshots.
- 🪜 Grow in small slices, simplest first, with an experiment before any slice that rests on an unverified assumption.
- 📁 Scoped to `ai-conversations/imessage-anywhere/` as its own project root (2026-11-11).
- 🏷️ The project is named **iMessage Anywhere** (folder `imessage-anywhere/`, 2026-11-12), replacing "iMessage proxy". "Proxy" stays as the glossary term for the program on the Host Mac.
- 🗂️ Work is tracked here and in `progress.md`, not GitHub Issues: the GitHub repo is the shared conversation archive.

## Done when

v1.0 works through the tailnet from `maxs-pc` (Debian) and the Android phone, and I've used it daily for a week without falling back to the Mac or iPhone for Messages.

## Spec: iMessage Anywhere v1.0

Goal and constraints are above; this section adds only what they don't say. Reviewed against the five-step engineering algorithm on 2026-11-12 (see `progress.md`).

### Problem Statement

My Messages live on my iPhone and the Host Mac. Most of my day is on GNU+Linux (and sometimes Windows or my Android dev phone), where I can't read or answer them without reaching for another device. Every Message I want to answer costs a context switch.

### Solution

A Proxy on the Host Mac serves one web page to any Client on my tailnet. In it I see my Conversations the way Messages.app shows them, read recent history, watch new Messages arrive, send text, and view attachments, with contact names instead of raw numbers. On desktop it looks and behaves like Messages.app on macOS; at phone width, like Messages on iOS. It works while the Host Mac is online, and needs nothing installed on the Client but a browser.

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

14. As Max, I want to open a Conversation and see its last 100 Messages, so that I have the context to reply. (Scrolling further back waits until the shakedown week shows I need it.)
15. As Max, I want my Messages and other people's on opposite sides in bubbles, coloured like Messages.app (blue iMessage, green SMS/RCS), so that the page reads the way I'm used to.
16. As Max, I want each Message in a group Conversation labelled with its sender, so that I know who said what.
17. As Max, I want timestamps grouped the way Messages.app groups them, so that I can tell when things were said without clutter.
18. As Max, I want links in Messages to be clickable, so that I can open them on the Client.
19. As Max, I want emoji and non-Latin text to render correctly, so that nothing comes through garbled.
20. As Max, I want a placeholder for Message types the Proxy can't show yet (tapbacks, stickers, polls, edits), so that I know something is there instead of silently missing it.

**Live**

21. As Max, I want new incoming Messages to appear in the open Conversation within a few seconds, so that I can have a real-time exchange.
22. As Max, I want Messages I send from my iPhone or the Host Mac to also appear, so that the page always matches Messages.app.
23. As Max, I want nothing lost or duplicated when my Client's connection drops and comes back, so that I can trust the page after sleep or a network change.
24. As Max, I want several tabs or Clients open at once, all updating, so that I can leave the page open on every machine.

**Sending**

25. As Max, I want a text box at the bottom of an open Conversation, so that I can reply in place.
26. As Max, I want Enter to send and Shift+Enter to insert a new line on desktop, so that typing feels like Messages.app.
27. As Max, I want my sent Message to show as sending, then sent once it's confirmed in the Messages database, so that I know it really went.
28. As Max, I want a clear warning when a send can't be confirmed (the "ghost row" case), with no automatic retry, so that I neither lose a Message nor send it twice.
29. As Max, I want to send into group Conversations as well as 1:1, so that I can answer anything I can read.
30. As Max, I want my reply to go out over the Conversation's own Service, so that the Proxy never changes how the other person receives it.

**Attachments** (ticket 05 starts with an experiment; see below)

31. As Max, I want received images shown inline in the Conversation, so that I see photos without downloading them.
32. As Max, I want iPhone photos (HEIC, about two thirds of received images) to display in Chrome, so that most received images aren't broken.
33. As Max, I want other attachments (video, audio, PDFs, files) shown as a named item I can open or download, so that I can still get at them.
34. As Max, I want attachments that aren't on the Host Mac to say so, so that a missing file isn't mistaken for a bug.

**Names**

35. As Max, I want contact names from the Host Mac's Contacts instead of phone numbers and emails, so that I recognise people.
36. As Max, I want the raw number or email shown when there's no contact, so that unknown senders are still identifiable.

**Look and feel**

37. As Max, I want the desktop layout to mirror Messages.app on macOS (sidebar plus Conversation pane), so that it feels familiar.
38. As Max, I want the phone layout to mirror Messages on iOS (list, then a full-screen Conversation with a back button), so that it fits my Android phone's screen.
39. As Max, I want the page to follow my system's light or dark mode, so that it matches the rest of my desktop.
40. As Max, I want it to stay usable with the keyboard and with larger text sizes, so that it's comfortable on every machine.

**Running it**

41. As Max, I want imsg upgraded only when I choose, so that a silent JSON change can't break the page behind my back.
42. As Max, I want a crash to heal by itself (the Proxy exits, launchd restarts it, browsers resume), so that the page doesn't stay dead until I notice.
43. As Max, I want the Proxy to start on login and keep running via launchd, retrying until the WireGuard tunnel is up, so that I don't have to babysit a terminal once v1.0 is done.
44. As Max, I want clear startup errors when Full Disk Access or Automation is missing, so that I can fix permissions without digging through logs.

### Implementation Decisions

- **Modules.** Three, each with one job:
  - *Engine client*: owns the single long-lived imsg child process and speaks its JSON-RPC: matches responses to requests by ID and routes each notification to its subscription. It holds no Message state. When the child exits, the Proxy exits. It is the only code that knows imsg exists.
  - *HTTP layer*: the routes below, the source-address filter, and SSE. It talks only to the Engine client.
  - *The page*: one static HTML file, embedded in the Proxy binary.
- **Engine-client interface** (in domain terms): list Conversations (with contact names when imsg has them); the last N Messages of a Conversation; subscribe to new Messages after a row ID, and unsubscribe; send text to a Conversation; startup status (imsg's `status`, whose database error explains missing permissions).
- **HTTP contract** (JSON unless noted):
  - the page itself;
  - the Conversation list;
  - one Conversation's last 100 Messages;
  - an SSE stream of new Messages: each connection subscribes from its `Last-Event-ID` (or the current tail) and unsubscribes when it closes; each event's ID is the Message's database row ID;
  - send text to a Conversation (a plain form POST), returning `confirmed` or `unconfirmed` (imsg's `may_have_completed`);
  - an attachment by ID (ticket 05, if its experiment says yes).
- **Identity.** A Conversation is addressed by imsg's chat ID everywhere, including sends (verified in P0).
- **Access.** The listener binds only to the WireGuard tunnel address and loopback, and rejects any source outside `100.64.0.0/10` and loopback before routing.
- **Offline behaviour.** Belongs to the page: the browser's built-in `EventSource` reconnect drives the offline/online state, with no custom retry loop.
- **Read state.** Viewing a Conversation in the page doesn't mark it read on the Host Mac (that needs imsg's SIP-off path).

### Testing Decisions

- **One seam: the whole Proxy, HTTP in and JSON-RPC out.** Tests call the real HTTP layer; the imsg child is replaced by a fake that speaks the same JSON-RPC over stdio. No interfaces exist just for testing beyond a configurable path to the imsg executable.
- **The fake replays recorded real transcripts.** Transcripts are recorded on the Host Mac and sanitized before they're committed, and re-recorded only when a ticket uses a new imsg method or response shape. A test fails if the Proxy sends a request the transcript doesn't cover.
- **Test only logic the Proxy owns**, through external behaviour (HTTP status, JSON, SSE events): the source-address filter (tailnet and loopback allowed, anything else refused); `Last-Event-ID` becoming the subscription's starting row; send results mapped to `confirmed` / `unconfirmed`; the Proxy exiting when the child exits. Pure pass-through gets no test of its own.
- **Prior art**: Go's own `os/exec` tests use the helper-process pattern (the test binary re-runs itself as the fake child). `testdata/imsg-0.21.0-p0.jsonl` is the first transcript.
- **Agent checks on the Host Mac, every ticket**, recorded in `progress.md`: real imsg, the page over loopback in Chrome at desktop and phone width, side by side with Messages.app, and a "🧪 imessage-anywhere test" Message where sending is involved.
- **Real-device checks at milestones only**: on `maxs-pc` and the Android phone over the tailnet after ticket 02, and throughout ticket 08.
- **No automated browser tests** for now: for one static page, they'd cost more than they catch. Revisit if the page grows past that.

### Out of Scope

- Starting a new Conversation and sending attachments (next after v1.0).
- Scrolling back past the last 100 Messages, until ticket 08 shows a need.
- Tapbacks, threaded replies, typing indicators, read receipts, edit and unsend, stickers, polls: later experiments with private APIs that work with SIP on.
- Marking Conversations read on the Host Mac.
- Browser notifications, an installable Android web app, and HTTPS.
- Search, Conversation management (delete, mute, pin, rename), and settings.
- Multiple users, logins, and access from outside my tailnet.
- Starting the WireGuard tunnel automatically (ticket 08 logs whether it's needed).
- Any Host Mac other than `work-macbook`; any Mac with SIP off.

### Further Notes

- Experiment E1 (2026-11-12): of 102 images received in the last 180 days, 69 are HEIC and only 2 are on the Host Mac's disk, even ones from yesterday. Why they're absent, and what brings them down, is unknown; ticket 05 finds out before building anything.
- Experiment E2 (2026-11-12): one `imsg rpc` child serves several `watch.subscribe` subscriptions with independent cursors, so the Proxy needs no fan-out or resume buffer.
- Experiment E3 (2026-11-12): imsg's Contacts permission request from a Rex terminal never prompted (status stays `notDetermined`), so `contact_name` was empty. With `SSH_CONNECTION` set, imsg reads the AddressBook database instead and names 7 of 20 recent Conversations (the rest: 2 groups, 6 SMS short codes, 5 unknown numbers). This relies on an imsg internal (`ContactResolver`), so re-check it when upgrading imsg.
- Full Disk Access under launchd and across `go build` is unverified and only matters there; ticket 07 tests it. Run from a Rex terminal, macOS checks Rex Server's grant, not the Proxy's.

## Work: iMessage Anywhere v1.0

Slices of the spec above, in dependency order. A slice is ready when every slice in its **Blocked by** line has a `✅ NN` entry in `progress.md`; the lowest-numbered ready slice goes first. Every slice also meets these shared criteria:

- Seam tests cover the Proxy-owned logic the slice adds (see Testing Decisions).
- An agent check on the Host Mac passes: the page over loopback in Chrome at desktop and phone width and in light and dark mode, side by side with Messages.app, recorded in `progress.md`.
- Any Messages.app style values the slice adopts are CSS custom properties with a comment naming their source.

### 01. Conversation list in a browser

**What to build:** Opening the Proxy in a browser shows my Conversations, newest activity first, each with its name (the contact name when there is one, else the raw number or email), last-Message preview, time, Service badge, and unread mark. This is the walking skeleton: Go Proxy, Engine client, HTTP layer, embedded page, and the test seam.
**Blocked by:** None
**Acceptance criteria:**
- Conversations with a contact show the contact name; others show the raw handle. **Contact** is defined in `GLOSSARY.md`.
- The Proxy listens only on the WireGuard tunnel address and loopback, and refuses callers outside `100.64.0.0/10` and loopback; a seam test proves the refusal.
- The Proxy exits when the imsg child exits; a seam test proves it.
- The replay fake fails a test when the Proxy sends a request its transcript doesn't cover.
- The Conversation list matches what Messages.app shows for the same Conversations (spot-check of at least five, including an SMS one and a group).
- `imsg` is pinned in Homebrew.

### 02. Read a Conversation

**What to build:** Choosing a Conversation shows its last 100 Messages the way Messages.app does: my Messages and others' in opposite bubbles coloured by Service, sender labels in groups, time grouping, clickable links, and a placeholder for each Message type the Proxy can't show yet. At phone width the list and the Conversation are separate screens with a back button.
**Blocked by:** 01
**Acceptance criteria:**
- A group Conversation labels each Message with its sender's contact name, or the raw handle when there's no contact.
- Tapbacks, stickers, polls, and edited or unsent Messages show a placeholder, never vanish silently.
- Emoji and non-Latin text render correctly.
- Milestone: the page works over the tailnet from `maxs-pc` and from the Android phone, recorded in `progress.md`.

### 03. Live Messages

**What to build:** New Messages appear in the open Conversation within a few seconds and the Conversation list reorders, in every open tab and Client. When the Host Mac goes offline the page says so, and it recovers on its own when the Host Mac is back.
**Blocked by:** 02
**Acceptance criteria:**
- Each SSE connection gets its own imsg subscription starting at its `Last-Event-ID`, and event IDs are database row IDs; a seam test proves the mapping.
- After a dropped connection, the page catches up with no missed or duplicated Messages.
- Two tabs open at once both update.
- Messages sent from the iPhone or the Host Mac appear too.

### 04. Send text

**What to build:** A text box at the bottom of an open Conversation sends text into it, 1:1 or group, over that Conversation's own Service. The new bubble shows "sending", then "sent" once confirmed; a send that can't be confirmed shows a warning instead.
**Blocked by:** 03
**Acceptance criteria:**
- Every send targets the Conversation's `chat_id` and asks imsg for the AppleScript transport; a seam test proves it.
- Confirmation comes from `send`'s own result, not from an immediate send-status check (which reports `pending` too early, per P0).
- An unconfirmed send (imsg's `may_have_completed`) shows a warning and is never retried automatically.
- The sent bubble and the live stream's copy of the same row, plus the received echo in a note-to-self Conversation, end up as one bubble each, never duplicates.
- On desktop, Enter sends and Shift+Enter inserts a new line.

### 05. Attachments: find out, then build

**What to build:** First an experiment: find what makes Messages put a received attachment on the Host Mac's disk (for example, viewing its Conversation in Messages.app, a Messages setting, or nothing reliable). Then, if a practical way exists, received images show inline (HEIC converted for Chrome), other files show as a named item to download, and attachments that aren't on disk say so. If none exists, record that and move attachments out of v1.0.
**Blocked by:** 02
**Acceptance criteria:**
- The experiment's result and evidence are recorded in `progress.md`, and `plan.md` says whether attachments stay in v1.0.
- If built: the Proxy serves files only for attachments imsg reported, never an arbitrary path; file types come from `uti` and `transfer_name`, never `mime_type` (empty in history reads, per P0); a HEIC photo displays in Chrome.

### 06. (Folded into 01 and 02)

Contact names come from imsg once its child runs with `SSH_CONNECTION` set (experiment E3), so the list shows them in 01 and group sender labels in 02. Number kept so later references stay stable.

### 07. Always running

**What to build:** The Proxy starts at login and stays running without a terminal, waits out a WireGuard tunnel that isn't up yet, and tells me clearly at startup when a permission it needs is missing.
**Blocked by:** 01
**Acceptance criteria:**
- A launchd agent starts the Proxy at login and restarts it whenever it exits, including while the tunnel address doesn't exist yet.
- Started by launchd, the Proxy reads the Messages database and can send, and still can after a `go build`; which process macOS attributes Full Disk Access and Automation to is recorded in `progress.md`.
- With Full Disk Access or Automation missing, the Proxy logs which permission is missing and how to grant it.

### 08. v1.0 shakedown week

**What to build:** The "Done when" test: a week of daily use from `maxs-pc` and the Android phone without falling back to the Mac or iPhone for Messages.
**Blocked by:** 03, 04, 05, 07
**Acceptance criteria:**
- A daily `progress.md` note for seven days, each naming any fallback to the Mac or iPhone and why, and any time the WireGuard tunnel was down.
- Each problem found becomes a new slice in this section.

# Progress

Append-only, newest last. Never rewrite old entries; append corrections.

## 2026-11-11 — Setup

- 🧰 Ran `/setup-matt-pocock-skills` with the `plan-progress` tracker: wrote `AGENTS.md` (plus a `CLAUDE.md` symlink), `plan.md`, this log, and `docs/agents/{issue-tracker,domain}.md`. No code exists yet.
- Open question: what "done" looks like.

Next:
- Grill or spec the first feature (`/grilling` → `/to-spec`).

## 2026-11-12 — Design grilling

- 🔥 Grilled the design (`/grill-with-docs`) and recorded every decision in `plan.md`: Host Mac = `work-macbook`; tailnet-only access on `172.16.1.2`; SIP always on; build our own Proxy, not BlueBubbles ([ADR 0001](docs/adr/0001-build-own-proxy-not-bluebubbles.md)); Go Proxy over `imsg rpc`; SSE live updates; one static HTML page styled after Messages.app; the P0–P6 ladder; and "Done when".
- 📖 Started `GLOSSARY.md`: Host Mac, Proxy, Client, Conversation, Message, Service.
- 🔬 Evidence: a prior-art survey (BlueBubbles, Beeper, imsg, mautrix) and a read-only review of the imsg v0.21.0 source. The survey dumped Messages.app's AppleScript dictionary with `sdef`; the imsg review ran nothing. Directly verified on the Host Mac: macOS 27.2; the terminal has no Full Disk Access yet; imsg is not installed; the WireGuard tunnel is up (`utun12`, `172.16.1.2`).
- ↩️ An accidental `/undo` lost part of this session; the lost answers were restored from the user's paste.
- Unverified: whether a Full Disk Access grant survives a rebuild or holds under launchd; whether HEIC images need conversion for Chrome.

Next:
- P0 by hand: install imsg, add its `~/.aliases` wrapper, grant Full Disk Access, list Conversations, send "🧪 imessage-proxy test" to myself.

## 2026-11-12 — Spec

- 📐 Ran `/to-spec`: appended `## Spec: iMessage proxy v1.0` to `plan.md` (45 user stories, three modules, HTTP contract, testing decisions, out of scope), approved by Max before saving.
- 🧵 Agreed one test seam: the whole Proxy, HTTP in and JSON-RPC out, with a fake imsg child that replays transcripts recorded in P0.

Next:
- P0 by hand: install imsg, add its `~/.aliases` wrapper, grant Full Disk Access, list Conversations, send "🧪 imessage-proxy test" to myself, and record sanitized `imsg rpc` transcripts.

## 2026-11-12 — ✅ P0 done: imsg reads and sends on the Host Mac

- 🍺 Installed imsg 0.21.0 from `steipete/tap`. The real binary (`libexec/imsg`, behind a bash shim) is signed "Developer ID Application: Peter Steinberger (Y5PE65HELJ)". The formula also installed the SIP-off bridge helper; with SIP on it stays inert (`status` reports `bridge.ready: false`).
- 🪪 Added an `imsg` wrapper to `~/.aliases` in dotfiles (prints the `findrepo imsg` hint); checked with `zsh -ic 'imsg --version'`. Not committed in dotfiles.
- 🔓 Full Disk Access: corporate device management did **not** block it. The grant went to `Rex Server.app` (`com.superlogical.rex.server`), the responsible process at the top of every Rex terminal, and took effect without restarting Rex. Automation (control Messages) was granted on the first send.
- 💬 `imsg chats` lists Conversations across iMessage and SMS with `any;-;…` GUIDs. No contact names yet: the Contacts permission isn't granted (`contacts.available: false`), which is P6.
- 📤 `imsg send` to my own iCloud address returned `status: sent`; row 183574 is `is_from_me=1`, `error=0`, `is_delivered=1`, joined to chat 242 (`any;-;<me>`), so it is not a ghost row. Not yet confirmed by eye on the iPhone.
- 📼 Recorded one real `imsg rpc` session (initialize, status, chats.list, messages.history, messages.after, watch.subscribe, send, message.send_status, an unknown param) and committed a sanitized copy as `testdata/imsg-0.21.0-p0.jsonl`, made by `scripts/sanitize-rpc-transcript.py` (allowlist: only enum and timestamp fields survive; everything else becomes consistent fakes). Leak check: none of the 28 real strings from the raw file appear. The raw file stays outside the repo.
- 🔬 Facts from the transcript for later rungs:
  - `send` accepts `transport: "applescript"` and echoes `transport` in its result. Unknown params return `-32602`.
  - `message.send_status` right after `send` says `pending` (`is_sent: false`): the `send` result itself is the confirmation, not an immediate status call.
  - In a note-to-self Conversation every send produces two rows, the sent one and a received echo (`is_from_me: false`); tests that send to myself must expect both.
  - An attachment's `mime_type` is empty in `messages.history` / `messages.after` but filled in watch notifications; on-disk paths are extensionless. Use `uti` / `transfer_name`. Attachments carry a `missing` flag (iCloud-only).

Next:
- P1: the Proxy lists Conversations in a browser on `maxs-pc` over the tailnet, plus the Full Disk Access checks (does the grant survive `go build`; how it is attributed under launchd).
- Or first `/to-tickets` to slice the spec into `## Work`.

## 2026-11-12 — P0 follow-up

- 📱 Confirmed by eye on the iPhone (iPhone Mirroring, 12:32 AM): both test Messages are in the note-to-self Conversation, each as a sent bubble plus a received echo; "(P0 rpc)" shows "Delivered". This closes P0's one unverified item.
- 🪪 Committed the `imsg` wrapper in dotfiles (`a0e91cb5a0b7`, `.aliases` only).

Next:
- Pick a project name, then `/to-tickets`, then P1.

## 2026-11-12 — Renamed to iMessage Anywhere

- 🏷️ Renamed the project from "iMessage proxy" to **iMessage Anywhere**: `sl mv` of the folder to `imessage-anywhere/`, plus the titles and paths in `AGENTS.md`, `plan.md`, `GLOSSARY.md`, and `docs/agents/domain.md`. "Proxy" stays as the glossary term for the program on the Host Mac. Earlier log entries and the recorded test Messages in `testdata/` keep the old name; they're history.
- Test Messages from now on say "🧪 imessage-anywhere test".

Next:
- `/to-tickets`, then P1.

## 2026-11-12 — Tickets

- 🎫 Ran `/to-tickets`: added `## Work: iMessage Anywhere v1.0` to `plan.md` with eight slices (01 Conversation list → 02 Read → 03 Live → 04 Send; 05 Attachments and 06 Contact names after 02; 07 Always running after 01; 08 shakedown week after all). Max delegated the three breakdown calls: the launchd Full Disk Access probe moved from 01 to 07, 04 stays blocked by 03 so duplicate-bubble merging is designed once, and no slices were merged or split.

Next:
- Start 01 (the only ready slice besides 07).

## 2026-11-12 — Five-step review of the plan

- 🔁 Re-evaluated the whole plan against the global CLAUDE.md values (simplicity, no paranoia, experiment-driven) and the five-step engineering algorithm; Max approved every change. `plan.md` was rewritten (the plan holds no status).
- 🔬 Three experiments, recorded with commands in [`experiments/`](experiments/README.md):
  - E1: 69 of 102 images received in 180 days are HEIC, but only 2 of the 102 are on the Host Mac's disk, even yesterday's. Ticket 05 now starts with an experiment on what brings attachments down.
  - E2: one `imsg rpc` child serves several `watch.subscribe` subscriptions with independent cursors. Each SSE connection gets its own subscription; the fan-out, resume buffer, and `messages.after` fallback are deleted.
  - E3: the Contacts permission request never prompted from a Rex terminal (`notDetermined`); with `SSH_CONNECTION` set, imsg reads the AddressBook database under Full Disk Access and names 7 of 20 recent Conversations. Ticket 06 folded into 01 and 02.
- 🗑️ Deleted: the imsg version check (replaced by `brew pin imsg`, done now: 0.21.0 pinned), in-process child restart (the Proxy exits, launchd restarts it, browsers resume), the GUID for sends (`chat_id` works, per P0), history paging (last 100 until ticket 08 shows a need), the `style-notes/` folder (values live in CSS custom properties), per-ticket transcript re-recording, and the duplicated `v1.0 scope` and `Prototype ladder` sections.
- 🧠 Corrected: 01's "Full Disk Access survives `go build`" check tested nothing (from a Rex terminal, macOS checks Rex Server's grant); it moved to 07, which also gains the tunnel-not-up-yet case.
- ⚡ Accelerated: the Proxy also listens on loopback, so the agent checks each ticket on the Host Mac side by side with Messages.app; real devices only after 02 and during 08.
- Unverified: whether anything reliably brings attachments to disk (ticket 05); Full Disk Access under launchd (ticket 07).

Next:
- Ticket 01.

## 2026-11-12 — ✅ 01 Conversation list in a browser

- 🦫 Walking skeleton: Go Proxy (`main.go`), Engine client over one `imsg rpc` child (`engine.go`), HTTP layer with the source filter (`server.go`), and the embedded page (`index.html`). The imsg child runs with `SSH_CONNECTION` set for contact names (E3). Messages endpoint added early: the list fills its previews with `?limit=1` (50 pipelined one-message reads take 2.8 s, too slow to block the list on).
- 🧪 Seam tests (5, all pass with `go test ./...`): Conversations newest first with display names (contact, group, unnamed group, RCS, +1 numbers formatted as `+1 (555) 010-0003`); only tailnet and loopback callers served (office, home LAN, just-outside-CGNAT, and the WireGuard peer refused); Engine signals Done when imsg exits; the fake flags uncovered requests; Messages oldest first with limit 100 by default. New transcript `testdata/imsg-0.21.0-01.jsonl` (100 Conversations, recorded with contact names); the recorder and sanitizer are now reusable scripts, and the sanitizer refuses to write output when a real value leaks.
- ✅ Acceptance, checked on the Host Mac against real imsg: the Proxy listened on `172.16.1.2:7171` and `127.0.0.1:7171`; Chrome over loopback showed 100 Conversations with previews; spot-check against Messages.app matched Max Coplan (12:27 AM, same preview), Max Berger, Mommy, the group Coplan Chat, and SMS 61746 (identical preview). Phone width (390 px iframe) shows the iOS large-title list; light and dark both work. `imsg` pinned (done in the review).
- 🎨 Style: dark colors measured from a Messages.app window capture (`screencapture -l`, kept out of the repo) and converted to `oklch()`; light colors are Apple system colors.
- 🙋 Opened `questions-for-max.md`: iPhone Mirroring needs an unlock for the iOS comparison; Messages.app filters some Conversations we show; the browser policy (no `light-dark()` fallback).

Next:
- Ticket 02 (Read a Conversation).

## 2026-11-12 — ✅ 02 Read a Conversation (real-device browser check pending)

- 💬 Choosing a Conversation shows its last 100 Messages: bubbles (blue iMessage, green SMS/RCS, gray received), group sender labels, Messages.app-style time dividers ("**Yesterday** 8:12 AM", after an hour of silence), clickable links, tapback badges with who reacted, "Edited" under edited Messages, and placeholders for photos, videos, audio, files, stickers, polls, app messages (e.g. "🧩 Apple Cash"), and unsent Messages. Whitespace-only Messages (seen in the wild: text "\n") are left out. At phone width the list and the Conversation are separate screens with a back link. Hash routing (`#/conversations/<id>`).
- 🧪 Seam tests: 8 pass. New transcript `testdata/imsg-0.21.0-02.jsonl` (group 21, edits 1947, polls 2093, Apple Cash 47); the fake now answers per `chat_id`; 01 re-recorded with `attachments: true` because the request shape changed. Sanitizer fixes found while recording: keep enums and timestamps (`emoji`, `edited_at`, `balloon_bundle_id`, poll `kind`), link GUID references (`reply_to_guid`, poll GUIDs), and keep U+FFFC attachment markers; its leak check caught two real values and refused to write.
- 🎨 Style: iOS 27 measured through iPhone Mirroring (Max unlocked it): small centered "Messages" title (not a large title), chevrons after the time, purple-gray default avatars, black background, received bubble `oklch(32.2% 0.009 284)`. Side effect: opening Coplan Chat on the iPhone marked its 15 unread Messages read (logged in questions).
- ✅ Checked: Chrome over loopback at desktop width (group 21 matches the iPhone: sender labels, ❤️❤️ on yesterday's photo, the "Yesterday" divider) and in 390 px frames (dark group, light 1:1 with edits/sticker/link, iOS list). From `maxs-pc` (Windows boot) over the tailnet: page 200, 100 Conversations, group 21 with 42 tapbacked Messages; via the home LAN to `172.16.1.2`: 403, as designed.
- ⚡ Preview filling uses 2 workers: with 4, opening a Conversation queued behind previews (imsg serves 4 reads at a time).
- Pending: a browser look on the Android phone and on `maxs-pc` (questions-for-max item 6).

Next:
- Ticket 03 (Live Messages).

## 2026-11-12 — ✅ 03 Live Messages

- ⚡ Each browser connection to `/api/events` (Server-Sent Events) gets its own imsg `watch.subscribe`, starting after `Last-Event-ID` (automatic EventSource reconnects) or `?since=` (the page's own reconnects), else at the tail; the Proxy unsubscribes when the browser leaves. Event IDs are row IDs and only move forward (edits reuse older IDs). A slow browser that fills its 256-Message buffer, or an imsg overflow, ends that stream; the browser resumes from its cursor. The read loop registers a subscription's channel before reading the next line, so a notification right after the response isn't lost.
- 😴 Heartbeat: a named `heartbeat` event every 25 s (SSE comments never reach JavaScript); the page reconnects with `?since=` if it hears nothing for 60 s, because a browser that only receives never notices a connection that died while the Host Mac slept. Offline banner: "Can't reach the Host Mac. Reconnecting…".
- 🧪 Seam tests: 11 pass, plus 3 runs under `-race`, clean. New: resume from Last-Event-ID (subscribes with since_rowid, events are the following rows), tail start + unsubscribe on disconnect, `?since=` cursor.
- ✅ Checked live on the Host Mac (Chrome over loopback, note-to-self Conversation): a Message sent from the Host Mac appeared within ~0.5 s with its echo, and the row moved to the top; stopping the Proxy showed the banner; a Message sent while it was down (183580/183581) arrived after restart via Last-Event-ID with zero duplicates; two tabs both updated. Not directly checked: a Message typed on the iPhone (the watch reads database rows regardless of origin).
- 🔬 Experiment: a page load took 4.6 s to show a Conversation. Not attachment metadata (0.49 s vs 0.50 s with/without); contention: chat 242's history took 0.50 s alone but 2.75 s with two preview reads in flight. Fix: preview workers wait for the open Conversation's load; it now shows in 2.5 s (history 0.66 s; the rest is the first list load after a restart).

Next:
- Ticket 04 (Send text).

## 2026-11-12 — ✅ 04 Send text

- ✉️ `POST /api/conversations/<id>/messages` (a plain form post) sends text into the Conversation by `chat_id` with `transport: "applescript"`. imsg's confirmed result (the outgoing row's ID) is the confirmation. Unconfirmed (`-32001` delivery outcome unknown, `-32004` lane blocked, imsg exiting mid-send, or `ok` without a row) is reported and never retried; anything else (e.g. `-32603` not started) is 502, certainly not sent. Once written to imsg, a send ignores the browser disconnecting (`context.WithoutCancel`).
- 🛡️ `http.NewCrossOriginProtection()` refuses cross-origin POSTs: otherwise any website open on a tailnet device could submit a hidden form and send Messages as me.
- 💬 Page: a composer (`field-sizing: content`, placeholder "iMessage" or "Text Message • SMS/RCS"), Enter sends and Shift+Enter adds a line on fine-pointer devices, a pending bubble shows "Sending…", then either the real row or a warning ("Not confirmed. Check Messages before resending." / "Not sent."). The live event for my own send usually arrives before the POST answers and takes over the pending bubble.
- 🧪 Seam tests: 16 pass. New: send targets chat_id over AppleScript and returns the row ID; unconfirmed is reported with exactly one send; not-started is 502; whitespace-only text isn't sent; cross-site POSTs are 403 with no send. The two error transcripts are synthetic (`testdata/synthetic-send-*.jsonl`), shaped from imsg 0.21.0's `deliveryFailure`, because those failures can't be produced on purpose.
- ✅ Checked live in Chrome: typed "🧪 imessage-anywhere test (04 typed in the page)" + Enter in the note-to-self Conversation; it became row 183584 (`is_from_me=1`, `error=0`, `is_delivered=1`, chat 242) with its echo 183585 and no duplicate bubble; Shift+Enter inserted a newline without sending; a stubbed unconfirmed reply (no real send) showed the warning. Not checked live: sending into a group (the only groups are real family/friends Conversations).

Next:
- Ticket 05 (Attachments: find out, then build) or 07 (Always running); both are ready.

## 2026-11-12 — 05 Attachments: experiment blocked on Max

- 🔬 Hypothesis: viewing a Conversation in Messages.app downloads its attachments. Candidates (counts only): chat 1947 has 0 of 11 recent received images on disk and 0 unread; group 21 has 1 of 13. Attempt: `open "sms:<handle>"` for chat 1947, then re-count after 15 s: still 0 of 11, but a window capture showed the link opened an empty "New Message" draft, not the Conversation, so the hypothesis is untested. imsg's docs have no download path.
- Next step needs a computer-use grant on Messages.app (questions-for-max item 7). Not built yet: serving on-disk attachments and HEIC conversion, which would help ~2% of recent images until the experiment says how to bring the rest down.

Next:
- Ticket 07 (Always running), then back to 05 when Max is here.

## 2026-11-12 — 07 Always running: installed, waiting for its Full Disk Access grant

- 🚀 `launchd/com.vegerot.imessage-anywhere.plist` (RunAtLoad, KeepAlive, ThrottleInterval 30 s, log at `~/Library/Logs/imessage-anywhere.log`) and `scripts/install-launch-agent.sh`, which builds into `~/.local/bin/imessage-anywhere` (so repo rebuilds never disturb the agent), signs it, copies the plist, and bootstraps the agent. Installed and running on the Host Mac.
- 🔬 Attribution, verified: under launchd macOS checks the Proxy's own binary for Full Disk Access, not Rex Server. The agent's first run logged imsg's reason ("…grant Full Disk Access to the supervising process…") plus the Proxy's own instruction naming `~/.local/bin/imessage-anywhere`; launchd shows last exit code 1 and "spawn scheduled" (retrying).
- 🔏 Go's default build is ad-hoc signed (`Identifier=a.out`, `flags=adhoc,linker-signed`), so its designated requirement is the code hash and, I infer, a grant wouldn't survive a rebuild. The install script now signs with the Mac's Apple Development identity and identifier `com.vegerot.imessage-anywhere`; the designated requirement becomes identifier + certificate. Whether the grant survives a rebuild is unverified until the grant exists.
- ✅ Startup readiness (story 44): the Proxy asks imsg for `status` and exits with an actionable message when the database isn't readable (seam tests with the real status and a synthetic no-access status: 18 pass). A missing tunnel address exits with "can't assign requested address" (checked with `-listen 172.16.9.9:7171`), so launchd retries until WireGuard is up. Failed sends are logged with imsg's reason (e.g. a missing Automation grant).
- Meanwhile the hand-run Proxy (from Rex, which holds a grant) keeps the page up; the agent fails its readiness check before binding the port, so they don't collide.
- Pending, needs Max: the grant (questions-for-max item 8); then check launchd reads and sends, the Automation prompt, and a rebuild.

Next:
- After Max's grant: finish 07. 05 needs Max too (item 7). Meanwhile: code review of everything so far.

## 2026-11-12 — ✅ 07 Always running

- 🔓 Max granted Full Disk Access to `~/.local/bin/imessage-anywhere`. After stopping the hand-run Proxy, the launchd agent passed its readiness check and served (loopback 200; `launchctl print`: running).
- ✅ The grant survives a rebuild, verified: `install-launch-agent.sh` rebuilt and re-signed the binary (its code hash changed) and the agent still read the database. Signing with the Apple Development identity (designated requirement = identifier + certificate) is what makes that work.
- ✅ The agent sent "🧪 imessage-anywhere test (07 sent by the launchd agent)": confirmed, row 183586, so it also has Automation access.
- 🙋 Max's answers: Q2 keep showing every Conversation; Q3 follow the system light/dark setting with no old-browser fallbacks (already the behavior); Q5 my call: unnamed groups as "Matt & +1 (240)…" after v1.0; Q6 a first look on Android looks good; granted computer use on Messages.app for ticket 05's experiment. Asked why URLs contain `/#/`; offered clean paths via the Navigation API.

Next:
- Fix the 10 code-review findings (test-first where they cross the seam), then ticket 05's experiment.

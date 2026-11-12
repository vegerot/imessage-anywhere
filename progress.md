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

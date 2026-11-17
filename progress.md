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

## 2026-11-12 — 🔍 Code review fixes (/code-review high, 10 findings, all fixed)

1. A blocked imsg send queue (`-32004`, after a send left `still_in_flight`) made every later send say "unconfirmed" and nothing went out until a manual restart. Now `-32004` is "not sent" (it never ran) and closes the Engine, as does a `-32001` with `still_in_flight`, so the Proxy exits and launchd restarts imsg. Tests: lane blocked → 502 + Done; still in flight → unconfirmed + Done.
2. imsg's terminal `error` notification (method `error`, shape from RPCServer+WatchHandlers.swift) left the stream open but deaf behind heartbeats. Now it ends the subscription like `watch.overflow`; the browser reconnects. Test: the stream ends promptly.
3. DNS rebinding could pass both the source filter and cross-origin protection. Now the Host must be an IP literal, `localhost`, `work-macbook`, or `*.ts.net`. Test, plus live from Windows: tailnet by IP 200, `Host: evil.example` 403.
4. Live tapbacks never arrived (imsg drops reaction rows unless `include_reactions`). Now the watch includes them; a tapback streams as its own event (`reactsTo` = target GUID, `tapback`, `tapbackRemoved`) and the page attaches it to the target bubble; Messages carry `guid`. Test on a real recorded transcript (`testdata/imsg-0.21.0-03.jsonl`: ❤️❤️ on photo 183536). Not yet seen live (no tapback happened during testing).
5. The live stream forwarded contentless rows that history leaves out. One shared `visible()` rule now covers both. Test with real row 183562.
6. Live edits/unsends of old Messages rewrote the list preview and marked unread. Now only new rows (ID past the newest seen) touch the preview, order, and unread mark.
7. An unconfirmed pending bubble whose row arrived late stayed as a duplicate with its warning. Now a late row takes over a pending bubble that's "sending" or "unconfirmed" (compared on trimmed text).
8. A browser leaving mid-subscribe could leak an imsg subscription (imsg allows 64). `watch.subscribe` now runs uncancellable, and the caller always unsubscribes. No test: depends on exact timing.
9. Live events for Conversations outside the list each reloaded the list and 100 previews. Now at most one reload runs, only for new rows; preview filling runs one at a time and skips filled rows.
10. The first subscription started at the tail only after loading, so a Message in between was lost. Now the page subscribes first and holds early events until the list exists.

- 🧪 23 seam tests pass. The sanitizer also keeps `reaction_emoji`/`reaction_type`. Three more synthetic transcripts (watch error, lane blocked, still in flight), shaped from imsg's source, since those failures can't be produced on purpose.
- ✅ Redeployed through the launchd agent (the re-signed build kept its grant again); page shows the Conversation in 1.0 s; a live Message arrived with its echo and no duplicates; no console errors.
- 🧭 Max asked about `/#/` vs clean paths and leans toward clean paths; compared in chat. Next: clean paths with `pushState`/`popstate` (modern-web-guidance `stack-drill-down`) and a same-document View Transition with focus routing (`same-document-transitions`).

## 2026-11-12 — 🧭 Clean paths and View Transitions

- 🧭 Conversations moved from `/#/conversations/<id>` to `/conversations/<id>`: the Proxy serves the page there (404 for non-numeric IDs; seam test), plain link clicks become `pushState`, and `popstate` (Back, the phone's gesture) re-routes without pushing. Per modern-web-guidance `stack-drill-down` and `same-document-transitions`: the update inside `document.startViewTransition` is synchronous (header and composer; Messages load after, so a slow fetch never freezes the animation); iOS push/pop at phone width, a 0.15 s fade on desktop, nothing with reduced motion; focus moves to the heading on open and back to the row on return. The hash routes are gone.
- ✅ Checked with chrome-devtools in an isolated context emulating 390×844 (via `/chrome-control`'s tooling): opening Coplan Chat set the path, ran a "forward" transition, focused the heading, hid the list, loaded 100 bubbles; Back returned to `/` with a "back" transition and focus on the Coplan Chat row.
- 🐛 Caught before committing: my router edit had also deleted `loadConversations` ("loadConversations is not defined"); restored. An undefined call like this is what moving the page to TypeScript (next) will catch.
- 🔬 Found: right after the Proxy restarts, imsg's first Conversation list can lack contact names (its Contacts catalog loads in the background, per imsg's docs), so names appear as raw numbers until a reload. Not fixed yet.

## 2026-11-12 — 🔷 The page's code is TypeScript

- 🔷 Max asked for the simplest way to write the page in TypeScript. Experiment: a pinned `npx --package=typescript@7.0.2 tsc` runs in 2.2 s with no `package.json`, emits clean JS, and on a type error exits 1 without emitting. So: `app.ts` (rewritten as idiomatic TypeScript: typed Proxy JSON with one documented cast in `fromJSON`, no `any`, DOM lookups narrowed with `instanceof` in a `find` helper, pending bubbles as Messages with negative IDs, `getTime()` date math), compiled by `go generate` into a committed `app.js` that the Proxy embeds and serves at `/app.js` (seam test). Node 26's `stripTypeScriptTypes` was the alternative; tsc alone both checks and strips.
- 🔒 Per Max: target/lib `esnext`, and every strictness option (`strict`, `exactOptionalPropertyTypes`, `noUncheckedIndexedAccess`, `noPropertyAccessFromIndexSignature`, `noImplicitReturns`, `noImplicitOverride`, `noFallthroughCasesInSwitch`, `noUnused*`, `allowUnreachableCode: false`, `allowUnusedLabels: false`, `noUncheckedSideEffectImports`, `erasableSyntaxOnly`, `verbatimModuleSyntax`, `isolatedModules`). They found three real issues (two unchecked `dataset` keys; a confirmed send setting `delivery` to undefined instead of removing it), all fixed. tsc also catches the deleted-function bug from the clean-paths slice ("Cannot find name … Did you mean …").
- ✅ Checked at 390 px in an isolated Chrome context: a deep link to `/conversations/242` opened directly; back to the list; a live Message (183590) and its echo arrived with no duplicates; Coplan Chat shows 100 bubbles and 42 tapbacks. 25 seam tests pass.

## 2026-11-12 — 05 experiment, step 1: viewing doesn't download

- 🔬 With computer use on Messages.app (granted by Max), opened chat 2276 (SMS, 0 unread per imsg; chosen after re-checking 1947, which has 86 unread, so my earlier SQL count was wrong) and scrolled its only image into view. After 15 s: attachment 19484 still `transfer_state 0`, not on disk. Messages.app draws it as a file card ("optimized-2.PNG · PNG Image · 112 KB") with a ⬇︎ download button, as it did for the `answer-key.md` cards: on this Mac, attachments wait for a manual download. Hypothesis rejected: viewing alone doesn't bring attachments down.
- Next: does clicking ⬇︎ put the file on disk? That's a download from an unknown sender, so asking Max first (questions-for-max item 9). The Alcove overlay blocked clicking the To field again; typing into the already-focused field worked.

## 2026-11-12 — Correction: contact names are there from the start

- ↩️ The clean-paths entry says imsg's first Conversation list after a restart can lack contact names. That was wrong. Measured with a fresh `imsg rpc` (SSH_CONNECTION set): 7 of 20 Conversations had names on the first call at 0.40 s, while `status` still said `contacts.available: false` (true at 0.93 s). The failed lookup at 01:43 was for "Coplan Chat", a group's own name that doesn't depend on Contacts; the list hadn't loaded at all because of the `loadConversations` deletion. Nothing to fix.

## 2026-11-12 — 05 Attachments, on-disk part; 🔍 second review fixed

- 🖼️ Built the part of ticket 05 that doesn't depend on the ⬇︎ experiment: Messages list their attachments (name, kind, and whether they're on the Host Mac); `GET /api/conversations/<id>/messages/<id>/attachments/<n>` serves a file only if imsg reports it for that exact Message (looked up with `messages.after`), it's on disk, and it lies under `~/Library/Messages/Attachments`; HEIC (and HEIF) is converted to JPEG with macOS's `sips`; `http.ServeContent` gives videos range requests. The page shows images inline (tapbacks on the photo, as on iOS), video and audio with native controls, files as downloads, and "📷 Photo, not downloaded on the Mac" for the rest; the list stays pinned to its end while images load. Link-preview payloads aren't shown as files.
- 🐛 The outside-the-store check caught a real bug on the first live request: imsg's `original_path` is absolute (its docs say so); my sanitizer had rewritten it into the database's `~/` form, so the code joined `$HOME` twice. Fixed in all three places: the sanitizer keeps each path's form (absolute under a synthetic `/Users/me`), the Engine accepts both, and the fake maps `/Users/me/` onto the test's `$HOME`.
- ✅ Tests: 29 pass, including a real HEIC (made with `sips` in a temporary `$HOME`) served as JPEG and 404s for missing, unreported, and absent files. Live: the ❤️❤️ photo in Coplan Chat (183536) is served as a 2.4 MB JPEG in 0.32 s and renders 3024 px wide at phone width; 14 attachments there say "not downloaded on the Mac".
- 🔍 Second code review (clean paths + TypeScript), 10 findings: (1) a pending bubble could be lost when history arrived or when switching Conversations: pending sends now live in their own per-Conversation map; (2) the back link pushed a new "/" so the phone's Back re-opened the Conversation: it now calls `history.back()` when the entry came from the list (verified: history didn't grow); (3) `popstate` animated on top of the browser's own swipe: skipped when `hasUAVisualTransition`; (4) history waited for the animation: now loads at `updateCallbackDone`; (5) returning lost the list's place: the row is focused with scrolling; (6) navigation listeners ran before the list existed: registered after it loads; (7) a navigation replaced mid-animation still fetched and focused: ignored unless current; (8) `install-launch-agent.sh` runs `go generate` (npx): kept on purpose, since deploys happen on this Mac and a stale `app.js` is the likelier mistake; (9) the Proxy accepted `+21`, `-3`, `021`: one `pathID` helper now requires plain digits everywhere (test); (10) three copies of the ID parser: folded into `pathID`.
- Still open in 05: whether a manual ⬇︎ in Messages.app brings a file to disk (questions-for-max item 9).

## 2026-11-12 — ✅ 05 Attachments (on-disk only)

- 🔬 With Max's OK: clicked ⬇︎ on `optimized-2.PNG` (chat 2276, SMS) and then double-clicked its card: after 20 s still `transfer_state 0`, no local path at all (inferred: an MMS relayed from the iPhone, so the Mac only ever had its metadata). Then an iMessage case: chat 1820 (Partiful, 0 unread per imsg) has image 19158 with a local path but no file; viewing the Conversation and scrolling to May didn't bring it down. The pictures Messages.app does show there are link-preview cards, rendered from preview data. Not tried: ⬇︎ on an iMessage photo (none was on screen).
- 📎 Decision recorded in `plan.md`: v1.0 shows attachments that are on the Host Mac's disk and labels the rest "not downloaded on the Mac"; making missing ones appear is out of v1.0. With the on-disk work already shipped, ticket 05 is done.
- 🧹 Cleaned up: closed the "New Message to +1 (555) 010-0992" draft with its row's ×, so no Conversation was opened. To reach Messages.app's controls under the "Alcove" overlay, I moved its window with AppleScript (`set bounds of front window`), using the Automation access this terminal already has; it moved back by itself later.

Next:
- Ticket 08 (shakedown week), with Max.

## 2026-11-16 — 🔵 Unread marks follow the newest Message

- 📋 Resumed from Max's `feedback.md` (now the "Backlog after v1.0" section of `plan.md`). Live state first: the launchd agent was serving (200); the log's only new failures were the 2026-11-16 12:33–12:39 bind errors while the WireGuard tunnel was down (see `proxy-down-tunnel-missing.md`). Max's commit `93d41a981453` moved `go generate` from `npx` to `bunx` and Go to 1.27.4. Ticket 08's daily notes were never started.
- 🔬 Must-fix "Conversations where I've sent a Message show as unread", measured before changing anything: 74 of 100 Conversations had imsg `unread_count > 0`; in 19 of them I had sent a Message within the last 100, and in 4 my Message was the newest. Cause: `unread_count` counts every received row with `is_read = 0`, and the Host Mac's database keeps old rows unread below Messages I sent.
- 🔵 New rule: a Conversation is unread when its newest Message is received and unread in the database (`isUnread`, from imsg's `is_read`), and this browser hasn't opened the Conversation since. The Proxy's Conversation JSON drops `unread`; Messages gain `isUnread`. The page sets the mark when a row's preview loads (it already fetches the newest Message), clears it when I open the Conversation or a Message of mine arrives live, and remembers the newest row ID seen per Conversation in `localStorage` (the page can't mark read on the Host Mac with SIP on). This also covers the nice-to-have "unread marks don't go away".
- ✅ Checked: 28 seam tests pass (new assertion on recorded row 183577). Redeployed through the launchd agent. Live: 62 unread, of which 55 are Conversations I never sent in and 7 have received Messages newer than my last one. In the browser: 62 marks; opening one cleared it; 61 after a reload; no console errors.
- ❓ Not verified: whether those 62 match the iPhone. If Messages read on the iPhone stay `is_read = 0` on this Mac, marks will still be too many; Max to compare a few.

Next:
- Max picks from the backlog; my suggestion is the cheap nits (skeletons, symlink, `is…` names), then profile photos and voice messages. The service worker needs HTTPS first.


## 2026-11-16 — 🔬 Where Messages.app keeps read state (database check, with Full Disk Access)

- 🗄️ Read state is one flag per received row: `message.is_read` (plus `date_read`). `chat.last_read_message_timestamp` gives the same answer (63 unread Conversations of the newest 100 either way), and Messages.app wasn't running, so the flags are kept current by the system's Messages service, not the app. The page's `isUnread` is this flag, read through imsg.
- 🧮 Why imsg's `unread_count` said 74: it also counts unread rows that aren't Messages (`item_type` 2–6: 364 rows in 12 known-sender Conversations, 321 of them `item_type = 4`). Counting real Messages only (`item_type = 0`): 63 Conversations, and in 62 of them the newest Message is unread. The page showed 62, so the newest-Message rule matches the database.
- 📂 Of those 63, 54 are in Conversations Messages.app files under unknown senders (`chat.is_filtered` 1 or 2); 9 are known senders.
- ✍️ Writing is the gap: with SIP on, nothing public marks a row read, so "opened in this browser" lives in `localStorage`. Untested alternatives: have Messages.app open the Conversation, or write `is_read` in the database directly (wouldn't reach the iPhone; the Messages service owns that file).

## 2026-11-16 — 📦 `app.js` is no longer committed

- 📦 Max asked why `app.js` was committed. It was so `go build` needed no JavaScript tools, but only the Host Mac builds the Proxy and `install-launch-agent.sh` regenerates it, so the committed copy only duplicated every page diff. Now ignored and untracked; a fresh checkout runs `go generate ./...` once before `go build` or `go test` (`AGENTS.md`, `plan.md`).
- 🙅 Max declined the "Messages.app opens the Conversation" experiment for shared read state: too complicated for now. "Opened in this browser" stays in `localStorage`.

## 2026-11-16 — 🎙️ Voice messages play, 🏷️ `is…` boolean names, 🦴 loading skeletons

- 🎙️ Voice messages. Measured: the 40 newest are all on the Host Mac's disk; since April 2026 they are Opus in a `.caf` file (`com.apple.coreaudio-format`), older SMS ones are AMR; no browser plays either. Experiment: macOS's `afconvert -f m4af -d aac` converts a 40 s voice message in 0.14 s. The Proxy's HEIC converter became a small table of converters (UTI → tool); both audio formats are served as `audio/mp4`. imsg returns the voice message's transcription as its text, so the page shows the player with the transcript under it. New real transcript `testdata/imsg-0.21.0-voice.jsonl` (row 183717); the test makes a `.caf` with `say` and expects an MPEG-4 file back.
- 🏷️ Boolean names start with `is`/`has`/`should` (Max's rule). JSON: `isFromMe`, `isEdited`, `isMissing`, `isTapbackRemoved` (with `isGroup`, `isUnread`). Page: `isAtEnd`, `isNearEnd`, `isListReady`, `isNewDivider`, `isNewSender`, `isPlainClick`, `isOnline`, `shouldFillAgain`, `shouldSendOnEnter`, `shouldScrollToEnd`, `shouldAnimate`. Proxy: `isVisible`, `isAllowedHost`, `isSubscribe`. 🐛 My search-and-replace also turned imsg's `edited_at` tag into `isEdited_at`; the edited-Message seam test failed and caught it.
- 🦴 Loading skeletons: 12 empty rows until the list loads, a gray bar where each preview will be until its newest Message loads, and 6 empty bubbles until a Conversation's Messages load. Mostly CSS: a row's name, time, and preview are styled by `:empty`, sized so that nothing shifts when text arrives; a gentle pulse, off with reduced motion.
- ✅ Checked: 29 seam tests pass. Redeployed through the launchd agent. In Chrome 156: the recorded voice message loads to "can play through" with a 39.85 s duration; 12 skeleton rows show before the list; a row is 82.47 px tall as a skeleton and 82.5 px with text; 6 skeleton bubbles appear on opening a Conversation and are gone once its 100 Messages load. Not checked: on the Android phone, and a voice message inside the open Conversation view (the recorded one is older than that Conversation's newest 100 Messages).

Next:
- Max decides on tunnel auto-start (plan explained in chat) and on patching imsg (my recommendation: not now).

## 2026-11-16 — 🔌 The WireGuard tunnel starts by itself; 🔗 symlinked executable tried and reverted

- 🔌 The existing root launchd job `com.vegerot.wg-home-endpoint` (every 30 s; source in `../vpn/wg-home-endpoint/`) now runs `wg-quick up home` when the tunnel is down. Off switch: `sudo touch /usr/local/etc/wg-home.off`.
- 🐛 First live test failed: the job logged "started the home tunnel", but 30 s later there was no interface. `wireguard-go` was gone and `/var/run/wireguard/home.name` was left behind. Cause (inferred from the fix working): launchd kills the processes a job leaves behind when its script exits. Fix: `AbandonProcessGroup` in the plist, and the script treats a name file whose interface doesn't answer as "tunnel down".
- ✅ Checked live: with the off switch set, the tunnel stayed down for 45 s after `wg-quick down home`; with it removed, the job started the tunnel within 30 s (handshake 2 s later, `wireguard-go` still running after the job's next run, tailnet ping 7 ms). Not checked: a reboot, and starting with no network.
- 🔬 Seen on the way: the already-running Proxy kept answering on loopback while the tunnel was down, and answered on `172.16.1.2` again once the tunnel was back, without a restart. A missing tunnel only stops the Proxy at startup.
- 🔗 Max asked for the install script to symlink the executable instead of placing a separate one in `~/.local/bin`. Experiment: built and signed the Proxy in the repo and made `~/.local/bin/imessage-anywhere` a symlink to it. The agent started through the symlink but imsg couldn't read the Messages database. The Full Disk Access record is keyed by the path `~/.local/bin/imessage-anywhere` plus the signature, and macOS matched the real file's path, which has no grant. Reverted; the Proxy is back (100 Conversations). To make a symlink work, Max would grant Full Disk Access to the repo's `imessage-anywhere` file. Even then a plain `go build` is ad-hoc signed, so it would not match the grant until signed (inferred from the grant's signature requirement, not tested).
- 🙋 Max's answers: no contact names are missing, the Contacts permission was meant as the better default; there is no 2FA Shortcut, only an idea. Both noted in `plan.md`'s backlog.

## 2026-11-16 — 🔗 The agent runs the repo's executable through a symlink

- 🔗 Max granted Full Disk Access to the repo's `imessage-anywhere` file. `install-launch-agent.sh` now builds and signs there and links `~/.local/bin/imessage-anywhere` to it. Checked: the agent started through the symlink and served 100 Conversations.
- 🔬 Experiment, replacing the earlier inference: a plain `go build` (forced to differ with `-ldflags=-s`) is ad-hoc signed (`Identifier=a.out`); after restarting the agent, imsg couldn't read the Messages database. Re-running the install script (signed build) fixed it. So the signature matters as well as the path: always build through the script. A `go build` with unchanged sources doesn't rewrite the file, so it keeps its signature.
- 🧹 The old grant for `~/.local/bin/imessage-anywhere` is now unused; Max can remove it in System Settings.

## 2026-11-16 — 🔔 Notifications while the page is open

- 🙋 Max's decisions: notifications only while the page is open somewhere, and none on the phone. That removed Web Push, the service worker, and an always-on watch in the Proxy from the plan.
- 🔔 The page shows a notification itself (`new Notification`) for a live Message when the page doesn't have focus, the Message isn't mine, and its sender is in my contacts. One notification per Conversation (a newer Message replaces it); clicking it focuses the page and opens the Conversation; opening the Conversation closes it. A "Turn on notifications" button under the list asks for permission (browsers need a click) and disappears once answered. Off on Android (Chrome there has no page notifications) and on plain HTTP (browsers need HTTPS or loopback).
- 🏷️ New JSON field `isUnknownSender`: a received Message whose sender imsg has no contact name for. Measured on the newest 5 Messages of 40 Conversations: 86 flagged, all 86 names are raw numbers; 49 not flagged, none is.
- ✅ Checked in Chrome on loopback with a stand-in live stream and a stand-in `Notification`: the button shows, then hides after granting; of four live Messages (known sender, unknown sender, mine, group) exactly the known one and the group one notify, with the Conversation name as title, "Pat: …" in the group, and the Conversation ID as tag; the click opens the Conversation and closes the notification. 29 seam tests pass. Not checked: a real notification on screen from a real Message, and whether Chrome's Memory Saver discards an idle tab (that would stop notifications).
- 🔬 Before deciding: the Host Mac reaches the push services (`fcm.googleapis.com`, Mozilla's, Apple's), and the Pi's `work-macbook` node may request an HTTPS certificate (`CertDomains` lists its `ts.net` name). Push is not needed now; the certificate is the next step.

Next:
- HTTPS with the node's certificate, then a web app manifest so the page installs as its own window.

## 2026-11-16 — 🔒 HTTPS with a real certificate; 📲 the page installs as its own window

- 🔒 The Proxy serves HTTPS on `172.16.1.2:7171` and plain HTTP on `127.0.0.1:7171`. New address: `https://work-macbook.<tailnet>.ts.net:7171`. The old `http://work-macbook:7171` now answers 400.
- 📜 The certificate (Let's Encrypt, expires 2027-02-15) comes from the Pi's `work-macbook` node through `scripts/fetch-certificate.sh`; a new launchd agent `com.vegerot.imessage-anywhere.certificate` runs it at login and every Monday, and restarts the Proxy when the file changed. `install-launch-agent.sh` installs both agents.
- 🐛 The first request failed: "lookup acme-v02.api.letsencrypt.org on [fd7a:115c:a1e0::53]:53: server misbehaving". Observed: the node's tailscaled no longer had `/etc/netns/wm/resolv.conf` mounted over `/etc/resolv.conf` (its mount list had no entry; the Pi's own tailscaled rewrote `/etc/resolv.conf` on 2026-11-16 04:06, after the node started on 2026-11-11). Inferred: replacing the file dropped the mount, so the node asked its own DNS, which has no upstream. Fix, in the fetch script: put the mount back in the running process (`nsenter --mount … mount --bind`) before asking; no restart of the node. The request then succeeded.
- 📲 `manifest.webmanifest`, `icon.svg`, and two PNG icons drawn from it, served by the Proxy and linked from the page.
- ✅ Checked: from the home PC over the tailnet, `curl https://<name>:7171/api/conversations` answers 200 with the certificate verified; on the Host Mac, by name, the handshake is HTTP/2 with the certificate verified. The certificate agent's first run under launchd logged "certificate unchanged". Chrome on loopback fires `beforeinstallprompt` (the page is installable) and shows the "Turn on notifications" button. 30 seam tests pass.
- ❓ Not checked: installing and a real notification on `maxs-pc` or Windows; a renewal (the first is due around mid-January 2027: tailscaled renews in the last third of the 90 days); the devbox can't reach the node at all (time-out by address, before and unrelated to HTTPS as far as I can tell).

Next:
- Max: open the new address on `maxs-pc`, install it (address bar → install icon), click "Turn on notifications", and wait for a real Message.

## 2026-11-16 — 📲 Installed on Windows

- ✅ Max installed the page from the new HTTPS address on the Windows PC; it works. No notification seen yet (no real Message from a known sender had arrived while the window was in the background).

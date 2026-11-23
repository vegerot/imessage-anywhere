---
aliases: [imessage-anywhere]
---

# 🗺️ iMessage Anywhere

A Go Proxy on the work Mac that serves Messages to tailnet browsers. This folder is also the project root (see `CLAUDE.md`, `plan.md`, `progress.md`).

## Conversations

1. [imessage-anywhere-design-and-build.md](imessage-anywhere-design-and-build.md) (2026-11-11): from an empty folder to a running v1.0 candidate; designed the Proxy and its tunnel-only listening.
2. [proxy-down-tunnel-missing.md](proxy-down-tunnel-missing.md) (2026-11-16): `localhost:7171` refused because the WireGuard tunnel was down; the Proxy exits on purpose and launchd retried until `wg-quick up home` ran.
3. [backlog-unread-voice-tunnel-https-notifications.md](backlog-unread-voice-tunnel-https-notifications.md) (2026-11-16): worked Max's feedback list: unread marks, voice messages, skeletons, tunnel auto-start, symlinked executable, then HTTPS, an installable page, and notifications.
4. [official-imessage-plugin-comparison.md](official-imessage-plugin-comparison.md) (2026-11-23): read Anthropic's `imessage@claude-plugins-official` plugin and compared it with the Proxy and the daily brief's iMessage read; decided not to use it. Nothing was built.

## Takeaways

- 🔌 The Proxy depends on the WireGuard tunnel. The design chose "exit and let launchd retry" over "skip the missing address", so a down tunnel makes loopback fail too. Conversation 1 built that, conversation 2 was the first time it bit, and conversation 3 made a root launchd job start the tunnel. When the page is unreachable, check `ifconfig | rg 172.16.1.2` and `~/Library/Logs/imessage-anywhere.log` first.
- 🔐 macOS ties Full Disk Access to the real file path plus the signature. Build only through `scripts/install-launch-agent.sh`; a plain `go build` loses the grant (conversations 1 and 3).
- ✂️ Asking what is really needed removed the hardest work: "notifications only while the page is open" turned Web Push, a service worker, and Proxy state into about 30 lines in the page (conversation 3).
- 🌐 The page is at `https://work-macbook.<tailnet>.ts.net:7171` since conversation 3; the Pi's stand-in node supplies the certificate because the Host Mac can't run Tailscale.
- 📡 Anthropic's iMessage plugin is not an alternative to the Proxy. It lets people text a Claude session; the Proxy lets Max use Messages from another machine. It reads only allowlisted chats and needs Full Disk Access on the app that launches Claude, so it does not fit the daily brief either, and the Claude app's Remote Control already covers steering a session from the phone (conversation 4, from source only, not run).

## 🗃️ Live table

Renders in Obsidian.

![[topic.base]]

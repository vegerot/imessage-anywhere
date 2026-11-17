---
aliases: [imessage-anywhere]
---

# 🗺️ iMessage Anywhere

A Go Proxy on the work Mac that serves Messages to tailnet browsers. This folder is also the project root (see `CLAUDE.md`, `plan.md`, `progress.md`).

## Conversations

1. [imessage-anywhere-design-and-build.md](imessage-anywhere-design-and-build.md) (2026-11-11): from an empty folder to a running v1.0 candidate; designed the Proxy and its tunnel-only listening.
2. [proxy-down-tunnel-missing.md](proxy-down-tunnel-missing.md) (2026-11-16): `localhost:7171` refused because the WireGuard tunnel was down; the Proxy exits on purpose and launchd retried until `wg-quick up home` ran.

## Takeaways

- 🔌 The Proxy depends on the WireGuard tunnel, which a root launchd job starts and keeps up since 2026-11-16 (before that, by hand). The design chose "exit and let launchd retry" over "skip the missing address", so a down tunnel makes loopback fail too. When the page is unreachable, check `ifconfig | rg 172.16.1.2` and the log in `~/Library/Logs/imessage-anywhere.log` first.
- 🧭 Conversation 1 built the retry behavior; conversation 2 was the first time it showed up in real use. Tunnel auto-start was added on 2026-11-16 (`progress.md`).

## 🗃️ Live table

Renders in Obsidian.

![[topic.base]]

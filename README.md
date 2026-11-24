# 🗺️ iMessage Anywhere

A Go Proxy on the work Mac that serves Messages to tailnet browsers. Start with `CLAUDE.md`, `plan.md`, `progress.md`.

## Conversations

The conversation notes about this project are in the `ai-conversations` archive, folder `imessage-anywhere/`. This project lived there until 2026-11-24. The takeaways below name those notes by number, oldest first.

## Takeaways

- 🔌 The Proxy depends on the WireGuard tunnel. The design chose "exit and let launchd retry" over "skip the missing address", so a down tunnel makes loopback fail too. Conversation 1 built that, conversation 2 was the first time it bit, and conversation 3 made a root launchd job start the tunnel. When the page is unreachable, check `ifconfig | rg 172.16.1.2` and `~/Library/Logs/imessage-anywhere.log` first.
- 🔐 macOS ties Full Disk Access to the real file path plus the signature. Build only through `scripts/setup.sh install-launch-agent`; a plain `go build` loses the grant (conversations 1 and 3).
- ✂️ Asking what is really needed removed the hardest work: "notifications only while the page is open" turned Web Push, a service worker, and Proxy state into about 30 lines in the page (conversation 3).
- 🌐 The page is at `https://work-macbook.<tailnet>.ts.net:7171` since conversation 3; the Pi's stand-in node supplies the certificate because the Host Mac can't run Tailscale.
- 📡 Anthropic's iMessage plugin is not an alternative to the Proxy. It lets people text a Claude session; the Proxy lets Max use Messages from another machine. It reads only allowlisted chats and needs Full Disk Access on the app that launches Claude, so it does not fit the daily brief either, and the Claude app's Remote Control already covers steering a session from the phone (conversation 4, from source only, not run).

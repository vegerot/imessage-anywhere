# 🙋 Questions for Max

Open items that need you, most important first. Resolved items move to `progress.md`.

## Open

1. **Ticket 05: may I click a download button in Messages.app?** Viewing a Conversation does not download its attachments (tested on chat 2276: its image stayed un-downloaded while on screen). Messages.app shows each attachment as a card with a ⬇︎ button. Next test: click ⬇︎ on `optimized-2.PNG` (PNG, 112 KB, from an unknown-sender campaign SMS in chat 2276) and check whether it lands on disk. If it does, the page could offer "Download on the Mac" for each attachment; if not, missing attachments stay labeled "not downloaded on the Mac". (Attachments already on disk are shown, HEIC included.)
2. **Ticket 08: the shakedown week.** Use <http://100.121.37.3:7171/> daily from `maxs-pc` and the Android phone (Tailscale on) and tell me each time you fall back to the Mac or iPhone, and whether the WireGuard tunnel was down.
3. **Messages.app has a leftover draft**, "New Message to +1 (555) 010-0992", from the attachment test. Clicking any Conversation removes it. I didn't, because clicking one marks it read.

## Resolved (2026-11-12)

- Full Disk Access for the launchd agent: granted; it serves and sends, and the grant survives rebuilds.
- Messages.app computer-use access: granted; used for the attachment test.
- Q: show every Conversation, or mimic Messages.app's filter? Show everything.
- Q: browser policy: follow the system light/dark setting, no old-browser fallbacks.
- Q: unnamed groups as "Matt & +1 (240)…" like iOS: my call, after v1.0.
- Android first look: looks good.
- iOS comparison: measured while the iPhone was unlocked.
- FYI: I marked Coplan Chat read on the iPhone during the iOS style check.

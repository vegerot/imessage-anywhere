# Build our own Proxy, not BlueBubbles

2026-11-12. BlueBubbles already gives Linux and Windows a Messages client, but its server is an Electron app, its richer features need SIP disabled, and its client and UI aren't ours to shape. We build a small Proxy and a plain web page instead. That keeps the system small, avoids Electron, works with SIP on, and lets the UI follow Apple's Messages.app on its own terms.

## Considered options

- **BlueBubbles** (server v1.9.12, client v2.1.6): turnkey and native on Linux, but rejected for the Electron server and because its tapbacks and replies need SIP off.
- **Beeper `platform-imessage`**: keeps SIP on through UI automation, but has no API for a web page to call. Its approach stays an option for rich features after v1.0.
- **mautrix-imessage**: needs a Matrix homeserver; no releases.

Evidence: the prior-art survey in the 2026-11-11 conversation archived in this folder.

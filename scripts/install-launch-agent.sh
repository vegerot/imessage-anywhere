#!/bin/sh
# Build the Proxy here, link ~/.local/bin/imessage-anywhere to it, and (re)start it as a
# launchd agent. The agent runs the repo's own build through that symlink, so there is one
# executable. Full Disk Access is granted to the real file here, not to the symlink
# (macOS matches the real path; verified 2026-11-16).
# Always build through this script: a plain `go build` is ad-hoc signed, which doesn't match
# the grant, so the agent can't read the Messages database after its next restart (verified).
set -eu
cd "$(dirname "$0")/.."
label=com.vegerot.imessage-anywhere

binary="$PWD/imessage-anywhere"
go generate ./... # type-check app.ts and compile it to app.js
go build -o "$binary" .
# Sign with a stable identity: macOS ties a Full Disk Access grant to the program's
# designated requirement. Go's default ad-hoc signature makes that the code hash, which
# changes on every build; identifier + certificate doesn't.
identity=$(security find-identity -v -p codesigning | awk '/Apple Development/ {print $2; exit}')
if [ -n "$identity" ]; then
	codesign --force --sign "$identity" --identifier "$label" "$binary"
else
	echo "warning: no Apple Development identity; ad-hoc signed, so each rebuild needs a new Full Disk Access grant" >&2
fi
ln -sf "$binary" "$HOME/.local/bin/imessage-anywhere"
# Two agents: the Proxy, and the weekly fetch of its HTTPS certificate (which also runs
# now, so a fresh install gets its certificate; the Proxy retries until it is there).
for agent in "$label" "$label.certificate"; do
	cp "launchd/$agent.plist" "$HOME/Library/LaunchAgents/"
	launchctl bootout "gui/$(id -u)/$agent" 2>/dev/null || true
	launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/$agent.plist"
done
echo "started $label; log: ~/Library/Logs/imessage-anywhere.log"

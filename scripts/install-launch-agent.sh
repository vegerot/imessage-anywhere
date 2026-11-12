#!/bin/sh
# Build the Proxy into ~/.local/bin and (re)start it as a launchd agent.
# The agent runs this installed copy, not the repo build, so rebuilding while developing
# never disturbs it (or a Full Disk Access grant that may be tied to its exact build).
set -eu
cd "$(dirname "$0")/.."
label=com.vegerot.imessage-anywhere
plist="$HOME/Library/LaunchAgents/$label.plist"

binary="$HOME/.local/bin/imessage-anywhere"
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
cp "launchd/$label.plist" "$plist"
launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$plist"
echo "started $label; log: ~/Library/Logs/imessage-anywhere.log"

#!/bin/sh
# Setup steps for iMessage Anywhere on the Host Mac.
#
# Usage: scripts/setup.sh <fetch-certificate | install-launch-agent>
set -eu
label=com.vegerot.imessage-anywhere

# Fetch the HTTPS certificate for the Proxy's tailnet name, and restart the Proxy if it changed.
# The Host Mac can't run Tailscale, so the Pi's `work-macbook` node (../vpn/work-macbook-stand-in)
# asks Tailscale for the certificate (Let's Encrypt, valid 90 days) and this subcommand copies it
# here over SSH. tailscaled keeps the certificate and renews it when it's close to expiring,
# so running this weekly (launchd/com.vegerot.imessage-anywhere.certificate.plist) is enough.
fetch_certificate() {
directory="$HOME/Library/Application Support/imessage-anywhere"
certificate="$directory/certificate.pem" # the certificate chain, then the private key

mkdir -p "$directory"
umask 077
# On the Pi. The node's tailscaled runs under `ip netns exec wm`, which mounts
# /etc/netns/wm/resolv.conf over /etc/resolv.conf for it. Whenever the Pi's own tailscaled
# rewrites /etc/resolv.conf, that mount is dropped, and the node then asks its own DNS,
# which answers nothing (seen 2026-11-16: "lookup acme-v02.api.letsencrypt.org ... server
# misbehaving"). So put the mount back before asking for the certificate.
ssh -o BatchMode=yes raspberrypi sh -s >"$certificate.new" <<'PI'
tailscale="sudo tailscale --socket=/run/tailscale-wm/tailscaled.sock"
pid=$(systemctl show --property=MainPID --value tailscaled-work-macbook)
if ! sudo grep --quiet ' /etc/resolv.conf ' "/proc/$pid/mountinfo"; then
	sudo nsenter --target="$pid" --mount mount --bind /etc/netns/wm/resolv.conf /etc/resolv.conf
fi
name=$($tailscale status --json | sed --quiet '/"CertDomains"/{n;s/[", ]//g;p}')
$tailscale cert --min-validity=336h --cert-file=- --key-file=- "$name"
PI
grep -q 'BEGIN CERTIFICATE' "$certificate.new" && grep -q 'PRIVATE KEY' "$certificate.new"

if cmp -s "$certificate.new" "$certificate"; then
	rm "$certificate.new"
	echo "$(date '+%Y-%m-%d %H:%M:%S') certificate unchanged"
	return 0
fi
mv "$certificate.new" "$certificate"
echo "$(date '+%Y-%m-%d %H:%M:%S') new certificate: $(openssl x509 -in "$certificate" -noout -enddate)"
launchctl kickstart -k "gui/$(id -u)/$label" 2>/dev/null || true # the Proxy reads it at startup
}

# Build the Proxy here, link ~/.local/bin/imessage-anywhere to it, and (re)start it as a
# launchd agent. The agent runs the repo's own build through that symlink, so there is one
# executable. Full Disk Access is granted to the real file here, not to the symlink
# (macOS matches the real path; verified 2026-11-16).
# Always build through this subcommand: a plain `go build` is ad-hoc signed, which doesn't match
# the grant, so the agent can't read the Messages database after its next restart (verified).
install_launch_agent() {
cd "$(dirname "$0")/.."

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
}

case "${1:-}" in
fetch-certificate) fetch_certificate ;;
install-launch-agent) install_launch_agent ;;
*) echo "usage: $0 <fetch-certificate | install-launch-agent>" >&2; exit 2 ;;
esac

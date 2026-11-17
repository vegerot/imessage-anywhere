#!/bin/sh
# Fetch the HTTPS certificate for the Proxy's tailnet name, and restart the Proxy if it changed.
# The Host Mac can't run Tailscale, so the Pi's `work-macbook` node (../vpn/work-macbook-stand-in)
# asks Tailscale for the certificate (Let's Encrypt, valid 90 days) and this script copies it
# here over SSH. tailscaled keeps the certificate and renews it when it's close to expiring,
# so running this weekly (launchd/com.vegerot.imessage-anywhere.certificate.plist) is enough.
set -eu
label=com.vegerot.imessage-anywhere
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
set -eu
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
	exit 0
fi
mv "$certificate.new" "$certificate"
echo "$(date '+%Y-%m-%d %H:%M:%S') new certificate: $(openssl x509 -in "$certificate" -noout -enddate)"
launchctl kickstart -k "gui/$(id -u)/$label" 2>/dev/null || true # the Proxy reads it at startup

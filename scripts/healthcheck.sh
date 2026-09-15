#!/usr/bin/env bash
set -Eeuo pipefail
iface="${1:-wg0}"
peer_ip="${2:-10.77.0.1}"
max_age="${MAX_HANDSHAKE_AGE:-180}"
command -v wg >/dev/null || { echo "wireguard-tools is missing"; exit 2; }
command -v ping >/dev/null || { echo "iputils-ping is missing"; exit 2; }
wg show "$iface" >/dev/null 2>&1 || { echo "FAIL: interface $iface is down"; exit 1; }
latest=$(wg show "$iface" latest-handshakes | awk 'NR==1 {print $2}')
[[ -n "$latest" && "$latest" != 0 ]] || { echo "FAIL: no handshake on $iface"; exit 1; }
now=$(date +%s); age=$((now-latest))
(( age <= max_age )) || { echo "FAIL: handshake age ${age}s exceeds ${max_age}s"; exit 1; }
ping -c 1 -W 2 "$peer_ip" >/dev/null || { echo "FAIL: tunnel IP $peer_ip is unreachable"; exit 1; }
echo "OK: $iface handshake=${age}s peer=$peer_ip"

#!/usr/bin/env bash
# Creates a standard GRE or IPIP interface between two networks you administer.
# These transports are not encrypted. Use inside WireGuard when confidentiality is required.
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run as root.' >&2; exit 1; }
kind="${1:-}"; name="${2:-}"; local_ip="${3:-}"; remote_ip="${4:-}"; local_tun="${5:-10.88.0.1/30}"; remote_tun="${6:-10.88.0.2/30}"
[[ "$kind" == gre || "$kind" == ipip ]] || { echo "Usage: $0 {gre|ipip} NAME LOCAL_PUBLIC_IP REMOTE_PUBLIC_IP LOCAL_TUNNEL_CIDR REMOTE_TUNNEL_IP"; exit 2; }
[[ -n "$name" && -n "$local_ip" && -n "$remote_ip" ]] || exit 2
modprobe "$kind" 2>/dev/null || true
ip tunnel del "$name" 2>/dev/null || true
ip tunnel add "$name" mode "$kind" local "$local_ip" remote "$remote_ip" ttl 64
ip link set dev "$name" mtu 1400 up
ip addr add "$local_tun" dev "$name"
echo "Created $kind interface $name. Add a route, for example:"
echo "  ip route add 10.90.0.0/16 via ${remote_tun%/*} dev $name"
echo "Persist this only after testing and add firewall rules for your owned networks."

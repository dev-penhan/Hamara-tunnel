#!/usr/bin/env bash
# Hamara Tunnel - secure site-to-site WireGuard installer
# Supported OS: Debian 11+, Ubuntu 20.04+, Rocky/Alma 9+
set -Eeuo pipefail
IFS=$'\n\t'

VERSION="1.0.0"
STATE_DIR="/etc/hamara-tunnel"
WG_DIR="/etc/wireguard"
LOG_FILE="/var/log/hamara-tunnel.log"
DEFAULT_IF="wg0"

log(){ printf '[%s] %s\n' "$(date '+%F %T')" "$*" | tee -a "$LOG_FILE"; }
warn(){ printf '\033[1;33m[WARN]\033[0m %s\n' "$*" >&2; }
die(){ printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; exit 1; }
need_root(){ [[ $EUID -eq 0 ]] || die "Run as root: sudo bash hamara-tunnel.sh"; }
command_exists(){ command -v "$1" >/dev/null 2>&1; }
valid_name(){ [[ "$1" =~ ^[a-zA-Z0-9._-]+$ ]]; }
valid_cidr(){ [[ "$1" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}/[0-9]{1,2}$ ]]; }

pkg_install(){
  export DEBIAN_FRONTEND=noninteractive
  if command_exists apt-get; then
    apt-get update -y
    apt-get install -y wireguard wireguard-tools iproute2 curl qrencode iptables resolvconf
  elif command_exists dnf; then
    dnf install -y wireguard-tools iproute curl qrencode iptables-nft
  elif command_exists yum; then
    yum install -y wireguard-tools iproute curl qrencode iptables-services
  else die "Unsupported package manager. Install wireguard-tools, iproute2 and iptables manually."; fi
}

sysctl_enable(){
  cat >/etc/sysctl.d/99-hamara-tunnel.conf <<'EOF'
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
net.ipv4.conf.all.src_valid_mark=1
EOF
  sysctl --system >/dev/null
}

public_ip(){
  curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || true
}

keypair(){
  local name="$1"
  umask 077
  wg genkey | tee "$STATE_DIR/${name}.private" | wg pubkey >"$STATE_DIR/${name}.public"
}

ask(){ local v; read -r -p "$1" v; printf '%s' "$v"; }
ask_default(){ local v; read -r -p "$1 [$2]: " v; printf '%s' "${v:-$2}"; }

select_role(){
  echo
  echo "Select this VPS role:"
  echo "  1) Iran-side / client-side VPS"
  echo "  2) Foreign-side / exit VPS"
  echo "  3) Generate both configs (offline handoff)"
  local n; n=$(ask "Choice")
  case "$n" in 1) ROLE=iran;; 2) ROLE=foreign;; 3) ROLE=both;; *) die "Invalid choice";; esac
}

select_profile(){
  echo
  echo "Select tunnel profile:"
  echo "  1) WireGuard Standard - encrypted routed tunnel"
  echo "  2) Hamara Multi-Tunnel - two independent WireGuard interfaces"
  echo "  3) ARA Tunnel - explicit IP-forwarding profile with strict forwarding rules"
  local n; n=$(ask "Choice")
  case "$n" in
    1) PROFILE=standard; INTERFACES=(wg0);;
    2) PROFILE=multi; INTERFACES=(wg0 wg1);;
    3) PROFILE=ara; INTERFACES=(ara0);;
    *) die "Invalid choice";;
  esac
}

make_server_conf(){
  local iface="$1" server_priv="$2" peer_pub="$3" endpoint="$4" server_addr="$5" peer_addr="$6" listen="$7" keepalive="$8" allowed="$9"
  cat >"$WG_DIR/${iface}.conf" <<EOF
# Hamara Tunnel ${iface} - generated $(date -u +%FT%TZ)
[Interface]
Address = ${server_addr}
ListenPort = ${listen}
PrivateKey = ${server_priv}
SaveConfig = false

# Routed forwarding is enabled by the installer. Adjust PostUp/PostDown only if needed.
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -A FORWARD -o %i -j ACCEPT
PostDown = iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true; iptables -D FORWARD -o %i -j ACCEPT 2>/dev/null || true

[Peer]
PublicKey = ${peer_pub}
AllowedIPs = ${peer_addr}, ${allowed}
PersistentKeepalive = ${keepalive}
EOF
  chmod 600 "$WG_DIR/${iface}.conf"
}

make_client_conf(){
  local iface="$1" client_priv="$2" server_pub="$3" endpoint="$4" client_addr="$5" server_addr="$6" listen="$7" keepalive="$8" allowed="$9"
  cat >"$STATE_DIR/${iface}-client.conf" <<EOF
# Hamara Tunnel ${iface} client configuration
[Interface]
Address = ${client_addr}
PrivateKey = ${client_priv}

[Peer]
PublicKey = ${server_pub}
Endpoint = ${endpoint}:${listen}
AllowedIPs = ${allowed}
PersistentKeepalive = ${keepalive}
EOF
  chmod 600 "$STATE_DIR/${iface}-client.conf"
}

apply_nat(){
  local iface="$1" wan="$2" subnet="$3"
  iptables -t nat -C POSTROUTING -s "$subnet" -o "$wan" -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s "$subnet" -o "$wan" -j MASQUERADE
  iptables -C FORWARD -i "$iface" -o "$wan" -j ACCEPT 2>/dev/null || iptables -A FORWARD -i "$iface" -o "$wan" -j ACCEPT
  iptables -C FORWARD -i "$wan" -o "$iface" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -i "$wan" -o "$iface" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
}

persist_firewall(){
  if command_exists netfilter-persistent; then netfilter-persistent save || true
  elif command_exists iptables-save; then iptables-save >/etc/iptables/rules.v4 2>/dev/null || true; fi
}

install_tunnel(){
  need_root
  mkdir -p "$STATE_DIR" "$WG_DIR"
  touch "$LOG_FILE"; chmod 600 "$LOG_FILE"
  pkg_install
  sysctl_enable
  select_role
  select_profile
  local detected; detected=$(public_ip)
  local endpoint; endpoint=$(ask_default "Public IP or DNS name of the foreign VPS" "${detected:-YOUR_FOREIGN_IP}")
  local listen; listen=$(ask_default "WireGuard UDP port" "51820")
  local keepalive; keepalive=$(ask_default "PersistentKeepalive seconds" "25")
  local allowed; allowed=$(ask_default "Traffic to route through the tunnel" "0.0.0.0/0, ::/0")
  local wan; wan=$(ip route show default | awk '/default/ {print $5; exit}')
  [[ -n "$wan" ]] || die "Could not detect the WAN interface."

  cat >"$STATE_DIR/install.env" <<EOF
PROFILE=$PROFILE
ROLE=$ROLE
WAN=$wan
ENDPOINT=$endpoint
LISTEN=$listen
KEEPALIVE=$keepalive
ALLOWED=$allowed
EOF
  chmod 600 "$STATE_DIR/install.env"

  local i=0 iface server_addr client_addr subnet server_priv client_priv server_pub client_pub
  for iface in "${INTERFACES[@]}"; do
    if [[ "$iface" == "wg1" ]]; then server_addr="10.77.1.1/30"; client_addr="10.77.1.2/30"; subnet="10.77.1.0/30"; listen=51821
    elif [[ "$iface" == "ara0" ]]; then server_addr="10.77.10.1/30"; client_addr="10.77.10.2/30"; subnet="10.77.10.0/30"; listen=51830
    else server_addr="10.77.0.1/30"; client_addr="10.77.0.2/30"; subnet="10.77.0.0/30"; fi
    keypair "${iface}-server"; keypair "${iface}-client"
    server_priv=$(cat "$STATE_DIR/${iface}-server.private"); server_pub=$(cat "$STATE_DIR/${iface}-server.public")
    client_priv=$(cat "$STATE_DIR/${iface}-client.private"); client_pub=$(cat "$STATE_DIR/${iface}-client.public")
    # The foreign side owns the server config; the Iran side uses the client config.
    make_server_conf "$iface" "$server_priv" "$client_pub" "$endpoint" "$server_addr" "$client_addr" "$listen" "$keepalive" "$client_addr"
    make_client_conf "$iface" "$client_priv" "$server_pub" "$endpoint" "$client_addr" "$server_addr" "$listen" "$keepalive" "$allowed"
    apply_nat "$iface" "$wan" "$subnet"
    if [[ "$ROLE" == iran ]]; then
      cp "$STATE_DIR/${iface}-client.conf" "$WG_DIR/${iface}.conf"
      wg-quick up "$iface" 2>/dev/null || true
      systemctl enable "wg-quick@${iface}" >/dev/null 2>&1 || true
    elif [[ "$ROLE" == foreign ]]; then
      wg-quick up "$iface" 2>/dev/null || true
      systemctl enable "wg-quick@${iface}" >/dev/null 2>&1 || true
    fi
    i=$((i+1))
  done
  persist_firewall
  if [[ "$ROLE" == foreign || "$ROLE" == both ]]; then
    for iface in "${INTERFACES[@]}"; do wg-quick up "$iface" 2>/dev/null || true; systemctl enable "wg-quick@${iface}" >/dev/null 2>&1 || true; done
  fi
  chmod 600 "$STATE_DIR"/* "$WG_DIR"/*.conf 2>/dev/null || true
  echo
  log "Installation complete."
  echo "Configs are in: $STATE_DIR"
  echo "Server config: $WG_DIR/${INTERFACES[0]}.conf"
  echo "Client config: $STATE_DIR/${INTERFACES[0]}-client.conf"
  echo "If this VPS is the foreign server, import the client config on the Iran-side VPS."
  command_exists qrencode && qrencode -t ansiutf8 <"$STATE_DIR/${INTERFACES[0]}-client.conf" || true
}

status(){
  need_root
  echo "Hamara Tunnel ${VERSION}"
  for f in "$WG_DIR"/*.conf; do [[ -e "$f" ]] || continue; iface=$(basename "$f" .conf); echo; wg show "$iface" 2>/dev/null || systemctl status "wg-quick@${iface}" --no-pager || true; done
  echo; sysctl net.ipv4.ip_forward net.ipv6.conf.all.forwarding
}

doctor(){
  need_root
  local fail=0
  for c in wg ip iptables; do command_exists "$c" && echo "OK: $c" || { echo "MISSING: $c"; fail=1; }; done
  [[ "$(sysctl -n net.ipv4.ip_forward)" == 1 ]] && echo "OK: IPv4 forwarding" || { echo "FAIL: IPv4 forwarding"; fail=1; }
  ip route show default | head -1
  wg show 2>&1 || true
  return "$fail"
}

uninstall(){
  need_root
  warn "This removes Hamara Tunnel configs and interfaces, but does not uninstall WireGuard packages."
  local c; c=$(ask "Type REMOVE to continue")
  [[ "$c" == REMOVE ]] || exit 0
  for f in "$WG_DIR"/*.conf; do [[ -e "$f" ]] || continue; wg-quick down "${f%.conf}" 2>/dev/null || true; done
  rm -rf "$STATE_DIR" /etc/sysctl.d/99-hamara-tunnel.conf
  sysctl --system >/dev/null 2>&1 || true
  log "Removed Hamara Tunnel files. Review firewall rules manually if required."
}

health(){
  need_root; local rc=0
  echo "== Hamara health =="
  command_exists wg || { echo "FAIL: wireguard-tools missing"; rc=1; }
  [[ -d "$WG_DIR" ]] || { echo "FAIL: /etc/wireguard missing"; rc=1; }
  [[ "$(sysctl -n net.ipv4.ip_forward 2>/dev/null || echo 0)" == 1 ]] || { echo "WARN: IPv4 forwarding disabled"; rc=1; }
  shopt -s nullglob
  local files=("$WG_DIR"/*.conf)
  ((${#files[@]})) || { echo "WARN: no WireGuard configs found"; rc=1; }
  local f iface
  for f in "${files[@]}"; do iface=$(basename "$f" .conf); echo "-- $iface --"; wg show "$iface" 2>&1 || rc=1; done
  return "$rc"
}

audit(){
  need_root; echo "== Hamara security audit =="
  for f in "$STATE_DIR" "$WG_DIR"; do [[ -d "$f" ]] && stat -c '%A %U:%G %n' "$f"; done
  find "$STATE_DIR" "$WG_DIR" -maxdepth 1 -type f -perm /077 -printf 'WEAK PERMISSIONS: %m %p\\n' 2>/dev/null || true
  echo "-- listening UDP sockets --"; ss -lunp || true
  echo "-- forwarding --"; sysctl net.ipv4.ip_forward net.ipv6.conf.all.forwarding
  echo "-- default route --"; ip route show default
}

backup(){
  need_root; local dest="${1:-/root/hamara-backups}"; mkdir -p "$dest"; chmod 700 "$dest"
  local out="$dest/hamara-$(hostname -s)-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
  tar --ignore-failed-read --exclude='*.log' -czf "$out" "$STATE_DIR" "$WG_DIR" /etc/sysctl.d/99-hamara-tunnel.conf 2>/dev/null || true
  chmod 600 "$out"; echo "Created root-only backup: $out"
}

mtu(){
  need_root; local host="${1:-10.77.0.1}" size
  echo "Testing authorized path to $host"
  for size in 1400 1380 1360 1320 1280; do
    if ping -c 1 -W 1 -M do -s "$size" "$host" >/dev/null 2>&1; then echo "PASS payload=$size mtu>=$((size+28))"; return 0; else echo "FAIL payload=$size"; fi
  done
  return 1
}

gost_forward(){
  need_root
  command_exists gost || die "GOST is not installed. Install a verified binary from https://github.com/go-gost/gost/releases"
  local local_port target
  local_port=$(ask_default "Local loopback port for GOST forwarding" "18080")
  target=$(ask_default "Authorized destination host:port" "10.77.0.1:8080")
  [[ "$local_port" =~ ^[0-9]+$ && "$local_port" -ge 1 && "$local_port" -le 65535 ]] || die "Invalid local port"
  [[ "$target" =~ ^[A-Za-z0-9_.:-]+:[0-9]+$ ]] || die "Use destination in host:port form"
  cat >/etc/systemd/system/hamara-gost-forward.service <<EOF
[Unit]
Description=Hamara GOST local TCP forward
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$(command -v gost) -L tcp://127.0.0.1:${local_port}/${target}
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
  chmod 644 /etc/systemd/system/hamara-gost-forward.service
  systemctl daemon-reload
  systemctl enable --now hamara-gost-forward.service
  echo "GOST forward active: 127.0.0.1:${local_port} -> ${target}"
}

install_command(){
  need_root
  local source; source=$(readlink -f "$0")
  mkdir -p /usr/local/lib/hamara-tunnel
  install -m 0755 "$source" /usr/local/lib/hamara-tunnel/hamara
  cat >/usr/local/bin/hamara <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
exec sudo /usr/local/lib/hamara-tunnel/hamara "$@"
EOF
  chmod 0755 /usr/local/bin/hamara
  echo "Global command installed: hamara"
}

usage(){ cat <<EOF
Hamara Tunnel ${VERSION}
Usage: $0 [install|status|doctor|health|audit|backup|mtu|gost|uninstall]

install  Interactive WireGuard site-to-site setup and install the global hamara command
status   Show interfaces, peers and forwarding state
doctor   Check required tools and forwarding
health   Show active interfaces and peer health
audit    Audit permissions, sockets, routes and forwarding
backup   Create a root-only encrypted-key archive
mtu      Test path MTU against a tunnel address
gost     Create a loopback-only GOST TCP forward as a systemd service
uninstall Remove Hamara-generated files and interfaces
EOF
}

main(){ local cmd="${1:-install}"; case "$cmd" in install) install_tunnel; install_command;; status) status;; doctor) doctor;; health) health;; audit) audit;; backup) backup "${2:-/root/hamara-backups}";; mtu) mtu "${2:-10.77.0.1}";; gost) gost_forward;; uninstall) uninstall;; -h|--help) usage;; *) usage; exit 2;; esac; }
main "$@"

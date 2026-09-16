package render

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/dev-penhan/Hamara-tunnel/internal/model"
)

func WireGuard(cfg model.Config, privateKey, presharedKey, peerPublicKey string) (string, error) {
	if cfg.Role != model.RoleIran && cfg.Role != model.RoleOutside {
		return "", fmt.Errorf("invalid role %q", cfg.Role)
	}
	address := cfg.GatewayAddress
	if cfg.Role == model.RoleIran {
		address = cfg.EdgeAddress
	}
	var b strings.Builder
	b.WriteString("# Managed by Hamara Tunnel. Manual changes may be overwritten.\n")
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "Address = %s\n", address)
	fmt.Fprintf(&b, "PrivateKey = %s\n", strings.TrimSpace(privateKey))
	fmt.Fprintf(&b, "MTU = %d\n", cfg.MTU)
	b.WriteString("SaveConfig = false\n")
	if cfg.Role == model.RoleOutside {
		fmt.Fprintf(&b, "ListenPort = %d\n", cfg.WireGuardPort)
	}
	if cfg.Role == model.RoleIran {
		b.WriteString("Table = off\n")
		fmt.Fprintf(&b, "PostUp = ip route replace default dev %%i metric 10 table %d\n", cfg.RouteTable)
		fmt.Fprintf(&b, "PostDown = ip route del default dev %%i table %d 2>/dev/null || true\n", cfg.RouteTable)
	}
	if strings.TrimSpace(peerPublicKey) == "" {
		return b.String(), nil
	}
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", strings.TrimSpace(peerPublicKey))
	if strings.TrimSpace(presharedKey) != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", strings.TrimSpace(presharedKey))
	}
	if cfg.Role == model.RoleOutside {
		fmt.Fprintf(&b, "AllowedIPs = %s/32\n", hostAddress(cfg.EdgeAddress))
	} else {
		b.WriteString("AllowedIPs = 0.0.0.0/0\n")
		if cfg.Mode == model.ModeARA {
			fmt.Fprintf(&b, "Endpoint = 127.0.0.1:%d\n", cfg.LocalRelayPort)
		} else {
			fmt.Fprintf(&b, "Endpoint = %s:%d\n", formatHost(cfg.EndpointHost), cfg.EndpointPort)
		}
		b.WriteString("PersistentKeepalive = 25\n")
	}
	return b.String(), nil
}

func ARAServerUnit(cfg model.Config) string {
	return fmt.Sprintf(`[Unit]
Description=Hamara ARA TLS transport server
Documentation=https://github.com/dev-penhan/Hamara-tunnel
After=network-online.target wg-quick@%s.service
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/wstunnel server --log-lvl WARN --restrict-to 127.0.0.1:%d --restrict-http-upgrade-path-prefix %s --tls-certificate %s --tls-private-key %s --tls-client-ca-certs %s wss://0.0.0.0:%d
Restart=always
RestartSec=3s
LimitNOFILE=1048576
NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
ProtectSystem=strict
ReadOnlyPaths=%s %s %s

[Install]
WantedBy=multi-user.target
`, cfg.Interface, cfg.WireGuardPort, shellArg(cfg.ARAPath), shellArg(cfg.TLSCertPath), shellArg(cfg.TLSKeyPath), shellArg(cfg.MTLSCAPath), cfg.EndpointPort, shellArg(cfg.TLSCertPath), shellArg(cfg.TLSKeyPath), shellArg(cfg.MTLSCAPath))
}

func ARAClientUnit(cfg model.Config) string {
	verify := ""
	if cfg.TLSVerify {
		verify = " --tls-verify-certificate"
	}
	url := fmt.Sprintf("wss://%s:%d", formatHost(cfg.EndpointHost), cfg.EndpointPort)
	forward := fmt.Sprintf("udp://127.0.0.1:%d:127.0.0.1:%d?timeout_sec=0", cfg.LocalRelayPort, cfg.WireGuardPort)
	return fmt.Sprintf(`[Unit]
Description=Hamara ARA TLS transport client
Documentation=https://github.com/dev-penhan/Hamara-tunnel
After=network-online.target
Wants=network-online.target
Before=wg-quick@%s.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/wstunnel client --log-lvl WARN --connection-retry-max-backoff 30s --websocket-ping-frequency 20s --dns-resolver-prefer-ipv4 -L %s -P %s --tls-certificate %s --tls-private-key %s%s %s
Restart=always
RestartSec=3s
LimitNOFILE=1048576
NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
ProtectSystem=strict
ReadOnlyPaths=%s %s

[Install]
WantedBy=multi-user.target
`, cfg.Interface, shellArg(forward), shellArg(cfg.ARAPath), shellArg(cfg.MTLSClientCertPath), shellArg(cfg.MTLSClientKeyPath), verify, shellArg(url), shellArg(cfg.MTLSClientCertPath), shellArg(cfg.MTLSClientKeyPath))
}

func FirewallScript(cfg model.Config) string {
	var inputUp, inputDown string
	switch cfg.Mode {
	case model.ModeARA:
		inputUp = fmt.Sprintf(`ipt_check -I INPUT -p tcp --dport %d -j ACCEPT
# ARA's inner WireGuard listener is local-only by policy. Block direct WAN UDP.
ipt_check -I INPUT -i "$WAN" -p udp --dport %d -j DROP`, cfg.EndpointPort, cfg.WireGuardPort)
		inputDown = fmt.Sprintf(`ipt_del INPUT -i "$WAN" -p udp --dport %d -j DROP
ipt_del INPUT -p tcp --dport %d -j ACCEPT`, cfg.WireGuardPort, cfg.EndpointPort)
	case model.ModeWireGuard:
		inputUp = fmt.Sprintf(`ipt_check -I INPUT -p udp --dport %d -j ACCEPT`, cfg.EndpointPort)
		inputDown = fmt.Sprintf(`ipt_del INPUT -p udp --dport %d -j ACCEPT`, cfg.EndpointPort)
	case model.ModeIPIP:
		inputUp = `ipt_check -I INPUT -p 4 -j ACCEPT`
		inputDown = `ipt_del INPUT -p 4 -j ACCEPT`
	}
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
ACTION="${1:-up}"
WAN=%s
IFACE=%s
CIDR=%s

ipt_check() {
  local table="filter"
  if [[ "$1" == "-t" ]]; then table="$2"; shift 2; fi
  local op="$1"; shift
  local check="-C"
  [[ "$op" == "-A" || "$op" == "-I" ]] || { echo "bad iptables op" >&2; exit 2; }
  iptables -t "$table" "$check" "$@" 2>/dev/null || iptables -t "$table" "$op" "$@"
}
ipt_del() {
  local table="filter"
  if [[ "$1" == "-t" ]]; then table="$2"; shift 2; fi
  local chain="$1"; shift
  while iptables -t "$table" -C "$chain" "$@" 2>/dev/null; do
    iptables -t "$table" -D "$chain" "$@"
  done
}

if [[ "$ACTION" == "up" ]]; then
  %s
  ipt_check -I FORWARD -i "$IFACE" -j ACCEPT
  ipt_check -I FORWARD -o "$IFACE" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  ipt_check -t nat -A POSTROUTING -s "$CIDR" -o "$WAN" -j MASQUERADE
else
  %s
  ipt_del FORWARD -i "$IFACE" -j ACCEPT
  ipt_del FORWARD -o "$IFACE" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  ipt_del -t nat POSTROUTING -s "$CIDR" -o "$WAN" -j MASQUERADE
fi
`, shellArg(cfg.WANInterface), shellArg(cfg.Interface), shellArg(cfg.TunnelCIDR), inputUp, inputDown)
}

func FirewallUnit() string {
	return `[Unit]
Description=Hamara Tunnel forwarding and NAT rules
After=network-online.target ufw.service nftables.service
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/lib/hamara/firewall up
ExecStop=/usr/local/lib/hamara/firewall down
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
}

const XrayMark = 72

func GuardScript(cfg model.Config) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
ACTION="${1:-up}"
TABLE=%d
SOURCE=%s
IFACE=%s
MARK=%d

ipt_add() {
  iptables -w 5 -C "$@" 2>/dev/null || iptables -w 5 -I "$@"
}
ipt_del() {
  while iptables -w 5 -C "$@" 2>/dev/null; do
    iptables -w 5 -D "$@"
  done
}
ip6_add() {
  command -v ip6tables >/dev/null 2>&1 || return 0
  ip6tables -w 5 -C "$@" 2>/dev/null || ip6tables -w 5 -I "$@"
}
ip6_del() {
  command -v ip6tables >/dev/null 2>&1 || return 0
  while ip6tables -w 5 -C "$@" 2>/dev/null; do
    ip6tables -w 5 -D "$@"
  done
}
del_rules() {
  while ip rule del fwmark "$MARK"/0xffffffff lookup "$TABLE" priority 109 2>/dev/null; do :; done
  while ip rule del from "$SOURCE"/32 lookup "$TABLE" priority 110 2>/dev/null; do :; done
}

if [[ "$ACTION" == "up" ]]; then
  # The unreachable fallback remains when the tunnel device disappears. This
  # prevents Linux policy routing from falling through to the normal WAN table.
  ip route replace unreachable default metric 32767 table "$TABLE"
  del_rules
  ip rule add fwmark "$MARK"/0xffffffff lookup "$TABLE" priority 109
  ip rule add from "$SOURCE"/32 lookup "$TABLE" priority 110
  ipt_add OUTPUT -m mark --mark "$MARK"/0xffffffff ! -o "$IFACE" -j REJECT --reject-with icmp-net-unreachable
  ipt_add OUTPUT -s "$SOURCE"/32 ! -o "$IFACE" -j REJECT --reject-with icmp-net-unreachable
  # Version 0.2 has no IPv6 tunnel. Any accidentally marked IPv6 flow fails closed.
  ip6_add OUTPUT -m mark --mark "$MARK"/0xffffffff -j REJECT --reject-with icmp6-no-route
else
  ipt_del OUTPUT -m mark --mark "$MARK"/0xffffffff ! -o "$IFACE" -j REJECT --reject-with icmp-net-unreachable
  ipt_del OUTPUT -s "$SOURCE"/32 ! -o "$IFACE" -j REJECT --reject-with icmp-net-unreachable
  ip6_del OUTPUT -m mark --mark "$MARK"/0xffffffff -j REJECT --reject-with icmp6-no-route
  del_rules
  ip route flush table "$TABLE" 2>/dev/null || true
fi
`, cfg.RouteTable, shellArg(hostAddress(cfg.EdgeAddress)), shellArg(cfg.Interface), XrayMark)
}

func GuardUnit() string {
	return `[Unit]
Description=Hamara strict egress guard and fail-closed policy table
After=network-online.target
Wants=network-online.target
Before=wg-quick@hamara0.service hamara-ipip.service

[Service]
Type=oneshot
ExecStart=/usr/local/lib/hamara/guard up
ExecStop=/usr/local/lib/hamara/guard down
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
}

func WireGuardGuardDropIn() string {
	return `[Unit]
Requires=hamara-guard.service
After=hamara-guard.service
`
}

func IPIPScript(cfg model.Config) string {
	local := cfg.GatewayAddress
	remote := cfg.EdgeAddress
	localEP := cfg.EndpointHost
	remoteEP := cfg.PeerEndpoint
	if cfg.Role == model.RoleIran {
		local, remote = cfg.EdgeAddress, cfg.GatewayAddress
		localEP, remoteEP = cfg.PeerEndpoint, cfg.EndpointHost
	}
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
ACTION="${1:-up}"
IFACE=%s
LOCAL_EP=%s
REMOTE_EP=%s
LOCAL_ADDR=%s
PEER_ADDR=%s

if [[ "$ACTION" == "up" ]]; then
  ip tunnel del "$IFACE" 2>/dev/null || true
  ip tunnel add "$IFACE" mode ipip local "$LOCAL_EP" remote "$REMOTE_EP" ttl 64
  ip addr add "$LOCAL_ADDR" peer "$PEER_ADDR" dev "$IFACE"
  ip link set dev "$IFACE" mtu %d up
`, shellArg(cfg.Interface), shellArg(localEP), shellArg(remoteEP), shellArg(local), shellArg(hostAddress(remote)), cfg.MTU) + func() string {
		if cfg.Role == model.RoleIran {
			return fmt.Sprintf("  ip route replace default dev \"$IFACE\" metric 10 table %d\n", cfg.RouteTable)
		}
		return ""
	}() + `else
` + func() string {
		if cfg.Role == model.RoleIran {
			return fmt.Sprintf("  ip route del default dev \"$IFACE\" table %d 2>/dev/null || true\n", cfg.RouteTable)
		}
		return ""
	}() + `  ip tunnel del "$IFACE" 2>/dev/null || true
fi
`
}

func IPIPUnit(role string) string {
	guard := ""
	if role == model.RoleIran {
		guard = "Requires=hamara-guard.service\nAfter=hamara-guard.service\n"
	}
	return fmt.Sprintf(`[Unit]
Description=Hamara IPIP routed tunnel
After=network-online.target
Wants=network-online.target
%sBefore=hamara-firewall.service

[Service]
Type=oneshot
ExecStart=/usr/local/lib/hamara/ipip up
ExecStop=/usr/local/lib/hamara/ipip down
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`, guard)
}

func SSHClientUnit(cfg model.Config) string {
	host := cfg.EndpointHost
	return fmt.Sprintf(`[Unit]
Description=Hamara SSH SOCKS tunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/bin/ssh -NT -D 127.0.0.1:%d -p %d -i /etc/hamara/secrets/ssh_ed25519 -o BatchMode=yes -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/etc/hamara/ssh_known_hosts -o ServerAliveInterval=20 -o ServerAliveCountMax=3 -o ConnectTimeout=10 %s@%s
Restart=always
RestartSec=3s
NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
ProtectSystem=strict
ReadOnlyPaths=/etc/hamara/secrets/ssh_ed25519 /etc/hamara/ssh_known_hosts

[Install]
WantedBy=multi-user.target
`, cfg.SOCKSPort, cfg.SSHPort, shellArg(cfg.SSHUser), shellArg(host))
}

func XrayOutbound(cfg model.Config) ([]byte, error) {
	var obj map[string]any
	if cfg.Mode == model.ModeSSHSocks {
		obj = map[string]any{
			"tag":      "hamara-egress",
			"protocol": "socks",
			"settings": map[string]any{"servers": []any{map[string]any{
				"address": "127.0.0.1", "port": cfg.SOCKSPort,
			}}},
		}
	} else {
		obj = map[string]any{
			"tag":         "hamara-egress",
			"protocol":    "freedom",
			"sendThrough": hostAddress(cfg.EdgeAddress),
			"settings":    map[string]any{},
			"streamSettings": map[string]any{
				"sockopt": map[string]any{
					"interface":      cfg.Interface,
					"mark":           XrayMark,
					"domainStrategy": "UseIPv4",
					"tcpFastOpen":    true,
				},
			},
		}
	}
	return json.MarshalIndent(obj, "", "  ")
}

func XrayRoutingAll() []byte {
	obj := map[string]any{
		"type":        "field",
		"network":     "tcp,udp",
		"outboundTag": "hamara-egress",
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return b
}

func XrayRoutingInbound(tags []string) []byte {
	obj := map[string]any{
		"type":        "field",
		"inboundTag":  tags,
		"outboundTag": "hamara-egress",
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return b
}

func KnownHosts(host string, port int, hostKey string) string {
	label := host
	if port != 22 {
		label = "[" + host + "]:" + strconv.Itoa(port)
	}
	return label + " " + strings.TrimSpace(hostKey) + "\n"
}

func Sysctl() string {
	return `# Managed by Hamara Tunnel
net.ipv4.ip_forward = 1
# Loose reverse-path filtering tolerates routed point-to-point links while
# retaining basic source validation.
net.ipv4.conf.all.rp_filter = 2
net.ipv4.conf.default.rp_filter = 2
`
}

func hostAddress(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

func formatHost(host string) string {
	if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// shellArg emits a systemd/shell-safe single token for generated values.
// Inputs are validated by the CLI; this extra quoting avoids accidental splits.
func shellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

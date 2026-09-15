# Hamara Tunnel — Production Guide

Hamara Tunnel is a modular toolkit for authorized VPS-to-VPS networking. It provisions standard, auditable Linux tunnel technologies and separates transport, routing, firewalling, monitoring, and 3x-ui integration.

## Scope and safety

This project is for connecting infrastructure that you administer. WireGuard and OpenVPN provide encryption. GRE and IPIP are **not encrypted** and must only be used inside an already protected network or together with an encrypted layer. No protocol camouflage, traffic impersonation, or DPI-evasion code is included.

## What Hamara Tunnel does

Hamara Tunnel is an operations wrapper around standard Linux networking. It creates a private tunnel interface, assigns a non-overlapping address range, enables only the forwarding features selected during setup, applies narrowly-scoped NAT rules on the exit host, and persists the interface with systemd. It does not magically turn every application into a full-tunnel client: routing and `AllowedIPs` decide what crosses the link.

A healthy deployment has four independent layers:

1. **Transport:** WireGuard handshake or a GOST service connection.
2. **Interface:** tunnel address is up and present in `ip addr`.
3. **Routing:** `ip route get DESTINATION` selects the intended path.
4. **Policy:** forwarding, NAT, provider firewall, and application listener allow the traffic.

Check these layers in order. Do not change 3x-ui while layer 1 or 2 is failing.

## Project layout

```text
hamara-tunnel/
├── hamara-tunnel.sh          # interactive WireGuard installer
├── hamara-tunnel.sh          # unified installer and operations CLI
├── scripts/
│   ├── create-ip-tunnel.sh    # authorized GRE/IPIP helper
│   └── healthcheck.sh         # non-destructive tunnel checks
├── docs/
│   ├── 3x-ui.en.md            # separate 3x-ui v3.8.x integration guide
│   ├── 3x-ui.fa.md
│   ├── gost.en.md              # optional GOST v3 integration
│   ├── gost.fa.md
│   └── gost-forward.example.yaml
├── README.en.md
├── README.fa.md
└── config/hamara.example.env
```

## Current compatibility notes

- **3x-ui:** the separate guide is aligned with the latest upstream release available while this documentation was updated: **v3.8.0**, published on 2026-09-14. Always review upstream release notes before production upgrades.
- **GOST:** use the official `go-gost/gost` project and its v3 documentation. The legacy `ginuerzh/gost` repository is maintenance-only and is not the source used by this project.
- **WireGuard:** remains the default for encrypted routed networking.

## Supported designs

### 1. WireGuard Standard
Encrypted Layer-3 tunnel. Best default for most deployments.

### 2. WireGuard Full Tunnel
The Iran-side host sends selected networks or the default route to the foreign-side exit. Start with split tunneling, validate connectivity, then consider a full route.

### 3. Hamara Multi-Tunnel
Two independent WireGuard interfaces with separate ports and tunnel subnets. Use one for primary traffic and one for controlled failover or a separate application route. Do not create routing loops.

### 4. ARA IP-forwarding profile
A dedicated `ara0` WireGuard interface with explicit forwarding, NAT, MTU, keepalive, and health checks. ARA is a routing profile, not a stealth protocol.

### 5. GRE and IPIP helpers
The helper in `scripts/create-ip-tunnel.sh` creates standard Linux GRE or IPIP interfaces. These transports are useful for routed networks you own, but they are cleartext. Put them inside WireGuard or another approved encrypted layer when confidentiality is required.

### 6. OpenVPN fallback
OpenVPN can be installed and configured separately when a legacy environment requires it. Keep it as a compatibility option, not the first choice for a new deployment.

## Installation

The repository is maintained at `https://github.com/dev-penhan/Hamara-tunnel`.

### Install with Git

```bash
git clone https://github.com/dev-penhan/Hamara-tunnel hamara-tunnel
cd hamara-tunnel
chmod +x hamara-tunnel.sh
sudo ./hamara-tunnel.sh install
```

After installation, the unified command is available globally:

```bash
hamara
hamara status
hamara doctor
```

The `hamara` launcher requests root privileges internally when needed; users do not need to type `sudo hamara`.

### Install directly from a raw script

```bash
curl -fsSL https://raw.githubusercontent.com/dev-penhan/Hamara-tunnel/main/hamara-tunnel.sh -o /tmp/hamara
chmod +x /tmp/hamara
sudo /tmp/hamara install
```

On both authorized VPSs, select the same profile and address plan. On the foreign VPS select `Foreign-side / exit VPS`; on the Iran-side VPS select `Iran-side / client-side VPS`.

Open the selected UDP port in both the cloud security group and the local firewall. Default ports are:

- `wg0`: UDP 51820
- `wg1`: UDP 51821
- `ara0`: UDP 51830

## Production checklist

1. Use Debian or Ubuntu with current security updates.
2. Disable password SSH authentication and use a separate admin account.
3. Restrict the WireGuard UDP port by source IP where practical.
4. Use split routes first; only then test a default route.
5. Keep tunnel subnets unique and non-overlapping.
6. Set an explicit MTU if the path has encapsulation or packet loss.
7. Back up configuration without exposing private keys.
8. Monitor handshake freshness and transfer counters.
9. Do not route the WireGuard endpoint itself into the tunnel.
10. Test rollback before making the tunnel a production dependency.

## Operations

```bash
sudo ./hamara-tunnel.sh status
sudo ./hamara-tunnel.sh doctor
hamara health
hamara audit
hamara backup /root/hamara-backups
```

The backup command creates a root-readable archive and excludes shell history. Treat the resulting archive as a secret because it contains private keys.

To create a loopback-only GOST TCP forward to an authorized service reachable through WireGuard:

```bash
hamara gost
```

The command verifies that GOST exists, writes a systemd service, and defaults to a loopback listener. It does not create a public open proxy. See `docs/gost.en.md` for the official GOST v3 guidance.

To test a route without changing configuration:

```bash
ip route get 1.1.1.1
ping -c 3 10.77.0.1
sudo wg show
```

## MTU guidance

The default WireGuard MTU is commonly 1420, but the correct value depends on the underlay. If large packets fail while small pings work, test a lower value such as 1380 or 1360 on both sides. Do not blindly set 1280; measure and document the change.

## Firewall model

The foreign side needs:

- UDP access to the WireGuard listen port
- IP forwarding enabled
- Forwarding from the tunnel interface to the WAN interface
- Masquerading only for the tunnel networks that need internet egress

Avoid broad `ACCEPT` rules in a shared production host. Replace them with source, destination, and interface-specific rules after the first successful test.

## Troubleshooting decision tree

Run these commands in order and save the output before changing anything:

```bash
sudo hamara doctor
sudo hamara status
ip -br addr
ip route
sudo wg show
ss -lunp
```

- **No interface:** the service/config has not loaded. Run `systemctl status wg-quick@wg0` and inspect the journal.
- **Interface exists, no handshake:** check endpoint, public keys, UDP provider firewall, local firewall, clock, and keepalive.
- **Handshake exists, no tunnel ping:** check tunnel addresses, `AllowedIPs`, route overlap, and input/forwarding policy.
- **Tunnel ping works, no internet:** check foreign default route, IP forwarding, NAT, return route, and DNS separately.
- **Only 3x-ui fails:** leave Hamara unchanged; check Xray listener bind address, inbound status, Xray logs, and the selected outbound route.
- **Intermittent transfer:** test MTU, inspect packet loss, check CPU limits, and compare `wg show` transfer counters.

### No handshake

Check the provider firewall, UDP port, endpoint address, system clock, public keys, and `PersistentKeepalive`. Use `sudo wg show` on both hosts.

### Handshake exists but no traffic

Check `AllowedIPs`, tunnel addresses, overlapping routes, forwarding sysctls, and host firewall policy. Test tunnel IPs before testing internet destinations.

### Full tunnel breaks SSH

Restore the last working config or use split routes. Add a management route through the regular WAN path before enabling a default route.

### Internet works but DNS fails

Configure a resolver explicitly. The tunnel is not a DNS service.

### 3x-ui does not work

Verify the tunnel separately, inspect Xray logs, confirm the inbound listener, then apply route or interface selection. See `docs/3x-ui.en.md`.

## Configuration locations

```text
/etc/wireguard/*.conf
/etc/hamara-tunnel/*
/etc/sysctl.d/99-hamara-tunnel.conf
/var/log/hamara-tunnel.log
```

Never commit generated key files. The included `.gitignore` protects common generated names.

## License

Add the license that matches your intended distribution before publishing this repository.

## Review of external tunnel projects

The following projects were reviewed as requested:

- [paqet-tunnel](https://github.com/g3ntrix/paqet-tunnel) uses raw-packet tunneling and explicitly targets bypassing network restrictions. It is not copied into Hamara because raw-packet handling and DPI-bypass behavior need a separate threat model, kernel/provider compatibility testing, and can create operational and legal risk.
- [Pahlavi-tunnel](https://github.com/Zehnovik/Pahlavi-tunnel) is a reverse TCP tunnel manager with multi-slot configuration, synchronization, health checks, systemd, and forwarding helpers. Its useful operational ideas—health checks, restart policy, slot isolation, and resource limits—are reflected in Hamara's design, but its code is not vendored.

Hamara favors smaller, auditable components and does not promise that any tunnel is invisible to inspection. Test every transport on infrastructure you administer and document the result.

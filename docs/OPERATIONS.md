# Hamara Tunnel operations runbook

## Daily checks

```bash
hamara status
systemctl --failed
```

For WireGuard-based modes, confirm the latest handshake is recent while traffic is expected. A quiet tunnel may legitimately have an old handshake.

## End-to-end check

On the Iran edge:

```bash
hamara doctor
```

The egress address should be the outside gateway's public IPv4 address. `hamara doctor` must also report an active strict guard and an unreachable fallback route. Alert if egress becomes the Iran VPS address or a marked request unexpectedly succeeds after the tunnel is stopped.

During a maintenance window:

```bash
hamara leak-test
```

The command stops only the routed transport, leaves `hamara-guard` active, verifies fail-closed routing, and restores the transport.

## Logs

```bash
sudo journalctl -u 'hamara-*' -u 'wg-quick@hamara0' --since today
```

Keep logs long enough to diagnose flapping, but understand that timing and destination errors can be sensitive metadata.

## Capacity

Monitor:

```bash
ip -s link show hamara0
sudo wg show hamara0 transfer
top
ss -s
```

ARA has both encryption and TLS encapsulation overhead. Benchmark direct WireGuard first to separate path limits from ARA transport limits.

## Firewall reloads

UFW or another manager may rebuild iptables after Hamara starts. Reapply the dedicated rules:

```bash
hamara repair
```

On the outside this reapplies forwarding/NAT. On a routed Iran edge it reapplies the mark/source kill switch and unreachable fallback. Then rerun `hamara doctor`.

## Certificate renewal

For Let's Encrypt:

```bash
sudo certbot renew --dry-run
sudo systemctl status certbot.timer
```

wstunnel reloads certificate files when they change. Confirm the served certificate after renewal:

```bash
openssl s_client -connect tunnel.example.com:443 -servername tunnel.example.com </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates
```

## Planned maintenance

1. Keep two root SSH sessions open to each VPS.
2. Back up 3x-ui.
3. Record `hamara status` and current egress IP.
4. Apply updates.
5. Restart one side at a time, outside first.
6. Run `hamara doctor` before reopening client traffic.
7. Run `hamara leak-test` on the Iran edge.
8. Test one TCP and one UDP-capable 3x-ui client, including tunnel-down behavior, when using a routed mode.

## Key rotation

Version 0.2 rotates by replacing the link:

```bash
hamara uninstall
```

Run this on both ends, reinstall if needed, initialize the outside gateway, and complete a new offer/response exchange.

## Backups

Back up the project source and 3x-ui state. Avoid long-lived backups of `/etc/hamara/secrets`, `/etc/hamara/mtls`, `/etc/wireguard/hamara0.conf`, and OFFER tokens; a clean pairing after restore gives better key hygiene. Use `hamara paths` for the complete on-host inventory.

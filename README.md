# Hamara Tunnel

[فارسی](README_FA.md) · [Repository](https://github.com/dev-penhan/Hamara-tunnel)

**Hamara Tunnel** is an open-source, two-server routing manager for Ubuntu VPSs. It links an **Iran edge server** to an **outside gateway**, then gives services such as 3x-ui/Xray a controlled egress path through the outside server.

The command is intentionally interactive: on first run it asks whether the machine is the **Iran server** or the **outside server**, chooses a transport, creates systemd services, exchanges short pairing tokens, enables the required Linux routing, and provides ready-to-paste Xray JSON.

> **Important reality check:** no tunnel is “undetectable.” Hamara does not promise invisibility, universal connectivity, or guaranteed DPI bypass. ARA makes the outer transport look like ordinary TLS/WebSocket traffic and avoids exposing WireGuard directly, which can reduce recognition by simple protocol classifiers. A capable censor can still use endpoint blocking, active probing, TLS metadata, timing, volume, and traffic analysis. Use only where lawful and authorized.

## Current release

- Version: `0.2.0`
- Target OS: **Ubuntu 22.04 LTS and 24.04 LTS**
- Release architectures: static Linux `amd64` and `arm64` binaries
- Networking: IPv4 routed payloads in version 0.2; marked IPv6 is rejected fail-closed
- Init system: systemd
- Firewall backend: `iptables` (Ubuntu's nftables-compatible backend is supported)
- Strict Iran-side egress guard: socket mark `72`, source guard, and unreachable fallback route
- ARA authentication: WireGuard keys + PSK + TLS + mandatory per-instance mutual TLS
- One Iran edge and one outside gateway per installation

## How it works

```text
Client
  |
  v
3x-ui / Xray inbound on the Iran VPS
  |
  |  Xray outbound tag: hamara-egress
  v
Linux source-policy route + hamara-guard kill switch
  |
  |  Xray mark 72; no WAN fallback is allowed
  v
hamara0 (routed point-to-point link)
  |
  v
Outside VPS -> kernel IP forwarding -> NAT -> Internet
```

Hamara does **not** replace 3x-ui. The panel continues to manage VLESS, VMess, Trojan, Shadowsocks, or other client-facing inbounds. Hamara supplies the server-to-server egress path underneath Xray.

## Included tunnel modes

| Mode | Encryption | TCP | UDP | Typical use | Main trade-off |
|---|---:|---:|---:|---|---|
| **ARA Routed TLS** | WireGuard + TLS + mTLS | Yes | Yes | Recommended when direct WireGuard is filtered | More overhead; WebSocket/TCP transport can reduce performance under packet loss |
| **WireGuard Direct** | WireGuard | Yes | Yes | Fastest general-purpose routed link | WireGuard UDP is directly visible and may be filtered |
| **IPIP Routed** | **None** | Yes | Yes | Very low-overhead trusted path or diagnostic fallback | Easily identified, IPv4 only, no confidentiality, often blocked by cloud firewalls/NAT |
| **SSH SOCKS** | SSH | Yes | **No** | TCP-only fallback where SSH is reachable | Not suitable for UDP, gaming, or QUIC |

### ARA Tunnel internals

ARA is Hamara's routed transport. It deliberately uses audited, existing components instead of inventing cryptography:

1. Xray binds the selected outbound to the Iran-side tunnel address and applies socket mark `72`.
2. `hamara-guard` sends that mark and source address only to a dedicated policy table.
3. The policy table always contains an `unreachable default`; the live tunnel adds a lower-metric route. If the tunnel disappears, Linux cannot fall through to the Iran WAN route.
4. IPv4 firewall guards reject marked/source-bound packets that try to leave any interface except `hamara0`; marked IPv6 is rejected because version 0.2 has no IPv6 tunnel.
5. WireGuard encrypts the layer-3 packets using public keys plus a per-link pre-shared key.
6. On the Iran server, WireGuard sends its UDP transport to a loopback listener.
7. `wstunnel` carries those UDP datagrams inside a TLS WebSocket connection to the outside server.
8. The outer ARA server also requires a unique P-256 client certificate (mutual TLS). Its certificate common name is bound to the random WebSocket path.
9. The outside `wstunnel` instance delivers accepted datagrams only to its loopback WireGuard listener.
10. The Linux kernel decrypts, IP-forwards, and NATs the traffic to the outside VPS's WAN interface.

On the public network, ARA exposes TCP/TLS—normally on port `443`—instead of a direct WireGuard UDP listener. A random, per-install WebSocket upgrade prefix and the matching mTLS client identity are required by the outside server. With a real domain and publicly trusted certificate, normal server-certificate verification is also enabled.

Self-signed mode is a fallback for IP-only deployments. In that mode, the Iran client does not verify the **outer server** certificate. Mandatory mTLS still authenticates the Iran client, and inner WireGuard still authenticates/encrypts routed packets, but an on-path attacker can identify, interrupt, or relay the outer connection. A real domain and valid certificate are strongly recommended.

## Requirements

### Both VPSs

- A fresh or known-good Ubuntu 22.04/24.04 server
- Root access
- A public IPv4 route
- systemd
- Correct date/time (`timedatectl status`)
- Provider firewall/security-group access for the chosen transport

### ARA recommended setup

- A DNS record such as `tunnel.example.com` pointing directly to the **outside** VPS
- TCP `443` allowed to the outside VPS
- TCP `80` temporarily allowed if Hamara should obtain a Let's Encrypt certificate
- No other outside-server service already bound to the selected ARA port

The Iran-side 3x-ui service may continue using port `443`; ARA's public listener is on the **outside** server.

## Installation

### From the official repository

Clone the project on each VPS, install it once, and then type `hamara` without `sudo`:

```bash
git clone https://github.com/dev-penhan/Hamara-tunnel.git
cd Hamara-tunnel
sudo ./install.sh
hamara
```

The CLI automatically asks `sudo` for root privileges when an operation needs them. `help` and `version` remain unprivileged. The login user must have sudo access; a root shell also works.

### One-line installation

```bash
curl -fsSL https://raw.githubusercontent.com/dev-penhan/Hamara-tunnel/main/scripts/quick-install.sh | sudo bash
hamara
```

For a fork:

```bash
curl -fsSL https://raw.githubusercontent.com/YOUR_ACCOUNT/hamara-tunnel/main/scripts/quick-install.sh \
  | sudo HAMARA_REPOSITORY=https://github.com/YOUR_ACCOUNT/hamara-tunnel.git HAMARA_VERSION=main bash
```

The installer builds the Go CLI, runs unit tests, and installs:

```text
/usr/local/bin/hamara
```

It does not configure a tunnel until `hamara` is run.

## Beginner quick start: ARA Tunnel

Pairing is intentionally offline: no management API or database is exposed to the Internet.

### 1. Prepare DNS

Create an `A` record such as:

```text
tunnel.example.com -> OUTSIDE_VPS_IPV4
```

For the simplest first deployment, use DNS-only/direct resolution. A reverse proxy or CDN adds another failure domain and is not required.

Verify from a third machine:

```bash
dig +short tunnel.example.com A
```

### 2. Start on the outside VPS

```bash
hamara
```

Choose:

```text
Which server is this?       Outside gateway server
Choose a tunnel mode        ARA Routed TLS
Public endpoint             tunnel.example.com
Public ARA TLS port         443
TLS certificate mode        Let's Encrypt
```

Hamara prints a long token beginning with:

```text
HAMARA1.OFFER.
```

The offer contains the WireGuard pre-shared key **and the one-peer ARA mTLS client private key**. Treat it like a password. Copy it through an authenticated channel and do not post it publicly. After the token is created, the outside server deletes the issued client key and its CA signing key; the root-only OFFER token is the transfer copy.

### 3. Join from the Iran VPS

```bash
hamara
```

Choose **Iran edge server**, then paste the OFFER token. Hamara configures the client and prints:

```text
HAMARA1.RESPONSE.
```

### 4. Accept on the outside VPS

Run `hamara`, choose **Accept a pairing response**, and paste the RESPONSE. This avoids putting the token in shell history.

For automation, transfer it as a root-only file and use:

```bash
chmod 600 /root/hamara-response.txt
hamara accept --file /root/hamara-response.txt
```

### 5. Verify both ends

On both VPSs:

```bash
hamara status
hamara doctor
```

A working ARA/WireGuard link shows a recent WireGuard handshake and transferred bytes. On the Iran server, `hamara doctor` also performs an HTTP request with the tunnel address as its source.

### 6. Configure 3x-ui on the Iran VPS

See [3x-ui / Xray configuration](#3x-ui--xray-configuration) below.

## Non-interactive setup

Interactive setup is best for a first installation. Automation uses the same pairing flow. Prefer `--file` with mode `0600`; passing a secret OFFER directly through `--offer` can expose it in shell history or a process listing.

### ARA with Let's Encrypt

Outside:

```bash
hamara init \
  --role outside \
  --mode ara \
  --endpoint tunnel.example.com \
  --port 443 \
  --tls acme \
  --email admin@example.com
```

Iran, after copying the OFFER to a root-only file:

```bash
chmod 600 /root/hamara-offer.txt
hamara join --file /root/hamara-offer.txt
```

Outside, after copying back the RESPONSE:

```bash
chmod 600 /root/hamara-response.txt
hamara accept --file /root/hamara-response.txt
```

### ARA with an existing certificate

```bash
hamara init \
  --role outside \
  --mode ara \
  --endpoint tunnel.example.com \
  --port 443 \
  --tls existing \
  --cert /etc/letsencrypt/live/tunnel.example.com/fullchain.pem \
  --key /etc/letsencrypt/live/tunnel.example.com/privkey.pem
```

`wstunnel` reloads changed certificate files. Certbot renewal can therefore continue normally.

### ARA without a domain

```bash
hamara init \
  --role outside \
  --mode ara \
  --endpoint 203.0.113.10 \
  --port 443 \
  --tls self-signed
```

This is an encrypted, WireGuard-authenticated fallback, but its outer certificate is not verified and its TLS appearance is less ordinary than a valid domain deployment.

### Direct WireGuard

```bash
hamara init \
  --role outside \
  --mode wireguard \
  --endpoint 203.0.113.10 \
  --port 51820
```

Allow UDP `51820` in the cloud firewall.

### IPIP

```bash
hamara init \
  --role outside \
  --mode ipip \
  --endpoint 203.0.113.10
```

IPIP requires protocol number `4`, public IPv4 addresses assigned directly to both VPS interfaces, and provider support. It does not work through ordinary NAT and is not encrypted.

### SSH SOCKS

```bash
hamara init \
  --role outside \
  --mode ssh-socks \
  --endpoint 203.0.113.10 \
  --port 22
```

Hamara creates a locked-down `hamara-proxy` account. The pairing key can open local TCP forwards but receives no interactive shell. The Iran-side SOCKS listener binds only to `127.0.0.1:10808`.

## 3x-ui / Xray configuration

> Configure 3x-ui on the **Iran edge**, after `hamara doctor` succeeds. Back up the panel database/config before changing Xray settings.

3x-ui labels can differ slightly by release. In current versions, outbounds and routing are under **Xray Settings** or **Xray Configuration**. The official 3x-ui documentation describes outbounds and top-to-bottom routing rules at <https://docs.sanaei.dev/docs/operations/outbounds-routing/>.

### Step 1: generate the exact JSON

On the Iran VPS:

```bash
hamara xray
```

For ARA, direct WireGuard, or IPIP, the generated outbound resembles:

```json
{
  "tag": "hamara-egress",
  "protocol": "freedom",
  "sendThrough": "10.73.0.2",
  "settings": {},
  "streamSettings": {
    "sockopt": {
      "interface": "hamara0",
      "mark": 72,
      "domainStrategy": "UseIPv4",
      "tcpFastOpen": true
    }
  }
}
```

Do not blindly copy `10.73.0.2` from this README for another mode. Use the output of `hamara xray`; it contains the installed link address.

SSH SOCKS produces:

```json
{
  "tag": "hamara-egress",
  "protocol": "socks",
  "settings": {
    "servers": [
      {
        "address": "127.0.0.1",
        "port": 10808
      }
    ]
  }
}
```

### Step 2: add the outbound

1. Sign in to 3x-ui on the Iran VPS.
2. Open **Xray Settings → Outbounds**.
3. Add a new outbound or switch the outbound editor to JSON.
4. Paste the single object printed under **Add this object to Xray Settings → Outbounds**.
5. Confirm that its tag is exactly `hamara-egress`.
6. Save, but do not delete the panel's `direct`, `block`, or `api` outbounds.

### Step 3A: route every client inbound through Hamara

Add this as a routing-rule object:

```json
{
  "type": "field",
  "network": "tcp,udp",
  "outboundTag": "hamara-egress"
}
```

Rule order is important. Xray uses the **first matching rule**.

- Keep the panel/API rule above this rule.
- Keep deliberate block rules above it.
- Keep any destinations that must remain direct above it.
- Place the Hamara catch-all rule near the bottom, but before an existing catch-all rule.
- For SSH SOCKS, use `"network": "tcp"` because OpenSSH dynamic forwarding does not carry UDP.

### Step 3B: route only selected inbounds

This is safer during testing. Find the inbound tags in 3x-ui, then generate a scoped rule:

```bash
hamara xray --inbound-tags inbound-443,inbound-test
```

The rule will resemble:

```json
{
  "inboundTag": [
    "inbound-443",
    "inbound-test"
  ],
  "outboundTag": "hamara-egress",
  "type": "field"
}
```

Use the exact tags generated by your panel; remarks displayed in the UI are not always the same as Xray tags.

### Step 4: save and restart Xray

1. Save the Xray configuration.
2. Restart Xray from 3x-ui.
3. Use the panel's **outbound connectivity test** and **route test**, if available.
4. Connect one test client.
5. Compare the egress address:

```bash
# Normal VPS route
curl -4 https://api.ipify.org; echo

# Hamara source-policy route on the Iran server
curl -4 --interface 10.73.0.2 https://api.ipify.org; echo
```

Use the address from `ip -4 address show hamara0`, not always `10.73.0.2`.

### DNS considerations

The generated routed outbound uses `UseIPv4`. Xray DNS behavior still depends on the panel's DNS object and rule order. If DNS privacy is required, configure a trusted remote DNS/DoH resolver in Xray and verify it with packet capture and leak tests. Do not assume that adding a freedom outbound automatically changes every system or panel DNS query.

### Preventing direct fallback and testing for IP leaks

The routed outbound combines three controls:

- `sendThrough` binds the Iran-side tunnel source address.
- `sockopt.interface` binds `hamara0`.
- `sockopt.mark: 72` selects Hamara's strict policy table.

`hamara-guard` keeps an unreachable fallback in that table even when `hamara0` is gone, rejects marked/source traffic attempting to use the Iran WAN, and rejects marked IPv6. The WireGuard/IPIP service requires the guard before it starts. The normal VPS default route is not replaced, so management SSH remains independent.

Run the automated maintenance-window check on the Iran server:

```bash
hamara leak-test
```

It verifies working egress, stops the tunnel while leaving the guard active, checks that mark `72` has no fallback route, confirms a source-bound HTTP request cannot escape, and restores the tunnel. Keep an SSH session open. Also test a real 3x-ui client because Hamara cannot enforce the panel's routing-rule order.

No software can honestly guarantee zero leaks in every application and configuration. System DNS, incorrectly ordered Xray rules, a separate unmarked outbound, root changes, containers, or another network namespace are outside this guard. Use `hamara doctor`, `hamara leak-test`, the panel route test, and an external client leak test before production.

## Commands

```text
hamara
    Interactive installer and management menu

hamara init --role outside --mode MODE --endpoint HOST [options]
    Create an outside gateway and print an OFFER token

hamara join --offer TOKEN
hamara join --file /root/offer.txt
    Configure the Iran edge and print a RESPONSE token

hamara accept --response TOKEN
hamara accept --file /root/response.txt
    Authorize the Iran peer on the outside gateway

hamara status
    Show role, mode, services, and WireGuard handshake/transfer data

hamara start | hamara stop | hamara restart
    Manage link services. On a routed Iran edge, stop intentionally leaves
    hamara-guard active so marked traffic cannot fall back to the WAN.

hamara doctor
    Check forwarding, strict guard, interfaces, services, and end-to-end egress

hamara leak-test
    Maintenance-window fail-closed test on a routed Iran edge

hamara logs
    Show the latest 200 journal lines for this instance's services

hamara repair
    Idempotently reapply the outside firewall/NAT or Iran strict guard

hamara paths
    List every saved file, whether it exists, and its purpose

hamara xray
hamara xray --inbound-tags tag-a,tag-b
    Print 3x-ui/Xray outbound and routing JSON

hamara pairing-token
    Reprint the local OFFER or RESPONSE token

hamara uninstall
hamara uninstall --purge-wstunnel
    Remove Hamara services/config; optionally remove the pinned wstunnel binary
```

## Ports and protocols

| Mode | Outside inbound | Inner/local listener | Notes |
|---|---|---|---|
| ARA | TCP `443` by default | UDP `51820` loopback target; Iran UDP `51821` loopback relay | Direct WAN access to inner UDP `51820` is blocked by Hamara's firewall unit |
| WireGuard | UDP `51820` by default | — | Port is configurable |
| IPIP | IP protocol `4` | — | Not TCP/UDP port 4 |
| SSH SOCKS | Existing SSH TCP port | Iran `127.0.0.1:10808` | TCP destinations only |

Remember to open the same transport in the VPS provider's security group. Hamara can modify the guest firewall, not the provider firewall.

## Saved files and exact locations

Run `hamara paths` on either VPS to see which files are present. Not every mode creates every file.

| Path | Server/mode | Stored content | Sensitive? |
|---|---|---|---:|
| `/usr/local/bin/hamara` | Both | Statically built management CLI | No |
| `/usr/local/bin/wstunnel` | ARA, both | Pinned checksum-verified transport binary | No |
| `/opt/hamara-tunnel/` | Both, source installer | Installed README, license, and docs copy | No |
| `/etc/hamara/config.json` | Both | Role, mode, addresses, ports, paths, state; mode `0600` | Operational |
| `/etc/hamara/offer.token` | Outside | OFFER containing WG PSK and ARA mTLS client key; mode `0600` | **Yes** |
| `/root/hamara-offer.txt` | Outside | Convenience copy of the same OFFER | **Yes** |
| `/etc/hamara/response.token` | Iran | RESPONSE containing the Iran public peer identity | Private |
| `/root/hamara-response.txt` | Iran | Convenience copy of RESPONSE | Private |
| `/etc/hamara/secrets/wg_private.key` | ARA/WireGuard, both | WireGuard private key | **Yes** |
| `/etc/hamara/secrets/wg_public.key` | ARA/WireGuard, both | WireGuard public key | No |
| `/etc/hamara/secrets/wg_preshared.key` | ARA/WireGuard, both | WireGuard PSK | **Yes** |
| `/etc/hamara/secrets/ssh_ed25519` | SSH fallback, Iran | SSH forwarding private key | **Yes** |
| `/etc/hamara/mtls/client-ca.crt` | ARA, outside | CA allowed to authenticate the one Iran client | No |
| `/etc/hamara/mtls/client.crt` | ARA, Iran | ARA mTLS client certificate | Private |
| `/etc/hamara/mtls/client.key` | ARA, Iran | ARA mTLS P-256 client private key | **Yes** |
| `/etc/hamara/tls/` | ARA self-signed, outside | Generated outer TLS certificate/key | **Key is secret** |
| `/etc/hamara/xray-outbound.json` | Iran | Generated 3x-ui/Xray outbound object | No secret |
| `/etc/hamara/ssh_known_hosts` | SSH fallback, Iran | Pinned outside SSH host key | No |
| `/etc/wireguard/hamara0.conf` | ARA/WireGuard, both | Interface addresses and WireGuard private/peer material; mode `0600` | **Yes** |
| `/etc/sysctl.d/90-hamara-tunnel.conf` | Routed modes | IPv4 forwarding and loose reverse-path filtering | No |
| `/usr/local/lib/hamara/guard` | Routed Iran | Strict mark/source kill-switch and policy-table helper | No |
| `/usr/local/lib/hamara/firewall` | Routed outside | Forwarding, NAT, and listener firewall helper | No |
| `/usr/local/lib/hamara/ipip` | IPIP | IPIP interface lifecycle helper | No |
| `/etc/systemd/system/hamara-guard.service` | Routed Iran | Persistent strict egress guard | No |
| `/etc/systemd/system/hamara-ara-client.service` | ARA Iran | mTLS WSS client lifecycle | Contains paths, not keys |
| `/etc/systemd/system/hamara-ara-server.service` | ARA outside | Restricted mTLS WSS server lifecycle | Contains paths, not keys |
| `/etc/systemd/system/hamara-firewall.service` | Routed outside | Persistent forwarding/NAT rules | No |
| `/etc/systemd/system/hamara-ipip.service` | IPIP | Persistent IPIP lifecycle | No |
| `/etc/systemd/system/hamara-ssh-socks.service` | SSH Iran | Persistent loopback SOCKS client | Contains paths, not keys |
| `/etc/systemd/system/wg-quick@hamara0.service.d/hamara-guard.conf` | ARA/WG Iran | Makes WireGuard require the strict guard | No |
| `/etc/ssh/sshd_config.d/90-hamara-tunnel.conf` | SSH outside | Restricted `hamara-proxy` Match block | No |
| `/var/lib/hamara-ssh/.ssh/authorized_keys` | SSH outside | Paired forwarding-only public key | Public identity |

The outside ARA server deletes `/etc/hamara/mtls/client-ca.key`, `client.crt`, and `client.key` after the OFFER is safely written; only the CA certificate remains there. Never share OFFER tokens, private keys, `/etc/wireguard/hamara0.conf`, or secret directories. The generated Xray JSON contains no private key.

## Service names

### ARA outside

```text
wg-quick@hamara0.service
hamara-ara-server.service
hamara-firewall.service
```

### ARA Iran

```text
hamara-guard.service
hamara-ara-client.service
wg-quick@hamara0.service
```

`hamara-guard` intentionally stays active when `hamara stop` is used on a routed Iran edge.

Other modes use `hamara-ipip.service` or `hamara-ssh-socks.service` as appropriate.

## Troubleshooting

Start with:

```bash
hamara status
hamara doctor
sudo systemctl --failed
sudo journalctl -u 'hamara-*' -u 'wg-quick@hamara0' --since '-15 min' --no-pager
```

### ARA: the TLS port is not reachable

On the outside VPS:

```bash
sudo ss -ltnp | grep ':443 '
sudo systemctl status hamara-ara-server --no-pager
sudo journalctl -u hamara-ara-server -n 100 --no-pager
```

From another network:

```bash
nc -vz tunnel.example.com 443
openssl s_client -connect tunnel.example.com:443 -servername tunnel.example.com </dev/null
```

Check all of the following:

- DNS points to the outside VPS.
- The cloud security group allows TCP `443`.
- UFW/iptables allows TCP `443`.
- No Nginx, Caddy, HAProxy, 3x-ui inbound, or another process owns that port on the outside server.
- The certificate contains the endpoint name and has not expired.
- The system clocks are correct.

If port `443` is already used, rerun with another port such as `8443`, or deliberately place a compatible WebSocket reverse proxy in front. Reverse-proxy integration is not automated in version 0.2.

### ARA: TLS connects, but WireGuard never handshakes

```bash
# Iran
sudo systemctl status hamara-ara-client wg-quick@hamara0 --no-pager
sudo ss -lunp | grep ':51821 '
sudo wg show hamara0

# Outside
sudo systemctl status hamara-ara-server wg-quick@hamara0 --no-pager
sudo ss -lunp | grep ':51820 '
sudo wg show hamara0
```

Likely causes:

- The outside server has not accepted the RESPONSE token.
- OFFER and RESPONSE tokens came from different instance IDs.
- The ARA path/OFFER token was truncated.
- `/etc/hamara/mtls/client.crt` or `client.key` is missing on Iran.
- The client certificate common name no longer matches the random ARA path.
- A TLS-inspecting proxy rejects long-lived WebSocket connections.
- The outer TCP connection is reset under load.

Reprint tokens with `hamara pairing-token`. Do not regenerate only one side's WireGuard keys manually; re-pair both servers instead.

### Direct WireGuard: no handshake

```bash
sudo wg show hamara0
sudo ss -lunp | grep ':51820 '
sudo iptables -S INPUT | grep 51820
```

Verify UDP access in the outside provider firewall. Some networks block or rate-limit UDP even when the local firewall is correct. In that case, use ARA or SSH SOCKS.

### Handshake works, but there is no Internet egress

On the outside VPS:

```bash
sysctl net.ipv4.ip_forward
sudo iptables -S FORWARD
sudo iptables -t nat -S POSTROUTING
ip -4 route
```

Expected:

- `net.ipv4.ip_forward = 1`
- A forwarding rule from `hamara0`
- A MASQUERADE rule for the Hamara `/30`
- A valid default route on the detected WAN interface

If UFW was reloaded after Hamara, reapply Hamara's rules:

```bash
sudo systemctl restart hamara-firewall
```

### `hamara doctor` works, but 3x-ui clients use the Iran IP

This is normally an Xray routing issue, not a tunnel issue.

1. Confirm the outbound tag is exactly `hamara-egress`.
2. Confirm the routing rule references that exact tag.
3. Move the Hamara rule above any earlier catch-all direct rule.
4. Keep the API rule above Hamara.
5. Check that selected `inboundTag` values are actual tags, not remarks.
6. Restart Xray from the panel.
7. Inspect Xray logs for invalid `sendThrough` or interface errors.
8. Run the panel's route test for the affected inbound/domain.

### `hamara-guard` is inactive or a leak test fails

Stop client traffic and inspect before continuing:

```bash
hamara status
hamara logs
sudo systemctl status hamara-guard --no-pager
sudo ip rule show
sudo ip route show table 52730   # ARA default; use the table shown by hamara doctor
sudo iptables -S OUTPUT
hamara repair
hamara doctor
```

The dedicated table must contain an `unreachable default` even when the tunnel is down. Do not disable the guard to make a failing tunnel appear connected. If `hamara leak-test` reports `CRITICAL`, leave 3x-ui client traffic disabled, collect redacted logs, and re-pair cleanly.

### Small pages work, but downloads stall

This is usually an MTU/PMTU problem. ARA defaults to MTU `1280`; direct WireGuard uses `1380`; IPIP uses `1400`.

Test with:

```bash
ping -4 -M do -s 1200 1.1.1.1
tracepath -4 1.1.1.1
```

For an ARA diagnostic, temporarily lower MTU on both ends:

```bash
sudo ip link set dev hamara0 mtu 1200
```

If this fixes the problem, update `MTU` in `/etc/wireguard/hamara0.conf` on both servers and restart `wg-quick@hamara0`. Keep the values equal. Manual edits may be overwritten by a future reconfiguration.

### ARA is slow on a lossy link

ARA's default outer transport is WebSocket over TCP. It is compatible with many HTTP paths but can suffer from head-of-line blocking. Try, in order:

1. Confirm there is no packet loss on the raw VPS link.
2. Lower MTU.
3. Test direct WireGuard as a baseline.
4. Move the outside gateway closer to the Iran VPS.
5. Avoid oversubscribed VPS CPUs.
6. Check TCP congestion control with `sysctl net.ipv4.tcp_congestion_control`.

Do not assume that more encapsulation is always more reliable.

### IPIP does not start

```bash
sudo journalctl -u hamara-ipip -n 100 --no-pager
ip -d tunnel show
```

IPIP requires:

- Public IPv4 assigned directly on both hosts
- Provider support for IP protocol 4
- No ordinary NAT between the hosts
- Protocol 4 allowed in both provider and guest firewalls

Use WireGuard if confidentiality is required.

### SSH SOCKS connects but UDP applications fail

That is expected. OpenSSH dynamic forwarding carries TCP, not UDP ASSOCIATE. In Xray, set the Hamara routing rule to `"network": "tcp"`, or switch to ARA/WireGuard.

### SSH SOCKS authentication fails

On the Iran VPS:

```bash
sudo journalctl -u hamara-ssh-socks -n 100 --no-pager
```

On the outside VPS:

```bash
sudo journalctl -u ssh -n 100 --no-pager
sudo sshd -t
sudo tail -n 5 /var/lib/hamara-ssh/.ssh/authorized_keys
```

Confirm the RESPONSE was accepted on the correct outside instance and that the SSH port/provider firewall is correct.

### Port changed or public IP changed

Pairing tokens embed the endpoint. Version 0.2 does not mutate a paired endpoint in place. The safest recovery is:

```bash
hamara uninstall
```

Then initialize and pair the link again on both servers.

## Safe recovery procedure

Keep an existing SSH session open while changing routing. Hamara uses source-policy routing and does not replace the VPS default route, so normal SSH management should remain on the original WAN path.

If anything behaves unexpectedly, use the manager so the Iran guard is not accidentally removed:

```bash
hamara stop
hamara status
hamara logs
sudo ip rule show
sudo ip route show table all
```

On a routed Iran edge, confirm `hamara-guard.service` remains active. Then inspect logs before repairing or uninstalling.

## Security notes

- Hamara uses WireGuard for ARA and direct routed encryption; it does not implement custom cryptography.
- ARA uses `wstunnel` `10.7.1`, downloaded from its official GitHub release and verified by an architecture-specific SHA-256 checksum.
- ARA restricts the remote UDP destination to `127.0.0.1:51820` and requires both a random WebSocket path and matching per-instance P-256 mTLS client certificate.
- The mTLS client certificate common name is the random path; the server requires the client CA before accepting a tunnel.
- The outside deletes the mTLS CA signing key and issued client private key after creating the root-only OFFER token.
- The outside ARA firewall blocks direct WAN access to the inner WireGuard UDP port.
- Routed Iran outbounds use socket mark `72`; `hamara-guard` supplies an unreachable fallback plus IPv4/IPv6 firewall rejects.
- The SSH fallback account has password login disabled, no TTY, no agent/X11 forwarding, and a forced `/bin/false` command. Its paired key is still intentionally allowed to create local TCP forwards.
- Pairing token checksums detect accidental truncation; they are **not digital signatures**. The OFFER contains WG and ARA mTLS secrets, so transfer it through an authenticated confidential channel.
- Self-signed ARA mode does not authenticate the outer TLS endpoint. Inner WireGuard authentication remains in place.
- IPIP offers no encryption or peer authentication.
- Hamara does not install or configure 3x-ui itself.
- Provider firewalls, host hardening, updates, backups, abuse handling, and legal compliance remain the operator's responsibility.

See [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) and [SECURITY.md](SECURITY.md).

## Updating

From a newer checkout:

```bash
cd Hamara-tunnel
git pull --ff-only
sudo ./install.sh
hamara status
```

The installer replaces the CLI and documentation; it does not silently rewrite an existing `/etc/hamara` link. **ARA 0.1 installations must be uninstalled on both sides and paired again** to gain mandatory mTLS and `hamara-guard`.

## Uninstalling

```bash
hamara uninstall
```

This removes Hamara interfaces, services, firewall rules, pairing material, sysctl file, and the dedicated SSH account when applicable. Installed Ubuntu packages remain. To remove the ARA helper binary too:

```bash
hamara uninstall --purge-wstunnel
```

Review `/etc/letsencrypt` separately; Hamara does not delete certificates that may be shared with other services.

## Development

```bash
make check
make integration   # mTLS rejection + UDP echo through the exact ARA commands
make build
make release       # static amd64/arm64 and source archives + SHA256SUMS
sudo make install
```

The integration test downloads the pinned, checksum-verified wstunnel release, creates ephemeral server and P-256 mutual-TLS identities, proves that a path-aware client without the mTLS key is rejected, and verifies a UDP request/response through authenticated WSS without changing system routes or firewall rules.

The CLI uses only the Go standard library. Unit tests cover token integrity, offer validation, platform parsing, WireGuard rendering, and Xray JSON rendering. CI builds both Linux `amd64` and `arm64` binaries.

Repository layout:

```text
cmd/hamara/          CLI entry point
internal/app/        Pairing and lifecycle orchestration
internal/model/      Versioned configuration/token models
internal/platform/   Ubuntu, filesystem, process helpers
internal/render/     WireGuard/systemd/firewall/Xray renderers
internal/token/      Checksummed copy/paste pairing tokens
docs/                Operations and threat-model documentation
examples/3x-ui/      Example Xray objects
```

## FAQ

### Does ARA guarantee DPI bypass?

No. It can reduce recognition by basic classifiers by carrying the WireGuard transport inside TLS WebSocket traffic, but endpoint and behavior-based blocking remain possible.

### Should I use a CDN?

Not for the first deployment. Direct DNS plus a valid certificate is simpler. Some CDNs prohibit arbitrary tunneling, buffer/limit WebSockets, or do not proxy the selected port. Follow the provider's terms.

### Which mode should I try first?

- Try **WireGuard Direct** for speed where UDP works.
- Use **ARA** where direct WireGuard is unstable or filtered.
- Use **SSH SOCKS** only as a TCP fallback.
- Use **IPIP** only when you explicitly accept no encryption.

### Does Hamara route the entire Iran VPS?

No. It installs a separate source-policy table. Only applications that bind to the Hamara source address/interface—such as the generated Xray outbound—use it. Normal SSH and system traffic keep the regular default route.

### Can multiple 3x-ui inbounds use one Hamara link?

Yes. Route all inbounds with the catch-all rule or list selected inbound tags.

### Can one outside server serve several Iran servers?

Not in version 0.2. Each install manages one `/30` peer link. Multi-peer instance management is planned architecture work, not silently emulated by sharing keys.

### Is IPv6 supported?

Not in version 0.2. Xray's generated outbound uses IPv4 intentionally.

### Is 3x-ui required?

No. Any application that can bind its source address to the Iran-side Hamara address can use the routed modes. The included integration is specifically documented for Xray/3x-ui.

## Credits

ARA transport uses:

- WireGuard: <https://www.wireguard.com/>
- wstunnel: <https://github.com/erebe/wstunnel>
- Linux policy routing, IP forwarding, and NAT

Hamara Tunnel is released under the MIT License.

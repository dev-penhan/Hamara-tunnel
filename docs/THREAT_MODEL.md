# Hamara Tunnel threat model

## Scope

Hamara Tunnel links one operator-controlled Iran edge VPS to one operator-controlled outside gateway. It protects the confidentiality and integrity of routed payload traffic against passive observation on the network path when ARA or direct WireGuard is used.

Hamara is not an anonymity network. The VPS providers, destination services, and anyone able to correlate both sides can observe metadata.

## Assets

- WireGuard private keys and pre-shared key
- ARA P-256 mutual-TLS client key and CA trust
- SSH private key in SSH SOCKS mode
- Pairing OFFER token (contains WG and ARA mTLS secrets)
- 3x-ui/Xray user configuration
- Routed user payload
- Availability of both VPSs

## Trust assumptions

- The operator controls root on both machines.
- Both operating systems and package sources are trusted and updated.
- Pairing tokens are copied through an authenticated channel.
- The outside gateway is allowed to forward the traffic under its provider policy and local law.
- DNS and certificate issuance are under the operator's control in public-certificate ARA mode.

## Protected cases

### ARA

- Inner layer-3 packets are authenticated and encrypted by WireGuard.
- The public WireGuard UDP listener is blocked on the outside WAN interface.
- The outer transport uses TLS WebSocket and a random path prefix.
- The server requires a per-instance P-256 mTLS client certificate whose common name matches that random path.
- With ACME/existing public certificates, the Iran client also verifies the outside server certificate.
- The CA signing key and issued client private key are removed from the outside filesystem after the OFFER token is created.
- The wstunnel server restricts forwarded datagrams to its loopback WireGuard port.
- Routed Xray sockets carry mark `72`; a persistent unreachable fallback, source guard, and marked-IPv6 rejection prevent normal-WAN fallback when the tunnel disappears.

### Direct WireGuard

- Layer-3 packets are authenticated and encrypted by WireGuard.
- A per-link pre-shared key is added to the normal WireGuard public-key handshake.

### SSH SOCKS

- TCP streams are encrypted and authenticated by OpenSSH.
- The generated account has no password, TTY, shell command, X11, or agent forwarding.

## Explicit non-goals

Hamara does not protect against:

- Endpoint blocking by IP address or ASN
- Global traffic correlation
- VPS compromise or malicious providers
- Malicious 3x-ui/Xray builds or client software
- Application-layer identifiers, cookies, account correlation, or browser fingerprinting
- Denial of service, packet dropping, throttling, or connection resets
- Active probing that determines a TLS service exists; mTLS/path checks prevent unauthorized tunnel use but cannot make the endpoint disappear
- Certificate transparency exposure of the ARA domain
- Traffic analysis based on packet sizes, timing, direction, and volume
- Legal or provider-policy consequences

## Mode-specific limitations

### ARA self-signed mode

The wstunnel client does not verify the outer server certificate. Mandatory mTLS authenticates the Iran client and inner WireGuard authentication prevents payload decryption or injection without the WireGuard keys, but an on-path actor can proxy, identify, record, reset, or deny the outer TLS connection. Use a public certificate whenever possible.

### ARA over TCP

TCP retransmission and head-of-line blocking can amplify performance problems. TLS/WebSocket changes the public protocol shape; it does not make traffic indistinguishable from every browser session.

### IPIP

IPIP has no cryptographic security. Source addresses can be spoofed where networks allow it, and payload is visible to path observers. It is included as a low-overhead interoperability/diagnostic mode, not a privacy mode.

### SSH SOCKS

OpenSSH dynamic forwarding supports TCP but not SOCKS UDP ASSOCIATE. Applications may fail or use a separate direct UDP path unless Xray routing is limited to TCP.

## Pairing-token security

Tokens use base64url-encoded JSON and a truncated SHA-256 checksum. The checksum detects copying errors; it does not authenticate a maliciously replaced token. The OFFER contains a WireGuard pre-shared key and, for ARA, the one-peer mTLS client private key. Treat it as a secret and verify the destination through a second channel when risk warrants it.

## Fail-closed expectations

Routed Xray configuration combines `sendThrough`, `sockopt.interface`, and socket mark `72`. `hamara-guard` installs mark/source policy rules, an always-present unreachable default, an IPv4 OUTPUT guard, and marked-IPv6 rejection. WireGuard/IPIP requires that guard before starting. `hamara stop` leaves the guard active.

This guard applies to the generated marked outbound. It cannot prevent a panel administrator from selecting an earlier unmarked/direct Xray rule, and it does not automatically capture unrelated system DNS, containers, root-created sockets, or separate network namespaces. Every deployment must run `hamara leak-test` and test a real 3x-ui client with the tunnel down before production.

## Operational recommendations

1. Use unique instances and never reuse OFFER tokens across gateways.
2. Prefer ARA with a valid certificate or direct WireGuard.
3. Keep 3x-ui and Xray updated from their official source.
4. Restrict panel access by firewall/VPN and use strong authentication.
5. Keep SSH management separate from Hamara policy routing.
6. Monitor WireGuard handshake age, service restarts, CPU, bandwidth, and provider notices.
7. Rotate by uninstalling and re-pairing after suspected key exposure.
8. Back up panel state, not tunnel private keys; fresh keys are safer during rebuilds.

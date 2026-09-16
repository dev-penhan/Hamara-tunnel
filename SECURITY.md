# Security policy

## Supported versions

Version `0.2.x` is the supported series. ARA links created with `0.1.x` must be re-paired to receive mandatory mTLS and the strict egress guard. Security fixes may require re-pairing both servers.

## Reporting a vulnerability

Do not open a public issue for a vulnerability that exposes credentials, enables unauthorized forwarding, or weakens peer authentication. Send a private report to the repository maintainer with:

- affected version and commit
- tunnel mode and Ubuntu version
- reproduction steps
- expected and observed impact
- logs with all keys, tokens, domains, and IPs redacted
- any proposed patch

The repository owner should replace this section with a monitored security email or GitHub private-vulnerability-reporting link before public release.

## Dependency integrity

ARA pins `erebe/wstunnel` version `10.7.1` and verifies release archives using SHA-256:

```text
linux amd64  fa842ed53fbb14b1c69cd98829f9895d7f8a6b0d562c57c1175851a52cea9ea2
linux arm64  99f9506d01d1b4073254609600ec5056dab8dc58aec75c32f6eb0508335a8fd2
```

Update the version and both hashes together. Confirm hashes from the upstream release page through an authenticated connection.

## Secret handling

Never include these in issues or screenshots:

- `HAMARA1.OFFER.*` tokens
- `/etc/hamara/secrets/*`
- `/etc/hamara/mtls/client.key`
- `/etc/hamara/tls/privkey.pem`
- `/etc/wireguard/hamara0.conf`
- SSH authorized keys if infrastructure identity is sensitive
- 3x-ui database, UUIDs, subscriptions, or API credentials

A RESPONSE token contains only the Iran peer public key in WireGuard modes, but it should still be handled privately to prevent pairing confusion. For automation, prefer `--file` with permission `0600`; command-line token values may appear in shell history or process listings.

## Operator response to suspected compromise

1. Disconnect affected client traffic.
2. Save redacted logs and timestamps.
3. Run `hamara uninstall` on both servers.
4. Rebuild compromised hosts from trusted images if root compromise is possible.
5. Reinstall and create a fresh pairing; do not reuse old token files.
6. Rotate 3x-ui credentials, Xray client identifiers, SSH host keys, API tokens, and TLS account credentials as appropriate.

# Changelog

All notable changes to Hamara Tunnel are documented here.

## 0.2.0 - 2026-09-16

### Security

- Added mandatory per-instance ARA mutual TLS with a P-256 client identity
- Bound the mTLS client certificate common name to the random WebSocket path
- Deleted ARA CA and issued-client private material from the outside filesystem after the root-only OFFER token is created
- Added `hamara-guard`, a fail-closed policy-routing and firewall kill switch for routed Iran edges
- Added Xray socket mark `72`, a persistent unreachable fallback route, IPv4 source guards, and marked-IPv6 rejection
- Made WireGuard/IPIP depend on the strict guard before starting
- Added an intrusive maintenance-window `hamara leak-test` command

### Added

- Automatic sudo elevation: users can type `hamara` directly
- Start, stop, restart, logs, repair, leak-test, and saved-path management commands
- Expanded interactive management menu
- Persian documentation in `README_FA.md`
- Explicit saved-file inventory in both READMEs and the `hamara paths` command
- Official repository links for `dev-penhan/Hamara-tunnel`
- Mutual-TLS coverage in the local ARA transport integration test

### Changed

- Routed Xray outbounds now combine source binding, interface binding, and socket marking
- Stopping a routed Iran link intentionally leaves the strict guard active
- Project date and release metadata updated for version 0.2.0

### Upgrade note

ARA installations made with 0.1.0 must be uninstalled and paired again to receive the new mTLS identity and strict guard.

## 0.1.0 - 2026-09-15

### Added

- Interactive Iran-edge/outside-gateway setup flow
- Checksummed offline OFFER/RESPONSE pairing
- ARA Routed TLS using WireGuard over TLS WebSocket transport
- Direct WireGuard routed mode
- Kernel IPIP routed mode
- SSH SOCKS TCP fallback
- Source-policy routing for 3x-ui/Xray without replacing the VPS default route
- Idempotent outside forwarding/NAT firewall service
- Let's Encrypt, existing-certificate, and self-signed ARA modes
- 3x-ui/Xray outbound and routing JSON generator
- Status, diagnostics, token reprint, and uninstall commands
- Ubuntu 22.04/24.04 validation
- SHA-256-verified wstunnel installation for amd64 and arm64
- Unit tests, race tests, vet, formatting checks, a local ARA transport integration test, and GitHub Actions CI
- Full installation, operations, troubleshooting, threat-model, and security documentation

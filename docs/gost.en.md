# GOST Integration

[GOST](https://github.com/go-gost/gost) is an optional Go-based proxy and forwarding toolkit. The current official GOST v3 documentation covers proxying, TCP/UDP port forwarding, reverse proxy, TUN/TAP, routing, rate limiting, metrics, and Web/API management. Hamara uses WireGuard as the default encrypted Layer-3 tunnel; GOST is for authorized application-level forwarding and proxy chaining.

## When to use which tool

| Requirement | Recommended component |
|---|---|
| Encrypted routed VPS-to-VPS link | WireGuard |
| A small number of TCP ports | GOST TCP forwarding or nftables |
| UDP forwarding with application awareness | GOST UDP forwarding |
| Reverse access to a private service | GOST reverse proxy, only with strict ACLs |
| Full host default route | WireGuard, not a basic GOST port forward |

GOST is not a replacement for IP forwarding. A basic TCP forward does not automatically carry arbitrary IP traffic, ICMP, or every UDP application.

## Official binary

Use the official releases and verify the checksum before installing:

```bash
curl -fsSLo /tmp/gost.tar.gz https://github.com/go-gost/gost/releases/download/v3.3.0/gost_3.3.0_linux_amd64.tar.gz
# Download checksums.txt from the same release and verify the archive.
tar -xzf /tmp/gost.tar.gz -C /tmp
test -x /tmp/gost
sudo install -m 0755 /tmp/gost /usr/local/bin/gost
gost -V
```

Replace the version and architecture with the release you have verified. Do not pipe an unverified binary into production.

## Local TCP forwarding example

This forwards a local listener to a service on a server you administer:

```bash
gost -L tcp://127.0.0.1:18080/10.77.0.1:8080
```

For an internet-facing listener, bind deliberately and protect it with firewall rules and authentication. Do not expose an unauthenticated open proxy.

## YAML service example

```yaml
services:
- name: hamara-app
  addr: 127.0.0.1:18080
  handler:
    type: tcp
  listener:
    type: tcp
  forwarder:
    nodes:
    - name: foreign-app
      addr: 10.77.0.1:8080
```

Start it with the version-appropriate GOST v3 configuration option from the official documentation. Keep configuration files root-readable when they contain credentials.

## Resource guidance

- Prefer one GOST process with multiple services rather than one process per port.
- Use loopback or the WireGuard address for listeners unless public access is required.
- Keep read/write buffers at defaults until measurements show a need to change them.
- Set explicit connection and idle timeouts for low-volume services.
- Monitor file descriptors and connection counts with `systemctl status` and `ss -s`.
- Do not enable metrics or an API on a public address without authentication and an ACL.

## 3x-ui relationship

3x-ui/Xray may use a GOST-forwarded application port as an outbound target, but the simplest design is usually:

```text
3x-ui/Xray → operating-system route → WireGuard → foreign VPS
```

Use GOST only when you need a specific TCP/UDP forwarding or proxy-chain feature. See `docs/3x-ui.en.md`.

## Sources

- [Official GOST site](https://gost.run/en/)
- [Official GOST repository](https://github.com/go-gost/gost)
- [Official port-forwarding documentation](https://gost.run/en/tutorials/port-forwarding/)

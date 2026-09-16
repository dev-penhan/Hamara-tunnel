#!/usr/bin/env bash
# Local integration test for the exact ARA UDP-over-WSS transport commands.
# It does not create WireGuard interfaces or alter firewall/routing state.
set -Eeuo pipefail

WSTUNNEL_VERSION="10.7.1"
SERVER_PORT="${HAMARA_TEST_SERVER_PORT:-18443}"
UDP_TARGET="${HAMARA_TEST_UDP_TARGET:-19000}"
UDP_RELAY="${HAMARA_TEST_UDP_RELAY:-19001}"
PATH_PREFIX="0123456789abcdef0123456789abcdef"

work="$(mktemp -d)"
cleanup() {
  status=$?
  if [[ $status -ne 0 ]]; then
    echo "--- ARA test server log ---" >&2
    cat "$work/server.log" 2>/dev/null >&2 || true
    echo "--- ARA test client log ---" >&2
    cat "$work/client.log" 2>/dev/null >&2 || true
  fi
  for pid in ${BAD_CLIENT_PID:-} ${CLIENT_PID:-} ${SERVER_PID:-} ${ECHO_PID:-}; do
    kill "$pid" 2>/dev/null || true
  done
  rm -rf "$work"
  return "$status"
}
trap cleanup EXIT

case "$(uname -m)" in
  x86_64)
    arch=amd64
    checksum=fa842ed53fbb14b1c69cd98829f9895d7f8a6b0d562c57c1175851a52cea9ea2
    ;;
  aarch64)
    arch=arm64
    checksum=99f9506d01d1b4073254609600ec5056dab8dc58aec75c32f6eb0508335a8fd2
    ;;
  *)
    echo "Unsupported test architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

url="https://github.com/erebe/wstunnel/releases/download/v${WSTUNNEL_VERSION}/wstunnel_${WSTUNNEL_VERSION}_linux_${arch}.tar.gz"
curl -fsSL --retry 3 "$url" -o "$work/wstunnel.tar.gz"
printf '%s  %s\n' "$checksum" "$work/wstunnel.tar.gz" | sha256sum -c -
tar -xzf "$work/wstunnel.tar.gz" -C "$work" wstunnel
chmod 0755 "$work/wstunnel"

openssl req -x509 -nodes -newkey rsa:2048 -days 1 \
  -subj /CN=localhost -addext subjectAltName=DNS:localhost \
  -keyout "$work/key.pem" -out "$work/cert.pem" >/dev/null 2>&1
# Ephemeral P-256 CA and client certificate for ARA mutual TLS.
openssl ecparam -name prime256v1 -genkey -noout -out "$work/client-ca.key"
openssl req -new -x509 -sha256 -days 1 -key "$work/client-ca.key" \
  -subj /CN=Hamara-Test-Client-CA -out "$work/client-ca.crt"
openssl ecparam -name prime256v1 -genkey -noout -out "$work/client.key"
openssl req -new -sha256 -key "$work/client.key" -subj "/CN=$PATH_PREFIX" -out "$work/client.csr"
printf '%s\n' 'basicConstraints=critical,CA:FALSE' 'keyUsage=critical,digitalSignature' 'extendedKeyUsage=clientAuth' > "$work/client-ext.cnf"
openssl x509 -req -sha256 -days 1 -in "$work/client.csr" \
  -CA "$work/client-ca.crt" -CAkey "$work/client-ca.key" -CAcreateserial \
  -extfile "$work/client-ext.cnf" -out "$work/client.crt" >/dev/null 2>&1

UDP_TARGET="$UDP_TARGET" python3 - <<'PY' >"$work/echo.log" 2>&1 &
import os
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("127.0.0.1", int(os.environ["UDP_TARGET"])))
while True:
    data, address = s.recvfrom(65535)
    s.sendto(b"echo:" + data, address)
PY
ECHO_PID=$!

"$work/wstunnel" server --log-lvl WARN \
  --restrict-to "127.0.0.1:${UDP_TARGET}" \
  --restrict-http-upgrade-path-prefix "$PATH_PREFIX" \
  --tls-certificate "$work/cert.pem" --tls-private-key "$work/key.pem" \
  --tls-client-ca-certs "$work/client-ca.crt" \
  "wss://127.0.0.1:${SERVER_PORT}" >"$work/server.log" 2>&1 &
SERVER_PID=$!
sleep 0.5

# Prove that knowing the path is insufficient without the mTLS client key.
BAD_RELAY=$((UDP_RELAY + 1))
"$work/wstunnel" client --log-lvl WARN \
  --connection-retry-max-backoff 1s --websocket-ping-frequency 20s --dns-resolver-prefer-ipv4 \
  -L "udp://127.0.0.1:${BAD_RELAY}:127.0.0.1:${UDP_TARGET}?timeout_sec=1" \
  -P "$PATH_PREFIX" "wss://localhost:${SERVER_PORT}" >"$work/bad-client.log" 2>&1 &
BAD_CLIENT_PID=$!
sleep 0.5
BAD_RELAY="$BAD_RELAY" python3 - <<'PY'
import os
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(1)
s.sendto(b"unauthorized", ("127.0.0.1", int(os.environ["BAD_RELAY"])))
try:
    data, _ = s.recvfrom(65535)
except socket.timeout:
    print("unauthorized mTLS-less client: rejected")
else:
    raise SystemExit(f"mTLS-less client unexpectedly received: {data!r}")
PY
kill "$BAD_CLIENT_PID" 2>/dev/null || true
wait "$BAD_CLIENT_PID" 2>/dev/null || true
BAD_CLIENT_PID=""

"$work/wstunnel" client --log-lvl WARN \
  --connection-retry-max-backoff 2s --websocket-ping-frequency 20s --dns-resolver-prefer-ipv4 \
  -L "udp://127.0.0.1:${UDP_RELAY}:127.0.0.1:${UDP_TARGET}?timeout_sec=0" \
  -P "$PATH_PREFIX" --tls-certificate "$work/client.crt" --tls-private-key "$work/client.key" \
  "wss://localhost:${SERVER_PORT}" >"$work/client.log" 2>&1 &
CLIENT_PID=$!
sleep 0.8

UDP_RELAY="$UDP_RELAY" python3 - <<'PY'
import os
import socket
payload = b"hamara-ara-integration-test"
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(5)
s.sendto(payload, ("127.0.0.1", int(os.environ["UDP_RELAY"])))
data, _ = s.recvfrom(65535)
expected = b"echo:" + payload
if data != expected:
    raise SystemExit(f"unexpected response: {data!r}")
print(data.decode())
PY

echo "ARA local UDP-over-WSS integration test: PASS"

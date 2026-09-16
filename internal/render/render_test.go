package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dev-penhan/Hamara-tunnel/internal/model"
)

func routedConfig(role, mode string) model.Config {
	return model.Config{
		Role: role, Mode: mode, Interface: "hamara0", TunnelCIDR: "10.73.0.0/30",
		GatewayAddress: "10.73.0.1/30", EdgeAddress: "10.73.0.2/30",
		EndpointHost: "gw.example.com", EndpointPort: 443, WireGuardPort: 51820,
		LocalRelayPort: 51821, MTU: 1280, RouteTable: 52730,
	}
}

func TestWireGuardIranARA(t *testing.T) {
	cfg := routedConfig(model.RoleIran, model.ModeARA)
	got, err := WireGuard(cfg, "private", "psk", "gateway-public")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Table = off", "AllowedIPs = 0.0.0.0/0", "Endpoint = 127.0.0.1:51821", "ip route replace default dev %i metric 10 table 52730"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestXrayOutboundJSON(t *testing.T) {
	cfg := routedConfig(model.RoleIran, model.ModeARA)
	b, err := XrayOutbound(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hamara-egress") || !strings.Contains(string(b), "10.73.0.2") || !strings.Contains(string(b), `"mark": 72`) {
		t.Fatalf("unexpected JSON: %s", b)
	}
}

func TestGuardIsFailClosed(t *testing.T) {
	cfg := routedConfig(model.RoleIran, model.ModeARA)
	got := GuardScript(cfg)
	for _, want := range []string{"unreachable default", "fwmark", "! -o \"$IFACE\"", "ip6_add"} {
		if !strings.Contains(got, want) {
			t.Errorf("guard is missing %q", want)
		}
	}
}

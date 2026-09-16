package token

import (
	"strings"
	"testing"

	"github.com/dev-penhan/Hamara-tunnel/internal/model"
)

func TestRoundTrip(t *testing.T) {
	in := model.Offer{Version: 1, InstanceID: "abc", Mode: model.ModeARA, EndpointHost: "gw.example.com", EndpointPort: 443}
	s, err := Encode("offer", in)
	if err != nil {
		t.Fatal(err)
	}
	var out model.Offer
	if err := Decode(s, "offer", &out); err != nil {
		t.Fatal(err)
	}
	if out.InstanceID != in.InstanceID || out.Mode != in.Mode {
		t.Fatalf("unexpected round trip value: %#v", out)
	}
}

func TestTamperDetected(t *testing.T) {
	s, err := Encode("response", model.Response{Version: 1, InstanceID: "abc", Mode: model.ModeWireGuard})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(s, ".")
	parts[2] = parts[2] + "A"
	var out model.Response
	if err := Decode(strings.Join(parts, "."), "response", &out); err == nil {
		t.Fatal("expected checksum failure")
	}
}

package model

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestValidEndpointHost(t *testing.T) {
	for _, value := range []string{"example.com", "sub.example.com", "203.0.113.10"} {
		if !ValidEndpointHost(value) {
			t.Errorf("expected valid host %q", value)
		}
	}
	for _, value := range []string{"", "bad host", "-bad.example", "example.com/path", "example.com\nX=1", "2001:db8::1"} {
		if ValidEndpointHost(value) {
			t.Errorf("expected invalid host %q", value)
		}
	}
}

func TestValidateOffer(t *testing.T) {
	path := "0123456789abcdef0123456789abcdef"
	certPEM, keyPEM := testClientIdentity(t, path)
	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	o := Offer{
		Version: 1, InstanceID: "deadbeefdeadbeef", Mode: ModeARA,
		EndpointHost: "example.com", EndpointPort: 443,
		TunnelCIDR: "10.73.0.0/30", GatewayAddress: "10.73.0.1/30", EdgeAddress: "10.73.0.2/30",
		GatewayPublicKey: key, PresharedKey: key, ARAPath: path,
		LocalRelayPort: 51821, WireGuardPort: 51820, MTU: 1280, RouteTable: 52730, TLSMode: "acme", TLSVerify: true,
		MTLSClientCert: certPEM, MTLSClientKey: keyPEM,
	}
	if err := ValidateOffer(o); err != nil {
		t.Fatal(err)
	}
	o.MTLSClientKey = keyPEM + "garbage"
	if err := ValidateOffer(o); err == nil {
		t.Fatal("expected malformed mTLS key to be rejected")
	}
}

func testClientIdentity(t *testing.T, commonName string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: commonName},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return string(certPEM), string(keyPEM)
}

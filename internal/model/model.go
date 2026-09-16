package model

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

const SchemaVersion = 1

var (
	instanceIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
	hostnamePattern   = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
	usernamePattern   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	hexPathPattern    = regexp.MustCompile(`^[0-9a-f]{32,128}$`)
)

const (
	RoleIran    = "iran"
	RoleOutside = "outside"

	ModeARA       = "ara"
	ModeWireGuard = "wireguard"
	ModeIPIP      = "ipip"
	ModeSSHSocks  = "ssh-socks"

	StatePending = "pending"
	StateReady   = "ready"
)

// Config is the non-secret, on-disk state for one Hamara link.
// Secret keys are stored as root-only files and are referenced by path.
type Config struct {
	Version                 int       `json:"version"`
	InstanceID              string    `json:"instance_id"`
	Role                    string    `json:"role"`
	Mode                    string    `json:"mode"`
	State                   string    `json:"state"`
	Interface               string    `json:"interface,omitempty"`
	WANInterface            string    `json:"wan_interface,omitempty"`
	TunnelCIDR              string    `json:"tunnel_cidr,omitempty"`
	GatewayAddress          string    `json:"gateway_address,omitempty"`
	EdgeAddress             string    `json:"edge_address,omitempty"`
	EndpointHost            string    `json:"endpoint_host"`
	EndpointPort            int       `json:"endpoint_port"`
	WireGuardPort           int       `json:"wireguard_port,omitempty"`
	LocalRelayPort          int       `json:"local_relay_port,omitempty"`
	MTU                     int       `json:"mtu,omitempty"`
	RouteTable              int       `json:"route_table,omitempty"`
	PeerPublicKey           string    `json:"peer_public_key,omitempty"`
	PeerEndpoint            string    `json:"peer_endpoint,omitempty"`
	ARAPath                 string    `json:"ara_path,omitempty"`
	TLSMode                 string    `json:"tls_mode,omitempty"`
	TLSVerify               bool      `json:"tls_verify,omitempty"`
	TLSCertPath             string    `json:"tls_cert_path,omitempty"`
	TLSKeyPath              string    `json:"tls_key_path,omitempty"`
	MTLSCAPath              string    `json:"mtls_ca_path,omitempty"`
	MTLSClientCertPath      string    `json:"mtls_client_cert_path,omitempty"`
	MTLSClientKeyPath       string    `json:"mtls_client_key_path,omitempty"`
	SSHUser                 string    `json:"ssh_user,omitempty"`
	SSHPort                 int       `json:"ssh_port,omitempty"`
	SSHHostKey              string    `json:"ssh_host_key,omitempty"`
	SOCKSPort               int       `json:"socks_port,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	AcceptedAt              time.Time `json:"accepted_at,omitempty"`
	PreviousIPForward       string    `json:"previous_ip_forward,omitempty"`
	PreviousRPFilterAll     string    `json:"previous_rp_filter_all,omitempty"`
	PreviousRPFilterDefault string    `json:"previous_rp_filter_default,omitempty"`
}

// Offer is copied from the outside gateway to the Iran edge. It contains a
// WireGuard PSK for routed modes and must therefore be handled as a secret.
type Offer struct {
	Version           int    `json:"version"`
	InstanceID        string `json:"instance_id"`
	Mode              string `json:"mode"`
	EndpointHost      string `json:"endpoint_host"`
	EndpointPort      int    `json:"endpoint_port"`
	TunnelCIDR        string `json:"tunnel_cidr,omitempty"`
	GatewayAddress    string `json:"gateway_address,omitempty"`
	EdgeAddress       string `json:"edge_address,omitempty"`
	MTU               int    `json:"mtu,omitempty"`
	RouteTable        int    `json:"route_table,omitempty"`
	GatewayPublicKey  string `json:"gateway_public_key,omitempty"`
	PresharedKey      string `json:"preshared_key,omitempty"`
	WireGuardPort     int    `json:"wireguard_port,omitempty"`
	LocalRelayPort    int    `json:"local_relay_port,omitempty"`
	ARAPath           string `json:"ara_path,omitempty"`
	TLSMode           string `json:"tls_mode,omitempty"`
	TLSVerify         bool   `json:"tls_verify,omitempty"`
	MTLSClientCert    string `json:"mtls_client_cert,omitempty"`
	MTLSClientKey     string `json:"mtls_client_key,omitempty"`
	SSHUser           string `json:"ssh_user,omitempty"`
	SSHPort           int    `json:"ssh_port,omitempty"`
	SSHHostKey        string `json:"ssh_host_key,omitempty"`
	SOCKSPort         int    `json:"socks_port,omitempty"`
	GatewayEndpointIP string `json:"gateway_endpoint_ip,omitempty"`
}

// Response is copied from the Iran edge back to the outside gateway.
type Response struct {
	Version        int    `json:"version"`
	InstanceID     string `json:"instance_id"`
	Mode           string `json:"mode"`
	EdgePublicKey  string `json:"edge_public_key,omitempty"`
	SSHPublicKey   string `json:"ssh_public_key,omitempty"`
	EdgeEndpointIP string `json:"edge_endpoint_ip,omitempty"`
}

func IsMode(v string) bool {
	switch v {
	case ModeARA, ModeWireGuard, ModeIPIP, ModeSSHSocks:
		return true
	default:
		return false
	}
}

func ModeLabel(v string) string {
	switch v {
	case ModeARA:
		return "ARA Routed TLS"
	case ModeWireGuard:
		return "WireGuard Direct"
	case ModeIPIP:
		return "IPIP Routed"
	case ModeSSHSocks:
		return "SSH SOCKS"
	default:
		return v
	}
}

func IsRoutedMode(v string) bool {
	return v == ModeARA || v == ModeWireGuard || v == ModeIPIP
}

func ValidateOffer(o Offer) error {
	if o.Version != SchemaVersion {
		return fmt.Errorf("unsupported offer version %d", o.Version)
	}
	if !instanceIDPattern.MatchString(o.InstanceID) || !IsMode(o.Mode) {
		return errors.New("offer has an invalid instance ID or tunnel mode")
	}
	if !ValidEndpointHost(o.EndpointHost) || o.EndpointPort < 1 || o.EndpointPort > 65535 {
		return errors.New("offer has an invalid endpoint")
	}
	if IsRoutedMode(o.Mode) {
		ip, network, err := net.ParseCIDR(o.TunnelCIDR)
		if err != nil || ip.To4() == nil || network == nil {
			return errors.New("invalid IPv4 tunnel CIDR")
		}
		gateway := net.ParseIP(stripPrefix(o.GatewayAddress))
		edge := net.ParseIP(stripPrefix(o.EdgeAddress))
		if gateway == nil || gateway.To4() == nil || edge == nil || edge.To4() == nil {
			return errors.New("invalid routed tunnel IPv4 addresses")
		}
		if !network.Contains(gateway) || !network.Contains(edge) || gateway.Equal(edge) {
			return errors.New("tunnel addresses must be distinct members of the tunnel CIDR")
		}
		if o.MTU < 576 || o.MTU > 1500 || o.RouteTable < 1 || o.RouteTable > 0x7fffffff {
			return errors.New("invalid routed MTU or policy-routing table")
		}
	}
	if o.Mode == ModeARA || o.Mode == ModeWireGuard {
		if !validWGKey(o.GatewayPublicKey) || !validWGKey(o.PresharedKey) {
			return errors.New("WireGuard material is missing or malformed")
		}
		if o.WireGuardPort < 1 || o.WireGuardPort > 65535 {
			return errors.New("invalid WireGuard port")
		}
	}
	if o.Mode == ModeARA {
		if !hexPathPattern.MatchString(o.ARAPath) || o.LocalRelayPort < 1 || o.LocalRelayPort > 65535 {
			return errors.New("ARA transport settings are incomplete")
		}
		if o.TLSMode != "acme" && o.TLSMode != "existing" && o.TLSMode != "self-signed" {
			return errors.New("ARA TLS mode is invalid")
		}
		if (o.TLSMode == "self-signed") == o.TLSVerify {
			return errors.New("ARA certificate-verification policy does not match its TLS mode")
		}
		if !validMTLSMaterial(o.MTLSClientCert, o.MTLSClientKey, o.ARAPath) {
			return errors.New("ARA mutual-TLS client identity is missing, malformed, or mismatched")
		}
	}
	if o.Mode == ModeWireGuard && o.WireGuardPort != o.EndpointPort {
		return errors.New("direct WireGuard listener and endpoint ports do not match")
	}
	if o.Mode == ModeSSHSocks {
		if !usernamePattern.MatchString(o.SSHUser) || !validSSHHostKey(o.SSHHostKey) {
			return errors.New("SSH SOCKS identity settings are incomplete")
		}
		if o.SSHPort < 1 || o.SSHPort > 65535 || o.SSHPort != o.EndpointPort || o.SOCKSPort < 1024 || o.SOCKSPort > 65535 {
			return errors.New("SSH SOCKS ports are invalid or inconsistent")
		}
	}
	if o.Mode == ModeIPIP {
		ip := net.ParseIP(o.GatewayEndpointIP)
		if ip == nil || ip.To4() == nil || o.GatewayEndpointIP != o.EndpointHost {
			return errors.New("IPIP requires a consistent public gateway IPv4 address")
		}
	}
	return nil
}

func ValidateResponse(r Response, cfg Config) error {
	if r.Version != SchemaVersion || r.InstanceID != cfg.InstanceID || r.Mode != cfg.Mode {
		return errors.New("response does not belong to this Hamara instance")
	}
	switch cfg.Mode {
	case ModeARA, ModeWireGuard:
		if !validWGKey(r.EdgePublicKey) {
			return errors.New("response does not contain a valid edge WireGuard public key")
		}
	case ModeIPIP:
		if ip := net.ParseIP(r.EdgeEndpointIP); ip == nil || ip.To4() == nil {
			return errors.New("response does not contain a valid edge IPv4 address")
		}
	case ModeSSHSocks:
		if !strings.HasPrefix(r.SSHPublicKey, "ssh-") {
			return errors.New("response does not contain a valid SSH public key")
		}
	}
	return nil
}

func ValidEndpointHost(s string) bool {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip != nil {
		return ip.To4() != nil
	}
	if !hostnamePattern.MatchString(s) || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}

func validWGKey(s string) bool {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	return err == nil && len(data) == 32 && !strings.ContainsAny(s, "\r\n")
}

func validSSHHostKey(s string) bool {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) != 2 || (fields[0] != "ssh-ed25519" && fields[0] != "ssh-rsa") {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(fields[1])
	return err == nil && len(data) >= 32 && !strings.ContainsAny(s, "\r\n")
}

func validMTLSMaterial(certPEM, keyPEM, expectedCommonName string) bool {
	if len(certPEM) == 0 || len(certPEM) > 16384 || len(keyPEM) == 0 || len(keyPEM) > 16384 {
		return false
	}
	certBlock, certRest := pem.Decode([]byte(certPEM))
	keyBlock, keyRest := pem.Decode([]byte(keyPEM))
	if certBlock == nil || certBlock.Type != "CERTIFICATE" || len(strings.TrimSpace(string(certRest))) != 0 {
		return false
	}
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" || len(strings.TrimSpace(string(keyRest))) != 0 {
		return false
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	now := time.Now()
	if err != nil || cert.Subject.CommonName != expectedCommonName || cert.IsCA || now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return false
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return false
	}
	public, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || public.X == nil || public.Y == nil || key.PublicKey.X == nil || key.PublicKey.Y == nil {
		return false
	}
	if public.X.Cmp(key.PublicKey.X) != 0 || public.Y.Cmp(key.PublicKey.Y) != 0 {
		return false
	}
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth || usage == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}

func stripPrefix(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

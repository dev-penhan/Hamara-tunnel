package app

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dev-penhan/Hamara-tunnel/internal/model"
	"github.com/dev-penhan/Hamara-tunnel/internal/platform"
	"github.com/dev-penhan/Hamara-tunnel/internal/render"
	"github.com/dev-penhan/Hamara-tunnel/internal/token"
)

const (
	wgPrivatePath  = "/etc/hamara/secrets/wg_private.key"
	wgPublicPath   = "/etc/hamara/secrets/wg_public.key"
	wgPSKPath      = "/etc/hamara/secrets/wg_preshared.key"
	wgConfigPath   = "/etc/wireguard/hamara0.conf"
	mtlsDir        = "/etc/hamara/mtls"
	mtlsCAPath     = "/etc/hamara/mtls/client-ca.crt"
	mtlsCAKeyPath  = "/etc/hamara/mtls/client-ca.key"
	mtlsClientCert = "/etc/hamara/mtls/client.crt"
	mtlsClientKey  = "/etc/hamara/mtls/client.key"
	wstunnelVer    = "10.7.1"
	sshUser        = "hamara-proxy"
)

type App struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Version string
	Runner  platform.Runner
	reader  *bufio.Reader
}

type InitOptions struct {
	Mode         string
	EndpointHost string
	EndpointPort int
	TLSMode      string
	TLSCertPath  string
	TLSKeyPath   string
	ACMEEmail    string
	Force        bool
}

func New(in io.Reader, out, errOut io.Writer, version string) *App {
	return &App{
		In: in, Out: out, Err: errOut, Version: version,
		Runner: platform.Runner{Out: out, Err: errOut}, reader: bufio.NewReader(in),
	}
}

func (a *App) Interactive() error {
	a.banner()
	if platform.ConfigExists() {
		cfg, err := platform.LoadConfig()
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Installed link: %s / %s / %s\n\n", model.ModeLabel(cfg.Mode), strings.ToUpper(cfg.Role), cfg.State)
		choice, err := a.choose("What do you want to do?", []string{
			"Show status",
			"Start tunnel services",
			"Stop tunnel services (strict guard stays active)",
			"Restart tunnel services",
			"Run diagnostics",
			"Run strict IP-leak fail-closed test",
			"Print 3x-ui/Xray configuration",
			"View recent service logs",
			"Repair/reapply firewall and egress guard",
			"Accept a pairing response",
			"Show pairing token",
			"Show saved files and paths",
			"Exit",
			"Uninstall",
		})
		if err != nil {
			return err
		}
		switch choice {
		case 0:
			return a.Status()
		case 1:
			return a.ManageLink("start")
		case 2:
			return a.ManageLink("stop")
		case 3:
			return a.ManageLink("restart")
		case 4:
			return a.Doctor()
		case 5:
			return a.LeakTest()
		case 6:
			return a.PrintXray(nil)
		case 7:
			return a.Logs()
		case 8:
			return a.RepairGuards()
		case 9:
			if cfg.Role != model.RoleOutside {
				return errors.New("only the outside gateway accepts a response token")
			}
			value, err := a.prompt("Paste the RESPONSE token", "")
			if err != nil {
				return err
			}
			return a.Accept(value, "")
		case 10:
			return a.ShowPairingToken()
		case 11:
			return a.ShowPaths()
		case 12:
			return nil
		case 13:
			confirm, err := a.prompt("Type REMOVE to uninstall Hamara", "")
			if err != nil {
				return err
			}
			if confirm != "REMOVE" {
				return errors.New("uninstall cancelled")
			}
			return a.Uninstall(false)
		}
	}

	roleChoice, err := a.choose("Which server is this?", []string{
		"Iran edge server", "Outside gateway server",
	})
	if err != nil {
		return err
	}
	if roleChoice == 0 {
		value, err := a.prompt("Paste the OFFER token from the outside gateway", "")
		if err != nil {
			return err
		}
		return a.Join(value, "")
	}

	modeChoice, err := a.choose("Choose a tunnel mode", []string{
		"ARA Routed TLS — kernel IP forwarding over WireGuard-over-WSS/TLS",
		"WireGuard Direct — fastest encrypted routed link",
		"IPIP Routed — low overhead, IPv4 only, NOT encrypted",
		"SSH SOCKS — TCP-only fallback for restrictive networks",
	})
	if err != nil {
		return err
	}
	modes := []string{model.ModeARA, model.ModeWireGuard, model.ModeIPIP, model.ModeSSHSocks}
	mode := modes[modeChoice]
	_, localIP, _ := platform.DetectWAN(a.Runner)

	defaultHost := localIP
	if mode == model.ModeARA {
		defaultHost = ""
	}
	host, err := a.prompt("Public endpoint domain or IPv4 address", defaultHost)
	if err != nil {
		return err
	}

	opts := InitOptions{Mode: mode, EndpointHost: host}
	switch mode {
	case model.ModeARA:
		port, err := a.promptInt("Public ARA TLS port", 443, 1, 65535)
		if err != nil {
			return err
		}
		opts.EndpointPort = port
		tlsChoice, err := a.choose("TLS certificate mode", []string{
			"Let's Encrypt (domain required; port 80 must be reachable)",
			"Use existing PEM certificate and key",
			"Self-signed fallback (outer TLS is not verified; inner WireGuard remains authenticated)",
		})
		if err != nil {
			return err
		}
		switch tlsChoice {
		case 0:
			opts.TLSMode = "acme"
			opts.ACMEEmail, err = a.prompt("ACME email (blank is allowed)", "")
		case 1:
			opts.TLSMode = "existing"
			opts.TLSCertPath, err = a.prompt("Full-chain certificate path", "/etc/letsencrypt/live/DOMAIN/fullchain.pem")
			if err == nil {
				opts.TLSKeyPath, err = a.prompt("Private-key path", "/etc/letsencrypt/live/DOMAIN/privkey.pem")
			}
		case 2:
			opts.TLSMode = "self-signed"
		}
		if err != nil {
			return err
		}
	case model.ModeWireGuard:
		opts.EndpointPort, err = a.promptInt("WireGuard UDP port", 51820, 1, 65535)
	case model.ModeIPIP:
		opts.EndpointPort = 4 // protocol number; not a TCP/UDP listener
	case model.ModeSSHSocks:
		opts.EndpointPort, err = a.promptInt("SSH port", 22, 1, 65535)
	}
	if err != nil {
		return err
	}
	return a.InitOutside(opts)
}

func (a *App) InitOutside(opts InitOptions) error {
	if err := platform.RequireRootUbuntu(); err != nil {
		return err
	}
	if !model.IsMode(opts.Mode) {
		return fmt.Errorf("unsupported mode %q", opts.Mode)
	}
	opts.EndpointHost = strings.TrimSpace(opts.EndpointHost)
	if platform.ConfigExists() {
		if !opts.Force {
			return errors.New("Hamara is already configured; uninstall it first or use --force")
		}
		if err := a.Uninstall(false); err != nil {
			return fmt.Errorf("replace existing configuration: %w", err)
		}
	}
	if !model.ValidEndpointHost(opts.EndpointHost) {
		return errors.New("--endpoint must be a valid IPv4 address or DNS hostname")
	}
	if opts.EndpointPort == 0 {
		opts.EndpointPort = defaultPort(opts.Mode)
	}
	if opts.EndpointPort < 1 || opts.EndpointPort > 65535 {
		return errors.New("endpoint port must be between 1 and 65535")
	}
	if opts.Mode == model.ModeIPIP {
		ip := net.ParseIP(opts.EndpointHost)
		if ip == nil || ip.To4() == nil {
			return errors.New("IPIP requires the gateway's public IPv4 address, not a domain")
		}
	}
	if opts.Mode == model.ModeARA && opts.TLSMode == "" {
		opts.TLSMode = "self-signed"
	}
	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	fmt.Fprintln(a.Out, "[1/6] Installing required Ubuntu packages...")
	if err := a.installForMode(opts.Mode, opts.TLSMode); err != nil {
		return err
	}
	wan, localIP, err := platform.DetectWAN(a.Runner)
	if err != nil {
		return err
	}
	if err := a.checkOutsideConflicts(opts.Mode, opts.EndpointPort); err != nil {
		return err
	}
	id, err := randomHex(8)
	if err != nil {
		return err
	}
	cfg := defaults(opts.Mode)
	cfg.Version = model.SchemaVersion
	cfg.InstanceID = id
	cfg.Role = model.RoleOutside
	cfg.State = model.StatePending
	cfg.EndpointHost = strings.TrimSpace(opts.EndpointHost)
	cfg.EndpointPort = opts.EndpointPort
	if opts.Mode == model.ModeWireGuard {
		cfg.WireGuardPort = opts.EndpointPort
	}
	cfg.WANInterface = wan
	cfg.CreatedAt = time.Now().UTC()
	cfg.PeerEndpoint = ""
	if model.IsRoutedMode(cfg.Mode) {
		capturePreviousSysctls(&cfg)
	}

	fmt.Fprintln(a.Out, "[2/6] Generating link credentials...")
	var offer model.Offer
	switch opts.Mode {
	case model.ModeARA, model.ModeWireGuard:
		if err := a.generateWGKeys(true); err != nil {
			return err
		}
		if opts.Mode == model.ModeARA {
			cfg.TLSMode = opts.TLSMode
			cfg.TLSVerify = opts.TLSMode == "acme" || opts.TLSMode == "existing"
			cfg.ARAPath, err = randomHex(24)
			if err != nil {
				return err
			}
			if err := a.prepareTLS(&cfg, opts); err != nil {
				return err
			}
			if err := a.prepareMTLS(&cfg); err != nil {
				return err
			}
			if err := a.installWSTunnel(); err != nil {
				return err
			}
		}
		if err := a.writeWireGuard(cfg, ""); err != nil {
			return err
		}
	case model.ModeIPIP:
		if cfg.EndpointHost != localIP {
			return fmt.Errorf("IPIP endpoint %s is not assigned to the detected WAN interface (source %s); 1:1 NAT is not supported", cfg.EndpointHost, localIP)
		}
	case model.ModeSSHSocks:
		cfg.SSHPort = opts.EndpointPort
		cfg.SSHUser = sshUser
		if err := a.setupSSHGateway(&cfg); err != nil {
			return err
		}
		if !platform.IsPortListening(a.Runner, "tcp", cfg.SSHPort) {
			return fmt.Errorf("sshd is not listening on TCP port %d; choose the server's existing SSH port", cfg.SSHPort)
		}
	}

	fmt.Fprintln(a.Out, "[3/6] Writing persistent system configuration...")
	if err := platform.SaveConfig(cfg); err != nil {
		return err
	}
	if model.IsRoutedMode(cfg.Mode) {
		if err := platform.WriteFile("/etc/sysctl.d/90-hamara-tunnel.conf", []byte(render.Sysctl()), 0644); err != nil {
			return err
		}
		if err := a.Runner.Run("sysctl", "--system"); err != nil {
			return err
		}
	}

	fmt.Fprintln(a.Out, "[4/6] Enabling systemd services...")
	if err := a.activateOutsidePending(cfg); err != nil {
		return err
	}

	fmt.Fprintln(a.Out, "[5/6] Building the pairing offer...")
	offer, err = a.makeOffer(cfg)
	if err != nil {
		return err
	}
	if err := model.ValidateOffer(offer); err != nil {
		return fmt.Errorf("generated pairing offer failed validation: %w", err)
	}
	offerToken, err := token.Encode("offer", offer)
	if err != nil {
		return err
	}
	if err := platform.WriteFile("/etc/hamara/offer.token", []byte(offerToken+"\n"), 0600); err != nil {
		return err
	}
	_ = platform.WriteFile("/root/hamara-offer.txt", []byte(offerToken+"\n"), 0600)
	if cfg.Mode == model.ModeARA {
		// One peer per v0.2 instance: retain only the client CA certificate on
		// the server. The issued client private key remains solely in the
		// root-only OFFER token and is installed on the Iran edge.
		_ = os.Remove(mtlsCAKeyPath)
		_ = os.Remove(mtlsClientCert)
		_ = os.Remove(mtlsClientKey)
	}

	fmt.Fprintln(a.Out, "[6/6] Outside gateway is waiting for its peer.")
	fmt.Fprintln(a.Out, "\n=== COPY THIS SECRET OFFER TOKEN TO THE IRAN SERVER ===")
	fmt.Fprintln(a.Out, offerToken)
	fmt.Fprintln(a.Out, "=== END OFFER TOKEN ===")
	fmt.Fprintln(a.Out, "\nNext: run `hamara`, choose Iran edge, and paste the token.")
	return nil
}

func (a *App) Join(rawToken, tokenFile string) error {
	if err := platform.RequireRootUbuntu(); err != nil {
		return err
	}
	if platform.ConfigExists() {
		return errors.New("Hamara is already configured on this server")
	}
	value, err := tokenInput(rawToken, tokenFile)
	if err != nil {
		return err
	}
	var offer model.Offer
	if err := token.Decode(value, "offer", &offer); err != nil {
		return err
	}
	if err := model.ValidateOffer(offer); err != nil {
		return err
	}
	if err := platform.EnsureDirs(); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Joining %s instance %s...\n", model.ModeLabel(offer.Mode), offer.InstanceID)
	if err := a.installForMode(offer.Mode, ""); err != nil {
		return err
	}
	wan, localIP, err := platform.DetectWAN(a.Runner)
	if err != nil {
		return err
	}
	if err := a.checkIranConflicts(offer); err != nil {
		return err
	}
	cfg := model.Config{
		Version: model.SchemaVersion, InstanceID: offer.InstanceID, Role: model.RoleIran,
		Mode: offer.Mode, State: model.StatePending, Interface: "hamara0", WANInterface: wan,
		TunnelCIDR: offer.TunnelCIDR, GatewayAddress: offer.GatewayAddress, EdgeAddress: offer.EdgeAddress,
		EndpointHost: offer.EndpointHost, EndpointPort: offer.EndpointPort,
		WireGuardPort: offer.WireGuardPort, LocalRelayPort: offer.LocalRelayPort,
		MTU: offer.MTU, RouteTable: offer.RouteTable, PeerPublicKey: offer.GatewayPublicKey,
		ARAPath: offer.ARAPath, TLSMode: offer.TLSMode, TLSVerify: offer.TLSVerify,
		SSHUser: offer.SSHUser, SSHPort: offer.SSHPort, SSHHostKey: offer.SSHHostKey,
		SOCKSPort: offer.SOCKSPort, CreatedAt: time.Now().UTC(), PeerEndpoint: localIP,
	}
	if model.IsRoutedMode(cfg.Mode) {
		capturePreviousSysctls(&cfg)
	}

	var response model.Response
	response.Version = model.SchemaVersion
	response.InstanceID = offer.InstanceID
	response.Mode = offer.Mode

	switch offer.Mode {
	case model.ModeARA, model.ModeWireGuard:
		if err := a.generateWGKeys(false); err != nil {
			return err
		}
		if err := platform.WriteFile(wgPSKPath, []byte(strings.TrimSpace(offer.PresharedKey)+"\n"), 0600); err != nil {
			return err
		}
		pub, err := readTrim(wgPublicPath)
		if err != nil {
			return err
		}
		response.EdgePublicKey = pub
		if offer.Mode == model.ModeARA {
			cfg.MTLSClientCertPath = mtlsClientCert
			cfg.MTLSClientKeyPath = mtlsClientKey
			if err := os.MkdirAll(mtlsDir, 0700); err != nil {
				return err
			}
			if err := platform.WriteFile(mtlsClientCert, []byte(strings.TrimSpace(offer.MTLSClientCert)+"\n"), 0600); err != nil {
				return err
			}
			if err := platform.WriteFile(mtlsClientKey, []byte(strings.TrimSpace(offer.MTLSClientKey)+"\n"), 0600); err != nil {
				return err
			}
			if err := a.installWSTunnel(); err != nil {
				return err
			}
			if err := platform.WriteFile("/etc/systemd/system/hamara-ara-client.service", []byte(render.ARAClientUnit(cfg)), 0644); err != nil {
				return err
			}
		}
		if err := a.writeWireGuard(cfg, offer.GatewayPublicKey); err != nil {
			return err
		}
	case model.ModeIPIP:
		response.EdgeEndpointIP = localIP
		if err := platform.WriteFile(platform.LibDir+"/ipip", []byte(render.IPIPScript(cfg)), 0755); err != nil {
			return err
		}
		if err := platform.WriteFile("/etc/systemd/system/hamara-ipip.service", []byte(render.IPIPUnit(cfg.Role)), 0644); err != nil {
			return err
		}
	case model.ModeSSHSocks:
		if err := a.setupSSHClient(&cfg); err != nil {
			return err
		}
		pub, err := readTrim("/etc/hamara/secrets/ssh_ed25519.pub")
		if err != nil {
			return err
		}
		response.SSHPublicKey = pub
	}

	if model.IsRoutedMode(cfg.Mode) {
		if err := a.writeStrictGuard(cfg); err != nil {
			return err
		}
	}
	if err := platform.SaveConfig(cfg); err != nil {
		return err
	}
	if model.IsRoutedMode(cfg.Mode) {
		if err := platform.WriteFile("/etc/sysctl.d/90-hamara-tunnel.conf", []byte(render.Sysctl()), 0644); err != nil {
			return err
		}
		if err := a.Runner.Run("sysctl", "--system"); err != nil {
			return err
		}
	}
	if err := a.activateIran(cfg); err != nil {
		return err
	}
	responseToken, err := token.Encode("response", response)
	if err != nil {
		return err
	}
	if err := platform.WriteFile("/etc/hamara/response.token", []byte(responseToken+"\n"), 0600); err != nil {
		return err
	}
	_ = platform.WriteFile("/root/hamara-response.txt", []byte(responseToken+"\n"), 0600)
	if b, err := render.XrayOutbound(cfg); err == nil {
		_ = platform.WriteFile("/etc/hamara/xray-outbound.json", append(b, '\n'), 0640)
	}

	fmt.Fprintln(a.Out, "\n=== COPY THIS RESPONSE TOKEN TO THE OUTSIDE SERVER ===")
	fmt.Fprintln(a.Out, responseToken)
	fmt.Fprintln(a.Out, "=== END RESPONSE TOKEN ===")
	fmt.Fprintln(a.Out, "\nNext: on the outside gateway run `hamara`, choose Accept a pairing response, and paste the token.")
	return nil
}

func (a *App) Accept(rawToken, tokenFile string) error {
	if err := platform.RequireRootUbuntu(); err != nil {
		return err
	}
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Role != model.RoleOutside {
		return errors.New("this command must run on the outside gateway")
	}
	value, err := tokenInput(rawToken, tokenFile)
	if err != nil {
		return err
	}
	var response model.Response
	if err := token.Decode(value, "response", &response); err != nil {
		return err
	}
	if err := model.ValidateResponse(response, cfg); err != nil {
		return err
	}

	switch cfg.Mode {
	case model.ModeARA, model.ModeWireGuard:
		cfg.PeerPublicKey = strings.TrimSpace(response.EdgePublicKey)
		if err := a.writeWireGuard(cfg, cfg.PeerPublicKey); err != nil {
			return err
		}
		if err := a.Runner.Run("systemctl", "restart", "wg-quick@hamara0.service"); err != nil {
			return err
		}
	case model.ModeIPIP:
		cfg.PeerEndpoint = response.EdgeEndpointIP
		if err := platform.WriteFile(platform.LibDir+"/ipip", []byte(render.IPIPScript(cfg)), 0755); err != nil {
			return err
		}
		if err := platform.WriteFile(platform.LibDir+"/firewall", []byte(render.FirewallScript(cfg)), 0755); err != nil {
			return err
		}
		if err := platform.WriteFile("/etc/systemd/system/hamara-ipip.service", []byte(render.IPIPUnit(cfg.Role)), 0644); err != nil {
			return err
		}
		if err := platform.WriteFile("/etc/systemd/system/hamara-firewall.service", []byte(render.FirewallUnit()), 0644); err != nil {
			return err
		}
		if err := a.Runner.Run("systemctl", "daemon-reload"); err != nil {
			return err
		}
		if err := a.Runner.Run("systemctl", "enable", "--now", "hamara-ipip.service", "hamara-firewall.service"); err != nil {
			return err
		}
	case model.ModeSSHSocks:
		if err := a.authorizeSSHPeer(cfg, response.SSHPublicKey); err != nil {
			return err
		}
	}
	cfg.State = model.StateReady
	cfg.AcceptedAt = time.Now().UTC()
	if err := platform.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Pairing accepted. The Hamara link is now enabled.")
	fmt.Fprintln(a.Out, "Run `hamara status` on both servers, then configure 3x-ui on the Iran edge.")
	return nil
}

func (a *App) ShowPairingToken() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	path := "/etc/hamara/response.token"
	label := "RESPONSE"
	if cfg.Role == model.RoleOutside {
		path, label = "/etc/hamara/offer.token", "OFFER"
	}
	value, err := readTrim(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "%s token (treat it as a secret):\n%s\n", label, value)
	return nil
}

func (a *App) PrintXray(inboundTags []string) error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Role != model.RoleIran {
		return errors.New("3x-ui/Xray egress configuration belongs on the Iran edge server")
	}
	outbound, err := render.XrayOutbound(cfg)
	if err != nil {
		return err
	}
	rule := render.XrayRoutingAll()
	if len(inboundTags) > 0 {
		rule = render.XrayRoutingInbound(inboundTags)
	}
	fmt.Fprintln(a.Out, "Add this object to Xray Settings -> Outbounds:")
	fmt.Fprintln(a.Out, string(outbound))
	fmt.Fprintln(a.Out, "\nAdd this rule near the end of Xray Settings -> Routing Rules:")
	fmt.Fprintln(a.Out, string(rule))
	if cfg.Mode == model.ModeSSHSocks {
		fmt.Fprintln(a.Out, "\nNote: SSH SOCKS is TCP-only; use ARA or WireGuard for UDP traffic.")
	}
	return nil
}

func (a *App) Status() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Hamara Tunnel %s\n", a.Version)
	fmt.Fprintf(a.Out, "  Instance : %s\n", cfg.InstanceID)
	fmt.Fprintf(a.Out, "  Role     : %s\n", strings.ToUpper(cfg.Role))
	fmt.Fprintf(a.Out, "  Mode     : %s\n", model.ModeLabel(cfg.Mode))
	fmt.Fprintf(a.Out, "  State    : %s\n", cfg.State)
	fmt.Fprintf(a.Out, "  Endpoint : %s:%d\n", cfg.EndpointHost, cfg.EndpointPort)
	for _, svc := range serviceNames(cfg) {
		fmt.Fprintf(a.Out, "  %-22s %s\n", svc, a.serviceState(svc))
	}
	if cfg.Mode == model.ModeARA || cfg.Mode == model.ModeWireGuard {
		if out, err := a.Runner.Output("wg", "show", cfg.Interface); err == nil {
			fmt.Fprintln(a.Out, "\nWireGuard:")
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "latest handshake") || strings.Contains(line, "transfer:") || strings.HasPrefix(line, "interface:") {
					fmt.Fprintln(a.Out, "  "+strings.TrimSpace(line))
				}
			}
		} else {
			fmt.Fprintln(a.Out, "\nWireGuard interface is not available yet.")
		}
	}
	return nil
}

func (a *App) Doctor() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Hamara diagnostics")
	checks := []struct{ name, value string }{
		{"Configuration", "OK"},
		{"IPv4 forwarding", readSysctl("/proc/sys/net/ipv4/ip_forward", "unknown")},
		{"WAN interface", cfg.WANInterface},
	}
	for _, c := range checks {
		fmt.Fprintf(a.Out, "  %-24s %s\n", c.name+":", c.value)
	}
	for _, svc := range serviceNames(cfg) {
		fmt.Fprintf(a.Out, "  %-24s %s\n", svc+":", a.serviceState(svc))
	}
	if cfg.Mode != model.ModeSSHSocks {
		if _, err := a.Runner.Output("ip", "link", "show", "dev", cfg.Interface); err != nil {
			fmt.Fprintf(a.Out, "  %-24s FAIL (%v)\n", "Tunnel interface:", err)
		} else {
			fmt.Fprintf(a.Out, "  %-24s OK (%s)\n", "Tunnel interface:", cfg.Interface)
		}
	}
	if cfg.Role == model.RoleIran && model.IsRoutedMode(cfg.Mode) {
		rules, _ := a.Runner.Output("ip", "rule", "show")
		routes, _ := a.Runner.Output("ip", "route", "show", "table", strconv.Itoa(cfg.RouteTable))
		if strings.Contains(rules, fmt.Sprintf("lookup %d", cfg.RouteTable)) && strings.Contains(routes, "unreachable default") {
			fmt.Fprintf(a.Out, "  %-24s OK (mark %d, table %d)\n", "Strict egress guard:", render.XrayMark, cfg.RouteTable)
		} else {
			fmt.Fprintf(a.Out, "  %-24s FAIL (policy rule or unreachable fallback missing)\n", "Strict egress guard:")
		}
	}
	if cfg.Role == model.RoleIran {
		fmt.Fprintln(a.Out, "\nEgress test (can take up to 12 seconds):")
		var cmd *exec.Cmd
		if cfg.Mode == model.ModeSSHSocks {
			cmd = exec.Command("curl", "-4fsS", "--max-time", "12", "--socks5-hostname", fmt.Sprintf("127.0.0.1:%d", cfg.SOCKSPort), "https://api.ipify.org")
		} else {
			cmd = exec.Command("curl", "-4fsS", "--max-time", "12", "--interface", hostOnly(cfg.EdgeAddress), "https://api.ipify.org")
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(a.Out, "  FAIL: %v (%s)\n", err, strings.TrimSpace(string(out)))
			fmt.Fprintln(a.Out, "  Inspect: journalctl -u 'hamara-*' -u 'wg-quick@hamara0' --since '-10 min'")
		} else {
			fmt.Fprintf(a.Out, "  OK: outside egress IPv4 is %s\n", strings.TrimSpace(string(out)))
			if cfg.State != model.StateReady {
				cfg.State = model.StateReady
				cfg.AcceptedAt = time.Now().UTC()
				_ = platform.SaveConfig(cfg)
			}
		}
	}
	return nil
}

func (a *App) ManageLink(action string) error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unsupported service action %q", action)
	}
	if cfg.Mode == model.ModeSSHSocks && cfg.Role == model.RoleOutside {
		return errors.New("the outside SSH fallback uses the existing ssh.service; Hamara will not stop or restart your management SSH daemon")
	}
	startUnits, stopUnits := managedLinkUnits(cfg)
	stop := func() error {
		for _, unit := range stopUnits {
			if err := a.Runner.Run("systemctl", "stop", unit); err != nil {
				return err
			}
		}
		return nil
	}
	start := func() error {
		for _, unit := range startUnits {
			if err := a.Runner.Run("systemctl", "enable", "--now", unit); err != nil {
				return err
			}
		}
		return nil
	}
	switch action {
	case "start":
		err = start()
	case "stop":
		err = stop()
	case "restart":
		if err = stop(); err == nil {
			err = start()
		}
	}
	if err != nil {
		return err
	}
	if action == "stop" && cfg.Role == model.RoleIran && model.IsRoutedMode(cfg.Mode) {
		fmt.Fprintln(a.Out, "Tunnel transport stopped. hamara-guard remains active so marked Xray traffic fails closed.")
	} else {
		fmt.Fprintf(a.Out, "Hamara tunnel services: %s complete.\n", action)
	}
	return nil
}

func managedLinkUnits(cfg model.Config) (start, stop []string) {
	switch cfg.Mode {
	case model.ModeARA:
		if cfg.Role == model.RoleOutside {
			return []string{"wg-quick@hamara0.service", "hamara-firewall.service", "hamara-ara-server.service"}, []string{"hamara-ara-server.service", "wg-quick@hamara0.service"}
		}
		return []string{"hamara-guard.service", "hamara-ara-client.service", "wg-quick@hamara0.service"}, []string{"wg-quick@hamara0.service", "hamara-ara-client.service"}
	case model.ModeWireGuard:
		if cfg.Role == model.RoleOutside {
			return []string{"wg-quick@hamara0.service", "hamara-firewall.service"}, []string{"wg-quick@hamara0.service"}
		}
		return []string{"hamara-guard.service", "wg-quick@hamara0.service"}, []string{"wg-quick@hamara0.service"}
	case model.ModeIPIP:
		if cfg.Role == model.RoleOutside {
			return []string{"hamara-ipip.service", "hamara-firewall.service"}, []string{"hamara-ipip.service"}
		}
		return []string{"hamara-guard.service", "hamara-ipip.service"}, []string{"hamara-ipip.service"}
	case model.ModeSSHSocks:
		return []string{"hamara-ssh-socks.service"}, []string{"hamara-ssh-socks.service"}
	}
	return nil, nil
}

func (a *App) Logs() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	args := []string{"--no-pager", "-n", "200"}
	for _, service := range serviceNames(cfg) {
		args = append(args, "-u", service)
	}
	if len(serviceNames(cfg)) == 0 {
		return errors.New("this pending gateway has no dedicated Hamara service logs yet")
	}
	return a.Runner.Run("journalctl", args...)
}

func (a *App) RepairGuards() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Role == model.RoleOutside && model.IsRoutedMode(cfg.Mode) {
		if err := a.Runner.Run("systemctl", "enable", "--now", "hamara-firewall.service"); err != nil {
			return err
		}
		if err := a.Runner.Run(platform.LibDir+"/firewall", "up"); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Outside forwarding, NAT, and listener firewall rules were reapplied.")
		return nil
	}
	if cfg.Role == model.RoleIran && model.IsRoutedMode(cfg.Mode) {
		if err := a.Runner.Run("systemctl", "enable", "--now", "hamara-guard.service"); err != nil {
			return err
		}
		if err := a.Runner.Run(platform.LibDir+"/guard", "up"); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Strict marked-traffic kill switch and unreachable fallback were reapplied.")
		return nil
	}
	return errors.New("this mode has no Hamara-managed IP forwarding/firewall guard")
}

func (a *App) LeakTest() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Role != model.RoleIran || !model.IsRoutedMode(cfg.Mode) {
		return errors.New("strict leak testing is available on the routed Iran edge only")
	}
	if a.serviceState("hamara-guard.service") != "active" {
		return errors.New("hamara-guard is not active; run `hamara repair` before testing")
	}
	source := hostOnly(cfg.EdgeAddress)
	fmt.Fprintln(a.Out, "1. Verifying normal tunnel egress...")
	before, err := exec.Command("curl", "-4fsS", "--max-time", "12", "--interface", source, "https://api.ipify.org").CombinedOutput()
	if err != nil {
		return fmt.Errorf("baseline tunnel request failed; fix the link before leak testing: %v (%s)", err, strings.TrimSpace(string(before)))
	}
	fmt.Fprintf(a.Out, "   Tunnel egress: %s\n", strings.TrimSpace(string(before)))

	unit := "wg-quick@hamara0.service"
	if cfg.Mode == model.ModeIPIP {
		unit = "hamara-ipip.service"
	}
	fmt.Fprintf(a.Out, "2. Stopping %s while keeping hamara-guard active...\n", unit)
	if err := a.Runner.Run("systemctl", "stop", unit); err != nil {
		return err
	}
	restarted := false
	defer func() {
		if !restarted {
			_ = a.Runner.Run("systemctl", "start", unit)
		}
	}()

	fmt.Fprintln(a.Out, "3. Testing policy route for Xray mark 72...")
	if route, routeErr := a.Runner.Output("ip", "route", "get", "1.1.1.1", "mark", strconv.Itoa(render.XrayMark)); routeErr == nil && !strings.Contains(route, "unreachable") && !strings.Contains(route, "prohibit") {
		return fmt.Errorf("CRITICAL: marked traffic found a route while the tunnel was down: %s", route)
	}
	fmt.Fprintln(a.Out, "   PASS: marked traffic has no fallback route.")

	fmt.Fprintln(a.Out, "4. Testing a source-bound HTTP request; failure is expected...")
	leakOut, leakErr := exec.Command("curl", "-4fsS", "--max-time", "6", "--interface", source, "https://api.ipify.org").CombinedOutput()
	if leakErr == nil {
		return fmt.Errorf("CRITICAL: source-bound request escaped while tunnel was down via %s", strings.TrimSpace(string(leakOut)))
	}
	fmt.Fprintln(a.Out, "   PASS: source-bound traffic failed closed.")

	fmt.Fprintln(a.Out, "5. Restoring the tunnel...")
	if err := a.Runner.Run("systemctl", "start", unit); err != nil {
		return fmt.Errorf("leak checks passed, but tunnel restart failed: %w", err)
	}
	restarted = true
	time.Sleep(2 * time.Second)
	fmt.Fprintln(a.Out, "Strict leak test: PASS. Also test a real 3x-ui client during a maintenance window because panel rule order is outside Hamara's control.")
	return nil
}

func (a *App) ShowPaths() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	type pathInfo struct{ path, purpose string }
	paths := []pathInfo{
		{"/usr/local/bin/hamara", "Hamara CLI"},
		{"/usr/local/bin/wstunnel", "ARA transport binary (ARA only)"},
		{"/opt/hamara-tunnel", "installed documentation and license"},
		{"/etc/hamara/config.json", "root-only instance configuration"},
		{"/etc/hamara/offer.token", "SECRET outside pairing offer"},
		{"/etc/hamara/response.token", "Iran pairing response"},
		{"/etc/hamara/secrets", "root-only WireGuard/SSH private keys"},
		{"/etc/hamara/mtls", "ARA mutual-TLS CA or client identity"},
		{"/etc/hamara/xray-outbound.json", "generated 3x-ui/Xray outbound"},
		{"/etc/wireguard/hamara0.conf", "WireGuard interface and private key"},
		{"/etc/sysctl.d/90-hamara-tunnel.conf", "IPv4 forwarding/rp_filter settings"},
		{"/usr/local/lib/hamara/guard", "Iran strict egress kill-switch helper"},
		{"/usr/local/lib/hamara/firewall", "outside forwarding/NAT helper"},
		{"/usr/local/lib/hamara/ipip", "IPIP interface helper"},
		{"/etc/systemd/system/hamara-guard.service", "strict egress guard unit"},
		{"/etc/systemd/system/hamara-ara-client.service", "ARA Iran transport unit"},
		{"/etc/systemd/system/hamara-ara-server.service", "ARA outside transport unit"},
		{"/etc/systemd/system/hamara-firewall.service", "outside firewall/NAT unit"},
		{"/etc/systemd/system/hamara-ipip.service", "IPIP unit"},
		{"/etc/systemd/system/hamara-ssh-socks.service", "SSH SOCKS client unit"},
		{"/etc/systemd/system/wg-quick@hamara0.service.d/hamara-guard.conf", "WireGuard requires strict guard"},
		{"/etc/ssh/sshd_config.d/90-hamara-tunnel.conf", "restricted SSH fallback account policy"},
		{"/var/lib/hamara-ssh/.ssh/authorized_keys", "paired SSH forwarding public key"},
		{"/root/hamara-offer.txt", "SECRET convenience copy of OFFER"},
		{"/root/hamara-response.txt", "convenience copy of RESPONSE"},
	}
	fmt.Fprintf(a.Out, "Hamara saved files for %s / %s:\n", strings.ToUpper(cfg.Role), model.ModeLabel(cfg.Mode))
	for _, item := range paths {
		state := "not used"
		if _, statErr := os.Stat(item.path); statErr == nil {
			state = "present"
		}
		fmt.Fprintf(a.Out, "  %-58s %-8s %s\n", item.path, state, item.purpose)
	}
	fmt.Fprintln(a.Out, "Never share OFFER tokens, private keys, WireGuard configuration, or files under /etc/hamara/secrets and /etc/hamara/mtls.")
	return nil
}

func (a *App) Uninstall(purge bool) error {
	if err := platform.RequireRootUbuntu(); err != nil {
		return err
	}
	cfg, err := platform.LoadConfig()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	services := []string{"hamara-ara-server.service", "hamara-ara-client.service", "hamara-ipip.service", "hamara-ssh-socks.service", "hamara-firewall.service", "wg-quick@hamara0.service", "hamara-guard.service"}
	for _, svc := range services {
		_ = a.Runner.Run("systemctl", "disable", "--now", svc)
	}
	if model.IsRoutedMode(cfg.Mode) {
		a.restorePreviousSysctls(cfg)
	}
	_ = os.Remove("/etc/wireguard/hamara0.conf")
	for _, path := range []string{
		"/etc/systemd/system/hamara-ara-server.service", "/etc/systemd/system/hamara-ara-client.service",
		"/etc/systemd/system/hamara-ipip.service", "/etc/systemd/system/hamara-ssh-socks.service",
		"/etc/systemd/system/hamara-firewall.service", "/etc/systemd/system/hamara-guard.service",
		"/etc/sysctl.d/90-hamara-tunnel.conf", "/etc/ssh/sshd_config.d/90-hamara-tunnel.conf",
		"/root/hamara-offer.txt", "/root/hamara-response.txt",
	} {
		_ = os.Remove(path)
	}
	_ = os.Remove("/etc/systemd/system/wg-quick@hamara0.service.d/hamara-guard.conf")
	_ = os.Remove("/etc/systemd/system/wg-quick@hamara0.service.d") // succeeds only when empty
	if cfg.Mode == model.ModeSSHSocks && cfg.Role == model.RoleOutside {
		_ = a.Runner.Run("userdel", "--remove", sshUser)
		_ = a.Runner.Run("systemctl", "reload", "ssh.service")
	}
	_ = os.RemoveAll(platform.ConfigDir)
	_ = os.RemoveAll(platform.LibDir)
	if purge {
		_ = os.Remove("/usr/local/bin/wstunnel")
	}
	_ = a.Runner.Run("systemctl", "daemon-reload")
	fmt.Fprintln(a.Out, "Hamara configuration and services were removed. Ubuntu packages were left installed.")
	return nil
}

func (a *App) writeStrictGuard(cfg model.Config) error {
	if cfg.Role != model.RoleIran || !model.IsRoutedMode(cfg.Mode) {
		return errors.New("strict egress guard can only be installed for a routed Iran edge")
	}
	if err := platform.WriteFile(platform.LibDir+"/guard", []byte(render.GuardScript(cfg)), 0755); err != nil {
		return err
	}
	if err := platform.WriteFile("/etc/systemd/system/hamara-guard.service", []byte(render.GuardUnit()), 0644); err != nil {
		return err
	}
	if cfg.Mode == model.ModeARA || cfg.Mode == model.ModeWireGuard {
		if err := platform.WriteFile("/etc/systemd/system/wg-quick@hamara0.service.d/hamara-guard.conf", []byte(render.WireGuardGuardDropIn()), 0644); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) activateOutsidePending(cfg model.Config) error {
	if cfg.Mode == model.ModeSSHSocks || cfg.Mode == model.ModeIPIP {
		return nil
	}
	if err := platform.WriteFile(platform.LibDir+"/firewall", []byte(render.FirewallScript(cfg)), 0755); err != nil {
		return err
	}
	if err := platform.WriteFile("/etc/systemd/system/hamara-firewall.service", []byte(render.FirewallUnit()), 0644); err != nil {
		return err
	}
	if cfg.Mode == model.ModeARA {
		if err := platform.WriteFile("/etc/systemd/system/hamara-ara-server.service", []byte(render.ARAServerUnit(cfg)), 0644); err != nil {
			return err
		}
	}
	if err := a.Runner.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	units := []string{"wg-quick@hamara0.service", "hamara-firewall.service"}
	if cfg.Mode == model.ModeARA {
		units = append(units, "hamara-ara-server.service")
	}
	return a.Runner.Run("systemctl", append([]string{"enable", "--now"}, units...)...)
}

func (a *App) activateIran(cfg model.Config) error {
	if err := a.Runner.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if model.IsRoutedMode(cfg.Mode) {
		if err := a.Runner.Run("systemctl", "enable", "--now", "hamara-guard.service"); err != nil {
			return fmt.Errorf("strict egress guard failed; refusing to start the tunnel: %w", err)
		}
	}
	switch cfg.Mode {
	case model.ModeARA:
		if err := a.Runner.Run("systemctl", "enable", "--now", "hamara-ara-client.service"); err != nil {
			return err
		}
		return a.Runner.Run("systemctl", "enable", "--now", "wg-quick@hamara0.service")
	case model.ModeWireGuard:
		return a.Runner.Run("systemctl", "enable", "--now", "wg-quick@hamara0.service")
	case model.ModeIPIP:
		return a.Runner.Run("systemctl", "enable", "--now", "hamara-ipip.service")
	case model.ModeSSHSocks:
		// The outside gateway has not accepted the key yet, so a failed first
		// connection is expected. systemd will keep retrying.
		_ = a.Runner.Run("systemctl", "enable", "--now", "hamara-ssh-socks.service")
		return nil
	}
	return nil
}

func (a *App) checkIranConflicts(offer model.Offer) error {
	if model.IsRoutedMode(offer.Mode) {
		if _, err := a.Runner.Output("ip", "link", "show", "dev", "hamara0"); err == nil {
			return errors.New("network interface hamara0 already exists; remove the conflicting interface first")
		}
	}
	if offer.Mode == model.ModeARA && platform.IsPortListening(a.Runner, "udp", offer.LocalRelayPort) {
		return fmt.Errorf("local UDP relay port %d is already in use", offer.LocalRelayPort)
	}
	if offer.Mode == model.ModeSSHSocks && platform.IsPortListening(a.Runner, "tcp", offer.SOCKSPort) {
		return fmt.Errorf("local SOCKS port %d is already in use", offer.SOCKSPort)
	}
	return nil
}

func (a *App) checkOutsideConflicts(mode string, port int) error {
	switch mode {
	case model.ModeARA:
		if platform.IsPortListening(a.Runner, "tcp", port) {
			return fmt.Errorf("TCP port %d is already in use on the outside server", port)
		}
		if platform.IsPortListening(a.Runner, "udp", 51820) {
			return errors.New("UDP port 51820 is already in use; ARA reserves it for the loopback WireGuard listener")
		}
	case model.ModeWireGuard:
		if platform.IsPortListening(a.Runner, "udp", port) {
			return fmt.Errorf("UDP port %d is already in use on the outside server", port)
		}
	}
	if model.IsRoutedMode(mode) {
		if _, err := a.Runner.Output("ip", "link", "show", "dev", "hamara0"); err == nil {
			return errors.New("network interface hamara0 already exists; remove the conflicting interface first")
		}
	}
	return nil
}

func (a *App) installForMode(mode, tlsMode string) error {
	packages := []string{"ca-certificates", "curl", "iproute2", "iptables"}
	switch mode {
	case model.ModeARA:
		packages = append(packages, "wireguard-tools", "openssl", "tar")
		if tlsMode == "acme" {
			packages = append(packages, "certbot")
		}
	case model.ModeWireGuard:
		packages = append(packages, "wireguard-tools")
	case model.ModeSSHSocks:
		packages = append(packages, "openssh-client", "openssh-server")
	}
	return platform.InstallPackages(a.Runner, packages...)
}

func (a *App) generateWGKeys(withPSK bool) error {
	private, err := a.Runner.Output("wg", "genkey")
	if err != nil {
		return err
	}
	if err := platform.WriteFile(wgPrivatePath, []byte(private+"\n"), 0600); err != nil {
		return err
	}
	public, err := a.Runner.Output("/bin/bash", "-c", "wg pubkey < "+wgPrivatePath)
	if err != nil {
		return err
	}
	if err := platform.WriteFile(wgPublicPath, []byte(public+"\n"), 0600); err != nil {
		return err
	}
	if withPSK {
		psk, err := a.Runner.Output("wg", "genpsk")
		if err != nil {
			return err
		}
		if err := platform.WriteFile(wgPSKPath, []byte(psk+"\n"), 0600); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) writeWireGuard(cfg model.Config, peerPublic string) error {
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		return err
	}
	if err := os.Chmod("/etc/wireguard", 0700); err != nil {
		return err
	}
	private, err := readTrim(wgPrivatePath)
	if err != nil {
		return err
	}
	psk, err := readTrim(wgPSKPath)
	if err != nil {
		return err
	}
	text, err := render.WireGuard(cfg, private, psk, peerPublic)
	if err != nil {
		return err
	}
	return platform.WriteFile(wgConfigPath, []byte(text), 0600)
}

func (a *App) makeOffer(cfg model.Config) (model.Offer, error) {
	o := model.Offer{
		Version: model.SchemaVersion, InstanceID: cfg.InstanceID, Mode: cfg.Mode,
		EndpointHost: cfg.EndpointHost, EndpointPort: cfg.EndpointPort,
		TunnelCIDR: cfg.TunnelCIDR, GatewayAddress: cfg.GatewayAddress, EdgeAddress: cfg.EdgeAddress,
		MTU: cfg.MTU, RouteTable: cfg.RouteTable, WireGuardPort: cfg.WireGuardPort,
		LocalRelayPort: cfg.LocalRelayPort, ARAPath: cfg.ARAPath, TLSMode: cfg.TLSMode,
		TLSVerify: cfg.TLSVerify, SSHUser: cfg.SSHUser, SSHPort: cfg.SSHPort,
		SSHHostKey: cfg.SSHHostKey, SOCKSPort: cfg.SOCKSPort,
	}
	switch cfg.Mode {
	case model.ModeARA, model.ModeWireGuard:
		var err error
		o.GatewayPublicKey, err = readTrim(wgPublicPath)
		if err != nil {
			return o, err
		}
		o.PresharedKey, err = readTrim(wgPSKPath)
		if err != nil {
			return o, err
		}
		if cfg.Mode == model.ModeARA {
			o.MTLSClientCert, err = readTrim(mtlsClientCert)
			if err != nil {
				return o, err
			}
			o.MTLSClientKey, err = readTrim(mtlsClientKey)
			if err != nil {
				return o, err
			}
		}
	case model.ModeIPIP:
		o.GatewayEndpointIP = cfg.EndpointHost
	}
	return o, nil
}

func (a *App) prepareTLS(cfg *model.Config, opts InitOptions) error {
	switch opts.TLSMode {
	case "existing":
		if !filepath.IsAbs(opts.TLSCertPath) || !filepath.IsAbs(opts.TLSKeyPath) || strings.ContainsAny(opts.TLSCertPath+opts.TLSKeyPath, "\r\n") {
			return errors.New("existing TLS mode requires absolute, single-line --cert and --key paths")
		}
		if _, err := os.Stat(opts.TLSCertPath); err != nil {
			return fmt.Errorf("certificate: %w", err)
		}
		if _, err := os.Stat(opts.TLSKeyPath); err != nil {
			return fmt.Errorf("private key: %w", err)
		}
		cfg.TLSCertPath, cfg.TLSKeyPath = opts.TLSCertPath, opts.TLSKeyPath
	case "acme":
		if net.ParseIP(cfg.EndpointHost) != nil {
			return errors.New("Let's Encrypt mode requires a DNS name, not an IP address")
		}
		if platform.IsPortListening(a.Runner, "tcp", 80) {
			return errors.New("TCP port 80 is already in use; stop that service for ACME or use an existing certificate")
		}
		args := []string{"certonly", "--standalone", "--non-interactive", "--agree-tos", "-d", cfg.EndpointHost}
		if strings.TrimSpace(opts.ACMEEmail) == "" {
			args = append(args, "--register-unsafely-without-email")
		} else {
			args = append(args, "--email", strings.TrimSpace(opts.ACMEEmail))
		}
		if err := a.Runner.Run("certbot", args...); err != nil {
			return err
		}
		cfg.TLSCertPath = filepath.Join("/etc/letsencrypt/live", cfg.EndpointHost, "fullchain.pem")
		cfg.TLSKeyPath = filepath.Join("/etc/letsencrypt/live", cfg.EndpointHost, "privkey.pem")
	case "self-signed":
		cert := "/etc/hamara/tls/fullchain.pem"
		key := "/etc/hamara/tls/privkey.pem"
		if err := os.MkdirAll(filepath.Dir(cert), 0700); err != nil {
			return err
		}
		san := "DNS:" + cfg.EndpointHost
		if net.ParseIP(cfg.EndpointHost) != nil {
			san = "IP:" + cfg.EndpointHost
		}
		if err := a.Runner.Run("openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048", "-sha256", "-days", "825", "-subj", "/CN="+cfg.EndpointHost, "-addext", "subjectAltName="+san, "-keyout", key, "-out", cert); err != nil {
			return err
		}
		_ = os.Chmod(key, 0600)
		_ = os.Chmod(cert, 0644)
		cfg.TLSCertPath, cfg.TLSKeyPath = cert, key
		cfg.TLSVerify = false
	default:
		return fmt.Errorf("unsupported TLS mode %q (use acme, existing, or self-signed)", opts.TLSMode)
	}
	return nil
}

func (a *App) prepareMTLS(cfg *model.Config) error {
	if err := os.MkdirAll(mtlsDir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(mtlsDir, 0700); err != nil {
		return err
	}
	clientCSR := filepath.Join(mtlsDir, "client.csr")
	extFile := filepath.Join(mtlsDir, "client-ext.cnf")
	serialFile := filepath.Join(mtlsDir, "client-ca.srl")
	for _, path := range []string{mtlsCAPath, mtlsCAKeyPath, mtlsClientCert, mtlsClientKey, clientCSR, serialFile} {
		_ = os.Remove(path)
	}
	if err := platform.WriteFile(extFile, []byte("basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\n"), 0600); err != nil {
		return err
	}
	commands := [][]string{
		{"ecparam", "-name", "prime256v1", "-genkey", "-noout", "-out", mtlsCAKeyPath},
		{"req", "-new", "-x509", "-sha256", "-days", "3650", "-key", mtlsCAKeyPath, "-subj", "/CN=Hamara-ARA-Client-CA-" + cfg.InstanceID, "-out", mtlsCAPath},
		{"ecparam", "-name", "prime256v1", "-genkey", "-noout", "-out", mtlsClientKey},
		// wstunnel binds mTLS identity to the WebSocket upgrade path. Using the
		// random path as the client certificate CN satisfies both checks.
		{"req", "-new", "-sha256", "-key", mtlsClientKey, "-subj", "/CN=" + cfg.ARAPath, "-out", clientCSR},
		{"x509", "-req", "-sha256", "-days", "825", "-in", clientCSR, "-CA", mtlsCAPath, "-CAkey", mtlsCAKeyPath, "-CAcreateserial", "-extfile", extFile, "-out", mtlsClientCert},
	}
	for _, args := range commands {
		if err := a.Runner.Run("openssl", args...); err != nil {
			return fmt.Errorf("create ARA mutual-TLS identity: %w", err)
		}
	}
	for _, path := range []string{mtlsCAKeyPath, mtlsClientKey} {
		if err := os.Chmod(path, 0600); err != nil {
			return err
		}
	}
	if err := os.Chmod(mtlsCAPath, 0644); err != nil {
		return err
	}
	if err := os.Chmod(mtlsClientCert, 0600); err != nil {
		return err
	}
	_ = os.Remove(clientCSR)
	_ = os.Remove(extFile)
	_ = os.Remove(serialFile)
	cfg.MTLSCAPath = mtlsCAPath
	return nil
}

func (a *App) installWSTunnel() error {
	if out, err := a.Runner.Output("/usr/local/bin/wstunnel", "--version"); err == nil && strings.Contains(out, wstunnelVer) {
		return nil
	}
	arch, err := a.Runner.Output("dpkg", "--print-architecture")
	if err != nil {
		return err
	}
	var assetArch, checksum string
	switch arch {
	case "amd64":
		assetArch = "amd64"
		checksum = "fa842ed53fbb14b1c69cd98829f9895d7f8a6b0d562c57c1175851a52cea9ea2"
	case "arm64":
		assetArch = "arm64"
		checksum = "99f9506d01d1b4073254609600ec5056dab8dc58aec75c32f6eb0508335a8fd2"
	default:
		return fmt.Errorf("ARA supports amd64 and arm64; detected %s", arch)
	}
	url := fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/v%s/wstunnel_%s_linux_%s.tar.gz", wstunnelVer, wstunnelVer, assetArch)
	script := fmt.Sprintf(`set -euo pipefail
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fL --retry 3 %q -o "$work/wstunnel.tar.gz"
printf '%%s  %%s\n' %q "$work/wstunnel.tar.gz" | sha256sum -c -
tar -xzf "$work/wstunnel.tar.gz" -C "$work" wstunnel
install -m 0755 "$work/wstunnel" /usr/local/bin/wstunnel
`, url, checksum)
	return a.Runner.Bash(script)
}

func (a *App) setupSSHGateway(cfg *model.Config) error {
	if !platform.CommandExists("sshd") {
		return errors.New("OpenSSH server is not installed")
	}
	if err := a.Runner.Bash(fmt.Sprintf(`set -e
if ! id -u %s >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/hamara-ssh --shell /bin/bash %s
fi
install -d -m 0700 -o %s -g %s /var/lib/hamara-ssh/.ssh
touch /var/lib/hamara-ssh/.ssh/authorized_keys
chown %s:%s /var/lib/hamara-ssh/.ssh/authorized_keys
chmod 0600 /var/lib/hamara-ssh/.ssh/authorized_keys
`, sshUser, sshUser, sshUser, sshUser, sshUser, sshUser)); err != nil {
		return err
	}
	sshd := fmt.Sprintf(`# Managed by Hamara Tunnel
Match User %s
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    PubkeyAuthentication yes
    AllowTcpForwarding local
    GatewayPorts no
    X11Forwarding no
    AllowAgentForwarding no
    PermitTTY no
    ForceCommand /bin/false
`, sshUser)
	if err := platform.WriteFile("/etc/ssh/sshd_config.d/90-hamara-tunnel.conf", []byte(sshd), 0644); err != nil {
		return err
	}
	if err := a.Runner.Run("sshd", "-t"); err != nil {
		return errors.New("generated sshd configuration failed validation")
	}
	key, _, err := platform.ReadFirst("/etc/ssh/ssh_host_ed25519_key.pub", "/etc/ssh/ssh_host_rsa_key.pub")
	if err != nil {
		return err
	}
	fields := strings.Fields(key)
	if len(fields) < 2 {
		return errors.New("could not parse SSH host public key")
	}
	cfg.SSHHostKey = fields[0] + " " + fields[1]
	return a.Runner.Run("systemctl", "reload", "ssh.service")
}

func (a *App) setupSSHClient(cfg *model.Config) error {
	key := "/etc/hamara/secrets/ssh_ed25519"
	_ = os.Remove(key)
	_ = os.Remove(key + ".pub")
	if err := a.Runner.Run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "hamara-"+cfg.InstanceID, "-f", key); err != nil {
		return err
	}
	known := render.KnownHosts(cfg.EndpointHost, cfg.SSHPort, cfg.SSHHostKey)
	if err := platform.WriteFile("/etc/hamara/ssh_known_hosts", []byte(known), 0600); err != nil {
		return err
	}
	return platform.WriteFile("/etc/systemd/system/hamara-ssh-socks.service", []byte(render.SSHClientUnit(*cfg)), 0644)
}

func (a *App) authorizeSSHPeer(cfg model.Config, pub string) error {
	fields := strings.Fields(strings.TrimSpace(pub))
	if len(fields) < 2 || fields[0] != "ssh-ed25519" {
		return errors.New("only a single Ed25519 public key generated by Hamara is accepted")
	}
	key := fields[0] + " " + fields[1]
	tmp := "/etc/hamara/peer_authorized_key.tmp"
	if err := platform.WriteFile(tmp, []byte(key+"\n"), 0600); err != nil {
		return err
	}
	if _, err := a.Runner.Output("ssh-keygen", "-lf", tmp); err != nil {
		_ = os.Remove(tmp)
		return errors.New("SSH response key failed validation")
	}
	_ = os.Remove(tmp)
	path := "/var/lib/hamara-ssh/.ssh/authorized_keys"
	data, _ := os.ReadFile(path)
	marker := "hamara-" + cfg.InstanceID
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, marker) && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	entry := fmt.Sprintf(`no-agent-forwarding,no-X11-forwarding,no-pty,no-user-rc %s %s`, key, marker)
	lines = append(lines, entry)
	if err := platform.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		return err
	}
	return a.Runner.Run("chown", sshUser+":"+sshUser, path)
}

func defaults(mode string) model.Config {
	cfg := model.Config{Mode: mode, Interface: "hamara0"}
	switch mode {
	case model.ModeARA:
		cfg.TunnelCIDR, cfg.GatewayAddress, cfg.EdgeAddress = "10.73.0.0/30", "10.73.0.1/30", "10.73.0.2/30"
		cfg.WireGuardPort, cfg.LocalRelayPort, cfg.MTU, cfg.RouteTable = 51820, 51821, 1280, 52730
	case model.ModeWireGuard:
		cfg.TunnelCIDR, cfg.GatewayAddress, cfg.EdgeAddress = "10.74.0.0/30", "10.74.0.1/30", "10.74.0.2/30"
		cfg.WireGuardPort, cfg.MTU, cfg.RouteTable = 51820, 1380, 52740
	case model.ModeIPIP:
		cfg.TunnelCIDR, cfg.GatewayAddress, cfg.EdgeAddress = "10.75.0.0/30", "10.75.0.1/30", "10.75.0.2/30"
		cfg.MTU, cfg.RouteTable = 1400, 52750
	case model.ModeSSHSocks:
		cfg.SOCKSPort = 10808
	}
	return cfg
}

func defaultPort(mode string) int {
	switch mode {
	case model.ModeARA:
		return 443
	case model.ModeWireGuard:
		return 51820
	case model.ModeSSHSocks:
		return 22
	case model.ModeIPIP:
		return 4
	}
	return 0
}

func serviceNames(cfg model.Config) []string {
	switch cfg.Mode {
	case model.ModeARA:
		if cfg.Role == model.RoleOutside {
			return []string{"wg-quick@hamara0", "hamara-ara-server", "hamara-firewall"}
		}
		return []string{"hamara-guard", "hamara-ara-client", "wg-quick@hamara0"}
	case model.ModeWireGuard:
		if cfg.Role == model.RoleOutside {
			return []string{"wg-quick@hamara0", "hamara-firewall"}
		}
		return []string{"hamara-guard", "wg-quick@hamara0"}
	case model.ModeIPIP:
		if cfg.Role == model.RoleOutside && cfg.State == model.StatePending {
			return nil
		}
		if cfg.Role == model.RoleOutside {
			return []string{"hamara-ipip", "hamara-firewall"}
		}
		return []string{"hamara-guard", "hamara-ipip"}
	case model.ModeSSHSocks:
		if cfg.Role == model.RoleIran {
			return []string{"hamara-ssh-socks"}
		}
	}
	return nil
}

func (a *App) serviceState(name string) string {
	out, err := a.Runner.Output("systemctl", "is-active", name)
	if err != nil {
		if out != "" {
			return out
		}
		return "inactive"
	}
	return out
}

func tokenInput(value, path string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	if path == "" {
		return "", errors.New("provide a token with --offer/--response or --file")
	}
	return readTrim(path)
}

func readTrim(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func capturePreviousSysctls(cfg *model.Config) {
	cfg.PreviousIPForward = readRaw("/proc/sys/net/ipv4/ip_forward")
	cfg.PreviousRPFilterAll = readRaw("/proc/sys/net/ipv4/conf/all/rp_filter")
	cfg.PreviousRPFilterDefault = readRaw("/proc/sys/net/ipv4/conf/default/rp_filter")
}

func (a *App) restorePreviousSysctls(cfg model.Config) {
	type item struct {
		path, key, managed, previous string
	}
	items := []item{
		{"/proc/sys/net/ipv4/ip_forward", "net.ipv4.ip_forward", "1", cfg.PreviousIPForward},
		{"/proc/sys/net/ipv4/conf/all/rp_filter", "net.ipv4.conf.all.rp_filter", "2", cfg.PreviousRPFilterAll},
		{"/proc/sys/net/ipv4/conf/default/rp_filter", "net.ipv4.conf.default.rp_filter", "2", cfg.PreviousRPFilterDefault},
	}
	for _, value := range items {
		if readRaw(value.path) == value.managed && (value.previous == "0" || value.previous == "1" || value.previous == "2") {
			_ = a.Runner.Run("sysctl", "-w", value.key+"="+value.previous)
		}
	}
}

func readRaw(path string) string {
	value, err := readTrim(path)
	if err != nil {
		return ""
	}
	return value
}

func readSysctl(path, fallback string) string {
	value := readRaw(path)
	if value == "" {
		return fallback
	}
	if path == "/proc/sys/net/ipv4/ip_forward" {
		if value == "1" {
			return "OK (enabled)"
		}
		return "FAIL (disabled)"
	}
	return value
}

func randomHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hostOnly(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

func (a *App) banner() {
	fmt.Fprintf(a.Out, `
  _   _                             
 | | | | __ _ _ __ ___   __ _ _ __ __ _ 
 | |_| |/ _`+"`"+` | '_ `+"`"+` _ \ / _`+"`"+` | '__/ _`+"`"+` |
 |  _  | (_| | | | | | | (_| | | | (_| |
 |_| |_|\__,_|_| |_| |_|\__,_|_|  \__,_|

 Hamara Tunnel %s
 Secure two-server routing for Ubuntu

`, a.Version)
}

func (a *App) choose(question string, choices []string) (int, error) {
	fmt.Fprintln(a.Out, question)
	for i, item := range choices {
		fmt.Fprintf(a.Out, "  %d) %s\n", i+1, item)
	}
	for {
		value, err := a.prompt("Select", "1")
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(value)
		if err == nil && n >= 1 && n <= len(choices) {
			return n - 1, nil
		}
		fmt.Fprintf(a.Err, "Enter a number from 1 to %d.\n", len(choices))
	}
}

func (a *App) prompt(question, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(a.Out, "%s [%s]: ", question, defaultValue)
	} else {
		fmt.Fprintf(a.Out, "%s: ", question)
	}
	line, err := a.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = defaultValue
	}
	return line, nil
}

func (a *App) promptInt(question string, defaultValue, min, max int) (int, error) {
	for {
		value, err := a.prompt(question, strconv.Itoa(defaultValue))
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(value)
		if err == nil && n >= min && n <= max {
			return n, nil
		}
		fmt.Fprintf(a.Err, "Enter a number from %d to %d.\n", min, max)
	}
}

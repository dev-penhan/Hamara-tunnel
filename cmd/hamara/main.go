package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dev-penhan/Hamara-tunnel/internal/app"
)

var version = "dev"

func main() {
	autoElevate()
	a := app.New(os.Stdin, os.Stdout, os.Stderr, version)
	if len(os.Args) == 1 {
		fatal(a.Interactive())
		return
	}

	switch os.Args[1] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		role := fs.String("role", "outside", "server role (outside only for init)")
		mode := fs.String("mode", "", "ara, wireguard, ipip, or ssh-socks")
		endpoint := fs.String("endpoint", "", "public gateway DNS name or IPv4")
		port := fs.Int("port", 0, "public transport port")
		tlsMode := fs.String("tls", "", "ARA TLS mode: acme, existing, or self-signed")
		cert := fs.String("cert", "", "existing full-chain PEM path")
		key := fs.String("key", "", "existing TLS private-key path")
		email := fs.String("email", "", "Let's Encrypt account email")
		force := fs.Bool("force", false, "replace an existing Hamara configuration")
		_ = fs.Parse(os.Args[2:])
		if *role != "outside" {
			fatal(fmt.Errorf("init creates an outside gateway; use `hamara join` on the Iran edge"))
		}
		fatal(a.InitOutside(app.InitOptions{
			Mode: strings.ToLower(*mode), EndpointHost: *endpoint, EndpointPort: *port,
			TLSMode: strings.ToLower(*tlsMode), TLSCertPath: *cert, TLSKeyPath: *key,
			ACMEEmail: *email, Force: *force,
		}))
	case "join":
		fs := flag.NewFlagSet("join", flag.ExitOnError)
		offer := fs.String("offer", "", "Hamara OFFER token")
		file := fs.String("file", "", "read OFFER token from file")
		_ = fs.Parse(os.Args[2:])
		fatal(a.Join(*offer, *file))
	case "accept":
		fs := flag.NewFlagSet("accept", flag.ExitOnError)
		response := fs.String("response", "", "Hamara RESPONSE token")
		file := fs.String("file", "", "read RESPONSE token from file")
		_ = fs.Parse(os.Args[2:])
		fatal(a.Accept(*response, *file))
	case "status":
		fatal(a.Status())
	case "start", "stop", "restart":
		fatal(a.ManageLink(os.Args[1]))
	case "doctor":
		fatal(a.Doctor())
	case "leak-test":
		fatal(a.LeakTest())
	case "logs":
		fatal(a.Logs())
	case "repair":
		fatal(a.RepairGuards())
	case "paths":
		fatal(a.ShowPaths())
	case "xray":
		fs := flag.NewFlagSet("xray", flag.ExitOnError)
		tags := fs.String("inbound-tags", "", "comma-separated 3x-ui inbound tags; blank routes all")
		_ = fs.Parse(os.Args[2:])
		var values []string
		for _, item := range strings.Split(*tags, ",") {
			if value := strings.TrimSpace(item); value != "" {
				values = append(values, value)
			}
		}
		fatal(a.PrintXray(values))
	case "pairing-token":
		fatal(a.ShowPairingToken())
	case "uninstall":
		fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
		purge := fs.Bool("purge-wstunnel", false, "also remove /usr/local/bin/wstunnel")
		_ = fs.Parse(os.Args[2:])
		fatal(a.Uninstall(*purge))
	case "version", "--version", "-v":
		fmt.Println("hamara", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func autoElevate() {
	if os.Geteuid() == 0 || isUnprivilegedCommand() {
		return
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Hamara needs root privileges. Install sudo or run this command as root.")
		os.Exit(1)
	}
	executable, err := os.Executable()
	if err != nil {
		executable = "/usr/local/bin/hamara"
	}
	args := append([]string{"--", executable}, os.Args[1:]...)
	cmd := exec.Command(sudo, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "Unable to start Hamara through sudo:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func isUnprivilegedCommand() bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "help", "--help", "-h", "version", "--version", "-v":
		return true
	default:
		return false
	}
}

func usage() {
	fmt.Print(`Hamara Tunnel — two-server routing for Ubuntu

Usage:
  hamara                         Interactive setup and management
  hamara init --role outside --mode MODE --endpoint HOST [options]
  hamara join --offer TOKEN      Configure the Iran edge from an offer
  hamara accept --response TOKEN Complete pairing on the outside gateway
  hamara status                  Show services and handshake state
  hamara start|stop|restart      Manage link services; the strict guard stays on
  hamara doctor                  Run local and end-to-end checks
  hamara leak-test               Prove marked traffic fails closed when link stops
  hamara logs                    Show recent Hamara service logs
  hamara repair                  Reapply firewall/NAT or strict egress guard
  hamara paths                   List saved files and their purpose
  hamara xray [--inbound-tags a,b]
                                 Print 3x-ui/Xray outbound and routing JSON
  hamara pairing-token           Reprint this server's pairing token
  hamara uninstall               Remove Hamara configuration and services

Modes:
  ara          ARA Routed TLS (WireGuard over WSS/TLS + kernel forwarding)
  wireguard    Direct routed WireGuard
  ipip         Direct kernel IPIP (IPv4, unencrypted)
  ssh-socks    TCP-only SSH SOCKS fallback

ARA TLS options:
  --tls acme --endpoint tunnel.example.com [--email you@example.com]
  --tls existing --cert /path/fullchain.pem --key /path/privkey.pem
  --tls self-signed

Run the command without arguments for beginner-friendly prompts.
`)
}

func fatal(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

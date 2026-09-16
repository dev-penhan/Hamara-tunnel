package platform

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dev-penhan/Hamara-tunnel/internal/model"
)

const (
	ConfigDir  = "/etc/hamara"
	ConfigFile = "/etc/hamara/config.json"
	SecretsDir = "/etc/hamara/secrets"
	LibDir     = "/usr/local/lib/hamara"
	StateDir   = "/var/lib/hamara"
)

type Runner struct {
	Out io.Writer
	Err io.Writer
}

func (r Runner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = r.Out
	cmd.Stderr = r.Err
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func (r Runner) Output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(b)), nil
}

func (r Runner) Bash(script string) error {
	return r.Run("/bin/bash", "-c", script)
}

func RequireRootUbuntu() error {
	if runtime.GOOS != "linux" {
		return errors.New("Hamara currently supports Linux only")
	}
	if os.Geteuid() != 0 {
		return errors.New("Hamara needs root; run `hamara` from a sudo-enabled account or use a root shell")
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("cannot read /etc/os-release: %w", err)
	}
	values := parseOSRelease(string(data))
	if values["ID"] != "ubuntu" {
		return fmt.Errorf("Hamara v1 supports Ubuntu only; detected %q", values["ID"])
	}
	version := values["VERSION_ID"]
	if version != "22.04" && version != "24.04" {
		return fmt.Errorf("Hamara v1 supports Ubuntu 22.04 and 24.04; detected %q", version)
	}
	return nil
}

func parseOSRelease(s string) map[string]string {
	m := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = strings.Trim(parts[1], "\"'")
		}
	}
	return m
}

func EnsureDirs() error {
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{
		{ConfigDir, 0750}, {SecretsDir, 0700}, {LibDir, 0755}, {StateDir, 0750},
	} {
		if err := os.MkdirAll(item.path, item.mode); err != nil {
			return err
		}
		if err := os.Chmod(item.path, item.mode); err != nil {
			return err
		}
	}
	return nil
}

func WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func SaveConfig(cfg model.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteFile(ConfigFile, data, 0600)
}

func LoadConfig() (model.Config, error) {
	var cfg model.Config
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("cannot parse %s: %w", ConfigFile, err)
	}
	return cfg, nil
}

func ConfigExists() bool {
	_, err := os.Stat(ConfigFile)
	return err == nil
}

func InstallPackages(r Runner, packages ...string) error {
	missing := make([]string, 0, len(packages))
	for _, p := range packages {
		cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", p)
		out, err := cmd.Output()
		if err != nil || !strings.Contains(string(out), "install ok installed") {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := r.Run("apt-get", "update"); err != nil {
		return err
	}
	args := append([]string{"install", "-y", "--no-install-recommends"}, missing...)
	return r.Run("apt-get", args...)
}

func DetectWAN(r Runner) (string, string, error) {
	out, err := r.Output("ip", "-4", "route", "get", "1.1.1.1")
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(out)
	var dev, src string
	for i := 0; i+1 < len(fields); i++ {
		switch fields[i] {
		case "dev":
			dev = fields[i+1]
		case "src":
			src = fields[i+1]
		}
	}
	if dev == "" || net.ParseIP(src) == nil {
		return "", "", fmt.Errorf("could not determine WAN interface/source from: %s", out)
	}
	return dev, src, nil
}

func IsPortListening(r Runner, proto string, port int) bool {
	flag := "-ltnH"
	if proto == "udp" {
		flag = "-lunH"
	}
	out, err := r.Output("ss", flag)
	if err != nil {
		return false
	}
	needle := fmt.Sprintf(":%d", port)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasSuffix(fields[3], needle) {
			return true
		}
	}
	return false
}

func ReadFirst(paths ...string) (string, string, error) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			return strings.TrimSpace(string(data)), p, nil
		}
	}
	return "", "", fmt.Errorf("none of the files exist: %s", strings.Join(paths, ", "))
}

func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

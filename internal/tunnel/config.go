package tunnel

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

// ProxyConfig holds all configuration parameters for sing-box config generation
type ProxyConfig struct {
	ListenPort      int
	ProxyType       string // "socks" or "http"
	ProxyHost       string
	ProxyPort       int
	ProxyUser       string
	ProxyPass       string
	UseSystemTunnel bool
}

const configTemplate = `{
  "log": { "level": "error", "timestamp": true },
  "dns": {
    "servers": [
      { "tag": "remote-dns", "address": "8.8.8.8", "detour": "profile-proxy" }
    ],
    "rules": [],
    "final": "remote-dns"
  },
  "inbounds": [
    {
      "type": "mixed",
      "tag": "local-in",
      "listen": "127.0.0.1",
      "listen_port": {{.ListenPort}},
      "sniff": true
    }
  ],
  "outbounds": [
    {
      "type": "{{.ProxyType}}",
      "tag": "profile-proxy",
      "server": "{{.ProxyHost}}",
      "server_port": {{.ProxyPort}}{{if .ProxyUser}},
      "username": "{{.ProxyUser}}",
      "password": "{{.ProxyPass}}"{{end}}{{if .UseSystemTunnel}},
      "detour": "system-tunnel"{{end}}
    },{{if .UseSystemTunnel}}
    {
      "type": "socks",
      "tag": "system-tunnel",
      "server": "127.0.0.1",
      "server_port": 10808
    },{{end}}
    {
      "type": "direct",
      "tag": "direct-out"
    },
    {
      "type": "block",
      "tag": "block-out"
    }
  ],
  "route": {
    "rules": [
      { "protocol": "dns", "outbound": "profile-proxy" },
      { "ip_cidr": ["127.0.0.0/8"], "outbound": "direct-out" },
      { "domain": ["localhost"], "outbound": "direct-out" }
    ],
    "final": "profile-proxy",
    "auto_detect_interface": true
  }
}`

// GenerateConfig creates a Sing-box JSON configuration based on the provided ProxyConfig.
// It writes the configuration to a temporary file and returns the file path.
func GenerateConfig(cfg ProxyConfig) (string, error) {
	// Normalize proxy type
	proxyType := cfg.ProxyType
	if proxyType == "socks5" {
		proxyType = "socks"
	}
	cfg.ProxyType = proxyType

	tmpl, err := template.New("singbox").Parse(configTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("failed to execute config template: %w", err)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "singbox_config_*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(buf.String()); err != nil {
		return "", fmt.Errorf("failed to write config to file: %w", err)
	}

	return tmpFile.Name(), nil
}

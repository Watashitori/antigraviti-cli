package tunnel

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

type ProxyConfig struct {
	ListenPort      int
	ProxyType       string
	ProxyHost       string
	ProxyPort       int
	ProxyUser       string
	ProxyPass       string
	UseSystemTunnel bool
}

const configTemplate = `{
  "log": {
    "level": "info",
    "timestamp": true
  },
  "dns": {
    "servers": [
      {
        "tag": "google-dns",
        "address": "8.8.8.8",
        "detour": "profile-proxy"
      }
    ],
    "rules": [
      {
        "outbound": "any",
        "server": "google-dns"
      }
    ],
    "final": "google-dns"
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
      "password": "{{.ProxyPass}}"{{end}}
    },
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

func GenerateConfig(cfg ProxyConfig) (string, error) {
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
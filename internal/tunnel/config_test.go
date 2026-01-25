package tunnel

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGenerateConfig_Direct(t *testing.T) {
	cfg := ProxyConfig{
		ListenPort:      12345,
		ProxyType:       "socks",
		ProxyHost:       "proxy.example.com",
		ProxyPort:       1080,
		ProxyUser:       "testuser",
		ProxyPass:       "testpass",
		UseSystemTunnel: false,
	}

	path, err := GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}

	contentStr := string(content)

	// Check if content contains the port
	if !strings.Contains(contentStr, `"listen_port": 12345`) {
		t.Errorf("Config does not contain correct listen_port")
	}

	// Check profile-proxy outbound exists
	if !strings.Contains(contentStr, `"tag": "profile-proxy"`) {
		t.Errorf("Config does not contain profile-proxy outbound")
	}

	// Check NO system-tunnel when UseSystemTunnel is false
	if strings.Contains(contentStr, `"tag": "system-tunnel"`) {
		t.Errorf("Config should NOT contain system-tunnel when UseSystemTunnel is false")
	}

	// Check NO detour when UseSystemTunnel is false
	if strings.Contains(contentStr, `"detour": "system-tunnel"`) {
		t.Errorf("Config should NOT contain detour to system-tunnel when UseSystemTunnel is false")
	}

	// Check if content is valid JSON
	var js map[string]interface{}
	if err := json.Unmarshal(content, &js); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}
}

func TestGenerateConfig_WithChaining(t *testing.T) {
	cfg := ProxyConfig{
		ListenPort:      54321,
		ProxyType:       "socks5", // Should be normalized to "socks"
		ProxyHost:       "proxy.example.com",
		ProxyPort:       1080,
		ProxyUser:       "user",
		ProxyPass:       "pass",
		UseSystemTunnel: true,
	}

	path, err := GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}

	contentStr := string(content)

	// Check system-tunnel exists when UseSystemTunnel is true
	if !strings.Contains(contentStr, `"tag": "system-tunnel"`) {
		t.Errorf("Config should contain system-tunnel when UseSystemTunnel is true")
	}

	// Check system-tunnel has correct port
	if !strings.Contains(contentStr, `"server_port": 10808`) {
		t.Errorf("system-tunnel should use port 10808")
	}

	// Check profile-proxy has detour
	if !strings.Contains(contentStr, `"detour": "system-tunnel"`) {
		t.Errorf("profile-proxy should have detour to system-tunnel")
	}

	// Check proxy type is normalized
	if !strings.Contains(contentStr, `"type": "socks"`) {
		t.Errorf("Proxy type should be normalized to 'socks'")
	}

	// Check if content is valid JSON
	var js map[string]interface{}
	if err := json.Unmarshal(content, &js); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}
}

func TestGenerateConfig_NoAuth(t *testing.T) {
	cfg := ProxyConfig{
		ListenPort:      11111,
		ProxyType:       "http",
		ProxyHost:       "proxy.example.com",
		ProxyPort:       8080,
		ProxyUser:       "", // No auth
		ProxyPass:       "",
		UseSystemTunnel: false,
	}

	path, err := GenerateConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}

	contentStr := string(content)

	// Check NO username/password fields when no auth
	if strings.Contains(contentStr, `"username"`) {
		t.Errorf("Config should NOT contain username when ProxyUser is empty")
	}

	// Check if content is valid JSON
	var js map[string]interface{}
	if err := json.Unmarshal(content, &js); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}
}

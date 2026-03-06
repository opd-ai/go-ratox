package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ratox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test loading configuration
	cfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default values
	if cfg.ConfigDir != tempDir {
		t.Errorf("Expected ConfigDir %s, got %s", tempDir, cfg.ConfigDir)
	}

	if cfg.Name != "ratox-go user" {
		t.Errorf("Expected default name 'ratox-go user', got %s", cfg.Name)
	}

	if cfg.StatusMessage != "Running ratox-go" {
		t.Errorf("Expected default status message 'Running ratox-go', got %s", cfg.StatusMessage)
	}

	if len(cfg.BootstrapNodes) == 0 {
		t.Error("Expected default bootstrap nodes, got none")
	}

	// Verify config file was created
	configFile := filepath.Join(tempDir, ConfigFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestSave(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ratox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create config
	cfg := &Config{
		ConfigDir:     tempDir,
		Name:          "Test User",
		StatusMessage: "Testing",
		Debug:         true,
	}

	// Save config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load config again
	loadedCfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	// Verify saved values
	if loadedCfg.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got %s", loadedCfg.Name)
	}

	if loadedCfg.StatusMessage != "Testing" {
		t.Errorf("Expected status message 'Testing', got %s", loadedCfg.StatusMessage)
	}

	if !loadedCfg.Debug {
		t.Error("Expected debug to be true")
	}
}

func TestFriendDir(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/test/config",
	}

	friendID := "1234567890ABCDEF"
	expected := "/test/config/1234567890ABCDEF"
	result := cfg.FriendDir(friendID)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGlobalFIFOPath(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/test/config",
	}

	fifoName := "request_in"
	expected := "/test/config/client/request_in"
	result := cfg.GlobalFIFOPath(fifoName)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFriendFIFOPath(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/test/config",
	}

	friendID := "1234567890ABCDEF"
	fifoName := "text_in"
	expected := "/test/config/1234567890ABCDEF/text_in"
	result := cfg.FriendFIFOPath(friendID, fifoName)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestValidateTransport(t *testing.T) {
	tests := []struct {
		name        string
		torEnabled  bool
		i2pEnabled  bool
		expectError bool
	}{
		{
			name:        "neither Tor nor I2P enabled",
			torEnabled:  false,
			i2pEnabled:  false,
			expectError: false,
		},
		{
			name:        "Tor only enabled",
			torEnabled:  true,
			i2pEnabled:  false,
			expectError: false,
		},
		{
			name:        "I2P only enabled",
			torEnabled:  false,
			i2pEnabled:  true,
			expectError: false,
		},
		{
			name:        "Tor and I2P simultaneously enabled",
			torEnabled:  true,
			i2pEnabled:  true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Transport: TransportConfig{
					TorEnabled: tt.torEnabled,
					I2PEnabled: tt.i2pEnabled,
				},
			}
			err := cfg.ValidateTransport()
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestBootstrapServerConfigDefaults(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ratox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.BootstrapServer.Enabled {
		t.Error("Expected bootstrap server to be disabled by default")
	}

	if !cfg.BootstrapServer.ClearnetEnabled {
		t.Error("Expected bootstrap server clearnet to be enabled by default")
	}

	if cfg.BootstrapServer.ClearnetPort != 33445 {
		t.Errorf("Expected default clearnet port 33445, got %d", cfg.BootstrapServer.ClearnetPort)
	}

	if cfg.BootstrapServer.OnionEnabled {
		t.Error("Expected bootstrap server onion to be disabled by default")
	}

	if cfg.BootstrapServer.I2PEnabled {
		t.Error("Expected bootstrap server I2P to be disabled by default")
	}

	if cfg.BootstrapServer.I2PSAMAddr != "127.0.0.1:7656" {
		t.Errorf("Expected default I2P SAM addr 127.0.0.1:7656, got %s", cfg.BootstrapServer.I2PSAMAddr)
	}
}

func TestBootstrapServerConfigPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ratox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &Config{
		ConfigDir: tempDir,
		Name:      "Test User",
		BootstrapServer: BootstrapServerConfig{
			Enabled:         true,
			ClearnetEnabled: true,
			ClearnetPort:    33445,
			OnionEnabled:    true,
			I2PEnabled:      true,
			I2PSAMAddr:      "127.0.0.1:7656",
		},
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	loaded, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if !loaded.BootstrapServer.Enabled {
		t.Error("Expected bootstrap server enabled to persist")
	}
	if !loaded.BootstrapServer.OnionEnabled {
		t.Error("Expected bootstrap server onion enabled to persist")
	}
	if !loaded.BootstrapServer.I2PEnabled {
		t.Error("Expected bootstrap server I2P enabled to persist")
	}
}

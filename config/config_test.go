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
	expected := "/test/config/request_in"
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

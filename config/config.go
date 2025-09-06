// Package config provides configuration management for ratox-go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// ConfigFileName is the name of the configuration file
	ConfigFileName = "ratox.json"
	// SaveDataFileName is the name of the Tox save data file
	SaveDataFileName = "ratox.tox"
)

// Config holds all configuration options for ratox-go
type Config struct {
	// ConfigDir is the directory where configuration files are stored
	ConfigDir string `json:"-"`

	// Debug enables debug logging
	Debug bool `json:"debug"`

	// Name is the user's display name
	Name string `json:"name"`

	// StatusMessage is the user's status message
	StatusMessage string `json:"status_message"`

	// AutoAcceptFiles enables automatic file transfer acceptance
	AutoAcceptFiles bool `json:"auto_accept_files"`

	// MaxFileSize is the maximum file size to accept (in bytes)
	MaxFileSize int64 `json:"max_file_size"`

	// BootstrapNodes contains DHT bootstrap nodes
	BootstrapNodes []BootstrapNode `json:"bootstrap_nodes"`

	// SaveFile is the path to the Tox save file
	SaveFile string `json:"-"`
}

// BootstrapNode represents a DHT bootstrap node
type BootstrapNode struct {
	Address   string `json:"address"`
	Port      uint16 `json:"port"`
	PublicKey string `json:"public_key"`
}

// DefaultBootstrapNodes contains a list of default bootstrap nodes
var DefaultBootstrapNodes = []BootstrapNode{
	{
		Address:   "nodes.tox.chat",
		Port:      33445,
		PublicKey: "6FC41E2BD381D37E9748FC0E0328CE086AF9598BECC8FEB7DDF2E440475F300E",
	},
	{
		Address:   "130.133.110.14",
		Port:      33445,
		PublicKey: "461FA3776EF0FA655F1A05477DF1B3B614F7D6B124F7DB1DD4FE3C08B03B640F",
	},
	{
		Address:   "tox.zodiaclabs.org",
		Port:      33445,
		PublicKey: "A09162D68618E742FFBCA1C2C70385E6679604B2D80EA6E84AD0996A1AC8A074",
	},
	{
		Address:   "tox2.abilinski.com",
		Port:      33445,
		PublicKey: "7A6098B590BDC73F9723FC59F82B3F9085A64D1B213AAF8E610FD351930D052D",
	},
}

// Load loads configuration from the specified directory
// If the configuration file doesn't exist, it creates a default one
func Load(configDir string) (*Config, error) {
	configFile := filepath.Join(configDir, ConfigFileName)
	saveFile := filepath.Join(configDir, SaveDataFileName)

	// Default configuration
	cfg := &Config{
		ConfigDir:       configDir,
		Debug:           false,
		Name:            "ratox-go user",
		StatusMessage:   "Running ratox-go",
		AutoAcceptFiles: false,
		MaxFileSize:     100 * 1024 * 1024, // 100MB default
		BootstrapNodes:  DefaultBootstrapNodes,
		SaveFile:        saveFile,
	}

	// Try to load existing configuration
	if data, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Ensure fields that aren't saved are set
	cfg.ConfigDir = configDir
	cfg.SaveFile = saveFile

	// Save the configuration to ensure it exists
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	return cfg, nil
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	configFile := filepath.Join(c.ConfigDir, ConfigFileName)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// FriendDir returns the directory path for a specific friend
func (c *Config) FriendDir(friendID string) string {
	return filepath.Join(c.ConfigDir, friendID)
}

// GlobalFIFOPath returns the path for a global FIFO file
func (c *Config) GlobalFIFOPath(name string) string {
	return filepath.Join(c.ConfigDir, name)
}

// FriendFIFOPath returns the path for a friend-specific FIFO file
func (c *Config) FriendFIFOPath(friendID, fifoName string) string {
	return filepath.Join(c.FriendDir(friendID), fifoName)
}

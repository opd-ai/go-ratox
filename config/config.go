// Package config provides configuration management for ratox-go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	// ConfigFileName is the name of the configuration file
	ConfigFileName = "config.json"
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
	pc, _, _, _ := runtime.Caller(0)
	funcName := runtime.FuncForPC(pc).Name()
	caller := funcName[strings.LastIndex(funcName, ".")+1:]

	logrus.WithFields(logrus.Fields{
		"caller":     caller,
		"config_dir": configDir,
		"operation":  "load_config",
	}).Debug("Starting configuration load")

	configFile := filepath.Join(configDir, ConfigFileName)
	saveFile := filepath.Join(configDir, SaveDataFileName)

	logrus.WithFields(logrus.Fields{
		"caller":      caller,
		"config_file": configFile,
		"save_file":   saveFile,
	}).Debug("Configuration file paths determined")

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

	logrus.WithFields(logrus.Fields{
		"caller":           caller,
		"default_name":     cfg.Name,
		"default_status":   cfg.StatusMessage,
		"default_max_size": cfg.MaxFileSize,
		"bootstrap_nodes":  len(cfg.BootstrapNodes),
	}).Debug("Default configuration created")

	// Try to load existing configuration
	logrus.WithFields(logrus.Fields{
		"caller":      caller,
		"config_file": configFile,
		"operation":   "read_existing_config",
	}).Debug("Attempting to read existing configuration file")

	if data, err := os.ReadFile(configFile); err == nil {
		logrus.WithFields(logrus.Fields{
			"caller":    caller,
			"file_size": len(data),
		}).Debug("Configuration file read successfully, parsing JSON")

		if err := json.Unmarshal(data, cfg); err != nil {
			logrus.WithFields(logrus.Fields{
				"caller": caller,
				"error":  err,
			}).Error("Failed to parse configuration file")
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"caller":             caller,
			"loaded_name":        cfg.Name,
			"loaded_debug":       cfg.Debug,
			"loaded_auto_accept": cfg.AutoAcceptFiles,
		}).Info("Existing configuration loaded successfully")
	} else {
		logrus.WithFields(logrus.Fields{
			"caller": caller,
			"error":  err,
		}).Info("No existing configuration file found, will create default")
	}

	// Ensure fields that aren't saved are set
	cfg.ConfigDir = configDir
	cfg.SaveFile = saveFile

	logrus.WithFields(logrus.Fields{
		"caller":     caller,
		"config_dir": cfg.ConfigDir,
		"save_file":  cfg.SaveFile,
	}).Debug("Configuration fields updated")

	// Save the configuration to ensure it exists
	logrus.WithField("caller", caller).Debug("Saving configuration to ensure it exists")
	if err := cfg.Save(); err != nil {
		logrus.WithFields(logrus.Fields{
			"caller": caller,
			"error":  err,
		}).Error("Failed to save configuration")
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"caller":     caller,
		"config_dir": configDir,
		"debug":      cfg.Debug,
		"name":       cfg.Name,
	}).Info("Configuration load completed successfully")

	return cfg, nil
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	pc, _, _, _ := runtime.Caller(0)
	funcName := runtime.FuncForPC(pc).Name()
	caller := funcName[strings.LastIndex(funcName, ".")+1:]

	configFile := filepath.Join(c.ConfigDir, ConfigFileName)

	logrus.WithFields(logrus.Fields{
		"caller":      caller,
		"config_file": configFile,
		"operation":   "save_config",
	}).Debug("Starting configuration save")

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"caller": caller,
			"error":  err,
		}).Error("Failed to marshal configuration to JSON")
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"caller":    caller,
		"json_size": len(data),
	}).Debug("Configuration marshaled to JSON successfully")

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		logrus.WithFields(logrus.Fields{
			"caller":      caller,
			"config_file": configFile,
			"error":       err,
		}).Error("Failed to write configuration file")
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"caller":      caller,
		"config_file": configFile,
		"file_size":   len(data),
	}).Info("Configuration saved successfully")

	return nil
}

// FriendDir returns the directory path for a specific friend
func (c *Config) FriendDir(friendID string) string {
	return filepath.Join(c.ConfigDir, friendID)
}

// GlobalFIFOPath returns the path for a global FIFO file
func (c *Config) GlobalFIFOPath(name string) string {
	return filepath.Join(c.ConfigDir, "client", name)
}

// FriendFIFOPath returns the path for a friend-specific FIFO file
func (c *Config) FriendFIFOPath(friendID, fifoName string) string {
	return filepath.Join(c.FriendDir(friendID), fifoName)
}

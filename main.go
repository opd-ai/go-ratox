// Package main provides the entry point for ratox-go, a FIFO-based Tox client
// that implements the same filesystem interface as the original ratox client.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/opd-ai/go-ratox/client"
	"github.com/opd-ai/go-ratox/config"
)

const (
	// DefaultConfigDir is the default directory for ratox configuration
	DefaultConfigDir = ".config/ratox-go"
	// Version of the ratox-go client
	Version = "0.1.0"
	// configDirPerm is the permission for the configuration directory.
	configDirPerm = 0o700
)

var (
	configPath = flag.String("profile", "", "Path to configuration directory")
	showHelp   = flag.Bool("help", false, "Show help message")
	showVer    = flag.Bool("version", false, "Show version")
	debug      = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	setupLogging()

	flag.Parse()

	if *showHelp {
		logrus.WithField("caller", "main").Info("Displaying help and exiting")
		printUsage()
		return
	}

	if *showVer {
		logrus.WithFields(logrus.Fields{
			"caller":  "main",
			"version": Version,
		}).Info("Displaying version and exiting")
		fmt.Printf("ratox-go %s\n", Version)
		return
	}

	cfg := loadConfig()
	startAndWait(cfg)

	logrus.WithField("caller", "main").Info("ratox-go shutdown complete")
}

// loadConfig resolves, creates, and loads the configuration directory.
// It calls Fatal on any error.
func loadConfig() *config.Config {
	logrus.WithFields(logrus.Fields{
		"caller":          "main",
		"config_path_arg": *configPath,
	}).Debug("Processing configuration directory argument")

	configDir, err := resolveConfigDir(*configPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"caller": "main",
			"error":  err,
		}).Fatal("Failed to get home directory")
	}

	logrus.WithFields(logrus.Fields{
		"caller":     "main",
		"config_dir": configDir,
	}).Info("Using configuration directory")

	logrus.WithFields(logrus.Fields{
		"caller":     "main",
		"config_dir": configDir,
		"operation":  "create_directory",
	}).Debug("Creating configuration directory")

	if err = os.MkdirAll(configDir, configDirPerm); err != nil {
		logrus.WithFields(logrus.Fields{
			"caller":     "main",
			"config_dir": configDir,
			"error":      err,
		}).Fatal("Failed to create config directory")
	}

	logrus.WithFields(logrus.Fields{
		"caller":     "main",
		"config_dir": configDir,
	}).Info("Configuration directory ready")

	cfg, err := config.Load(configDir)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"caller":     "main",
			"config_dir": configDir,
			"error":      err,
		}).Fatal("Failed to load configuration")
	}

	logrus.WithFields(logrus.Fields{
		"caller":            "main",
		"debug":             cfg.Debug,
		"name":              cfg.Name,
		"auto_accept_files": cfg.AutoAcceptFiles,
	}).Info("Configuration loaded successfully")

	if *debug {
		logrus.WithField("caller", "main").Info("Debug logging enabled via command line")
		cfg.Debug = true
		logrus.SetLevel(logrus.DebugLevel)
	} else if cfg.Debug {
		logrus.WithField("caller", "main").Info("Debug logging enabled via configuration")
		logrus.SetLevel(logrus.DebugLevel)
	}

	return cfg
}

// startAndWait creates a Tox client, starts it, and waits for completion or a signal.
func startAndWait(cfg *config.Config) {
	logrus.WithFields(logrus.Fields{
		"caller":    "main",
		"operation": "create_client",
	}).Info("Creating Tox client")

	toxClient, err := client.New(cfg)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"caller": "main",
			"error":  err,
		}).Fatal("Failed to create Tox client")
	}

	logrus.WithField("caller", "main").Info("Tox client created successfully")

	logrus.WithField("caller", "main").Debug("Setting up signal handlers")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logrus.WithField("caller", "main").Info("Starting Tox client in background goroutine")
	errChan := make(chan error, 1)
	go func() {
		logrus.WithField("caller", "main.goroutine").Debug("Client goroutine started")
		errChan <- toxClient.Run()
		logrus.WithField("caller", "main.goroutine").Debug("Client goroutine completed")
	}()

	logrus.WithField("caller", "main").Info("Waiting for shutdown signal or client error")
	select {
	case runErr := <-errChan:
		if runErr != nil {
			logrus.WithFields(logrus.Fields{
				"caller": "main",
				"error":  runErr,
			}).Fatal("Client error occurred")
		}
		logrus.WithField("caller", "main").Info("Client completed without error")
	case sig := <-sigChan:
		logrus.WithFields(logrus.Fields{
			"caller": "main",
			"signal": sig,
		}).Info("Received shutdown signal")

		logrus.WithField("caller", "main").Info("Initiating client shutdown")
		toxClient.Shutdown()
		logrus.WithField("caller", "main").Info("Client shutdown completed")
	}
}

// setupLogging configures the logrus logger with caller information.
func setupLogging() {
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			return "", fmt.Sprintf(" [%s:%d %s()]", filepath.Base(f.File), f.Line, f.Function[strings.LastIndex(f.Function, ".")+1:])
		},
	})
	logrus.WithField("caller", "main").Info("Starting ratox-go application")
}

// resolveConfigDir returns the effective configuration directory.
// If configPath is non-empty it is returned as-is; otherwise the OS home
// directory is used to build the default path.
func resolveConfigDir(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, DefaultConfigDir), nil
}

func printUsage() {
	logrus.WithField("caller", "printUsage").Debug("Displaying usage information")

	fmt.Printf("ratox-go %s - FIFO-based Tox client\n\n", Version)
	fmt.Println("Usage:")
	fmt.Printf("  %s [options]\n\n", os.Args[0])
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println("\nExamples:")
	fmt.Printf("  %s -p ~/.config/ratox-go\n", os.Args[0])
	fmt.Printf("  %s -d  # Enable debug logging\n", os.Args[0])
	fmt.Println("\nFileSystem Interface:")
	fmt.Println("  ~/.config/ratox-go/")
	fmt.Println("  ├── <friend_id>/")
	fmt.Println("  │   ├── text_in     # Send messages")
	fmt.Println("  │   ├── text_out    # Receive messages")
	fmt.Println("  │   ├── file_in     # Send files")
	fmt.Println("  │   ├── file_out    # Receive files")
	fmt.Println("  │   └── status      # Friend status")
	fmt.Println("  ├── request_in      # Accept friend requests")
	fmt.Println("  ├── request_out     # Incoming friend requests")
	fmt.Println("  ├── name            # Your display name")
	fmt.Println("  └── status_message  # Your status message")

	logrus.WithField("caller", "printUsage").Debug("Usage information displayed")
}

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

	"github.com/opd-ai/go-ratox/client"
	"github.com/opd-ai/go-ratox/config"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultConfigDir is the default directory for ratox configuration
	DefaultConfigDir = ".config/ratox-go"
	// Version of the ratox-go client
	Version = "0.1.0"
)

var (
	configPath = flag.String("profile", "", "Path to configuration directory")
	showHelp   = flag.Bool("help", false, "Show help message")
	showVer    = flag.Bool("version", false, "Show version")
	debug      = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	// Configure logrus with caller information
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			return "", fmt.Sprintf(" [%s:%d %s()]", filepath.Base(f.File), f.Line, f.Function[strings.LastIndex(f.Function, ".")+1:])
		},
	})

	logrus.WithField("caller", "main").Info("Starting ratox-go application")

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

	// Determine configuration directory
	configDir := *configPath
	logrus.WithFields(logrus.Fields{
		"caller":          "main",
		"config_path_arg": *configPath,
	}).Debug("Processing configuration directory argument")

	if configDir == "" {
		logrus.WithField("caller", "main").Debug("No config path provided, using default")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"caller": "main",
				"error":  err,
			}).Fatal("Failed to get home directory")
		}
		configDir = filepath.Join(homeDir, DefaultConfigDir)
		logrus.WithFields(logrus.Fields{
			"caller":     "main",
			"home_dir":   homeDir,
			"config_dir": configDir,
		}).Info("Using default configuration directory")
	}

	// Create configuration directory if it doesn't exist
	logrus.WithFields(logrus.Fields{
		"caller":     "main",
		"config_dir": configDir,
		"operation":  "create_directory",
	}).Debug("Creating configuration directory")

	if err := os.MkdirAll(configDir, 0700); err != nil {
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

	// Load or create configuration
	logrus.WithFields(logrus.Fields{
		"caller":     "main",
		"config_dir": configDir,
		"operation":  "load_config",
	}).Debug("Loading configuration")

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

	// Enable debug logging if requested
	if *debug {
		logrus.WithField("caller", "main").Info("Debug logging enabled via command line")
		cfg.Debug = true
		logrus.SetLevel(logrus.DebugLevel)
	} else if cfg.Debug {
		logrus.WithField("caller", "main").Info("Debug logging enabled via configuration")
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Create and start the Tox client
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

	// Handle graceful shutdown
	logrus.WithField("caller", "main").Debug("Setting up signal handlers")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the client in a goroutine
	logrus.WithField("caller", "main").Info("Starting Tox client in background goroutine")
	errChan := make(chan error, 1)
	go func() {
		logrus.WithField("caller", "main.goroutine").Debug("Client goroutine started")
		errChan <- toxClient.Run()
		logrus.WithField("caller", "main.goroutine").Debug("Client goroutine completed")
	}()

	// Wait for shutdown signal or error
	logrus.WithField("caller", "main").Info("Waiting for shutdown signal or client error")
	select {
	case err := <-errChan:
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"caller": "main",
				"error":  err,
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

	logrus.WithField("caller", "main").Info("ratox-go shutdown complete")
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

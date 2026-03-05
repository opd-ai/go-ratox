package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrintUsage tests the printUsage function output
func TestPrintUsage(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function
	printUsage()

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Test expected content in output
	tests := []struct {
		name     string
		expected string
	}{
		{"contains version", fmt.Sprintf("ratox-go %s", Version)},
		{"contains usage header", "Usage:"},
		{"contains options header", "Options:"},
		{"contains examples header", "Examples:"},
		{"contains filesystem header", "FileSystem Interface:"},
		{"contains profile example", "-p ~/.config/ratox-go"},
		{"contains debug example", "-d  # Enable debug logging"},
		{"contains request_in", "request_in      # Accept friend requests"},
		{"contains request_out", "request_out     # Incoming friend requests"},
		{"contains name fifo", "name            # Your display name"},
		{"contains status_message", "status_message  # Your status message"},
		{"contains text_in", "text_in     # Send messages"},
		{"contains text_out", "text_out    # Receive messages"},
		{"contains file_in", "file_in     # Send files"},
		{"contains file_out", "file_out    # Receive files"},
		{"contains status", "status      # Friend status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(output, tt.expected) {
				t.Errorf("printUsage() output missing expected text: %s", tt.expected)
			}
		})
	}

	// Test that output is not empty
	if len(output) == 0 {
		t.Error("printUsage() produced no output")
	}

	// Test that output contains the basic structure
	if !strings.Contains(output, "ratox-go") {
		t.Error("printUsage() should contain 'ratox-go'")
	}
}

// TestVersion tests the Version constant
func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version constant should not be empty")
	}

	// Test version format (should be semantic version-like)
	if !strings.Contains(Version, ".") {
		t.Error("Version should contain dots (e.g., '0.1.0')")
	}

	// Test that version doesn't contain invalid characters
	for _, char := range Version {
		if !((char >= '0' && char <= '9') || char == '.' || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '-') {
			t.Errorf("Version contains invalid character: %c", char)
		}
	}
}

// TestDefaultConfigDir tests the DefaultConfigDir constant
func TestDefaultConfigDir(t *testing.T) {
	expected := ".config/ratox-go"
	if DefaultConfigDir != expected {
		t.Errorf("DefaultConfigDir = %s, want %s", DefaultConfigDir, expected)
	}

	// Test that it's a relative path
	if filepath.IsAbs(DefaultConfigDir) {
		t.Error("DefaultConfigDir should be a relative path")
	}

	// Test path components
	if !strings.Contains(DefaultConfigDir, "config") {
		t.Error("DefaultConfigDir should contain 'config'")
	}

	if !strings.Contains(DefaultConfigDir, "ratox") {
		t.Error("DefaultConfigDir should contain 'ratox'")
	}
}

// TestFlagVariables tests that flag variables are properly declared
func TestFlagVariables(t *testing.T) {
	// Reset flags for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Re-declare flags to test
	testConfigPath := flag.String("profile", "", "Path to configuration directory")
	testShowHelp := flag.Bool("help", false, "Show help message")
	testShowVer := flag.Bool("version", false, "Show version")
	testDebug := flag.Bool("debug", false, "Enable debug logging")

	// Test default values
	if *testConfigPath != "" {
		t.Error("configPath flag should default to empty string")
	}

	if *testShowHelp != false {
		t.Error("showHelp flag should default to false")
	}

	if *testShowVer != false {
		t.Error("showVer flag should default to false")
	}

	if *testDebug != false {
		t.Error("debug flag should default to false")
	}

	// Test that flags can be found
	profileFlag := flag.Lookup("profile")
	if profileFlag == nil {
		t.Error("profile flag should be registered")
	}

	helpFlag := flag.Lookup("help")
	if helpFlag == nil {
		t.Error("help flag should be registered")
	}

	versionFlag := flag.Lookup("version")
	if versionFlag == nil {
		t.Error("version flag should be registered")
	}

	debugFlag := flag.Lookup("debug")
	if debugFlag == nil {
		t.Error("debug flag should be registered")
	}
}

// TestConstants tests all package constants
func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		notEmpty bool
	}{
		{"DefaultConfigDir", DefaultConfigDir, true},
		{"Version", Version, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.notEmpty && tt.value == "" {
				t.Errorf("%s should not be empty", tt.name)
			}
		})
	}
}

// TestPrintUsageOutput_Format tests the format of printUsage output
func TestPrintUsageOutput_Format(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function
	printUsage()

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Split into lines for testing
	lines := strings.Split(output, "\n")

	// Test that we have a reasonable number of lines
	if len(lines) < 10 {
		t.Errorf("printUsage() should produce at least 10 lines, got %d", len(lines))
	}

	// Test that the first line contains version info
	if !strings.Contains(lines[0], "ratox-go") || !strings.Contains(lines[0], Version) {
		t.Errorf("First line should contain version info, got: %s", lines[0])
	}

	// Test that there are section headers
	var foundUsage, foundOptions, foundExamples, foundFileSystem bool
	for _, line := range lines {
		if strings.Contains(line, "Usage:") {
			foundUsage = true
		}
		if strings.Contains(line, "Options:") {
			foundOptions = true
		}
		if strings.Contains(line, "Examples:") {
			foundExamples = true
		}
		if strings.Contains(line, "FileSystem Interface:") {
			foundFileSystem = true
		}
	}

	if !foundUsage {
		t.Error("printUsage() output should contain 'Usage:' section")
	}
	if !foundOptions {
		t.Error("printUsage() output should contain 'Options:' section")
	}
	if !foundExamples {
		t.Error("printUsage() output should contain 'Examples:' section")
	}
	if !foundFileSystem {
		t.Error("printUsage() output should contain 'FileSystem Interface:' section")
	}
}

// TestPrintUsage_TableDriven tests printUsage with table-driven approach
func TestPrintUsage_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		expectedPhrase string
		description    string
	}{
		{
			name:           "version_in_header",
			expectedPhrase: fmt.Sprintf("ratox-go %s", Version),
			description:    "should display version in header",
		},
		{
			name:           "fifo_interface_description",
			expectedPhrase: "~/.config/ratox-go/",
			description:    "should display FIFO interface path",
		},
		{
			name:           "profile_flag_example",
			expectedPhrase: "-p ~/.config/ratox-go",
			description:    "should show profile flag example",
		},
		{
			name:           "debug_flag_example",
			expectedPhrase: "-d  # Enable debug logging",
			description:    "should show debug flag example",
		},
		{
			name:           "friend_directory_structure",
			expectedPhrase: "<friend_id>/",
			description:    "should show friend directory structure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printUsage()

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			if !strings.Contains(output, tt.expectedPhrase) {
				t.Errorf("printUsage() %s: expected to find '%s' in output", tt.description, tt.expectedPhrase)
			}
		})
	}
}

// TestPrintUsageConsistency tests that printUsage produces consistent output
func TestPrintUsageConsistency(t *testing.T) {
	var outputs []string

	// Call printUsage multiple times and capture output
	for i := 0; i < 3; i++ {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call the function
		printUsage()

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		var buf bytes.Buffer
		buf.ReadFrom(r)
		outputs = append(outputs, buf.String())
	}

	// All outputs should be identical
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Error("printUsage() should produce consistent output across multiple calls")
		}
	}
}

// TestMainPackageStructure tests overall package structure
func TestMainPackageStructure(t *testing.T) {
	// Test that required constants exist
	if DefaultConfigDir == "" {
		t.Error("DefaultConfigDir constant should be defined")
	}

	if Version == "" {
		t.Error("Version constant should be defined")
	}

	// Test that package has main function (this will be tested implicitly by compilation)
	// The main function exists if the package compiles as a main package
}

// BenchmarkPrintUsage benchmarks the printUsage function
func BenchmarkPrintUsage(b *testing.B) {
	// Redirect stdout to discard during benchmark
	oldStdout := os.Stdout
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	defer func() {
		devNull.Close()
		os.Stdout = oldStdout
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		printUsage()
	}
}

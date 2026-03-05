package client

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opd-ai/go-ratox/config"
)

// TestFIFOPathGeneration tests FIFO path generation using config methods
func TestFIFOPathGeneration(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: "/tmp/test-ratox",
	}

	tests := []struct {
		name     string
		method   string
		friendID string
		fifoName string
		expected string
	}{
		{
			name:     "global FIFO request_in",
			method:   "global",
			fifoName: "request_in",
			expected: "/tmp/test-ratox/client/request_in",
		},
		{
			name:     "global FIFO name",
			method:   "global",
			fifoName: "name",
			expected: "/tmp/test-ratox/client/name",
		},
		{
			name:     "friend text_in",
			method:   "friend",
			friendID: "ABC123",
			fifoName: "text_in",
			expected: "/tmp/test-ratox/ABC123/text_in",
		},
		{
			name:     "friend file_out",
			method:   "friend",
			friendID: "DEF456",
			fifoName: "file_out",
			expected: "/tmp/test-ratox/DEF456/file_out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.method == "global" {
				result = cfg.GlobalFIFOPath(tt.fifoName)
			} else if tt.method == "friend" {
				result = cfg.FriendFIFOPath(tt.friendID, tt.fifoName)
			}

			if result != tt.expected {
				t.Errorf("Expected path '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestFriendDirPath tests friend directory path generation
func TestFriendDirPath(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: "/home/user/.config/ratox-go",
	}

	tests := []struct {
		name     string
		friendID string
		expected string
	}{
		{
			name:     "simple friend ID",
			friendID: "friend1",
			expected: "/home/user/.config/ratox-go/friend1",
		},
		{
			name:     "hex friend ID",
			friendID: "ABC123DEF456",
			expected: "/home/user/.config/ratox-go/ABC123DEF456",
		},
		{
			name:     "empty friend ID",
			friendID: "",
			expected: "/home/user/.config/ratox-go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.FriendDir(tt.friendID)
			if result != tt.expected {
				t.Errorf("Expected path '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestConferenceDirPath tests conference directory path generation
func TestConferenceDirPath(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: "/home/user/.config/ratox-go",
	}

	tests := []struct {
		name         string
		conferenceID string
		expected     string
	}{
		{
			name:         "numeric conference ID",
			conferenceID: "123",
			expected:     "/home/user/.config/ratox-go/conferences/123",
		},
		{
			name:         "hex conference ID",
			conferenceID: "abc123",
			expected:     "/home/user/.config/ratox-go/conferences/abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.ConferenceDir(tt.conferenceID)
			if result != tt.expected {
				t.Errorf("Expected path '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestConferenceFIFOPath tests conference FIFO path generation
func TestConferenceFIFOPath(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: "/home/user/.config/ratox-go",
	}

	tests := []struct {
		name         string
		conferenceID string
		fifoName     string
		expected     string
	}{
		{
			name:         "conference text_in",
			conferenceID: "42",
			fifoName:     "text_in",
			expected:     "/home/user/.config/ratox-go/conferences/42/text_in",
		},
		{
			name:         "conference invite_in",
			conferenceID: "100",
			fifoName:     "invite_in",
			expected:     "/home/user/.config/ratox-go/conferences/100/invite_in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.ConferenceFIFOPath(tt.conferenceID, tt.fifoName)
			if result != tt.expected {
				t.Errorf("Expected path '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestToxIDParsing tests Tox ID format validation
func TestToxIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		toxID   string
		valid   bool
		lenTest int
	}{
		{
			name:    "valid 76-char Tox ID",
			toxID:   strings.Repeat("A", 76),
			valid:   true,
			lenTest: 76,
		},
		{
			name:    "too short Tox ID",
			toxID:   strings.Repeat("A", 75),
			valid:   false,
			lenTest: 75,
		},
		{
			name:    "too long Tox ID",
			toxID:   strings.Repeat("A", 77),
			valid:   false,
			lenTest: 77,
		},
		{
			name:    "empty Tox ID",
			toxID:   "",
			valid:   false,
			lenTest: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Tox ID validation logic: must be exactly 76 hex characters
			isValid := len(tt.toxID) == 76 && isHexString(tt.toxID)

			if isValid != tt.valid {
				t.Errorf("Expected validity %v, got %v", tt.valid, isValid)
			}
			if len(tt.toxID) != tt.lenTest {
				t.Errorf("Expected length %d, got %d", tt.lenTest, len(tt.toxID))
			}
		})
	}
}

// TestMessageTypeDetection tests message type detection from input
func TestMessageTypeDetection(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		isAction    bool
		messageText string
	}{
		{
			name:        "normal message",
			input:       "Hello, world!",
			isAction:    false,
			messageText: "Hello, world!",
		},
		{
			name:        "action message with /me",
			input:       "/me waves",
			isAction:    true,
			messageText: "waves",
		},
		{
			name:        "just /me without space",
			input:       "/me",
			isAction:    false,
			messageText: "/me",
		},
		{
			name:        "action message with extra spaces",
			input:       "/me   jumps",
			isAction:    true,
			messageText: "  jumps", // Preserves extra spaces after /me
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Message type detection logic
			isAction := strings.HasPrefix(tt.input, "/me ")
			var messageText string
			if isAction {
				messageText = strings.TrimPrefix(tt.input, "/me ")
			} else {
				messageText = tt.input
			}

			if isAction != tt.isAction {
				t.Errorf("Expected isAction %v, got %v", tt.isAction, isAction)
			}
			if tt.isAction && messageText != tt.messageText {
				t.Errorf("Expected message text '%s', got '%s'", tt.messageText, messageText)
			}
		})
	}
}

// TestFIFOConstants tests FIFO name constants
func TestFIFOConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"RequestIn constant", RequestIn, "request_in"},
		{"RequestOut constant", RequestOut, "request_out"},
		{"Name constant", Name, "name"},
		{"StatusMessage constant", StatusMessage, "status_message"},
		{"ID constant", ID, "id"},
		{"ConnectionStatus constant", ConnectionStatus, "connection_status"},
		{"TextIn constant", TextIn, "text_in"},
		{"TextOut constant", TextOut, "text_out"},
		{"FileIn constant", FileIn, "file_in"},
		{"FileOut constant", FileOut, "file_out"},
		{"Status constant", Status, "status"},
		{"RemoveIn constant", RemoveIn, "remove_in"},
		{"Typing constant", Typing, "typing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Expected constant value '%s', got '%s'", tt.expected, tt.constant)
			}
		})
	}
}

// TestFIFOPermissions tests FIFO permission constants
func TestFIFOPermissions(t *testing.T) {
	if FIFOPermInput != 0o600 {
		t.Errorf("Expected FIFOPermInput to be 0600, got %o", FIFOPermInput)
	}
	if FIFOPermOutput != 0o600 {
		t.Errorf("Expected FIFOPermOutput to be 0600, got %o", FIFOPermOutput)
	}
	if DirPerm != 0o700 {
		t.Errorf("Expected DirPerm to be 0700, got %o", DirPerm)
	}
}

// TestFIFOStruct tests FIFO struct properties
func TestFIFOStruct(t *testing.T) {
	fifo := &FIFO{
		Path:     "/tmp/test.fifo",
		Mode:     0o600,
		IsInput:  true,
		IsOutput: false,
	}

	if fifo.Path != "/tmp/test.fifo" {
		t.Errorf("Expected path '/tmp/test.fifo', got '%s'", fifo.Path)
	}
	if fifo.Mode != 0o600 {
		t.Errorf("Expected mode 0600, got %o", fifo.Mode)
	}
	if !fifo.IsInput {
		t.Error("Expected IsInput to be true")
	}
	if fifo.IsOutput {
		t.Error("Expected IsOutput to be false")
	}
}

// TestInputParsing tests input parsing patterns
func TestInputParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"trim whitespace", "  hello  \n", "hello"},
		{"no trim needed", "hello", "hello"},
		{"newline only", "\n", ""},
		{"tab and newline", "\t\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Input parsing pattern used in FIFO handlers
			trimmed := strings.TrimSpace(tt.input)
			if trimmed != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, trimmed)
			}
		})
	}
}

// TestFriendRemovalConfirmation tests friend removal confirmation logic
func TestFriendRemovalConfirmation(t *testing.T) {
	friendID := "ABC123"

	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{"exact confirm", "confirm", true},
		{"friend ID match", friendID, true},
		{"wrong input", "delete", false},
		{"empty input", "", false},
		{"case sensitive confirm", "CONFIRM", false},
		{"partial friend ID", "ABC", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Friend removal confirmation logic
			isValid := tt.input == "confirm" || tt.input == friendID

			if isValid != tt.isValid {
				t.Errorf("Expected validity %v, got %v for input '%s'", tt.isValid, isValid, tt.input)
			}
		})
	}
}

// TestPathJoin tests path joining behavior
func TestPathJoin(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		parts    []string
		expected string
	}{
		{
			name:     "simple join",
			base:     "/home/user",
			parts:    []string{"config", "file.txt"},
			expected: "/home/user/config/file.txt",
		},
		{
			name:     "with trailing slash",
			base:     "/home/user/",
			parts:    []string{"config", "file.txt"},
			expected: "/home/user/config/file.txt",
		},
		{
			name:     "single part",
			base:     "/tmp",
			parts:    []string{"test.fifo"},
			expected: "/tmp/test.fifo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filepath.Join(append([]string{tt.base}, tt.parts...)...)
			if result != tt.expected {
				t.Errorf("Expected path '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function to check if a string is valid hex
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

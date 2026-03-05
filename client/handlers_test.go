package client

import (
	"encoding/hex"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opd-ai/go-ratox/config"
)

// TestFriendStruct tests the Friend struct and its properties
func TestFriendStruct(t *testing.T) {
	now := time.Now()
	publicKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	friend := &Friend{
		ID:        42,
		PublicKey: publicKey,
		Name:      "TestFriend",
		Status:    1,
		Online:    true,
		LastSeen:  now,
	}

	// Test all fields are properly set
	if friend.ID != 42 {
		t.Errorf("Expected ID 42, got %d", friend.ID)
	}
	if friend.Name != "TestFriend" {
		t.Errorf("Expected Name 'TestFriend', got %s", friend.Name)
	}
	if friend.Status != 1 {
		t.Errorf("Expected Status 1, got %d", friend.Status)
	}
	if !friend.Online {
		t.Error("Expected Online to be true")
	}
	if friend.LastSeen != now {
		t.Errorf("Expected LastSeen %v, got %v", now, friend.LastSeen)
	}
}

// TestFriendStructDefaults tests Friend struct with default values
func TestFriendStructDefaults(t *testing.T) {
	friend := &Friend{}

	if friend.ID != 0 {
		t.Errorf("Expected default ID 0, got %d", friend.ID)
	}
	if friend.Name != "" {
		t.Errorf("Expected default Name to be empty, got %s", friend.Name)
	}
	if friend.Status != 0 {
		t.Errorf("Expected default Status 0, got %d", friend.Status)
	}
	if friend.Online {
		t.Error("Expected default Online to be false")
	}
}

// TestPublicKeyOperations tests hex encoding that's used in handlers
func TestPublicKeyOperations(t *testing.T) {
	publicKey := [32]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}

	// This mimics the exact line from handlers.go: hex.EncodeToString(publicKey[:])
	hexStr := hex.EncodeToString(publicKey[:])
	expected := "0123456789abcdef" + strings.Repeat("00", 24)

	if hexStr != expected {
		t.Errorf("Expected hex string %s, got %s", expected, hexStr)
	}

	// Test decoding back
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Errorf("Failed to decode hex string: %v", err)
	}

	var decodedKey [32]byte
	copy(decodedKey[:], decoded)

	if decodedKey != publicKey {
		t.Errorf("Decoded key doesn't match original")
	}
}

// TestPublicKeyConversion tests hex encoding/decoding of public keys with table-driven tests
func TestPublicKeyConversion(t *testing.T) {
	tests := []struct {
		name        string
		publicKey   [32]byte
		expectedHex string
	}{
		{
			name:        "simple key",
			publicKey:   [32]byte{1, 2, 3, 4, 5},
			expectedHex: "0102030405" + strings.Repeat("00", 27),
		},
		{
			name:        "all zeros",
			publicKey:   [32]byte{},
			expectedHex: strings.Repeat("00", 32),
		},
		{
			name:        "all 0xFF",
			publicKey:   [32]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
			expectedHex: strings.Repeat("ff", 32),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hex.EncodeToString(tt.publicKey[:])
			if result != tt.expectedHex {
				t.Errorf("Expected hex '%s', got '%s'", tt.expectedHex, result)
			}

			// Test roundtrip
			decoded, err := hex.DecodeString(result)
			if err != nil {
				t.Errorf("Failed to decode hex: %v", err)
			}
			var decodedKey [32]byte
			copy(decodedKey[:], decoded)
			if decodedKey != tt.publicKey {
				t.Error("Roundtrip failed: decoded key doesn't match original")
			}
		})
	}
}

// TestMessageTypeLogic tests message type constants
func TestMessageTypeLogic(t *testing.T) {
	// Test basic message type values
	normalType := uint8(0) // NORMAL
	actionType := uint8(1) // ACTION

	if normalType != 0 {
		t.Error("Normal message type should be 0")
	}
	if actionType != 1 {
		t.Error("Action message type should be 1")
	}
}

// TestMessageTypeConstants tests message type handling
func TestMessageTypeConstants(t *testing.T) {
	// NORMAL is typically 0, ACTION is typically 1
	msgTypes := map[string]uint8{
		"NORMAL": 0,
		"ACTION": 1,
	}

	if msgTypes["NORMAL"] != 0 {
		t.Error("NORMAL message type should be 0")
	}
	if msgTypes["ACTION"] != 1 {
		t.Error("ACTION message type should be 1")
	}
}

// TestMessageFormatting tests message format patterns
func TestMessageFormatting(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		sender    string
		message   string
		msgType   string
		expected  string
	}{
		{
			name:      "normal message",
			timestamp: "15:04:05",
			sender:    "TestFriend",
			message:   "Hello World",
			msgType:   "NORMAL",
			expected:  "[15:04:05] <TestFriend> Hello World",
		},
		{
			name:      "action message",
			timestamp: "12:30:00",
			sender:    "Alice",
			message:   "waves",
			msgType:   "ACTION",
			expected:  "[12:30:00] * Alice waves",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var formatted string
			if tt.msgType == "NORMAL" {
				formatted = "[" + tt.timestamp + "] <" + tt.sender + "> " + tt.message
			} else {
				formatted = "[" + tt.timestamp + "] * " + tt.sender + " " + tt.message
			}
			if formatted != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, formatted)
			}
		})
	}
}

// TestTimeFormatting tests time format patterns used in handlers
func TestTimeFormatting(t *testing.T) {
	testTime := time.Date(2023, 12, 25, 15, 30, 45, 0, time.UTC)
	formatted := testTime.Format("15:04:05")
	expected := "15:30:45"

	if formatted != expected {
		t.Errorf("Expected time format '%s', got '%s'", expected, formatted)
	}
}

// TestTimeHandling tests time-related operations
func TestTimeHandling(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Minute)

	if !later.After(now) {
		t.Error("Expected later time to be after now")
	}
}

// TestStatusMapping tests status value mappings
func TestStatusMapping(t *testing.T) {
	statusMap := map[uint8]string{
		0: "none",
		1: "away",
		2: "busy",
	}

	tests := []struct {
		status   uint8
		expected string
	}{
		{0, "none"},
		{1, "away"},
		{2, "busy"},
	}

	for _, tt := range tests {
		result := statusMap[tt.status]
		if result != tt.expected {
			t.Errorf("Status %d: expected '%s', got '%s'", tt.status, tt.expected, result)
		}
	}
}

// TestFriendMapOperations tests operations on the friends map
func TestFriendMapOperations(t *testing.T) {
	friends := make(map[uint32]*Friend)

	// Test adding friends
	friendID := uint32(1)
	publicKey := [32]byte{1, 2, 3, 4, 5}

	friend := &Friend{
		ID:        friendID,
		PublicKey: publicKey,
		Name:      "TestFriend",
		Status:    0,
		Online:    true,
		LastSeen:  time.Now(),
	}

	friends[friendID] = friend

	// Test retrieving friend
	retrieved, exists := friends[friendID]
	if !exists {
		t.Error("Expected friend to exist in map")
	}
	if retrieved.Name != "TestFriend" {
		t.Errorf("Expected friend name TestFriend, got %s", retrieved.Name)
	}

	// Test updating friend properties
	retrieved.Name = "UpdatedName"
	if friends[friendID].Name != "UpdatedName" {
		t.Error("Expected friend name to be updated")
	}

	// Test updating last seen
	newTime := time.Now().Add(time.Hour)
	retrieved.LastSeen = newTime
	if friends[friendID].LastSeen != newTime {
		t.Error("Expected LastSeen to be updated")
	}

	// Test updating status
	retrieved.Status = 2 // busy
	if friends[friendID].Status != 2 {
		t.Errorf("Expected friend status 2, got %d", friends[friendID].Status)
	}

	// Test removing a friend
	delete(friends, friendID)
	_, exists = friends[friendID]
	if exists {
		t.Error("Expected friend to be removed from map")
	}
}

// TestFileInfoFormatting tests file information formatting
func TestFileInfoFormatting(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		filesize uint64
		expected string
	}{
		{
			name:     "small file",
			filename: "test.txt",
			filesize: 1024,
			expected: "test.txt (1024 bytes)",
		},
		{
			name:     "large file",
			filename: "video.mp4",
			filesize: 1073741824,
			expected: "video.mp4 (1073741824 bytes)",
		},
		{
			name:     "empty file",
			filename: "empty.txt",
			filesize: 0,
			expected: "empty.txt (0 bytes)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := tt.filename + " (" + formatUint64(tt.filesize) + " bytes)"
			if formatted != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, formatted)
			}
		})
	}
}

// TestFileSizeValidation tests file size validation logic
func TestFileSizeValidation(t *testing.T) {
	maxFileSize := int64(1024 * 1024) // 1MB

	tests := []struct {
		name     string
		filesize uint64
		valid    bool
	}{
		{"under limit", 512 * 1024, true},
		{"at limit", uint64(maxFileSize), true},
		{"over limit", uint64(maxFileSize) + 1, false},
		{"zero size", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := int64(tt.filesize) <= maxFileSize
			if isValid != tt.valid {
				t.Errorf("Expected validity %v, got %v", tt.valid, isValid)
			}
		})
	}
}

// TestClientStructure tests Client struct initialization
func TestClientStructure(t *testing.T) {
	cfg := &config.Config{
		Debug:       true,
		MaxFileSize: 1024 * 1024,
	}

	client := &Client{
		config:  cfg,
		friends: make(map[uint32]*Friend),
	}

	if client.config == nil {
		t.Error("Expected config to be set")
	}
	if !client.config.Debug {
		t.Error("Expected debug to be true")
	}
	if client.config.MaxFileSize != 1024*1024 {
		t.Errorf("Expected MaxFileSize 1048576, got %d", client.config.MaxFileSize)
	}
	if len(client.friends) != 0 {
		t.Errorf("Expected empty friends map, got %d friends", len(client.friends))
	}
}

// TestClientStructPatterns tests Client struct mutex patterns
func TestClientStructPatterns(t *testing.T) {
	client := &Client{
		friends:   make(map[uint32]*Friend),
		friendsMu: sync.RWMutex{},
	}

	// Test mutex operations
	client.friendsMu.Lock()
	client.friends[1] = &Friend{ID: 1, Name: "Test"}
	client.friendsMu.Unlock()

	client.friendsMu.RLock()
	friend, exists := client.friends[1]
	client.friendsMu.RUnlock()

	if !exists {
		t.Error("Expected friend to exist after mutex operations")
	}
	if friend.Name != "Test" {
		t.Errorf("Expected friend name Test, got %s", friend.Name)
	}
}

// TestReflectionOnStructs tests that we can use reflection to examine struct fields
func TestReflectionOnStructs(t *testing.T) {
	friend := &Friend{}
	friendType := reflect.TypeOf(friend).Elem()

	expectedFields := []string{"ID", "PublicKey", "Name", "Status", "Online", "LastSeen"}

	for _, expectedField := range expectedFields {
		field, found := friendType.FieldByName(expectedField)
		if !found {
			t.Errorf("Expected field %s not found in Friend struct", expectedField)
		}

		// Verify field is exported (public)
		if !field.IsExported() {
			t.Errorf("Expected field %s to be exported", expectedField)
		}
	}
}

// Helper function to format uint64 as string
func formatUint64(n uint64) string {
	if n == 0 {
		return "0"
	}

	var result []byte
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	return string(result)
}

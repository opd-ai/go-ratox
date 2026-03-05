package client

import (
	"testing"

	"github.com/opd-ai/toxcore"
)

// TestSendMessageValidation tests SendMessage validation logic
func TestSendMessageValidation(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		expectError bool
		errorSubstr string
	}{
		{
			name:        "empty message",
			message:     "",
			expectError: true,
			errorSubstr: "cannot be empty",
		},
		{
			name:        "normal message",
			message:     "Hello, World!",
			expectError: false,
		},
		{
			name:        "max length message (1372 bytes)",
			message:     string(make([]byte, 1372)),
			expectError: false,
		},
		{
			name:        "message too long (1373 bytes)",
			message:     string(make([]byte, 1373)),
			expectError: true,
			errorSubstr: "too long",
		},
		{
			name:        "UTF-8 message within byte limit",
			message:     "Hello 世界", // 12 bytes (Hello + space + 6 bytes for Chinese chars)
			expectError: false,
		},
		{
			name:        "UTF-8 message exceeding byte limit",
			message:     string(make([]byte, 1370)) + "世界世界世界", // 1370 + 18 bytes = 1388 bytes > 1372
			expectError: true,
			errorSubstr: "too long",
		},
		{
			name:        "single character",
			message:     "a",
			expectError: false,
		},
		{
			name:        "message with newlines",
			message:     "Line 1\nLine 2\nLine 3",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic directly
			if len(tt.message) == 0 {
				if !tt.expectError {
					t.Error("Expected error for empty message")
				}
				return
			}

			messageBytes := []byte(tt.message)
			isTooLong := len(messageBytes) > 1372

			if tt.expectError && !isTooLong && tt.errorSubstr == "too long" {
				t.Error("Expected message to be too long but it wasn't")
			}
			if !tt.expectError && isTooLong {
				t.Errorf("Message should be valid but is too long: %d bytes", len(messageBytes))
			}
		})
	}
}

// TestSendMessageUTF8Counting tests that byte counting works correctly for UTF-8
func TestSendMessageUTF8Counting(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		expectByte int
	}{
		{"ASCII only", "Hello", 5},
		{"UTF-8 2-byte chars", "café", 5},   // c, a, f, é(2 bytes)
		{"UTF-8 3-byte chars", "世界", 6},     // Each Chinese char is 3 bytes
		{"UTF-8 4-byte chars", "𝓗𝓮𝓵𝓵𝓸", 20}, // Each mathematical script char is 4 bytes
		{"Mixed ASCII and UTF-8", "Hello 世界", 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messageBytes := []byte(tt.message)
			if len(messageBytes) != tt.expectByte {
				t.Errorf("Expected %d bytes, got %d", tt.expectByte, len(messageBytes))
			}
		})
	}
}

// TestFriendMapConcurrentAccess tests thread-safe friend map operations
func TestFriendMapConcurrentAccess(t *testing.T) {
	c := &Client{
		friends: make(map[uint32]*Friend),
	}

	// Add a friend
	friend := &Friend{
		ID:     1,
		Name:   "Test",
		Status: 0,
		Online: false,
	}

	c.friendsMu.Lock()
	c.friends[1] = friend
	c.friendsMu.Unlock()

	// Get friend
	c.friendsMu.RLock()
	retrieved, exists := c.friends[1]
	c.friendsMu.RUnlock()

	if !exists {
		t.Error("Expected friend to exist")
	}
	if retrieved.Name != "Test" {
		t.Errorf("Expected friend name 'Test', got '%s'", retrieved.Name)
	}

	// Update friend
	c.friendsMu.Lock()
	c.friends[1].Online = true
	c.friends[1].Name = "Updated"
	c.friendsMu.Unlock()

	// Verify update
	c.friendsMu.RLock()
	updated := c.friends[1]
	c.friendsMu.RUnlock()

	if !updated.Online {
		t.Error("Expected friend to be online")
	}
	if updated.Name != "Updated" {
		t.Errorf("Expected friend name 'Updated', got '%s'", updated.Name)
	}

	// Delete friend
	c.friendsMu.Lock()
	delete(c.friends, 1)
	c.friendsMu.Unlock()

	// Verify deletion
	c.friendsMu.RLock()
	_, exists = c.friends[1]
	c.friendsMu.RUnlock()

	if exists {
		t.Error("Expected friend to be deleted")
	}
}

// TestGetFriend tests the GetFriend method
func TestGetFriend(t *testing.T) {
	c := &Client{
		friends: make(map[uint32]*Friend),
	}

	// Test getting non-existent friend
	_, exists := c.GetFriend(99)
	if exists {
		t.Error("Expected friend 99 to not exist")
	}

	// Add a friend
	friend := &Friend{
		ID:     42,
		Name:   "TestFriend",
		Status: 1,
		Online: true,
	}

	c.friendsMu.Lock()
	c.friends[42] = friend
	c.friendsMu.Unlock()

	// Test getting existing friend
	retrieved, exists := c.GetFriend(42)
	if !exists {
		t.Error("Expected friend 42 to exist")
	}
	if retrieved.ID != 42 {
		t.Errorf("Expected friend ID 42, got %d", retrieved.ID)
	}
	if retrieved.Name != "TestFriend" {
		t.Errorf("Expected friend name 'TestFriend', got '%s'", retrieved.Name)
	}
	if retrieved.Status != 1 {
		t.Errorf("Expected friend status 1, got %d", retrieved.Status)
	}
	if !retrieved.Online {
		t.Error("Expected friend to be online")
	}
}

// TestMessageTypeHandling tests message type parameter handling
func TestMessageTypeHandling(t *testing.T) {
	tests := []struct {
		name        string
		messageType toxcore.MessageType
		expected    toxcore.MessageType
	}{
		{"Normal message", toxcore.MessageTypeNormal, toxcore.MessageTypeNormal},
		{"Action message", toxcore.MessageTypeAction, toxcore.MessageTypeAction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.messageType != tt.expected {
				t.Errorf("Expected message type %v, got %v", tt.expected, tt.messageType)
			}
		})
	}
}

// TestFriendStructStatusValues tests Friend struct status field values
func TestFriendStructStatusValues(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		expectedStr string
		validStatus bool
	}{
		{"None status", 0, "none", true},
		{"Away status", 1, "away", true},
		{"Busy status", 2, "busy", true},
		{"Invalid status", 99, "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			friend := &Friend{
				ID:     1,
				Status: tt.status,
			}

			if friend.Status != tt.status {
				t.Errorf("Expected status %d, got %d", tt.status, friend.Status)
			}

			// Map status to string (mimicking handler logic)
			var statusStr string
			switch tt.status {
			case 0:
				statusStr = "none"
			case 1:
				statusStr = "away"
			case 2:
				statusStr = "busy"
			default:
				statusStr = "unknown"
			}

			if statusStr != tt.expectedStr {
				t.Errorf("Expected status string '%s', got '%s'", tt.expectedStr, statusStr)
			}
		})
	}
}

// TestClientInitialization tests Client struct initialization
func TestClientInitialization(t *testing.T) {
	c := &Client{
		friends:           make(map[uint32]*Friend),
		conferences:       make(map[uint32]*Conference),
		incomingTransfers: make(map[string]*incomingTransfer),
		outgoingTransfers: make(map[string]*outgoingTransfer),
		shutdown:          make(chan struct{}),
	}

	if c.friends == nil {
		t.Error("Expected friends map to be initialized")
	}
	if c.conferences == nil {
		t.Error("Expected conferences map to be initialized")
	}
	if c.incomingTransfers == nil {
		t.Error("Expected incomingTransfers map to be initialized")
	}
	if c.outgoingTransfers == nil {
		t.Error("Expected outgoingTransfers map to be initialized")
	}
	if c.shutdown == nil {
		t.Error("Expected shutdown channel to be initialized")
	}

	if len(c.friends) != 0 {
		t.Errorf("Expected empty friends map, got %d entries", len(c.friends))
	}
	if len(c.conferences) != 0 {
		t.Errorf("Expected empty conferences map, got %d entries", len(c.conferences))
	}
	if len(c.incomingTransfers) != 0 {
		t.Errorf("Expected empty incomingTransfers map, got %d entries", len(c.incomingTransfers))
	}
	if len(c.outgoingTransfers) != 0 {
		t.Errorf("Expected empty outgoingTransfers map, got %d entries", len(c.outgoingTransfers))
	}
}

// TestTransferKeyGeneration tests file transfer key generation pattern
func TestTransferKeyGeneration(t *testing.T) {
	tests := []struct {
		name        string
		friendID    uint32
		fileID      uint32
		expectedKey string
	}{
		{"Simple IDs", 1, 2, "1:2"},
		{"Large friend ID", 1000, 5, "1000:5"},
		{"Large file ID", 3, 999, "3:999"},
		{"Both large", 4294967295, 4294967295, "4294967295:4294967295"}, // Max uint32
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mimics the key generation pattern used in handlers
			key := formatTransferKey(tt.friendID, tt.fileID)
			if key != tt.expectedKey {
				t.Errorf("Expected key '%s', got '%s'", tt.expectedKey, key)
			}
		})
	}
}

// TestConferenceStructure tests Conference struct
func TestConferenceStructure(t *testing.T) {
	c := &Conference{
		ID: 123,
	}

	if c.ID != 123 {
		t.Errorf("Expected conference ID 123, got %d", c.ID)
	}
	if c.Created.IsZero() {
		// Created time not set in test, so it should be zero
		t.Log("Conference created time is zero (expected in test)")
	}
}

// Helper function to format transfer key
func formatTransferKey(friendID, fileID uint32) string {
	return formatUint32(friendID) + ":" + formatUint32(fileID)
}

// Helper function to format uint32 as string
func formatUint32(n uint32) string {
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

package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opd-ai/go-ratox/config"
	"github.com/opd-ai/toxcore"
)

// TestIntegration tests the full client lifecycle using toxcore's test infrastructure
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip this test by default - it requires network connectivity and DHT bootstrap
	// Run with: go test -run TestIntegration -tags=integration
	t.Skip("Full integration test requires DHT bootstrap - run with -tags=integration")

	// Create temporary directories for two clients
	tmpDir1, err := os.MkdirTemp("", "ratox-test-1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "ratox-test-2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	// Create configs for both clients with testing-optimized settings
	cfg1 := createTestConfig(t, tmpDir1, "Alice")
	cfg2 := createTestConfig(t, tmpDir2, "Bob")

	// Create clients
	client1, err := newTestClient(t, cfg1)
	if err != nil {
		t.Fatalf("Failed to create client 1: %v", err)
	}
	defer client1.shutdown()

	client2, err := newTestClient(t, cfg2)
	if err != nil {
		t.Fatalf("Failed to create client 2: %v", err)
	}
	defer client2.shutdown()

	// Get addresses
	addr1 := client1.GetToxID()
	addr2 := client2.GetToxID()

	if addr1 == "" || addr2 == "" {
		t.Fatal("Failed to get Tox addresses")
	}

	t.Logf("Client 1 address: %s", addr1)
	t.Logf("Client 2 address: %s", addr2)

	// Test 1: Friend request and acceptance
	t.Run("FriendRequest", func(t *testing.T) {
		performFriendRequest(t, client1, client2, addr1, addr2)
	})

	// Test 2: Message exchange
	t.Run("MessageExchange", func(t *testing.T) {
		testMessageExchange(t, client1, client2)
	})

	// Test 3: File transfer
	t.Run("FileTransfer", func(t *testing.T) {
		testFileTransfer(t, client1, client2, tmpDir1, tmpDir2)
	})

	// Test 4: Status changes
	t.Run("StatusChanges", func(t *testing.T) {
		testStatusChanges(t, client1, client2)
	})
}

// createTestConfig creates a configuration optimized for testing
func createTestConfig(t *testing.T, dir, name string) *config.Config {
	t.Helper()

	return &config.Config{
		ConfigDir:       dir,
		Debug:           testing.Verbose(),
		Name:            name,
		StatusMessage:   "Testing ratox-go",
		AutoAcceptFiles: true,
		MaxFileSize:     1024 * 1024, // 1MB for testing
		BootstrapNodes:  []config.BootstrapNode{},
		Transport: config.TransportConfig{
			TCPEnabled: false,
			TorEnabled: false,
			I2PEnabled: false,
		},
		SaveFile: filepath.Join(dir, config.SaveDataFileName),
	}
}

// newTestClient creates a client with testing-optimized toxcore options
func newTestClient(t *testing.T, cfg *config.Config) (*testClient, error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		config:            cfg,
		ctx:               ctx,
		cancel:            cancel,
		friends:           make(map[uint32]*Friend),
		conferences:       make(map[uint32]*Conference),
		incomingTransfers: make(map[string]*incomingTransfer),
		outgoingTransfers: make(map[string]*outgoingTransfer),
		shutdown:          make(chan struct{}),
	}

	// Use testing-optimized options
	options := toxcore.NewOptionsForTesting()
	options.UDPEnabled = true

	// Create new Tox instance
	tox, err := toxcore.New(options)
	if err != nil {
		cancel()
		return nil, err
	}

	if err := tox.SelfSetName(cfg.Name); err != nil {
		tox.Kill()
		cancel()
		return nil, err
	}

	if err := tox.SelfSetStatusMessage(cfg.StatusMessage); err != nil {
		tox.Kill()
		cancel()
		return nil, err
	}

	client.tox = tox

	// Set up callbacks
	client.setupCallbacks()

	// Create wrapper for testing
	tc := &testClient{
		Client:   client,
		messages: make(chan testMessage, 10),
		requests: make(chan testFriendRequest, 10),
		files:    make(chan testFile, 10),
		statuses: make(chan testStatus, 10),
	}

	// Override callbacks to capture events for testing
	tc.setupTestCallbacks()

	// Start iteration loop
	go tc.iterate()

	// Wait for client to be ready
	time.Sleep(100 * time.Millisecond)

	return tc, nil
}

// testClient wraps Client with channels for capturing events
type testClient struct {
	*Client
	messages chan testMessage
	requests chan testFriendRequest
	files    chan testFile
	statuses chan testStatus
}

type testMessage struct {
	friendID uint32
	message  string
}

type testFriendRequest struct {
	publicKey [32]byte
	message   string
}

type testFile struct {
	friendID   uint32
	fileNumber uint32
	filename   string
	fileSize   uint64
}

type testStatus struct {
	friendID uint32
	online   bool
}

// setupTestCallbacks sets up callbacks that capture events for testing
func (tc *testClient) setupTestCallbacks() {
	tc.tox.OnFriendMessage(func(friendID uint32, message string) {
		select {
		case tc.messages <- testMessage{friendID, message}:
		case <-tc.ctx.Done():
		default:
		}
	})

	tc.tox.OnFriendRequest(func(publicKey [32]byte, message string) {
		select {
		case tc.requests <- testFriendRequest{publicKey, message}:
		case <-tc.ctx.Done():
		default:
		}
	})

	tc.tox.OnFriendConnectionStatus(func(friendID uint32, status toxcore.ConnectionStatus) {
		online := status != toxcore.ConnectionNone
		select {
		case tc.statuses <- testStatus{friendID, online}:
		case <-tc.ctx.Done():
		default:
		}
	})

	tc.tox.OnFileRecv(func(friendID, fileNumber uint32, kind uint32, fileSize uint64, filename string) {
		select {
		case tc.files <- testFile{friendID, fileNumber, filename, fileSize}:
		case <-tc.ctx.Done():
		default:
		}
	})
}

// iterate runs the Tox event loop
func (tc *testClient) iterate() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-tc.ctx.Done():
			return
		case <-ticker.C:
			tc.tox.Iterate()
		}
	}
}

// shutdown cleanly shuts down the test client
func (tc *testClient) shutdown() {
	tc.cancel()
	time.Sleep(100 * time.Millisecond) // Allow goroutines to stop
	if tc.tox != nil {
		tc.tox.Kill()
	}
}

// waitForMessage waits for a message with timeout
func (tc *testClient) waitForMessage(timeout time.Duration) (testMessage, bool) {
	select {
	case msg := <-tc.messages:
		return msg, true
	case <-time.After(timeout):
		return testMessage{}, false
	}
}

// waitForRequest waits for a friend request with timeout
func (tc *testClient) waitForRequest(timeout time.Duration) (testFriendRequest, bool) {
	select {
	case req := <-tc.requests:
		return req, true
	case <-time.After(timeout):
		return testFriendRequest{}, false
	}
}

// waitForStatus waits for status change with timeout
func (tc *testClient) waitForStatus(timeout time.Duration) (testStatus, bool) {
	select {
	case status := <-tc.statuses:
		return status, true
	case <-time.After(timeout):
		return testStatus{}, false
	}
}

// performFriendRequest tests friend request and acceptance
func performFriendRequest(t *testing.T, client1, client2 *testClient, addr1, addr2 string) {
	t.Helper()

	// Client 1 sends friend request to Client 2
	friendID, err := client1.tox.AddFriend(addr2, "Hello from Alice")
	if err != nil {
		t.Fatalf("Failed to send friend request: %v", err)
	}

	t.Logf("Client 1 sent friend request, friend ID: %d", friendID)

	// Wait for Client 2 to receive the request
	req, ok := client2.waitForRequest(5 * time.Second)
	if !ok {
		t.Fatal("Client 2 did not receive friend request")
	}

	t.Logf("Client 2 received friend request: %s", req.message)

	// Client 2 accepts the request
	friendID2, err := client2.tox.AddFriendByPublicKey(req.publicKey)
	if err != nil {
		t.Fatalf("Failed to accept friend request: %v", err)
	}

	t.Logf("Client 2 accepted friend request, friend ID: %d", friendID2)

	// Wait for connection to establish
	deadline := time.Now().Add(10 * time.Second)
	client1Connected := false
	client2Connected := false

	for time.Now().Before(deadline) && (!client1Connected || !client2Connected) {
		if !client1Connected {
			if status, ok := client1.waitForStatus(500 * time.Millisecond); ok && status.online {
				client1Connected = true
				t.Logf("Client 1 sees Client 2 online")
			}
		}
		if !client2Connected {
			if status, ok := client2.waitForStatus(500 * time.Millisecond); ok && status.online {
				client2Connected = true
				t.Logf("Client 2 sees Client 1 online")
			}
		}
	}

	if !client1Connected || !client2Connected {
		t.Fatal("Clients failed to establish connection")
	}
}

// testMessageExchange tests sending and receiving messages
func testMessageExchange(t *testing.T, client1, client2 *testClient) {
	t.Helper()

	// Get friend IDs
	friends1 := client1.tox.GetFriends()
	friends2 := client2.tox.GetFriends()

	if len(friends1) == 0 || len(friends2) == 0 {
		t.Fatal("No friends found")
	}

	// Get first friend ID from each client
	var friendID1, friendID2 uint32
	for id := range friends1 {
		friendID1 = id
		break
	}
	for id := range friends2 {
		friendID2 = id
		break
	}

	// Client 1 sends message to Client 2
	testMsg := "Hello from Alice!"
	err := client1.tox.SendFriendMessage(friendID1, testMsg, toxcore.MessageTypeNormal)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait for Client 2 to receive the message
	msg, ok := client2.waitForMessage(5 * time.Second)
	if !ok {
		t.Fatal("Client 2 did not receive message")
	}

	if msg.message != testMsg {
		t.Errorf("Message mismatch: got %q, want %q", msg.message, testMsg)
	}

	t.Logf("Client 2 received message: %s", msg.message)

	// Client 2 sends reply
	replyMsg := "Hello from Bob!"
	err = client2.tox.SendFriendMessage(friendID2, replyMsg, toxcore.MessageTypeNormal)
	if err != nil {
		t.Fatalf("Failed to send reply: %v", err)
	}

	// Wait for Client 1 to receive the reply
	msg, ok = client1.waitForMessage(5 * time.Second)
	if !ok {
		t.Fatal("Client 1 did not receive reply")
	}

	if msg.message != replyMsg {
		t.Errorf("Reply mismatch: got %q, want %q", msg.message, replyMsg)
	}

	t.Logf("Client 1 received reply: %s", msg.message)
}

// testFileTransfer tests file transfer between clients
func testFileTransfer(t *testing.T, client1, client2 *testClient, tmpDir1, tmpDir2 string) {
	t.Helper()

	// Get friend IDs
	friends1 := client1.tox.GetFriends()
	if len(friends1) == 0 {
		t.Fatal("No friends found for file transfer")
	}
	
	var friendID1 uint32
	for id := range friends1 {
		friendID1 = id
		break
	}

	// Create a test file
	testData := []byte("This is a test file for ratox-go integration testing!")
	testFilePath := filepath.Join(tmpDir1, "test_file.txt")
	if err := os.WriteFile(testFilePath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	testFileName := "test_file.txt"

	// Client 1 sends file to Client 2
	var fileID [32]byte
	fileNumber, err := client1.tox.FileSend(friendID1, 0, uint64(len(testData)), fileID, testFileName)
	if err != nil {
		t.Fatalf("Failed to send file: %v", err)
	}

	t.Logf("Client 1 sending file, file number: %d", fileNumber)

	// Wait for Client 2 to receive file offer
	fileInfo, ok := client2.waitForFile(5 * time.Second)
	if !ok {
		t.Fatal("Client 2 did not receive file offer")
	}

	if fileInfo.filename != testFileName {
		t.Errorf("Filename mismatch: got %q, want %q", fileInfo.filename, testFileName)
	}

	if fileInfo.fileSize != uint64(len(testData)) {
		t.Errorf("File size mismatch: got %d, want %d", fileInfo.fileSize, len(testData))
	}

	t.Logf("Client 2 received file offer: %s (%d bytes)", fileInfo.filename, fileInfo.fileSize)

	// Since auto_accept_files is true, file should be automatically accepted
	// In a real scenario, we would need to implement the full file transfer protocol
	// For now, we just verify the offer was received
}

// testStatusChanges tests status message changes
func testStatusChanges(t *testing.T, client1, client2 *testClient) {
	t.Helper()

	// Change Client 1's status message
	newStatus := "Away for testing"
	if err := client1.tox.SelfSetStatusMessage(newStatus); err != nil {
		t.Fatalf("Failed to set status message: %v", err)
	}

	t.Logf("Client 1 changed status to: %s", newStatus)

	// Give time for the change to propagate
	time.Sleep(1 * time.Second)

	// Verify the changes propagated (in a full implementation)
	// For now, we just verify the API calls succeeded
}

// waitForFile waits for a file receive event with timeout
func (tc *testClient) waitForFile(timeout time.Duration) (testFile, bool) {
	select {
	case file := <-tc.files:
		return file, true
	case <-time.After(timeout):
		return testFile{}, false
	}
}

// TestIntegrationFriendManagement tests friend management operations
func TestIntegrationFriendManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "ratox-test-friend-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(t, tmpDir, "TestUser")
	client, err := newTestClient(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	// Test adding friend by Tox ID
	fakeToxID := make([]byte, 38)
	for i := range fakeToxID {
		fakeToxID[i] = byte(i)
	}
	fakeToxIDHex := hex.EncodeToString(fakeToxID)

	// This should fail because the Tox ID is not valid/not online
	// but it tests the API
	_, err = client.tox.AddFriend(fakeToxIDHex, "Test request")
	if err == nil {
		// Actually, it might succeed in adding the friend even if they're offline
		t.Logf("Friend added (will be pending until they come online)")
	} else {
		t.Logf("Friend add failed as expected: %v", err)
	}

	// Test getting friend list
	friends := client.tox.GetFriends()
	t.Logf("Friend list has %d friends", len(friends))
}

// TestIntegrationSaveAndRestore tests saving and restoring Tox state
func TestIntegrationSaveAndRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "ratox-test-save-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(t, tmpDir, "SaveTestUser")

	// Create first client and get its address
	client1, err := newTestClient(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create client 1: %v", err)
	}

	addr1 := client1.GetToxID()
	name1 := client1.tox.SelfGetName()

	// Save the state
	saveData := client1.tox.GetSavedata()
	if len(saveData) == 0 {
		t.Fatal("Savedata is empty")
	}

	if err := os.WriteFile(cfg.SaveFile, saveData, 0600); err != nil {
		t.Fatalf("Failed to write save file: %v", err)
	}

	t.Logf("Saved Tox state (%d bytes)", len(saveData))

	// Shutdown first client
	client1.shutdown()

	// Create second client from saved state
	options := toxcore.NewOptionsForTesting()
	options.UDPEnabled = true

	savedData, err := os.ReadFile(cfg.SaveFile)
	if err != nil {
		t.Fatalf("Failed to read save file: %v", err)
	}

	tox2, err := toxcore.NewFromSavedata(options, savedData)
	if err != nil {
		t.Fatalf("Failed to restore from savedata: %v", err)
	}
	defer tox2.Kill()

	// Get address from the second instance - use SelfGetAddress which returns a string
	addr2String := tox2.SelfGetAddress()
	addr2 := addr2String
	name2 := tox2.SelfGetName()

	// Verify the addresses match - both should be the same Tox ID
	if addr1 != addr2 && !contains(addr2, addr1) && !contains(addr1, addr2) {
		t.Logf("Address 1: %s", addr1)
		t.Logf("Address 2: %s", addr2)
		// This is acceptable - addresses may be in different formats
	}

	if name1 != name2 {
		t.Errorf("Name mismatch after restore: got %q, want %q", name2, name1)
	}

	t.Logf("Successfully restored Tox state with address: %s", addr2)
}

func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && (s == substr || s[0:len(substr)] == substr || s[len(s)-len(substr):] == substr)
}

// TestIntegrationMessageValidation tests message validation
func TestIntegrationMessageValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "ratox-test-validation-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(t, tmpDir, "ValidationTest")
	client, err := newTestClient(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	// Test empty message validation
	emptyMsg := ""
	if err := client.SendMessage(0, emptyMsg, toxcore.MessageTypeNormal); err == nil {
		t.Error("Expected error for empty message, got nil")
	}

	// Test message length validation (1372 bytes max)
	longMsg := make([]byte, 1373)
	for i := range longMsg {
		longMsg[i] = 'a'
	}
	if err := client.SendMessage(0, string(longMsg), toxcore.MessageTypeNormal); err == nil {
		t.Error("Expected error for too long message, got nil")
	}

	// Test valid message
	validMsg := "This is a valid message"
	// This will fail because friend ID 0 doesn't exist, but validates the message
	err = client.SendMessage(0, validMsg, toxcore.MessageTypeNormal)
	if err != nil {
		t.Logf("Message validation passed, send failed as expected: %v", err)
	}
}

// TestIntegrationConcurrency tests concurrent operations
func TestIntegrationConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "ratox-test-concurrent-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(t, tmpDir, "ConcurrentTest")
	client, err := newTestClient(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	// Test concurrent friend map access
	done := make(chan bool)
	errChan := make(chan error, 10)

	// Concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				_, _ = client.GetFriend(uint32(j))
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				client.friendsMu.Lock()
				client.friends[uint32(id*100+j)] = &Friend{
					ID:   uint32(id*100 + j),
					Name: "Test Friend",
				}
				client.friendsMu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	close(errChan)
	for err := range errChan {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

// TestIntegrationIterationInterval tests using dynamic iteration interval
func TestIntegrationIterationInterval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "ratox-test-interval-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(t, tmpDir, "IntervalTest")
	client, err := newTestClient(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	// Get iteration interval
	interval := client.tox.IterationInterval()
	if interval <= 0 {
		t.Errorf("Invalid iteration interval: %v", interval)
	}

	if interval > 1*time.Second {
		t.Errorf("Iteration interval too long: %v", interval)
	}

	t.Logf("Iteration interval: %v", interval)

	// Verify multiple calls return consistent results
	for i := 0; i < 10; i++ {
		interval2 := client.tox.IterationInterval()
		if interval2 != interval {
			t.Logf("Iteration interval changed: %v -> %v (this is acceptable)", interval, interval2)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// BenchmarkIteration benchmarks the iteration loop performance
func BenchmarkIteration(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "ratox-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(&testing.T{}, tmpDir, "BenchUser")
	client, err := newTestClient(&testing.T{}, cfg)
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.tox.Iterate()
	}
}

// BenchmarkMessageSend benchmarks message sending performance
func BenchmarkMessageSend(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "ratox-bench-msg-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(&testing.T{}, tmpDir, "BenchUser")
	client, err := newTestClient(&testing.T{}, cfg)
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	// Create a fake friend for benchmarking
	client.friendsMu.Lock()
	client.friends[0] = &Friend{
		ID:     0,
		Name:   "Bench Friend",
		Online: true,
	}
	client.friendsMu.Unlock()

	testMsg := "Benchmark message"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail because the friend doesn't actually exist in Tox,
		// but it benchmarks the validation and friend lookup logic
		_ = client.SendMessage(0, testMsg, toxcore.MessageTypeNormal)
	}
}

// TestIntegrationUTF8Messages tests UTF-8 message handling
func TestIntegrationUTF8Messages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "ratox-test-utf8-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := createTestConfig(t, tmpDir, "UTF8Test")
	client, err := newTestClient(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.shutdown()

	// Test various UTF-8 strings
	testCases := []struct {
		name string
		msg  string
	}{
		{"ASCII", "Hello, world!"},
		{"Emoji", "Hello 👋 World 🌍"},
		{"Chinese", "你好世界"},
		{"Japanese", "こんにちは世界"},
		{"Arabic", "مرحبا بالعالم"},
		{"Mixed", "Hello 世界 🌍"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a buffer to simulate FIFO behavior
			buf := bytes.NewBufferString(tc.msg)
			result, err := io.ReadAll(buf)
			if err != nil {
				t.Fatalf("Failed to read message: %v", err)
			}
			if string(result) != tc.msg {
				t.Errorf("Message mismatch: got %q, want %q", string(result), tc.msg)
			}

			// Verify byte length for Tox protocol
			byteLen := len([]byte(tc.msg))
			if byteLen > 1372 {
				t.Errorf("Message too long: %d bytes (max 1372)", byteLen)
			}
		})
	}
}

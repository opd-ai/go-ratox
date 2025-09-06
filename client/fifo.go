// Package client implements FIFO management for ratox-go
package client

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/opd-ai/go-ratox/config"
	"github.com/opd-ai/toxcore"
)

// FIFOManager handles all FIFO operations for the client
type FIFOManager struct {
	config  *config.Config
	client  *Client
	fifos   map[string]*FIFO
	fifosMu sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// FIFO represents a named pipe with associated metadata
type FIFO struct {
	Path     string
	Mode     os.FileMode
	IsInput  bool // true for write-only FIFOs (user writes to them)
	IsOutput bool // true for read-only FIFOs (user reads from them)
	File     *os.File
	Reader   *bufio.Reader
	Writer   *bufio.Writer
	LastUsed time.Time
	mu       sync.Mutex
}

// FIFO names and permissions
const (
	// Global FIFOs
	RequestIn     = "request_in"     // Write-only - accept friend requests
	RequestOut    = "request_out"    // Read-only - incoming friend requests
	Name          = "name"           // Write-only - set display name
	StatusMessage = "status_message" // Write-only - set status message
	ID            = "id"             // Read-only - Tox ID file

	// Friend-specific FIFOs
	TextIn  = "text_in"  // Write-only - send messages
	TextOut = "text_out" // Read-only - receive messages
	FileIn  = "file_in"  // Write-only - send files
	FileOut = "file_out" // Read-only - receive files
	Status  = "status"   // Read-only - friend status

	// FIFO permissions
	FIFOPermInput  = 0600 // Read/write for owner
	FIFOPermOutput = 0600 // Read/write for owner
	DirPerm        = 0700 // rwx------
)

// NewFIFOManager creates a new FIFO manager
func NewFIFOManager(client *Client) *FIFOManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &FIFOManager{
		config: client.config,
		client: client,
		fifos:  make(map[string]*FIFO),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Run starts the FIFO manager
func (fm *FIFOManager) Run(ctx context.Context) {
	defer fm.wg.Wait()

	// Create global FIFOs
	if err := fm.createGlobalFIFOs(); err != nil {
		log.Printf("Failed to create global FIFOs: %v", err)
		return
	}

	// Start monitoring global FIFOs
	fm.wg.Add(1)
	go func() {
		defer fm.wg.Done()
		fm.monitorGlobalFIFOs(ctx)
	}()

	// Start periodic cleanup
	fm.wg.Add(1)
	go func() {
		defer fm.wg.Done()
		fm.periodicCleanup(ctx)
	}()

	<-ctx.Done()
}

// createGlobalFIFOs creates the global FIFO files
func (fm *FIFOManager) createGlobalFIFOs() error {
	// Create client subdirectory for global FIFOs
	clientDir := filepath.Join(fm.config.ConfigDir, "client")
	if err := os.MkdirAll(clientDir, DirPerm); err != nil {
		return fmt.Errorf("failed to create client directory: %w", err)
	}

	globalFIFOs := []struct {
		name     string
		isInput  bool
		isOutput bool
	}{
		{RequestIn, true, false},
		{RequestOut, false, true},
		{Name, true, false},
		{StatusMessage, true, false},
	}

	for _, fifo := range globalFIFOs {
		path := fm.config.GlobalFIFOPath(fifo.name)
		if err := fm.createFIFO(path, fifo.isInput, fifo.isOutput); err != nil {
			return fmt.Errorf("failed to create FIFO %s: %w", fifo.name, err)
		}
	}

	// Create ID file with Tox ID
	if err := fm.createIDFile(); err != nil {
		return fmt.Errorf("failed to create ID file: %w", err)
	}

	return nil
}

// CreateFriendFIFOs creates FIFO files for a specific friend
func (fm *FIFOManager) CreateFriendFIFOs(friendID string) error {
	friendDir := fm.config.FriendDir(friendID)
	if err := os.MkdirAll(friendDir, DirPerm); err != nil {
		return fmt.Errorf("failed to create friend directory: %w", err)
	}

	friendFIFOs := []struct {
		name     string
		isInput  bool
		isOutput bool
	}{
		{TextIn, true, false},
		{TextOut, false, true},
		{FileIn, true, false},
		{FileOut, false, true},
		{Status, false, true},
	}

	for _, fifo := range friendFIFOs {
		path := fm.config.FriendFIFOPath(friendID, fifo.name)
		if err := fm.createFIFO(path, fifo.isInput, fifo.isOutput); err != nil {
			return fmt.Errorf("failed to create FIFO %s: %w", fifo.name, err)
		}
	}

	// Start monitoring friend FIFOs
	fm.wg.Add(1)
	go func() {
		defer fm.wg.Done()
		fm.monitorFriendFIFOs(fm.ctx, friendID)
	}()

	return nil
}

// createFIFO creates a named pipe with the specified permissions
func (fm *FIFOManager) createFIFO(path string, isInput, isOutput bool) error {
	// Clean up existing FIFO resources if they exist
	fm.fifosMu.Lock()
	if existingFIFO, exists := fm.fifos[path]; exists {
		// Close any open file handles
		if existingFIFO.File != nil {
			existingFIFO.File.Close()
		}
		// Remove from map
		delete(fm.fifos, path)
	}
	fm.fifosMu.Unlock()

	// Remove existing FIFO file if it exists
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing FIFO: %w", err)
	}

	// Create the FIFO
	var perm os.FileMode = 0600 // Default permission
	if isInput {
		perm = FIFOPermInput
	} else if isOutput {
		perm = FIFOPermOutput
	}

	if err := syscall.Mkfifo(path, uint32(perm)); err != nil {
		return fmt.Errorf("failed to create FIFO: %w", err)
	}

	// Set correct permissions
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("failed to set FIFO permissions: %w", err)
	}

	// Register FIFO
	fm.fifosMu.Lock()
	fm.fifos[path] = &FIFO{
		Path:     path,
		Mode:     perm,
		IsInput:  isInput,
		IsOutput: isOutput,
		LastUsed: time.Now(),
	}
	fm.fifosMu.Unlock()

	if fm.config.Debug {
		log.Printf("Created FIFO: %s (input: %v, output: %v)", path, isInput, isOutput)
	}

	return nil
}

// createIDFile creates a file containing the Tox ID for user reference
func (fm *FIFOManager) createIDFile() error {
	idPath := fm.config.GlobalFIFOPath(ID)
	toxID := fm.client.GetToxID()

	if err := os.WriteFile(idPath, []byte(toxID+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write ID file: %w", err)
	}

	if fm.config.Debug {
		log.Printf("Created ID file: %s", idPath)
	}

	return nil
}

// monitorGlobalFIFOs monitors global FIFO files for input
func (fm *FIFOManager) monitorGlobalFIFOs(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Monitor request_in
			if err := fm.readFIFO(ctx, fm.config.GlobalFIFOPath(RequestIn), fm.handleRequestIn); err != nil {
				if fm.config.Debug {
					log.Printf("Error reading request_in: %v", err)
				}
			}

			// Monitor name
			if err := fm.readFIFO(ctx, fm.config.GlobalFIFOPath(Name), fm.handleNameChange); err != nil {
				if fm.config.Debug {
					log.Printf("Error reading name: %v", err)
				}
			}

			// Monitor status_message
			if err := fm.readFIFO(ctx, fm.config.GlobalFIFOPath(StatusMessage), fm.handleStatusMessageChange); err != nil {
				if fm.config.Debug {
					log.Printf("Error reading status_message: %v", err)
				}
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}

// monitorFriendFIFOs monitors FIFO files for a specific friend
func (fm *FIFOManager) monitorFriendFIFOs(ctx context.Context, friendID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Monitor text_in
			textInPath := fm.config.FriendFIFOPath(friendID, TextIn)
			if err := fm.readFIFO(ctx, textInPath, func(data string) { fm.handleFriendTextIn(friendID, data) }); err != nil {
				if fm.config.Debug {
					log.Printf("Error reading text_in for %s: %v", friendID, err)
				}
			}

			// Monitor file_in
			fileInPath := fm.config.FriendFIFOPath(friendID, FileIn)
			if err := fm.readFIFO(ctx, fileInPath, func(data string) { fm.handleFriendFileIn(friendID, data) }); err != nil {
				if fm.config.Debug {
					log.Printf("Error reading file_in for %s: %v", friendID, err)
				}
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}

// readFIFO reads data from a FIFO and calls the handler function
func (fm *FIFOManager) readFIFO(ctx context.Context, path string, handler func(string)) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Open FIFO for reading (non-blocking)
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read data
	reader := bufio.NewReader(file)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line != "" {
			handler(line)
		}
	}

	return nil
}

// writeFIFO writes data to a FIFO
func (fm *FIFOManager) writeFIFO(path, data string) error {
	fm.fifosMu.RLock()
	fifo, exists := fm.fifos[path]
	fm.fifosMu.RUnlock()

	if !exists {
		return fmt.Errorf("FIFO not found: %s", path)
	}

	if !fifo.IsOutput {
		return fmt.Errorf("FIFO is not an output FIFO: %s", path)
	}

	// Open FIFO for writing (non-blocking)
	file, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("failed to open FIFO for writing: %w", err)
	}
	defer file.Close()

	// Write data with newline
	if _, err := fmt.Fprintln(file, data); err != nil {
		return fmt.Errorf("failed to write to FIFO: %w", err)
	}

	fifo.LastUsed = time.Now()
	return nil
}

// FIFO event handlers

// handleRequestIn processes friend request acceptance
func (fm *FIFOManager) handleRequestIn(toxID string) {
	toxID = strings.TrimSpace(toxID)

	// Accept both 64-character public key and 76-character full Tox ID
	var publicKeyHex string
	if len(toxID) == 64 {
		// 64-character public key format
		publicKeyHex = toxID
	} else if len(toxID) == 76 {
		// 76-character full Tox ID format (public key + nospam + checksum)
		publicKeyHex = toxID[:64]
	} else {
		log.Printf("Invalid Tox ID format: expected 64 or 76 characters, got %d", len(toxID))
		return
	}

	// Decode public key from hex
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		log.Printf("Invalid public key in Tox ID: %v", err)
		return
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	// Accept friend request
	if _, err := fm.client.AcceptFriendRequest(publicKey); err != nil {
		log.Printf("Failed to accept friend request: %v", err)
	}
}

// handleNameChange processes display name changes
func (fm *FIFOManager) handleNameChange(name string) {
	name = strings.TrimSpace(name)
	if len(name) == 0 {
		log.Printf("Empty name provided")
		return
	}

	if err := fm.client.UpdateSelfName(name); err != nil {
		log.Printf("Failed to update name: %v", err)
	}
}

// handleStatusMessageChange processes status message changes
func (fm *FIFOManager) handleStatusMessageChange(message string) {
	message = strings.TrimSpace(message)

	if err := fm.client.UpdateSelfStatusMessage(message); err != nil {
		log.Printf("Failed to update status message: %v", err)
	}
}

// handleFriendTextIn processes outgoing text messages
func (fm *FIFOManager) handleFriendTextIn(friendID, message string) {
	message = strings.TrimSpace(message)
	if len(message) == 0 {
		return
	}

	// Find friend by public key
	publicKeyBytes, err := hex.DecodeString(friendID)
	if err != nil {
		log.Printf("Invalid friend ID: %v", err)
		return
	}

	// Validate public key length to prevent buffer overflow
	if len(publicKeyBytes) != 32 {
		log.Printf("Invalid friend ID length: expected 32 bytes, got %d", len(publicKeyBytes))
		return
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	// Find friend number by public key
	var friendNum uint32
	var found bool
	fm.client.friendsMu.RLock()
	for _, friend := range fm.client.friends {
		if friend.PublicKey == publicKey {
			friendNum = friend.ID
			found = true
			break
		}
	}
	fm.client.friendsMu.RUnlock()

	if !found {
		log.Printf("Friend not found: %s", friendID)
		return
	}

	// Determine message type (action messages start with "/me ")
	messageType := toxcore.MessageTypeNormal
	if strings.HasPrefix(message, "/me ") {
		messageType = toxcore.MessageTypeAction
		message = strings.TrimPrefix(message, "/me ")
	}

	// Send message
	if err := fm.client.SendMessage(friendNum, message, messageType); err != nil {
		log.Printf("Failed to send message to friend %s: %v", friendID, err)
	}
}

// handleFriendFileIn processes outgoing file transfers
func (fm *FIFOManager) handleFriendFileIn(friendID, filePath string) {
	filePath = strings.TrimSpace(filePath)
	if len(filePath) == 0 {
		return
	}

	log.Printf("File transfer request for %s: %s", friendID, filePath)

	// Find friend by public key
	publicKeyBytes, err := hex.DecodeString(friendID)
	if err != nil {
		log.Printf("Invalid friend ID: %v", err)
		return
	}

	if len(publicKeyBytes) != 32 {
		log.Printf("Invalid friend ID length: expected 32 bytes, got %d", len(publicKeyBytes))
		return
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	// Find friend
	var friendNum uint32
	var found bool
	fm.client.friendsMu.RLock()
	for _, friend := range fm.client.friends {
		if friend.PublicKey == publicKey {
			friendNum = friend.ID
			found = true
			break
		}
	}
	fm.client.friendsMu.RUnlock()

	if !found {
		log.Printf("Friend not found: %s", friendID)
		return
	}

	// Check if file exists and get its size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("File not found or inaccessible: %v", err)
		return
	}

	if fileInfo.IsDir() {
		log.Printf("Cannot send directory: %s", filePath)
		return
	}

	fileSize := uint64(fileInfo.Size())

	// Check file size limits
	if fm.client.config.MaxFileSize > 0 && int64(fileSize) > fm.client.config.MaxFileSize {
		log.Printf("File too large (%d bytes), maximum allowed: %d", fileSize, fm.client.config.MaxFileSize)
		return
	}

	// Generate file ID (use first 32 bytes of file path hash for simplicity)
	h := sha256.Sum256([]byte(filePath))
	fileID := h

	// Start file transfer
	filename := filepath.Base(filePath)
	transferID, err := fm.client.tox.FileSend(friendNum, 0, fileSize, fileID, filename)
	if err != nil {
		log.Printf("Failed to initiate file transfer: %v", err)
		return
	}

	log.Printf("File transfer initiated: %s (%d bytes) to friend %d, transfer ID: %d", filename, fileSize, friendNum, transferID)
}

// Write functions for output FIFOs

// WriteRequestOut writes a friend request to the request_out FIFO
func (fm *FIFOManager) WriteRequestOut(friendID, message string) error {
	path := fm.config.GlobalFIFOPath(RequestOut)
	data := fmt.Sprintf("%s %s", friendID, message)
	return fm.writeFIFO(path, data)
}

// WriteFriendTextOut writes a message to a friend's text_out FIFO
func (fm *FIFOManager) WriteFriendTextOut(friendID, message string) error {
	path := fm.config.FriendFIFOPath(friendID, TextOut)
	return fm.writeFIFO(path, message)
}

// WriteFriendStatus writes status to a friend's status FIFO
func (fm *FIFOManager) WriteFriendStatus(friendID, status string) error {
	path := fm.config.FriendFIFOPath(friendID, Status)
	return fm.writeFIFO(path, status)
}

// WriteFriendFileOut writes file transfer info to a friend's file_out FIFO
func (fm *FIFOManager) WriteFriendFileOut(friendID, fileInfo string) error {
	path := fm.config.FriendFIFOPath(friendID, FileOut)
	return fm.writeFIFO(path, fileInfo)
}

// periodicCleanup performs periodic maintenance tasks
func (fm *FIFOManager) periodicCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Clean up unused FIFOs
			fm.cleanupUnusedFIFOs()
		}
	}
}

// cleanupUnusedFIFOs removes FIFOs that haven't been used recently
func (fm *FIFOManager) cleanupUnusedFIFOs() {
	cutoff := time.Now().Add(-30 * time.Minute)

	fm.fifosMu.Lock()
	defer fm.fifosMu.Unlock()

	for path, fifo := range fm.fifos {
		if fifo.LastUsed.Before(cutoff) && !isGlobalFIFO(path) {
			if fm.config.Debug {
				log.Printf("Cleaning up unused FIFO: %s", path)
			}
			delete(fm.fifos, path)
		}
	}
}

// isGlobalFIFO returns true if the path is a global FIFO
func isGlobalFIFO(path string) bool {
	name := filepath.Base(path)
	return name == RequestIn || name == RequestOut || name == Name || name == StatusMessage
}

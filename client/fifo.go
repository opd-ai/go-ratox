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
	RequestIn        = "request_in"        // Write-only - accept friend requests
	RequestOut       = "request_out"       // Read-only - incoming friend requests
	Name             = "name"              // Write-only - set display name
	StatusMessage    = "status_message"    // Write-only - set status message
	ID               = "id"                // Read-only - Tox ID file
	ConnectionStatus = "connection_status" // Read-only - connection status info
	TransportStatus  = "transport_status"  // Read-only - transport status info
	ConferenceIn     = "conference_in"     // Write-only - create/join conferences

	// Friend-specific FIFOs
	TextIn              = "text_in"        // Write-only - send messages
	TextOut             = "text_out"       // Read-only - receive messages
	FileIn              = "file_in"        // Write-only - send files
	FileOut             = "file_out"       // Read-only - receive files
	Status              = "status"         // Read-only - friend status
	FriendStatusMessage = "status_message" // Read-only - friend status message
	RemoveIn            = "remove_in"      // Write-only - remove friend
	Typing              = "typing"         // Read-only - typing indicator

	// Conference-specific FIFOs (per-conference directory)
	ConferenceTextIn   = "text_in"   // Write-only - send conference messages
	ConferenceInviteIn = "invite_in" // Write-only - invite friends to conference

	// FIFO permissions
	FIFOPermInput  = 0o600 // Read/write for owner
	FIFOPermOutput = 0o600 // Read/write for owner
	DirPerm        = 0o700 // rwx------
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
		{ConferenceIn, true, false},
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

	// Create connection status file
	if err := fm.createConnectionStatusFile(); err != nil {
		return fmt.Errorf("failed to create connection status file: %w", err)
	}

	// Create transport status file
	if err := fm.createTransportStatusFile(); err != nil {
		return fmt.Errorf("failed to create transport status file: %w", err)
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
		{FriendStatusMessage, false, true},
		{RemoveIn, true, false},
		{Typing, false, true},
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

// createConferenceFIFOs creates FIFO files for a conference
func (fm *FIFOManager) createConferenceFIFOs(conferenceID uint32) error {
	conferenceIDStr := fmt.Sprintf("%d", conferenceID)
	conferenceDir := fm.config.ConferenceDir(conferenceIDStr)
	if err := os.MkdirAll(conferenceDir, DirPerm); err != nil {
		return fmt.Errorf("failed to create conference directory: %w", err)
	}

	conferenceFIFOs := []struct {
		name     string
		isInput  bool
		isOutput bool
	}{
		{ConferenceTextIn, true, false},
		{ConferenceInviteIn, true, false},
	}

	for _, fifo := range conferenceFIFOs {
		path := fm.config.ConferenceFIFOPath(conferenceIDStr, fifo.name)
		if err := fm.createFIFO(path, fifo.isInput, fifo.isOutput); err != nil {
			return fmt.Errorf("failed to create FIFO %s: %w", fifo.name, err)
		}
	}

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
	var perm os.FileMode = 0o600 // Default permission
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

	if err := os.WriteFile(idPath, []byte(toxID+"\n"), 0o600); err != nil {
		return fmt.Errorf("failed to write ID file: %w", err)
	}

	if fm.config.Debug {
		log.Printf("Created ID file: %s", idPath)
	}

	return nil
}

// createConnectionStatusFile creates a file containing connection status information
func (fm *FIFOManager) createConnectionStatusFile() error {
	statusPath := fm.config.GlobalFIFOPath(ConnectionStatus)

	connectionStatus := fm.client.tox.SelfGetConnectionStatus()
	friends := fm.client.tox.GetFriends()
	friendsCount := len(friends)

	// Count online friends
	onlineFriends := 0
	fm.client.friendsMu.RLock()
	for _, friend := range fm.client.friends {
		if friend.Online {
			onlineFriends++
		}
	}
	fm.client.friendsMu.RUnlock()

	var statusStr string
	switch connectionStatus {
	case 0: // ConnectionNone
		statusStr = "offline"
	case 1: // ConnectionTCP
		statusStr = "tcp"
	case 2: // ConnectionUDP
		statusStr = "udp"
	default:
		statusStr = "unknown"
	}

	statusInfo := fmt.Sprintf("connection: %s\nfriends: %d total, %d online\n", statusStr, friendsCount, onlineFriends)

	if err := os.WriteFile(statusPath, []byte(statusInfo), 0o600); err != nil {
		return fmt.Errorf("failed to write connection status file: %w", err)
	}

	if fm.config.Debug {
		log.Printf("Updated connection status file: %s", statusPath)
	}

	return nil
}

// createTransportStatusFile creates a file containing transport status information
func (fm *FIFOManager) createTransportStatusFile() error {
	statusPath := fm.config.GlobalFIFOPath(TransportStatus)

	transportInfo := fm.getTransportInfo()

	if err := os.WriteFile(statusPath, []byte(transportInfo), 0o600); err != nil {
		return fmt.Errorf("failed to write transport status file: %w", err)
	}

	if fm.config.Debug {
		log.Printf("Created transport status file: %s", statusPath)
	}

	return nil
}

// getTransportInfo returns formatted transport information string
func (fm *FIFOManager) getTransportInfo() string {
	cfg := fm.config.Transport
	var transportType string

	if cfg.TorEnabled && cfg.I2PEnabled {
		transportType = fmt.Sprintf("tor+i2p (SOCKS5: %s, SAM: %s)", cfg.TorSOCKSAddr, cfg.I2PSAMAddr)
	} else if cfg.TorEnabled {
		transportType = fmt.Sprintf("tor (SOCKS5: %s)", cfg.TorSOCKSAddr)
	} else if cfg.I2PEnabled {
		transportType = fmt.Sprintf("i2p (SAM: %s)", cfg.I2PSAMAddr)
	} else if cfg.TCPEnabled {
		transportType = fmt.Sprintf("tcp+udp (TCP port: %d)", cfg.TCPPort)
	} else {
		transportType = "udp"
	}

	return fmt.Sprintf("transport: %s\n", transportType)
}

// monitorGlobalFIFOs monitors global FIFO files for input
func (fm *FIFOManager) monitorGlobalFIFOs(ctx context.Context) {
	// Monitor each FIFO in a separate goroutine to avoid busy polling
	var wg sync.WaitGroup

	// Monitor request_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		fm.monitorSingleFIFO(ctx, fm.config.GlobalFIFOPath(RequestIn), fm.handleRequestIn)
	}()

	// Monitor name
	wg.Add(1)
	go func() {
		defer wg.Done()
		fm.monitorSingleFIFO(ctx, fm.config.GlobalFIFOPath(Name), fm.handleNameChange)
	}()

	// Monitor status_message
	wg.Add(1)
	go func() {
		defer wg.Done()
		fm.monitorSingleFIFO(ctx, fm.config.GlobalFIFOPath(StatusMessage), fm.handleStatusMessageChange)
	}()

	// Monitor conference_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		fm.monitorSingleFIFO(ctx, fm.config.GlobalFIFOPath(ConferenceIn), fm.handleConferenceIn)
	}()

	// Wait for all monitoring goroutines to finish
	wg.Wait()
}

// monitorSingleFIFO monitors a single FIFO with blocking reads to avoid busy polling
func (fm *FIFOManager) monitorSingleFIFO(ctx context.Context, path string, handler func(string)) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Use blocking read to avoid busy polling
		if err := fm.readFIFOBlocking(ctx, path, handler); err != nil {
			if fm.config.Debug {
				log.Printf("Error reading FIFO %s: %v", path, err)
			}
			// Brief sleep before retrying to avoid rapid error loops
			time.Sleep(1 * time.Second)
		}
	}
}

// readFIFOBlocking reads from a FIFO with blocking I/O
func (fm *FIFOManager) readFIFOBlocking(ctx context.Context, path string, handler func(string)) error {
	// Open FIFO for reading (blocking mode)
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// ReadString blocks until data is available or EOF
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// FIFO was closed (writer disconnected), need to reopen
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line != "" {
			handler(line)
		}
	}
}

// monitorFriendFIFOs monitors FIFO files for a specific friend
func (fm *FIFOManager) monitorFriendFIFOs(ctx context.Context, friendID string) {
	// Monitor each friend FIFO in a separate goroutine to avoid busy polling
	var wg sync.WaitGroup

	// Monitor text_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		textInPath := fm.config.FriendFIFOPath(friendID, TextIn)
		fm.monitorSingleFIFO(ctx, textInPath, func(data string) { fm.handleFriendTextIn(friendID, data) })
	}()

	// Monitor file_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		fileInPath := fm.config.FriendFIFOPath(friendID, FileIn)
		fm.monitorSingleFIFO(ctx, fileInPath, func(data string) { fm.handleFriendFileIn(friendID, data) })
	}()

	// Monitor remove_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		removeInPath := fm.config.FriendFIFOPath(friendID, RemoveIn)
		fm.monitorSingleFIFO(ctx, removeInPath, func(data string) { fm.handleFriendRemoveIn(friendID, data) })
	}()

	// Wait for all monitoring goroutines to finish
	wg.Wait()
}

// monitorConferenceFIFOs monitors FIFO files for a conference
func (fm *FIFOManager) monitorConferenceFIFOs(ctx context.Context, conferenceID uint32) {
	conferenceIDStr := fmt.Sprintf("%d", conferenceID)
	var wg sync.WaitGroup

	// Monitor text_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		textInPath := fm.config.ConferenceFIFOPath(conferenceIDStr, ConferenceTextIn)
		fm.monitorSingleFIFO(ctx, textInPath, func(data string) {
			fm.handleConferenceTextIn(conferenceID, data)
		})
	}()

	// Monitor invite_in
	wg.Add(1)
	go func() {
		defer wg.Done()
		inviteInPath := fm.config.ConferenceFIFOPath(conferenceIDStr, ConferenceInviteIn)
		fm.monitorSingleFIFO(ctx, inviteInPath, func(data string) {
			fm.handleConferenceInviteIn(conferenceID, data)
		})
	}()

	// Wait for all monitoring goroutines to finish
	wg.Wait()
}

// readFIFO reads data from a FIFO and calls the handler function
func (fm *FIFOManager) readFIFO(ctx context.Context, path string, handler func(string)) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	file, err := fm.openFIFONonBlocking(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return fm.processLines(ctx, bufio.NewReader(file), handler)
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (fm *FIFOManager) openFIFONonBlocking(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func (fm *FIFOManager) processLines(ctx context.Context, reader *bufio.Reader, handler func(string)) error {
	for {
		if err := checkContext(ctx); err != nil {
			return err
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if line = strings.TrimSpace(line); line != "" {
			handler(line)
		}
	}
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

// handleConferenceIn processes conference creation requests
func (fm *FIFOManager) handleConferenceIn(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// Create a new conference
	conferenceID, err := fm.client.CreateConference()
	if err != nil {
		log.Printf("Failed to create conference: %v", err)
		return
	}

	// Create conference directory and FIFOs
	if err := fm.createConferenceFIFOs(conferenceID); err != nil {
		log.Printf("Failed to create conference FIFOs: %v", err)
		return
	}

	// Start monitoring conference FIFOs
	go fm.monitorConferenceFIFOs(fm.ctx, conferenceID)

	if fm.config.Debug {
		log.Printf("Created conference %d", conferenceID)
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

	// Find friend number by public key using toxcore API
	friendNum, err := fm.client.tox.FriendByPublicKey(publicKey)
	if err != nil {
		log.Printf("Friend not found: %s (%v)", friendID, err)
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

	friendNum, err := fm.resolveFriendNumber(friendID)
	if err != nil {
		return
	}

	fileInfo, fileSize, err := fm.validateFileForSending(filePath)
	if err != nil {
		return
	}

	file, transferID, err := fm.initiateFileSend(friendNum, filePath, fileSize, fileInfo)
	if err != nil {
		return
	}

	fm.trackOutgoingTransfer(friendNum, transferID, file, filePath, fileInfo.Name(), fileSize)
	log.Printf("File transfer initiated: %s (%d bytes) to friend %d, transfer ID: %d", fileInfo.Name(), fileSize, friendNum, transferID)
}

func (fm *FIFOManager) resolveFriendNumber(friendID string) (uint32, error) {
	publicKeyBytes, err := hex.DecodeString(friendID)
	if err != nil {
		log.Printf("Invalid friend ID: %v", err)
		return 0, err
	}

	if len(publicKeyBytes) != 32 {
		err := fmt.Errorf("invalid friend ID length: expected 32 bytes, got %d", len(publicKeyBytes))
		log.Print(err)
		return 0, err
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	friendNum, err := fm.client.tox.FriendByPublicKey(publicKey)
	if err != nil {
		log.Printf("Friend not found: %s (%v)", friendID, err)
		return 0, err
	}

	return friendNum, nil
}

func (fm *FIFOManager) validateFileForSending(filePath string) (os.FileInfo, uint64, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("File not found or inaccessible: %v", err)
		return nil, 0, err
	}

	if fileInfo.IsDir() {
		err := fmt.Errorf("cannot send directory: %s", filePath)
		log.Print(err)
		return nil, 0, err
	}

	rawSize := fileInfo.Size()
	if rawSize < 0 {
		err := fmt.Errorf("invalid file size (negative): %d", rawSize)
		log.Print(err)
		return nil, 0, err
	}
	if fm.client.config.MaxFileSize > 0 && rawSize > fm.client.config.MaxFileSize {
		err := fmt.Errorf("file too large (%d bytes), maximum allowed: %d", rawSize, fm.client.config.MaxFileSize)
		log.Print(err)
		return nil, 0, err
	}
	fileSize := uint64(rawSize)

	return fileInfo, fileSize, nil
}

func (fm *FIFOManager) initiateFileSend(friendNum uint32, filePath string, fileSize uint64, fileInfo os.FileInfo) (*os.File, uint32, error) {
	h := sha256.Sum256([]byte(filePath))
	fileID := h

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file: %v", err)
		return nil, 0, err
	}

	filename := filepath.Base(filePath)
	transferID, err := fm.client.tox.FileSend(friendNum, 0, fileSize, fileID, filename)
	if err != nil {
		log.Printf("Failed to initiate file transfer: %v", err)
		file.Close()
		return nil, 0, err
	}

	return file, transferID, nil
}

func (fm *FIFOManager) trackOutgoingTransfer(friendNum, transferID uint32, file *os.File, filePath, filename string, fileSize uint64) {
	transferKey := fmt.Sprintf("%d:%d", friendNum, transferID)
	fm.client.transfersMu.Lock()
	defer fm.client.transfersMu.Unlock()

	fm.client.outgoingTransfers[transferKey] = &outgoingTransfer{
		File:         file,
		FilePath:     filePath,
		Filename:     filename,
		FileSize:     fileSize,
		Sent:         0,
		LastActivity: time.Now(),
	}
}

// handleFriendRemoveIn processes friend removal requests
func (fm *FIFOManager) handleFriendRemoveIn(friendID, data string) {
	data = strings.TrimSpace(data)
	if data == "" {
		return
	}

	if fm.client.config.Debug {
		log.Printf("Friend removal requested for %s with confirmation: %s", friendID, data)
	}

	if !fm.validateRemovalConfirmation(friendID, data) {
		return
	}

	friendNum, err := fm.getFriendNumber(friendID)
	if err != nil {
		return
	}

	fm.removeFriend(friendID, friendNum)
}

func (fm *FIFOManager) validateRemovalConfirmation(friendID, data string) bool {
	if data != "confirm" && data != friendID {
		log.Printf("Invalid removal confirmation. Expected 'confirm' or friend ID, got: %s", data)
		return false
	}
	return true
}

func (fm *FIFOManager) getFriendNumber(friendID string) (uint32, error) {
	publicKeyBytes, err := hex.DecodeString(friendID)
	if err != nil {
		log.Printf("Invalid friend ID: %v", err)
		return 0, err
	}

	if len(publicKeyBytes) != 32 {
		log.Printf("Invalid friend ID length: expected 32 bytes, got %d", len(publicKeyBytes))
		return 0, fmt.Errorf("invalid length")
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	friendNum, err := fm.client.tox.FriendByPublicKey(publicKey)
	if err != nil {
		log.Printf("Friend not found: %s (%v)", friendID, err)
		return 0, err
	}

	return friendNum, nil
}

func (fm *FIFOManager) removeFriend(friendID string, friendNum uint32) {
	if err := fm.client.tox.DeleteFriend(friendNum); err != nil {
		log.Printf("Failed to delete friend %d: %v", friendNum, err)
		return
	}

	fm.client.friendsMu.Lock()
	delete(fm.client.friends, friendNum)
	fm.client.friendsMu.Unlock()

	friendDir := fm.config.FriendDir(friendID)
	if err := os.RemoveAll(friendDir); err != nil {
		log.Printf("Failed to remove friend directory %s: %v", friendDir, err)
	}

	fm.client.saveToxData()

	log.Printf("Friend %s removed successfully", friendID)
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

// WriteFriendStatusMessage writes status message to a friend's status_message FIFO
func (fm *FIFOManager) WriteFriendStatusMessage(friendID, statusMessage string) error {
	path := fm.config.FriendFIFOPath(friendID, FriendStatusMessage)
	return fm.writeFIFO(path, statusMessage)
}

// WriteFriendTyping writes typing status to a friend's typing FIFO
func (fm *FIFOManager) WriteFriendTyping(friendID, typingStatus string) error {
	path := fm.config.FriendFIFOPath(friendID, Typing)
	return fm.writeFIFO(path, typingStatus)
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
	return name == RequestIn || name == RequestOut || name == Name || name == StatusMessage || name == ConferenceIn
}

// handleConferenceTextIn processes outgoing conference messages
func (fm *FIFOManager) handleConferenceTextIn(conferenceID uint32, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	if err := fm.client.SendConferenceMessage(conferenceID, message); err != nil {
		log.Printf("Failed to send conference message: %v", err)
	}
}

// handleConferenceInviteIn processes conference invite requests
func (fm *FIFOManager) handleConferenceInviteIn(conferenceID uint32, friendID string) {
	friendID = strings.TrimSpace(friendID)
	if friendID == "" {
		return
	}

	// Convert hex friend ID to friend number
	publicKeyBytes, err := hex.DecodeString(friendID)
	if err != nil {
		log.Printf("Invalid friend ID for conference invite: %v", err)
		return
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	// Look up friend number
	friendNum, err := fm.client.tox.FriendByPublicKey(publicKey)
	if err != nil {
		log.Printf("Friend not found for conference invite: %v", err)
		return
	}

	if err := fm.client.InviteToConference(friendNum, conferenceID); err != nil {
		log.Printf("Failed to invite friend to conference: %v", err)
	}
}

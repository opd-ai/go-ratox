// Package client implements the core Tox client functionality for ratox-go
package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/opd-ai/go-ratox/config"
	"github.com/opd-ai/toxcore"
	"github.com/opd-ai/toxcore/async"
	"github.com/opd-ai/toxcore/bootstrap"
)

// Client represents the main Tox client with FIFO interface
type Client struct {
	tox          *toxcore.Tox
	config       *config.Config
	fifoManager  *FIFOManager
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
	mu           sync.RWMutex
	shutdownOnce sync.Once

	// Bootstrap server (optional)
	bootstrapServer *bootstrap.Server

	// Friend management
	friends   map[uint32]*Friend
	friendsMu sync.RWMutex

	// Conference management
	conferences   map[uint32]*Conference
	conferencesMu sync.RWMutex

	// File transfer tracking
	incomingTransfers map[string]*incomingTransfer
	outgoingTransfers map[string]*outgoingTransfer
	transfersMu       sync.RWMutex

	// Shutdown channel
	shutdown chan struct{}
}

// incomingTransfer tracks an active incoming file transfer
type incomingTransfer struct {
	File         *os.File
	FilePath     string
	Filename     string
	FileSize     uint64
	Received     uint64
	LastActivity time.Time
}

// outgoingTransfer tracks an active outgoing file transfer
type outgoingTransfer struct {
	File         *os.File
	FilePath     string
	Filename     string
	FileSize     uint64
	Sent         uint64
	LastActivity time.Time
}

// Friend represents a Tox friend with associated metadata
type Friend struct {
	ID            uint32
	PublicKey     [32]byte
	Name          string
	Status        int // User status (0=none, 1=away, 2=busy)
	StatusMessage string
	Online        bool
	LastSeen      time.Time
}

// Conference represents an active conference/group chat
type Conference struct {
	ID      uint32
	Created time.Time
}

// New creates a new Tox client instance
func New(cfg *config.Config) (*Client, error) {
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

	// Initialize Tox
	if err := client.initTox(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize Tox: %w", err)
	}

	// Initialize FIFO manager
	fifoManager := NewFIFOManager(client)
	client.fifoManager = fifoManager

	// Initialize bootstrap server if configured
	if cfg.BootstrapServer.Enabled {
		if err := client.initBootstrapServer(); err != nil {
			client.tox.Kill()
			cancel()
			return nil, fmt.Errorf("failed to initialize bootstrap server: %w", err)
		}
	}

	// Load existing friends
	if err := client.loadFriends(); err != nil {
		client.tox.Kill()
		cancel()
		return nil, fmt.Errorf("failed to load friends: %w", err)
	}

	// Set up Tox callbacks
	client.setupCallbacks()

	if cfg.Debug {
		log.Printf("Tox client initialized. Tox ID: %s", client.GetToxID())
	}

	return client, nil
}

// initTox initializes the Tox instance
func (c *Client) initTox() error {
	if err := c.config.ValidateTransport(); err != nil {
		return fmt.Errorf("invalid transport configuration: %w", err)
	}

	options := c.configureTransportOptions()
	return c.createToxInstance(options)
}

// configureTransportOptions configures transport options based on config
func (c *Client) configureTransportOptions() *toxcore.Options {
	options := toxcore.NewOptions()

	if c.config.Transport.TorEnabled || c.config.Transport.I2PEnabled {
		options.UDPEnabled = false
		if c.config.Debug {
			if c.config.Transport.TorEnabled && c.config.Transport.I2PEnabled {
				log.Printf("Tor and I2P simultaneously enabled: disabling UDP (DHT packets will route through I2P)")
			} else {
				log.Printf("Anonymizing overlay enabled, disabling UDP")
			}
		}
	} else {
		options.UDPEnabled = true
	}

	if c.config.Transport.TCPPort > 0 {
		options.TCPPort = c.config.Transport.TCPPort
	}

	return options
}

// createToxInstance creates or restores a Tox instance
func (c *Client) createToxInstance(options *toxcore.Options) error {
	saveData, readErr := os.ReadFile(c.config.SaveFile)
	if readErr == nil {
		return c.restoreToxFromSavedata(options, saveData)
	}
	return c.createNewToxInstance(options)
}

// restoreToxFromSavedata restores Tox from existing save data
func (c *Client) restoreToxFromSavedata(options *toxcore.Options, saveData []byte) error {
	if c.config.Debug {
		log.Printf("Loading existing save data from %s", c.config.SaveFile)
	}
	tox, err := toxcore.NewFromSavedata(options, saveData)
	if err != nil {
		return fmt.Errorf("failed to restore Tox from savedata: %w", err)
	}
	c.tox = tox
	return nil
}

// createNewToxInstance creates a new Tox instance
func (c *Client) createNewToxInstance(options *toxcore.Options) error {
	tox, err := toxcore.New(options)
	if err != nil {
		return fmt.Errorf("failed to create Tox instance: %w", err)
	}

	if err := tox.SelfSetName(c.config.Name); err != nil {
		if c.config.Debug {
			log.Printf("Warning: failed to set name: %v", err)
		}
	}

	if err := tox.SelfSetStatusMessage(c.config.StatusMessage); err != nil {
		if c.config.Debug {
			log.Printf("Warning: failed to set status message: %v", err)
		}
	}

	c.tox = tox
	return nil
}

// setupCallbacks sets up all Tox event callbacks
func (c *Client) setupCallbacks() {
	// Friend request callback
	c.tox.OnFriendRequest(func(publicKey [32]byte, message string) {
		c.handleFriendRequest(publicKey, message)
	})

	// Friend message callback
	c.tox.OnFriendMessageDetailed(func(friendID uint32, message string, messageType toxcore.MessageType) {
		c.handleFriendMessage(friendID, message, messageType)
	})

	// Friend name change callback
	c.tox.OnFriendName(func(friendID uint32, name string) {
		c.handleFriendNameChange(friendID, name)
	})

	// Friend status callback
	c.tox.OnFriendStatus(func(friendID uint32, status toxcore.FriendStatus) {
		c.handleFriendStatusChange(friendID, int(status))
	})

	// Friend status message callback
	c.tox.OnFriendStatusMessage(func(friendID uint32, statusMessage string) {
		c.handleFriendStatusMessageChange(friendID, statusMessage)
	})

	// Friend connection status callback
	c.tox.OnFriendConnectionStatus(func(friendID uint32, status toxcore.ConnectionStatus) {
		c.handleFriendConnectionStatusChange(friendID, status)
	})

	// Self connection status callback
	c.tox.OnConnectionStatus(func(status toxcore.ConnectionStatus) {
		c.handleSelfConnectionStatusChange(status)
	})

	// File receive callback
	c.tox.OnFileRecv(func(friendID, fileID, kind uint32, fileSize uint64, filename string) {
		c.handleFileReceive(friendID, fileID, int(kind), fileSize, filename)
	})

	// File receive chunk callback
	c.tox.OnFileRecvChunk(func(friendID, fileID uint32, position uint64, data []byte) {
		c.handleFileReceiveChunk(friendID, fileID, position, data)
	})

	// File chunk request callback
	c.tox.OnFileChunkRequest(func(friendID, fileID uint32, position uint64, length int) {
		c.handleFileChunkRequest(friendID, fileID, position, length)
	})

	// Friend typing callback
	c.tox.OnFriendTyping(func(friendID uint32, isTyping bool) {
		c.handleFriendTyping(friendID, isTyping)
	})

	// Async message callback
	c.tox.OnAsyncMessage(func(senderPK [32]byte, message string, messageType async.MessageType) {
		c.handleAsyncMessage(senderPK, message, messageType)
	})
}

// loadFriends loads existing friends from Tox save data
func (c *Client) loadFriends() error {
	toxFriends := c.tox.GetFriends()

	for friendID, toxFriend := range toxFriends {
		friend := &Friend{
			ID:        friendID,
			PublicKey: toxFriend.PublicKey,
			Name:      toxFriend.Name,
			Status:    0, // Default status
			Online:    false,
			LastSeen:  time.Now(),
		}

		c.friendsMu.Lock()
		c.friends[friendID] = friend
		c.friendsMu.Unlock()

		// Create friend directory and FIFOs
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		if err := c.fifoManager.CreateFriendFIFOs(friendIDStr); err != nil {
			log.Printf("Warning: failed to create FIFOs for friend %s: %v", friendIDStr, err)
		}

		if c.config.Debug {
			log.Printf("Loaded friend: %s (%s)", friend.Name, friendIDStr)
		}
	}

	return nil
} // Run starts the Tox client main loop
func (c *Client) Run() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("client is already running")
	}
	c.running = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	// Start bootstrap server if configured
	if c.bootstrapServer != nil {
		if err := c.bootstrapServer.Start(c.ctx); err != nil {
			return fmt.Errorf("failed to start bootstrap server: %w", err)
		}
		if c.config.Debug {
			bsCfg := c.config.BootstrapServer
			if bsCfg.ClearnetEnabled {
				log.Printf("Bootstrap server clearnet: %s", c.bootstrapServer.GetClearnetAddr())
			}
			if bsCfg.OnionEnabled {
				log.Printf("Bootstrap server onion: %s", c.bootstrapServer.GetOnionAddr())
			}
			if bsCfg.I2PEnabled {
				log.Printf("Bootstrap server i2p: %s", c.bootstrapServer.GetI2PAddr())
			}
			log.Printf("Bootstrap server public key: %s", c.bootstrapServer.GetPublicKeyHex())
		}
	}

	// Start FIFO manager
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.fifoManager.Run(c.ctx)
	}()

	// Bootstrap to DHT
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.bootstrap()
	}()

	// Auto-save periodically
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.autoSave()
	}()

	// Update connection status periodically
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.updateConnectionStatus()
	}()

	// Monitor stalled file transfers
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.monitorStalledTransfers()
	}()

	// Main Tox iteration loop with dynamic interval
	for {
		select {
		case <-c.ctx.Done():
			return nil
		case <-c.shutdown:
			return nil
		default:
			c.tox.Iterate()
			time.Sleep(c.tox.IterationInterval())
		}
	}
}

// initBootstrapServer initialises the bootstrap.Server from config.
func (c *Client) initBootstrapServer() error {
	bsCfg := &bootstrap.Config{
		ClearnetEnabled:   c.config.BootstrapServer.ClearnetEnabled,
		ClearnetPort:      c.config.BootstrapServer.ClearnetPort,
		OnionEnabled:      c.config.BootstrapServer.OnionEnabled,
		I2PEnabled:        c.config.BootstrapServer.I2PEnabled,
		I2PSAMAddr:        c.config.BootstrapServer.I2PSAMAddr,
		IterationInterval: 50 * time.Millisecond,
	}

	srv, err := bootstrap.New(bsCfg)
	if err != nil {
		return fmt.Errorf("bootstrap.New: %w", err)
	}
	c.bootstrapServer = srv
	return nil
}

// bootstrap connects to DHT bootstrap nodes
func (c *Client) bootstrap() {
	for _, node := range c.config.BootstrapNodes {
		if c.config.Debug {
			log.Printf("Bootstrapping to %s:%d", node.Address, node.Port)
		}

		if err := c.tox.Bootstrap(node.Address, node.Port, node.PublicKey); err != nil {
			log.Printf("Warning: failed to bootstrap to %s:%d: %v", node.Address, node.Port, err)
		}
	}
}

// autoSave periodically saves Tox state to disk
func (c *Client) autoSave() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			// Final save on shutdown
			c.saveToxData()
			return
		case <-ticker.C:
			c.saveToxData()
		}
	}
}

// updateConnectionStatus periodically updates the connection status file
func (c *Client) updateConnectionStatus() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.fifoManager.createConnectionStatusFile(); err != nil {
				if c.config.Debug {
					log.Printf("Failed to update connection status: %v", err)
				}
			}
		}
	}
}

// monitorStalledTransfers checks for and cancels stalled file transfers
func (c *Client) monitorStalledTransfers() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	const transferTimeout = 5 * time.Minute

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()

			// Check incoming transfers
			c.transfersMu.Lock()
			for key, transfer := range c.incomingTransfers {
				if now.Sub(transfer.LastActivity) > transferTimeout {
					log.Printf("Incoming transfer stalled: %s (last activity: %v ago)",
						transfer.Filename, now.Sub(transfer.LastActivity))

					// Parse friendID and fileNumber from key
					var friendID, fileNumber uint32
					if _, err := fmt.Sscanf(key, "%d:%d", &friendID, &fileNumber); err == nil {
						// Abort the transfer (must be done without holding the lock)
						transferCopy := *transfer
						keyCopy := key
						c.transfersMu.Unlock()
						c.abortFileReceive(friendID, fileNumber, keyCopy, &transferCopy)
						c.transfersMu.Lock()
					}
				}
			}

			// Check outgoing transfers
			for key, transfer := range c.outgoingTransfers {
				if now.Sub(transfer.LastActivity) > transferTimeout {
					log.Printf("Outgoing transfer stalled: %s (last activity: %v ago)",
						transfer.Filename, now.Sub(transfer.LastActivity))

					// Parse friendID and fileNumber from key
					var friendID, fileNumber uint32
					if _, err := fmt.Sscanf(key, "%d:%d", &friendID, &fileNumber); err == nil {
						// Abort the transfer (must be done without holding the lock)
						transferCopy := *transfer
						keyCopy := key
						c.transfersMu.Unlock()
						c.cancelFileTransfer(friendID, fileNumber)
						c.abortFileSend(friendID, fileNumber, keyCopy, &transferCopy)
						c.transfersMu.Lock()
					}
				}
			}
			c.transfersMu.Unlock()
		}
	}
}

// saveToxData saves Tox state to disk
func (c *Client) saveToxData() {
	saveData := c.tox.GetSavedata()
	if err := os.WriteFile(c.config.SaveFile, saveData, 0o600); err != nil {
		log.Printf("Error saving Tox data: %v", err)
	} else if c.config.Debug {
		log.Printf("Tox data saved to %s", c.config.SaveFile)
	}
}

// Shutdown gracefully shuts down the client. It is safe to call multiple times.
func (c *Client) Shutdown() {
	c.shutdownOnce.Do(func() {
		c.mu.Lock()
		running := c.running
		c.mu.Unlock()

		if !running {
			return
		}

		if c.config.Debug {
			log.Println("Shutting down client...")
		}

		// Signal shutdown
		close(c.shutdown)

		// Cancel context to stop all goroutines
		c.cancel()

		// Wait for all goroutines to finish
		c.wg.Wait()

		// Save final state
		c.saveToxData()

		// Cleanup Tox
		if c.tox != nil {
			c.tox.Kill()
		}

		// Stop bootstrap server if running
		if c.bootstrapServer != nil {
			if err := c.bootstrapServer.Stop(); err != nil && c.config.Debug {
				log.Printf("Warning: bootstrap server stop error: %v", err)
			}
		}

		if c.config.Debug {
			log.Println("Client shutdown complete")
		}
	})
}

// GetToxID returns the client's Tox ID as a hex string
func (c *Client) GetToxID() string {
	return c.tox.SelfGetAddress()
}

// GetFriend returns friend information by ID
func (c *Client) GetFriend(friendID uint32) (*Friend, bool) {
	c.friendsMu.RLock()
	defer c.friendsMu.RUnlock()
	friend, exists := c.friends[friendID]
	return friend, exists
}

// SendMessage sends a text message to a friend
func (c *Client) SendMessage(friendID uint32, message string, messageType toxcore.MessageType) error {
	if len(message) == 0 {
		return fmt.Errorf("message cannot be empty")
	}

	// Check byte length, not character count, for UTF-8 messages
	messageBytes := []byte(message)
	if len(messageBytes) > 1372 {
		return fmt.Errorf("message too long (max 1372 bytes, got %d)", len(messageBytes))
	}

	return c.tox.SendFriendMessage(friendID, message, messageType)
}

// AddFriend adds a friend by Tox ID
func (c *Client) AddFriend(toxID, message string) (uint32, error) {
	friendID, err := c.tox.AddFriend(toxID, message)
	if err != nil {
		return 0, err
	}

	// Create a basic friend entry - we'll get more info via callbacks
	friend := &Friend{
		ID:        friendID,
		PublicKey: [32]byte{}, // Will be filled by callback
		Name:      "Unknown",  // Will be updated by callback
		Status:    0,          // Default status
		Online:    false,
		LastSeen:  time.Now(),
	}

	c.friendsMu.Lock()
	c.friends[friendID] = friend
	c.friendsMu.Unlock()

	// Save state
	c.saveToxData()

	if c.config.Debug {
		log.Printf("Added friend with ID: %d", friendID)
	}

	return friendID, nil
} // AcceptFriendRequest accepts a friend request by public key
func (c *Client) AcceptFriendRequest(publicKey [32]byte) (uint32, error) {
	friendID, err := c.tox.AddFriendByPublicKey(publicKey)
	if err != nil {
		return 0, err
	}

	// Create friend entry
	friend := &Friend{
		ID:        friendID,
		PublicKey: publicKey,
		Name:      "Unknown", // Will be updated by callback
		Status:    0,         // Default status
		Online:    false,
		LastSeen:  time.Now(),
	}

	c.friendsMu.Lock()
	c.friends[friendID] = friend
	c.friendsMu.Unlock()

	// Create friend directory and FIFOs
	friendIDStr := hex.EncodeToString(friend.PublicKey[:])
	if err := c.fifoManager.CreateFriendFIFOs(friendIDStr); err != nil {
		log.Printf("Warning: failed to create FIFOs for friend %s: %v", friendIDStr, err)
	}

	// Save state
	c.saveToxData()

	if c.config.Debug {
		log.Printf("Accepted friend request: %s", friendIDStr)
	}

	return friendID, nil
}

// UpdateSelfName updates the client's display name
func (c *Client) UpdateSelfName(name string) error {
	if err := c.tox.SelfSetName(name); err != nil {
		return err
	}

	c.config.Name = name
	return c.config.Save()
}

// UpdateSelfStatusMessage updates the client's status message
func (c *Client) UpdateSelfStatusMessage(message string) error {
	if err := c.tox.SelfSetStatusMessage(message); err != nil {
		return err
	}

	c.config.StatusMessage = message
	return c.config.Save()
}

// CreateConference creates a new conference and returns its ID
func (c *Client) CreateConference() (uint32, error) {
	conferenceID, err := c.tox.ConferenceNew()
	if err != nil {
		return 0, fmt.Errorf("failed to create conference: %w", err)
	}

	conference := &Conference{
		ID:      conferenceID,
		Created: time.Now(),
	}

	c.conferencesMu.Lock()
	c.conferences[conferenceID] = conference
	c.conferencesMu.Unlock()

	c.saveToxData()

	return conferenceID, nil
}

// SendConferenceMessage sends a message to a conference
func (c *Client) SendConferenceMessage(conferenceID uint32, message string) error {
	if len(message) == 0 {
		return fmt.Errorf("message cannot be empty")
	}

	return c.tox.ConferenceSendMessage(conferenceID, message, toxcore.MessageTypeNormal)
}

// InviteToConference invites a friend to a conference
func (c *Client) InviteToConference(friendID, conferenceID uint32) error {
	return c.tox.ConferenceInvite(friendID, conferenceID)
}

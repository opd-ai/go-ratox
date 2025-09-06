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
)

// Client represents the main Tox client with FIFO interface
type Client struct {
	tox         *toxcore.Tox
	config      *config.Config
	fifoManager *FIFOManager
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	running     bool
	mu          sync.RWMutex

	// Friend management
	friends   map[uint32]*Friend
	friendsMu sync.RWMutex

	// Shutdown channel
	shutdown chan struct{}
}

// Friend represents a Tox friend with associated metadata
type Friend struct {
	ID        uint32
	PublicKey [32]byte
	Name      string
	Status    int // User status (0=none, 1=away, 2=busy)
	Online    bool
	LastSeen  time.Time
}

// New creates a new Tox client instance
func New(cfg *config.Config) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		config:   cfg,
		ctx:      ctx,
		cancel:   cancel,
		friends:  make(map[uint32]*Friend),
		shutdown: make(chan struct{}),
	}

	// Initialize Tox
	if err := client.initTox(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize Tox: %w", err)
	}

	// Initialize FIFO manager
	fifoManager := NewFIFOManager(client)
	client.fifoManager = fifoManager

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
	options := toxcore.NewOptions()
	options.UDPEnabled = true

	// Load existing save data if available
	if saveData, err := os.ReadFile(c.config.SaveFile); err == nil {
		options.SavedataType = toxcore.SaveDataTypeToxSave
		options.SavedataData = saveData
		options.SavedataLength = uint32(len(saveData))

		if c.config.Debug {
			log.Printf("Loading existing save data from %s", c.config.SaveFile)
		}
	}

	tox, err := toxcore.New(options)
	if err != nil {
		return fmt.Errorf("failed to create Tox instance: %w", err)
	}

	c.tox = tox

	// Set self info
	if err := c.tox.SelfSetName(c.config.Name); err != nil {
		if c.config.Debug {
			log.Printf("Warning: failed to set name: %v", err)
		}
	}

	if err := c.tox.SelfSetStatusMessage(c.config.StatusMessage); err != nil {
		if c.config.Debug {
			log.Printf("Warning: failed to set status message: %v", err)
		}
	}

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

	// TODO: Fix callback signatures for these when we understand the API better
	// Friend status callback
	// c.tox.OnFriendStatus(func(friendID uint32, status int) {
	//     c.handleFriendStatusChange(friendID, status)
	// })

	// File receive callback
	c.tox.OnFileRecv(func(friendID uint32, fileID uint32, kind uint32, fileSize uint64, filename string) {
		c.handleFileReceive(friendID, fileID, int(kind), fileSize, filename)
	})

	// File receive chunk callback
	c.tox.OnFileRecvChunk(func(friendID uint32, fileID uint32, position uint64, data []byte) {
		c.handleFileReceiveChunk(friendID, fileID, position, data)
	})

	// File chunk request callback
	c.tox.OnFileChunkRequest(func(friendID uint32, fileID uint32, position uint64, length int) {
		c.handleFileChunkRequest(friendID, fileID, position, length)
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

	// Main Tox iteration loop
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return nil
		case <-c.shutdown:
			return nil
		case <-ticker.C:
			c.tox.Iterate()
		}
	}
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

// saveToxData saves Tox state to disk
func (c *Client) saveToxData() {
	saveData := c.tox.GetSavedata()
	if err := os.WriteFile(c.config.SaveFile, saveData, 0600); err != nil {
		log.Printf("Error saving Tox data: %v", err)
	} else if c.config.Debug {
		log.Printf("Tox data saved to %s", c.config.SaveFile)
	}
}

// Shutdown gracefully shuts down the client
func (c *Client) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
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

	if c.config.Debug {
		log.Println("Client shutdown complete")
	}
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

	if len(message) > 1372 {
		return fmt.Errorf("message too long (max 1372 bytes)")
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

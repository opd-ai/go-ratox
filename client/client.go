// Package client implements the core Tox client functionality for ratox-go
package client

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/opd-ai/go-ratox/config"
	"github.com/opd-ai/toxcore"
)

// Client represents the main Tox client
type Client struct {
	config *config.Config
	tox    *toxcore.Tox
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// FIFO managers
	fifoManager *FIFOManager

	// Friend management
	friends     map[uint32]*Friend
	friendsByID map[string]uint32

	// Callbacks and handlers
	handlers *Handlers

	// Shutdown flag
	shutdown bool
}

// Friend represents a Tox friend
type Friend struct {
	Number     uint32
	PublicKey  string
	Name       string
	StatusMsg  string
	Status     toxcore.FriendStatus
	ConnStatus toxcore.ConnectionStatus
	LastSeen   time.Time
}

// New creates a new Tox client instance
func New(cfg *config.Config) (*Client, error) {
	// Create Tox options
	opts := toxcore.NewOptions()
	opts.IPv6Enabled = true
	opts.UDPEnabled = true
	opts.LocalDiscoveryEnabled = true
	opts.ProxyType = toxcore.ProxyTypeNone

	// Load existing save data if available
	var tox *toxcore.Tox
	var err error

	if data, loadErr := os.ReadFile(cfg.SaveFile); loadErr == nil {
		tox, err = toxcore.NewFromSavedata(opts, data)
		if cfg.Debug {
			log.Printf("Loaded save data from %s (%d bytes)", cfg.SaveFile, len(data))
		}
	} else {
		tox, err = toxcore.New(opts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Tox instance: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		config:      cfg,
		tox:         tox,
		ctx:         ctx,
		cancel:      cancel,
		friends:     make(map[uint32]*Friend),
		friendsByID: make(map[string]uint32),
	}

	// Initialize FIFO manager
	client.fifoManager = NewFIFOManager(client)

	// Initialize handlers
	client.handlers = NewHandlers(client)

	// Set up Tox callbacks
	client.setupCallbacks()

	// Set initial name and status message
	if err := client.setInitialProfile(); err != nil {
		tox.Kill()
		return nil, fmt.Errorf("failed to set initial profile: %w", err)
	}

	return client, nil
}

// Run starts the client and blocks until shutdown
func (c *Client) Run() error {
	if c.config.Debug {
		log.Printf("Starting ratox-go client in %s", c.config.ConfigDir)
		log.Printf("Tox ID: %s", c.GetToxID())
	}

	// Bootstrap to DHT
	if err := c.bootstrap(); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	// Start FIFO manager
	if err := c.fifoManager.Start(); err != nil {
		return fmt.Errorf("failed to start FIFO manager: %w", err)
	}

	// Load existing friends
	if err := c.loadFriends(); err != nil {
		log.Printf("Warning: failed to load friends: %v", err)
	}

	// Start main event loop
	c.wg.Add(1)
	go c.eventLoop()

	// Start save loop
	c.wg.Add(1)
	go c.saveLoop()

	// Wait for shutdown
	c.wg.Wait()

	return nil
}

// Shutdown gracefully shuts down the client
func (c *Client) Shutdown() {
	c.mu.Lock()
	if c.shutdown {
		c.mu.Unlock()
		return
	}
	c.shutdown = true
	c.mu.Unlock()

	if c.config.Debug {
		log.Println("Shutting down client...")
	}

	// Cancel context to stop all goroutines
	c.cancel()

	// Stop FIFO manager
	if c.fifoManager != nil {
		c.fifoManager.Stop()
	}

	// Save Tox data
	c.saveToxData()

	// Kill Tox instance
	if c.tox != nil {
		c.tox.Kill()
	}

	// Wait for all goroutines to finish
	c.wg.Wait()

	if c.config.Debug {
		log.Println("Client shutdown complete")
	}
}

// GetToxID returns the client's Tox ID
func (c *Client) GetToxID() string {
	if c.tox == nil {
		return ""
	}
	address := c.tox.SelfGetAddress()
	return fmt.Sprintf("%X", address)
}

// GetConfig returns the client configuration
func (c *Client) GetConfig() *config.Config {
	return c.config
}

// GetTox returns the underlying Tox instance
func (c *Client) GetTox() *toxcore.Tox {
	return c.tox
}

// eventLoop runs the main Tox event loop
func (c *Client) eventLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Duration(c.tox.IterationInterval()) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.tox.Iterate()

			// Update ticker interval if it changed
			newInterval := time.Duration(c.tox.IterationInterval()) * time.Millisecond
			if newInterval != ticker.C {
				ticker.Stop()
				ticker = time.NewTicker(newInterval)
			}
		}
	}
}

// saveLoop periodically saves Tox data
func (c *Client) saveLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.saveToxData()
		}
	}
}

// saveToxData saves the Tox save data to disk
func (c *Client) saveToxData() {
	data := c.tox.GetSavedata()
	if err := os.WriteFile(c.config.SaveFile, data, 0600); err != nil {
		log.Printf("Error saving Tox data: %v", err)
	} else if c.config.Debug {
		log.Printf("Saved Tox data (%d bytes)", len(data))
	}
}

// bootstrap connects to the DHT network
func (c *Client) bootstrap() error {
	for _, node := range c.config.BootstrapNodes {
		if err := c.tox.Bootstrap(node.Address, node.Port, node.PublicKey); err != nil {
			log.Printf("Failed to bootstrap to %s:%d: %v", node.Address, node.Port, err)
			continue
		}

		if err := c.tox.AddTCPRelay(node.Address, node.Port, node.PublicKey); err != nil {
			log.Printf("Failed to add TCP relay %s:%d: %v", node.Address, node.Port, err)
		}

		if c.config.Debug {
			log.Printf("Bootstrapped to %s:%d", node.Address, node.Port)
		}
	}

	return nil
}

// setInitialProfile sets the initial name and status message
func (c *Client) setInitialProfile() error {
	if err := c.tox.SelfSetName(c.config.Name); err != nil {
		return fmt.Errorf("failed to set name: %w", err)
	}

	if err := c.tox.SelfSetStatusMessage(c.config.StatusMessage); err != nil {
		return fmt.Errorf("failed to set status message: %w", err)
	}

	return nil
}

// loadFriends loads existing friends from Tox
func (c *Client) loadFriends() error {
	friendNumbers := c.tox.SelfGetFriendList()

	for _, friendNum := range friendNumbers {
		publicKey, err := c.tox.FriendGetPublicKey(friendNum)
		if err != nil {
			log.Printf("Failed to get public key for friend %d: %v", friendNum, err)
			continue
		}

		friend := &Friend{
			Number:    friendNum,
			PublicKey: fmt.Sprintf("%X", publicKey),
			LastSeen:  time.Now(),
		}

		// Get friend name
		if name, err := c.tox.FriendGetName(friendNum); err == nil {
			friend.Name = name
		}

		// Get friend status message
		if statusMsg, err := c.tox.FriendGetStatusMessage(friendNum); err == nil {
			friend.StatusMsg = statusMsg
		}

		// Get friend status
		if status, err := c.tox.FriendGetStatus(friendNum); err == nil {
			friend.Status = status
		}

		// Get friend connection status
		if connStatus, err := c.tox.FriendGetConnectionStatus(friendNum); err == nil {
			friend.ConnStatus = connStatus
		}

		c.friends[friendNum] = friend
		c.friendsByID[friend.PublicKey] = friendNum

		// Create friend directory and FIFOs
		if err := c.fifoManager.CreateFriendFIFOs(friend.PublicKey); err != nil {
			log.Printf("Failed to create FIFOs for friend %s: %v", friend.PublicKey, err)
		}

		if c.config.Debug {
			log.Printf("Loaded friend %d: %s (%s)", friendNum, friend.Name, friend.PublicKey)
		}
	}

	return nil
}

// setupCallbacks sets up all Tox event callbacks
func (c *Client) setupCallbacks() {
	c.tox.CallbackSelfConnectionStatus(c.handlers.OnSelfConnectionStatus)
	c.tox.CallbackFriendRequest(c.handlers.OnFriendRequest)
	c.tox.CallbackFriendMessage(c.handlers.OnFriendMessage)
	c.tox.CallbackFriendName(c.handlers.OnFriendName)
	c.tox.CallbackFriendStatusMessage(c.handlers.OnFriendStatusMessage)
	c.tox.CallbackFriendStatus(c.handlers.OnFriendStatus)
	c.tox.CallbackFriendConnectionStatus(c.handlers.OnFriendConnectionStatus)
	c.tox.CallbackFileRecvControl(c.handlers.OnFileRecvControl)
	c.tox.CallbackFileChunkRequest(c.handlers.OnFileChunkRequest)
	c.tox.CallbackFileRecv(c.handlers.OnFileRecv)
	c.tox.CallbackFileRecvChunk(c.handlers.OnFileRecvChunk)
}

// GetFriend returns a friend by their number
func (c *Client) GetFriend(friendNum uint32) *Friend {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.friends[friendNum]
}

// GetFriendByID returns a friend by their public key
func (c *Client) GetFriendByID(publicKey string) *Friend {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if friendNum, exists := c.friendsByID[publicKey]; exists {
		return c.friends[friendNum]
	}
	return nil
}

// AddFriend adds a new friend
func (c *Client) AddFriend(friend *Friend) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.friends[friend.Number] = friend
	c.friendsByID[friend.PublicKey] = friend.Number
}

// RemoveFriend removes a friend
func (c *Client) RemoveFriend(friendNum uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if friend, exists := c.friends[friendNum]; exists {
		delete(c.friendsByID, friend.PublicKey)
		delete(c.friends, friendNum)
	}
}

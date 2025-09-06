// Package client implements Tox event handlers for ratox-go
package client

import (
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/opd-ai/toxcore"
)

// handleFriendRequest processes incoming friend requests
func (c *Client) handleFriendRequest(publicKey [32]byte, message string) {
	if c.config.Debug {
		friendIDStr := hex.EncodeToString(publicKey[:])
		log.Printf("Friend request from %s: %s", friendIDStr, message)
	}

	// Write request to request_out FIFO
	friendIDStr := hex.EncodeToString(publicKey[:])

	if err := c.fifoManager.WriteRequestOut(friendIDStr, message); err != nil {
		log.Printf("Failed to write friend request to FIFO: %v", err)
	}
}

// handleFriendMessage processes incoming messages from friends
func (c *Client) handleFriendMessage(friendID uint32, message string, messageType toxcore.MessageType) {
	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if !exists {
		log.Printf("Received message from unknown friend %d", friendID)
		return
	}

	// Update last seen
	friend.LastSeen = time.Now()

	// Format message with timestamp and type
	timestamp := time.Now().Format("15:04:05")
	var formattedMessage string

	switch messageType {
	case toxcore.MessageTypeAction:
		formattedMessage = fmt.Sprintf("[%s] * %s %s", timestamp, friend.Name, message)
	default: // MessageTypeNormal
		formattedMessage = fmt.Sprintf("[%s] <%s> %s", timestamp, friend.Name, message)
	}

	// Write to friend's text_out FIFO
	friendIDStr := hex.EncodeToString(friend.PublicKey[:])
	if err := c.fifoManager.WriteFriendTextOut(friendIDStr, formattedMessage); err != nil {
		log.Printf("Failed to write message to text_out FIFO: %v", err)
	}

	if c.config.Debug {
		log.Printf("Message from %s (%d): %s", friend.Name, friendID, message)
	}
}

// handleFriendNameChange processes friend name changes
func (c *Client) handleFriendNameChange(friendID uint32, name string) {
	c.friendsMu.Lock()
	friend, exists := c.friends[friendID]
	if exists {
		friend.Name = name
		// Update public key if not set
		if friend.PublicKey == ([32]byte{}) {
			// We'll get this from the Tox friends map
			toxFriends := c.tox.GetFriends()
			if toxFriend, ok := toxFriends[friendID]; ok {
				friend.PublicKey = toxFriend.PublicKey
			}
		}
	}
	c.friendsMu.Unlock()

	// Create FIFOs outside the lock to prevent deadlock
	if exists && friend.PublicKey != ([32]byte{}) {
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		if err := c.fifoManager.CreateFriendFIFOs(friendIDStr); err != nil {
			log.Printf("Warning: failed to create FIFOs for friend %s: %v", friendIDStr, err)
		}
	}

	if c.config.Debug && exists {
		log.Printf("Friend %d changed name to: %s", friendID, name)
	}
}

// handleFriendStatusChange processes friend status changes
func (c *Client) handleFriendStatusChange(friendID uint32, status int) {
	c.friendsMu.Lock()
	friend, exists := c.friends[friendID]
	if exists {
		friend.Status = status
	}
	c.friendsMu.Unlock()

	if exists {
		// Write status to friend's status FIFO
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		statusStr := "offline"
		switch status {
		case 0:
			statusStr = "online"
		case 1:
			statusStr = "away"
		case 2:
			statusStr = "busy"
		}

		if err := c.fifoManager.WriteFriendStatus(friendIDStr, statusStr); err != nil {
			log.Printf("Failed to write friend status to FIFO: %v", err)
		}

		if c.config.Debug {
			log.Printf("Friend %s (%d) status changed to: %s", friend.Name, friendID, statusStr)
		}
	}
}

// handleFileReceive processes incoming file transfer requests
func (c *Client) handleFileReceive(friendID uint32, fileNumber uint32, kind int, fileSize uint64, filename string) {
	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if !exists {
		log.Printf("File receive from unknown friend %d", friendID)
		return
	}

	if c.config.Debug {
		log.Printf("File receive from %s: %s (%d bytes)", friend.Name, filename, fileSize)
	}

	// Check file size limits
	if c.config.MaxFileSize > 0 && int64(fileSize) > c.config.MaxFileSize {
		log.Printf("File too large (%d bytes), rejecting", fileSize)
		// TODO: Implement file control rejection
		return
	}

	// Write file receive notification to file_out FIFO
	friendIDStr := hex.EncodeToString(friend.PublicKey[:])
	fileInfo := fmt.Sprintf("%s %d", filename, fileSize)

	if err := c.fifoManager.WriteFriendFileOut(friendIDStr, fileInfo); err != nil {
		log.Printf("Failed to write file receive notification: %v", err)
	}

	// Auto-accept files if configured
	if c.config.AutoAcceptFiles {
		// TODO: Implement file control accept
		log.Printf("Auto-accepted file transfer: %s", filename)
	}
}

// handleFileReceiveChunk processes incoming file data chunks
func (c *Client) handleFileReceiveChunk(friendID uint32, fileNumber uint32, position uint64, data []byte) {
	if c.config.Debug {
		c.friendsMu.RLock()
		friend, exists := c.friends[friendID]
		c.friendsMu.RUnlock()

		if exists {
			log.Printf("File chunk from %s: file %d, position %d, size %d", friend.Name, fileNumber, position, len(data))
		}
	}

	// TODO: Implement file chunk writing to disk
	// This would involve maintaining file transfer state and writing chunks to files
}

// handleFileChunkRequest processes outgoing file chunk requests
func (c *Client) handleFileChunkRequest(friendID uint32, fileNumber uint32, position uint64, length int) {
	if c.config.Debug {
		c.friendsMu.RLock()
		friend, exists := c.friends[friendID]
		c.friendsMu.RUnlock()

		if exists {
			log.Printf("File chunk request from %s: file %d, position %d, length %d", friend.Name, fileNumber, position, length)
		}
	}

	// TODO: Implement file chunk reading and sending
	// This would involve reading the requested chunk from disk and sending it
}

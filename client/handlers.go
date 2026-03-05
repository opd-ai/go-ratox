// Package client implements Tox event handlers for ratox-go
package client

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
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
func (c *Client) handleFileReceive(friendID, fileNumber uint32, kind int, fileSize uint64, filename string) {
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
		c.rejectFileTransfer(friendID, fileNumber, fileSize)
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
		c.acceptFileTransfer(friendID, fileNumber, friendIDStr, filename, fileSize)
	}
}

func (c *Client) rejectFileTransfer(friendID, fileNumber uint32, fileSize uint64) {
	log.Printf("File too large (%d bytes), rejecting", fileSize)
	if err := c.tox.FileControl(friendID, fileNumber, toxcore.FileControlCancel); err != nil {
		log.Printf("Failed to reject file transfer: %v", err)
	}
}

func (c *Client) acceptFileTransfer(friendID, fileNumber uint32, friendIDStr, filename string, fileSize uint64) {
	friendDir := c.config.FriendDir(friendIDStr)
	destPath := fmt.Sprintf("%s/%s", friendDir, filename)

	file, err := os.Create(destPath)
	if err != nil {
		log.Printf("Failed to create destination file: %v", err)
		c.cancelFileTransfer(friendID, fileNumber)
		return
	}

	transferKey := fmt.Sprintf("%d:%d", friendID, fileNumber)
	c.transfersMu.Lock()
	c.incomingTransfers[transferKey] = &incomingTransfer{
		File:     file,
		Filename: filename,
		FileSize: fileSize,
		Received: 0,
	}
	c.transfersMu.Unlock()

	if err := c.tox.FileControl(friendID, fileNumber, toxcore.FileControlResume); err != nil {
		log.Printf("Failed to accept file transfer: %v", err)
		file.Close()
		c.transfersMu.Lock()
		delete(c.incomingTransfers, transferKey)
		c.transfersMu.Unlock()
	} else {
		log.Printf("Auto-accepted file transfer: %s", filename)
	}
}

func (c *Client) cancelFileTransfer(friendID, fileNumber uint32) {
	if err := c.tox.FileControl(friendID, fileNumber, toxcore.FileControlCancel); err != nil {
		log.Printf("Failed to cancel file transfer: %v", err)
	}
}

// handleFileReceiveChunk processes incoming file data chunks
func (c *Client) handleFileReceiveChunk(friendID, fileNumber uint32, position uint64, data []byte) {
	transferKey := fmt.Sprintf("%d:%d", friendID, fileNumber)

	c.transfersMu.Lock()
	transfer, exists := c.incomingTransfers[transferKey]
	c.transfersMu.Unlock()

	if !exists {
		if c.config.Debug {
			log.Printf("Received chunk for unknown transfer: %s", transferKey)
		}
		return
	}

	if len(data) == 0 {
		c.completeFileReceive(friendID, transferKey, transfer)
		return
	}

	if err := c.writeFileChunk(transfer, position, data); err != nil {
		c.abortFileReceive(friendID, fileNumber, transferKey, transfer)
		return
	}

	if c.config.Debug {
		log.Printf("Received file chunk: %d bytes at position %d (%d/%d total)",
			len(data), position, transfer.Received, transfer.FileSize)
	}
}

func (c *Client) completeFileReceive(friendID uint32, transferKey string, transfer *incomingTransfer) {
	transfer.File.Close()

	c.transfersMu.Lock()
	delete(c.incomingTransfers, transferKey)
	c.transfersMu.Unlock()

	log.Printf("File transfer completed: %s (%d bytes)", transfer.Filename, transfer.Received)

	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if exists {
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		completionMsg := fmt.Sprintf("COMPLETE %s %d", transfer.Filename, transfer.Received)
		if err := c.fifoManager.WriteFriendFileOut(friendIDStr, completionMsg); err != nil {
			log.Printf("Failed to write file completion notification: %v", err)
		}
	}
}

func (c *Client) writeFileChunk(transfer *incomingTransfer, position uint64, data []byte) error {
	if _, err := transfer.File.WriteAt(data, int64(position)); err != nil {
		log.Printf("Failed to write file chunk: %v", err)
		return err
	}
	transfer.Received += uint64(len(data))
	return nil
}

func (c *Client) abortFileReceive(friendID, fileNumber uint32, transferKey string, transfer *incomingTransfer) {
	transfer.File.Close()
	c.transfersMu.Lock()
	delete(c.incomingTransfers, transferKey)
	c.transfersMu.Unlock()
	c.cancelFileTransfer(friendID, fileNumber)
}

func (c *Client) completeFileSend(friendID uint32, transferKey string, transfer *outgoingTransfer) {
	transfer.File.Close()

	c.transfersMu.Lock()
	delete(c.outgoingTransfers, transferKey)
	c.transfersMu.Unlock()

	log.Printf("File send completed: %s (%d bytes)", transfer.Filename, transfer.Sent)

	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if exists {
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		completionMsg := fmt.Sprintf("SENT %s %d", transfer.Filename, transfer.Sent)
		if err := c.fifoManager.WriteFriendFileOut(friendIDStr, completionMsg); err != nil {
			log.Printf("Failed to write file send completion notification: %v", err)
		}
	}
}

// handleFileChunkRequest processes outgoing file chunk requests
func (c *Client) handleFileChunkRequest(friendID, fileNumber uint32, position uint64, length int) {
	transferKey := fmt.Sprintf("%d:%d", friendID, fileNumber)

	c.transfersMu.RLock()
	transfer, exists := c.outgoingTransfers[transferKey]
	c.transfersMu.RUnlock()

	if !exists {
		log.Printf("No outgoing transfer found for key %s", transferKey)
		return
	}

	// If length is 0, the transfer is being paused
	if length == 0 {
		if c.config.Debug {
			log.Printf("File transfer paused for key %s", transferKey)
		}
		return
	}

	// Read chunk from file
	chunk := make([]byte, length)
	n, err := transfer.File.ReadAt(chunk, int64(position))
	if err != nil && err != io.EOF {
		log.Printf("Failed to read file chunk: %v", err)
		c.cancelFileTransfer(friendID, fileNumber)
		c.completeFileSend(friendID, transferKey, transfer)
		return
	}

	// Send the chunk (or empty chunk if EOF)
	var dataToSend []byte
	if n > 0 {
		dataToSend = chunk[:n]
		transfer.Sent += uint64(n)
	} else {
		dataToSend = nil // Empty chunk signals EOF
	}

	if err := c.tox.FileSendChunk(friendID, fileNumber, position, dataToSend); err != nil {
		log.Printf("Failed to send file chunk: %v", err)
		c.cancelFileTransfer(friendID, fileNumber)
		c.completeFileSend(friendID, transferKey, transfer)
		return
	}

	// If we sent an empty chunk (EOF), complete the transfer
	if dataToSend == nil {
		c.completeFileSend(friendID, transferKey, transfer)
	} else if c.config.Debug {
		log.Printf("Sent file chunk: %d bytes at position %d (%d/%d total)",
			n, position, transfer.Sent, transfer.FileSize)
	}
}

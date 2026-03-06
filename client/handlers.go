// Package client implements Tox event handlers for ratox-go
package client

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/opd-ai/toxcore"
	"github.com/opd-ai/toxcore/async"
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

// handleFriendConnectionStatusChange processes friend connection status changes
func (c *Client) handleFriendConnectionStatusChange(friendID uint32, status toxcore.ConnectionStatus) {
	c.friendsMu.Lock()
	friend, exists := c.friends[friendID]
	if exists {
		friend.Online = status != toxcore.ConnectionNone
	}
	c.friendsMu.Unlock()

	if exists {
		// Write connection status to friend's status FIFO
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		var statusStr string
		switch status {
		case toxcore.ConnectionNone:
			statusStr = "offline"
		case toxcore.ConnectionTCP:
			statusStr = "online (TCP)"
		case toxcore.ConnectionUDP:
			statusStr = "online (UDP)"
		default:
			statusStr = "unknown"
		}

		if err := c.fifoManager.WriteFriendStatus(friendIDStr, statusStr); err != nil {
			log.Printf("Failed to write connection status to FIFO: %v", err)
		}

		if c.config.Debug {
			log.Printf("Friend %s (%d) connection status changed to: %s", friend.Name, friendID, statusStr)
		}
	}
}

// handleFriendStatusMessageChange processes friend status message changes
func (c *Client) handleFriendStatusMessageChange(friendID uint32, statusMessage string) {
	c.friendsMu.Lock()
	friend, exists := c.friends[friendID]
	if exists {
		friend.StatusMessage = statusMessage
	}
	c.friendsMu.Unlock()

	if exists {
		// Write status message to friend's status_message FIFO
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		if err := c.fifoManager.WriteFriendStatusMessage(friendIDStr, statusMessage); err != nil {
			log.Printf("Failed to write friend status message to FIFO: %v", err)
		}

		if c.config.Debug {
			log.Printf("Friend %s (%d) status message changed to: %s", friend.Name, friendID, statusMessage)
		}
	}
}

// handleFriendTyping processes friend typing notifications
func (c *Client) handleFriendTyping(friendID uint32, isTyping bool) {
	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if !exists {
		return
	}

	friendIDStr := hex.EncodeToString(friend.PublicKey[:])
	typingStr := "0"
	if isTyping {
		typingStr = "1"
	}

	if err := c.fifoManager.WriteFriendTyping(friendIDStr, typingStr); err != nil {
		log.Printf("Failed to write typing status to FIFO: %v", err)
	}

	if c.config.Debug {
		log.Printf("Friend %s (%d) typing: %v", friend.Name, friendID, isTyping)
	}
}

// handleSelfConnectionStatusChange processes self connection status changes
func (c *Client) handleSelfConnectionStatusChange(status toxcore.ConnectionStatus) {
	// Update connection status file immediately
	if err := c.fifoManager.createConnectionStatusFile(); err != nil {
		if c.config.Debug {
			log.Printf("Failed to update connection status file: %v", err)
		}
	}

	if c.config.Debug {
		var statusStr string
		switch status {
		case toxcore.ConnectionNone:
			statusStr = "offline"
		case toxcore.ConnectionTCP:
			statusStr = "tcp"
		case toxcore.ConnectionUDP:
			statusStr = "udp"
		default:
			statusStr = "unknown"
		}
		log.Printf("Self connection status changed to: %s", statusStr)
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
	if c.config.MaxFileSize > 0 && fileSize > uint64(c.config.MaxFileSize) { //nolint:gosec // MaxFileSize>0 ensures safe uint64 conversion
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
		File:         file,
		FilePath:     destPath,
		Filename:     filename,
		FileSize:     fileSize,
		Received:     0,
		LastActivity: time.Now(),
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

// notifyFileTransferComplete sends a completion notification to the friend's file_out FIFO.
// msgPrefix is the prefix for the completion message (e.g. "COMPLETE" or "SENT").
func (c *Client) notifyFileTransferComplete(friendID uint32, filename string, size uint64, msgPrefix string) {
	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if exists {
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		completionMsg := fmt.Sprintf("%s %s %d", msgPrefix, filename, size)
		if err := c.fifoManager.WriteFriendFileOut(friendIDStr, completionMsg); err != nil {
			log.Printf("Failed to write file transfer notification: %v", err)
		}
	}
}

func (c *Client) completeFileReceive(friendID uint32, transferKey string, transfer *incomingTransfer) {
	transfer.File.Close()

	c.transfersMu.Lock()
	delete(c.incomingTransfers, transferKey)
	c.transfersMu.Unlock()

	log.Printf("File transfer completed: %s (%d bytes)", transfer.Filename, transfer.Received)
	c.notifyFileTransferComplete(friendID, transfer.Filename, transfer.Received, "COMPLETE")
}

func (c *Client) writeFileChunk(transfer *incomingTransfer, position uint64, data []byte) error {
	if position > math.MaxInt64 {
		return fmt.Errorf("file position %d exceeds maximum supported offset (max: %d)", position, uint64(math.MaxInt64))
	}
	if _, err := transfer.File.WriteAt(data, int64(position)); err != nil {
		if isOutOfDiskSpaceError(err) {
			log.Printf("Disk full: failed to write file chunk: %v", err)
		} else {
			log.Printf("Failed to write file chunk: %v", err)
		}
		return err
	}
	transfer.Received += uint64(len(data))
	transfer.LastActivity = time.Now()
	return nil
}

// isOutOfDiskSpaceError checks if an error indicates out of disk space
func isOutOfDiskSpaceError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no space left on device") ||
		strings.Contains(errStr, "disk full") ||
		strings.Contains(errStr, "quota exceeded")
}

func (c *Client) abortFileReceive(friendID, fileNumber uint32, transferKey string, transfer *incomingTransfer) {
	filePath := transfer.FilePath
	transfer.File.Close()
	c.transfersMu.Lock()
	delete(c.incomingTransfers, transferKey)
	c.transfersMu.Unlock()
	c.cancelFileTransfer(friendID, fileNumber)

	// Clean up partial file
	if filePath != "" {
		if err := os.Remove(filePath); err != nil {
			log.Printf("Warning: failed to remove partial file %s: %v", filePath, err)
		} else {
			log.Printf("Removed partial file: %s", filePath)
		}
	}
}

func (c *Client) completeFileSend(friendID uint32, transferKey string, transfer *outgoingTransfer) {
	transfer.File.Close()

	c.transfersMu.Lock()
	delete(c.outgoingTransfers, transferKey)
	c.transfersMu.Unlock()

	log.Printf("File send completed: %s (%d bytes)", transfer.Filename, transfer.Sent)
	c.notifyFileTransferComplete(friendID, transfer.Filename, transfer.Sent, "SENT")
}

func (c *Client) abortFileSend(friendID, fileNumber uint32, transferKey string, transfer *outgoingTransfer) {
	transfer.File.Close()
	c.transfersMu.Lock()
	delete(c.outgoingTransfers, transferKey)
	c.transfersMu.Unlock()

	log.Printf("File send aborted: %s (sent %d/%d bytes)", transfer.Filename, transfer.Sent, transfer.FileSize)

	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if exists {
		friendIDStr := hex.EncodeToString(friend.PublicKey[:])
		abortMsg := fmt.Sprintf("ABORTED %s %d %d", transfer.Filename, transfer.Sent, transfer.FileSize)
		if err := c.fifoManager.WriteFriendFileOut(friendIDStr, abortMsg); err != nil {
			log.Printf("Failed to write file send abort notification: %v", err)
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
	if position > math.MaxInt64 {
		log.Printf("File position %d exceeds maximum supported offset (max: %d)", position, uint64(math.MaxInt64))
		c.cancelFileTransfer(friendID, fileNumber)
		c.abortFileSend(friendID, fileNumber, transferKey, transfer)
		return
	}
	n, err := transfer.File.ReadAt(chunk, int64(position))
	if err != nil && err != io.EOF {
		if os.IsNotExist(err) {
			log.Printf("File no longer exists: %s (%v)", transfer.FilePath, err)
		} else {
			log.Printf("Failed to read file chunk: %v", err)
		}
		c.cancelFileTransfer(friendID, fileNumber)
		c.abortFileSend(friendID, fileNumber, transferKey, transfer)
		return
	}

	// Send the chunk (or empty chunk if EOF)
	var dataToSend []byte
	if n > 0 {
		dataToSend = chunk[:n]
		transfer.Sent += uint64(n)
		transfer.LastActivity = time.Now()
	} else {
		dataToSend = nil // Empty chunk signals EOF
	}

	if err := c.tox.FileSendChunk(friendID, fileNumber, position, dataToSend); err != nil {
		log.Printf("Failed to send file chunk: %v", err)
		c.cancelFileTransfer(friendID, fileNumber)
		c.abortFileSend(friendID, fileNumber, transferKey, transfer)
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

// handleAsyncMessage processes async messages (offline messages)
func (c *Client) handleAsyncMessage(senderPK [32]byte, message string, messageType async.MessageType) {
	friendID, err := c.tox.GetFriendByPublicKey(senderPK)
	if err != nil {
		log.Printf("Received async message from unknown sender: %v", err)
		return
	}

	c.friendsMu.RLock()
	friend, exists := c.friends[friendID]
	c.friendsMu.RUnlock()

	if !exists {
		log.Printf("Received async message from friend %d not in map", friendID)
		return
	}

	timestamp := time.Now().Format("15:04:05")
	var formattedMessage string

	switch messageType {
	case async.MessageTypeAction:
		formattedMessage = fmt.Sprintf("[%s] [ASYNC] * %s %s", timestamp, friend.Name, message)
	default:
		formattedMessage = fmt.Sprintf("[%s] [ASYNC] <%s> %s", timestamp, friend.Name, message)
	}

	friendIDStr := hex.EncodeToString(friend.PublicKey[:])
	if err := c.fifoManager.WriteFriendTextOut(friendIDStr, formattedMessage); err != nil {
		log.Printf("Failed to write async message to text_out FIFO: %v", err)
	}

	if c.config.Debug {
		log.Printf("Async message from %s (%d): %s", friend.Name, friendID, message)
	}
}

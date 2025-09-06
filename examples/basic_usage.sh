#!/bin/bash
# Basic usage example for ratox-go
# This script demonstrates how to interact with ratox-go through the filesystem interface

set -e

RATOX_DIR="$HOME/.ratox"
CLIENT_DIR="$RATOX_DIR/client"

echo "=== ratox-go Basic Usage Example ==="

# Check if ratox-go is running
if ! pgrep -f "ratox-go" > /dev/null; then
    echo "Starting ratox-go in the background..."
    ratox-go -debug &
    RATOX_PID=$!
    
    # Wait for initialization
    echo "Waiting for ratox-go to initialize..."
    sleep 3
    
    # Check if FIFOs are created
    if [ ! -p "$CLIENT_DIR/name" ]; then
        echo "Error: ratox-go FIFOs not found. Make sure ratox-go is running."
        exit 1
    fi
else
    echo "ratox-go is already running"
fi

echo "ratox-go directory structure:"
find "$RATOX_DIR" -type p -exec ls -la {} \; 2>/dev/null || echo "No FIFOs found yet"

echo ""
echo "=== Basic Operations ==="

# Change display name
echo "1. Changing display name to 'Example User'..."
echo "Example User" > "$CLIENT_DIR/name"
echo "   ✓ Name changed"

# Change status message  
echo "2. Changing status message..."
echo "Using ratox-go example" > "$CLIENT_DIR/status_message"
echo "   ✓ Status message changed"

# Show current Tox ID
echo "3. Your Tox ID:"
if [ -f "$CLIENT_DIR/id" ]; then
    echo "   $(cat "$CLIENT_DIR/id")"
else
    echo "   ID file not available yet (still bootstrapping)"
fi

echo ""
echo "=== Friend Management ==="
echo "To add a friend, write their Tox ID to request_in:"
echo "  echo 'FRIEND_TOX_ID_HERE' > $CLIENT_DIR/request_in"
echo ""
echo "Incoming friend requests will appear in:"
echo "  $CLIENT_DIR/request_out"
echo ""
echo "Once you have friends, directories will be created for each friend:"
echo "  $RATOX_DIR/FRIEND_ID/text_in   - Send messages"
echo "  $RATOX_DIR/FRIEND_ID/text_out  - Receive messages" 
echo "  $RATOX_DIR/FRIEND_ID/file_in   - Send files"
echo "  $RATOX_DIR/FRIEND_ID/file_out  - Receive files"

echo ""
echo "=== Monitoring ==="
echo "To monitor incoming messages, use:"
echo "  tail -f $RATOX_DIR/*/text_out"
echo ""
echo "To monitor friend requests:"
echo "  tail -f $CLIENT_DIR/request_out"

# If we started ratox-go, offer to stop it
if [ ! -z "$RATOX_PID" ]; then
    echo ""
    echo "Example complete. ratox-go is running in background (PID: $RATOX_PID)"
    echo "To stop it: kill $RATOX_PID"
    echo "Or press Ctrl+C to stop it now..."
    
    # Wait for user input
    read -p "Press Enter to stop ratox-go or Ctrl+C to leave it running..."
    kill "$RATOX_PID" 2>/dev/null || true
    echo "ratox-go stopped"
fi

echo "Example complete!"

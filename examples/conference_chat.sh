#!/bin/bash
# Conference/Group Chat Example for ratox-go
# Demonstrates how to create and use conference chats
#
# NOTE: Conference support is experimental and send-only.
# Receiving messages requires toxcore API enhancements.

RATOX_DIR="${HOME}/.config/ratox-go"
CLIENT_DIR="${RATOX_DIR}/client"
CONFERENCE_DIR="${RATOX_DIR}/conferences"

echo "=== ratox-go Conference Chat Example ==="
echo

# Check if ratox-go is running
if ! pgrep -f "ratox-go" > /dev/null; then
    echo "Error: ratox-go is not running"
    echo "Start ratox-go first with: ratox-go -debug"
    exit 1
fi

echo "1. Creating a new conference..."
echo

# Create a conference by writing to conference_in
if [ -p "$CLIENT_DIR/conference_in" ]; then
    echo "create" > "$CLIENT_DIR/conference_in"
    echo "✓ Conference creation request sent"
    echo "  Check debug output for the conference ID"
    sleep 1
else
    echo "✗ conference_in FIFO not found. Is ratox-go running?"
    exit 1
fi

echo
echo "2. Finding conference directories..."
echo

# List available conferences
if [ -d "$CONFERENCE_DIR" ]; then
    conferences=($(ls -1 "$CONFERENCE_DIR" 2>/dev/null))
    if [ ${#conferences[@]} -eq 0 ]; then
        echo "No conferences found yet. The conference may take a moment to initialize."
        echo "Check ratox-go debug output for the conference ID."
        exit 0
    fi
    
    echo "Available conferences:"
    for conf_id in "${conferences[@]}"; do
        echo "  - Conference ID: $conf_id"
        echo "    Directory: $CONFERENCE_DIR/$conf_id"
    done
    
    # Use the first conference for examples
    CONF_ID="${conferences[0]}"
else
    echo "Conference directory not found: $CONFERENCE_DIR"
    exit 1
fi

echo
echo "3. Sending a message to conference $CONF_ID..."
echo

# Send a test message
CONF_TEXT_IN="$CONFERENCE_DIR/$CONF_ID/text_in"
if [ -p "$CONF_TEXT_IN" ]; then
    echo "Hello, group chat! This is a test message." > "$CONF_TEXT_IN"
    echo "✓ Message sent to conference $CONF_ID"
else
    echo "✗ text_in FIFO not found for conference $CONF_ID"
    exit 1
fi

echo
echo "4. Inviting a friend to the conference..."
echo

# To invite a friend, you need their public key (TOX_ID without the nospam/checksum)
# This is just an example - replace with an actual friend's public key
echo "To invite a friend, write their 64-character public key to:"
echo "  echo 'FRIEND_PUBLIC_KEY_HERE' > $CONFERENCE_DIR/$CONF_ID/invite_in"
echo
echo "Example:"
echo "  echo '76518406F6A9F2217E8DC487CC783C25CC16A15EB36FF32E335364EC37CBA151' \\"
echo "    > $CONFERENCE_DIR/$CONF_ID/invite_in"

echo
echo "=== Conference Chat Usage Summary ==="
echo
echo "Create conference:"
echo "  echo 'create' > $CLIENT_DIR/conference_in"
echo
echo "Send message to conference:"
echo "  echo 'Your message here' > $CONFERENCE_DIR/<conference_id>/text_in"
echo
echo "Invite friend to conference:"
echo "  echo '<friend_public_key>' > $CONFERENCE_DIR/<conference_id>/invite_in"
echo
echo "Monitor conference activity:"
echo "  tail -f /path/to/ratox-go/debug/output"
echo
echo "LIMITATIONS (experimental feature):"
echo "  - Cannot receive messages from others (toxcore API limitation)"
echo "  - Cannot see member list (toxcore API limitation)"
echo "  - Cannot accept conference invites from friends (toxcore API limitation)"
echo
echo "These limitations will be resolved when toxcore adds the necessary callbacks."

#!/bin/bash
# Friend Deletion Example for ratox-go
# Demonstrates how to safely remove friends

RATOX_DIR="${HOME}/.config/ratox-go"

echo "=== ratox-go Friend Deletion Example ==="
echo

# Check if ratox-go is running
if ! pgrep -f "ratox-go" > /dev/null; then
    echo "Error: ratox-go is not running"
    echo "Start ratox-go first with: ratox-go"
    exit 1
fi

echo "WARNING: This will permanently delete a friend from your contact list!"
echo "The friend's directory and all message history will be removed."
echo

# List all friends
echo "Current friends:"
friend_count=0

for friend_dir in "${RATOX_DIR}"/*/ ; do
    # Skip client directory and conference directory
    if [[ "$friend_dir" == *"/client/"* ]] || [[ "$friend_dir" == *"/conferences/"* ]]; then
        continue
    fi
    
    friend_id=$(basename "$friend_dir")
    
    # Check if this is a valid friend directory (has FIFOs)
    if [ -p "${friend_dir}text_in" ]; then
        ((friend_count++))
        echo "  $friend_count. $friend_id"
        
        # Try to show friend name if available
        if [ -f "${friend_dir}name" ]; then
            name=$(cat "${friend_dir}name" 2>/dev/null)
            if [ -n "$name" ]; then
                echo "     Name: $name"
            fi
        fi
        
        # Show current status
        if [ -f "${friend_dir}status" ]; then
            status=$(cat "${friend_dir}status" 2>/dev/null)
            if [ -n "$status" ]; then
                echo "     Status: $status"
            fi
        fi
        echo
    fi
done

if [ $friend_count -eq 0 ]; then
    echo "No friends found. Nothing to delete."
    exit 0
fi

# Get friend ID to delete
echo -n "Enter the friend ID to delete (or 'cancel' to abort): "
read friend_id

if [ "$friend_id" == "cancel" ] || [ -z "$friend_id" ]; then
    echo "Deletion cancelled."
    exit 0
fi

# Validate friend exists
friend_dir="${RATOX_DIR}/${friend_id}"
if [ ! -d "$friend_dir" ]; then
    echo "Error: Friend directory not found: $friend_dir"
    exit 1
fi

# Confirm deletion
echo
echo "You are about to delete friend: $friend_id"
echo "Directory: $friend_dir"
echo
echo -n "Are you sure? Type 'yes' to confirm: "
read confirmation

if [ "$confirmation" != "yes" ]; then
    echo "Deletion cancelled."
    exit 0
fi

# Perform deletion
remove_in="${friend_dir}/remove_in"

if [ ! -p "$remove_in" ]; then
    echo "Error: remove_in FIFO not found: $remove_in"
    echo "Is ratox-go running and is this a valid friend?"
    exit 1
fi

echo
echo "Deleting friend $friend_id..."

# Write "confirm" to the remove_in FIFO
echo "confirm" > "$remove_in"

echo "✓ Deletion request sent"
echo

# Give the client a moment to process
sleep 2

# Verify deletion
if [ ! -d "$friend_dir" ]; then
    echo "✓ Friend successfully deleted"
    echo "  The friend's directory has been removed: $friend_dir"
    echo "  All message history has been deleted."
else
    echo "⚠ Friend directory still exists: $friend_dir"
    echo "  Check ratox-go logs for error messages."
    echo "  The friend may need to be removed manually."
fi

echo
echo "=== Friend Deletion Summary ==="
echo
echo "To delete a friend, write 'confirm' to their remove_in FIFO:"
echo "  echo 'confirm' > ~/.config/ratox-go/FRIEND_ID/remove_in"
echo
echo "This will:"
echo "  1. Remove the friend from your Tox contact list"
echo "  2. Delete their FIFO directory and all files"
echo "  3. Clean up all message history"
echo
echo "The deletion is permanent and cannot be undone."
echo "To re-add the friend, you'll need to send a new friend request."

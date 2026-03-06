#!/bin/bash
# File Transfer Monitor for ratox-go
# Monitors and logs all file transfer activity

RATOX_DIR="${HOME}/.config/ratox-go"
LOG_DIR="${HOME}/.config/ratox-go/logs"
TRANSFER_LOG="${LOG_DIR}/file_transfers.log"

# Create log directory
mkdir -p "$LOG_DIR"

echo "=== ratox-go File Transfer Monitor ==="
echo "Monitoring all file transfers in: $RATOX_DIR"
echo "Logging to: $TRANSFER_LOG"
echo "Press Ctrl+C to stop"
echo

# Function to log transfer event
log_transfer() {
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local friend_id="$1"
    local event_type="$2"
    local details="$3"
    
    echo "[$timestamp] [$friend_id] $event_type: $details" | tee -a "$TRANSFER_LOG"
}

# Function to monitor a friend's file_out FIFO
monitor_friend_transfers() {
    local friend_id="$1"
    local file_out="${RATOX_DIR}/${friend_id}/file_out"
    
    if [ ! -p "$file_out" ]; then
        return
    fi
    
    echo "Monitoring transfers for friend: $friend_id"
    
    # Monitor file_out for incoming transfer notifications
    tail -f "$file_out" 2>/dev/null | while read -r line; do
        # Parse transfer information
        # Format typically: "filename|size|file_number" or status messages
        
        if [[ "$line" =~ "Receiving file:" ]]; then
            log_transfer "$friend_id" "INCOMING" "$line"
        elif [[ "$line" =~ "File received:" ]]; then
            log_transfer "$friend_id" "COMPLETE" "$line"
        elif [[ "$line" =~ "Transfer failed:" ]]; then
            log_transfer "$friend_id" "FAILED" "$line"
        elif [[ "$line" =~ "File rejected:" ]]; then
            log_transfer "$friend_id" "REJECTED" "$line"
        else
            log_transfer "$friend_id" "STATUS" "$line"
        fi
    done &
}

# Function to get file size in human-readable format
human_size() {
    local bytes=$1
    if [ $bytes -lt 1024 ]; then
        echo "${bytes}B"
    elif [ $bytes -lt 1048576 ]; then
        echo "$((bytes / 1024))KB"
    elif [ $bytes -lt 1073741824 ]; then
        echo "$((bytes / 1048576))MB"
    else
        echo "$((bytes / 1073741824))GB"
    fi
}

# Find all friend directories
echo "Scanning for friends..."
friend_count=0

for friend_dir in "${RATOX_DIR}"/*/ ; do
    # Skip client directory and conference directory
    if [[ "$friend_dir" == *"/client/"* ]] || [[ "$friend_dir" == *"/conferences/"* ]]; then
        continue
    fi
    
    friend_id=$(basename "$friend_dir")
    
    # Check if this is a valid friend directory (has FIFOs)
    if [ -p "${friend_dir}text_in" ]; then
        monitor_friend_transfers "$friend_id"
        ((friend_count++))
    fi
done

if [ $friend_count -eq 0 ]; then
    echo "No friends found. Add friends first to monitor their transfers."
    exit 1
fi

echo
echo "Monitoring $friend_count friend(s) for file transfers..."
echo

# Keep the script running
wait

echo
echo "File transfer monitoring stopped."
echo "Transfer log saved to: $TRANSFER_LOG"

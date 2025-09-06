#!/bin/bash
# Auto-responder bot for ratox-go
# Responds to specific messages automatically

RATOX_DIR="${HOME}/.config/ratox-go"
LOG_FILE="${HOME}/.config/ratox-go/autoresponder.log"

# Response mappings
declare -A responses=(
    ["ping"]="pong"
    ["hello"]="Hello there! I'm an auto-responder bot."
    ["help"]="Available commands: ping, hello, time, status"
    ["time"]="Current time: $(date)"
    ["status"]="Bot is running and ready to respond!"
)

log_message() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

# Function to send response
send_response() {
    local friend_id="$1"
    local response="$2"
    local friend_dir="${RATOX_DIR}/${friend_id}"
    
    if [ -p "${friend_dir}/text_in" ]; then
        echo "$response" > "${friend_dir}/text_in"
        log_message "Sent to $friend_id: $response"
    fi
}

# Function to process message
process_message() {
    local friend_id="$1"
    local message="$2"
    
    # Convert to lowercase for matching
    local lower_message=$(echo "$message" | tr '[:upper:]' '[:lower:]')
    
    # Check for exact matches
    for trigger in "${!responses[@]}"; do
        if [[ "$lower_message" == *"$trigger"* ]]; then
            local response="${responses[$trigger]}"
            # Handle dynamic responses
            if [[ "$trigger" == "time" ]]; then
                response="Current time: $(date)"
            fi
            send_response "$friend_id" "$response"
            return
        fi
    done
    
    # Log unhandled message
    log_message "Unhandled message from $friend_id: $message"
}

# Main monitoring loop
main() {
    log_message "Auto-responder started"
    
    # Monitor all friend text_out files
    for friend_dir in "${RATOX_DIR}"/*/; do
        if [ -d "$friend_dir" ] && [ -p "${friend_dir}/text_out" ]; then
            friend_id=$(basename "$friend_dir")
            
            # Start monitoring this friend in background
            {
                tail -f "${friend_dir}/text_out" | while IFS= read -r line; do
                    # Extract message (remove timestamp if present)
                    message=$(echo "$line" | sed 's/^\[[0-9:]*\] //')
                    process_message "$friend_id" "$message"
                done
            } &
        fi
    done
    
    # Wait for all background processes
    wait
}

# Handle cleanup
cleanup() {
    log_message "Auto-responder stopping"
    kill $(jobs -p) 2>/dev/null
    exit 0
}

trap cleanup INT TERM

main

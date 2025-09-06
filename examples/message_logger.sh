#!/bin/bash
# Message logger for ratox-go
# Logs all messages with timestamps and friend information

RATOX_DIR="${HOME}/.config/ratox-go"
LOG_DIR="${HOME}/.config/ratox-go/logs"
MAIN_LOG="${LOG_DIR}/all_messages.log"

# Create log directory
mkdir -p "$LOG_DIR"

# Function to log message
log_message() {
    local friend_id="$1"
    local message="$2"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local friend_log="${LOG_DIR}/${friend_id}.log"
    
    # Log to friend-specific file
    echo "[$timestamp] $message" >> "$friend_log"
    
    # Log to main file
    echo "[$timestamp] [$friend_id] $message" >> "$MAIN_LOG"
    
    # Also display on stdout with color
    echo -e "\033[1;32m[$timestamp]\033[0m \033[1;34m[$friend_id]\033[0m $message"
}

# Function to log friend request
log_friend_request() {
    local request="$1"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local requests_log="${LOG_DIR}/friend_requests.log"
    
    echo "[$timestamp] $request" >> "$requests_log"
    echo "[$timestamp] $request" >> "$MAIN_LOG"
    
    echo -e "\033[1;33m[$timestamp]\033[0m \033[1;35m[FRIEND REQUEST]\033[0m $request"
}

# Main function
main() {
    echo "Message logger started. Logging to: $LOG_DIR"
    echo "Press Ctrl+C to stop."
    
    # Monitor all existing friends
    for friend_dir in "${RATOX_DIR}"/*/; do
        if [ -d "$friend_dir" ] && [ -p "${friend_dir}/text_out" ]; then
            friend_id=$(basename "$friend_dir")
            
            {
                tail -f "${friend_dir}/text_out" | while IFS= read -r line; do
                    log_message "$friend_id" "$line"
                done
            } &
        fi
    done
    
    # Monitor friend requests
    if [ -p "${RATOX_DIR}/request_out" ]; then
        {
            tail -f "${RATOX_DIR}/request_out" | while IFS= read -r line; do
                log_friend_request "$line"
            done
        } &
    fi
    
    # Monitor for new friends (check every 30 seconds)
    {
        while true; do
            sleep 30
            for friend_dir in "${RATOX_DIR}"/*/; do
                if [ -d "$friend_dir" ] && [ -p "${friend_dir}/text_out" ]; then
                    friend_id=$(basename "$friend_dir")
                    
                    # Check if we're already monitoring this friend
                    if ! pgrep -f "tail -f ${friend_dir}/text_out" > /dev/null; then
                        echo "New friend detected: $friend_id"
                        {
                            tail -f "${friend_dir}/text_out" | while IFS= read -r line; do
                                log_message "$friend_id" "$line"
                            done
                        } &
                    fi
                fi
            done
        done
    } &
    
    # Wait for interrupt
    wait
}

# Cleanup function
cleanup() {
    echo -e "\n\nStopping message logger..."
    kill $(jobs -p) 2>/dev/null
    exit 0
}

trap cleanup INT TERM

main

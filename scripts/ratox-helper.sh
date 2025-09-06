#!/bin/bash

# ratox-go example scripts
# Demonstration of common usage patterns

set -e

# Configuration
RATOX_DIR="${HOME}/.config/ratox-go"
SCRIPT_DIR="$(dirname "$0")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if ratox-go is running
check_ratox_running() {
    if pgrep -f "ratox-go" > /dev/null; then
        return 0
    else
        return 1
    fi
}

# Wait for ratox to be ready
wait_for_ratox() {
    local timeout=30
    local count=0
    
    print_info "Waiting for ratox-go to be ready..."
    
    while [ $count -lt $timeout ]; do
        if [ -p "${RATOX_DIR}/name" ]; then
            print_success "ratox-go is ready!"
            return 0
        fi
        sleep 1
        count=$((count + 1))
    done
    
    print_error "Timeout waiting for ratox-go to be ready"
    return 1
}

# Start ratox-go in background
start_ratox() {
    if check_ratox_running; then
        print_warning "ratox-go is already running"
        return 0
    fi
    
    print_info "Starting ratox-go..."
    ratox-go -p "${RATOX_DIR}" > /dev/null 2>&1 &
    
    if wait_for_ratox; then
        print_success "ratox-go started successfully"
        show_tox_id
    else
        print_error "Failed to start ratox-go"
        return 1
    fi
}

# Stop ratox-go
stop_ratox() {
    if check_ratox_running; then
        print_info "Stopping ratox-go..."
        pkill -f "ratox-go"
        sleep 2
        
        if ! check_ratox_running; then
            print_success "ratox-go stopped"
        else
            print_warning "Force killing ratox-go..."
            pkill -9 -f "ratox-go"
        fi
    else
        print_warning "ratox-go is not running"
    fi
}

# Show Tox ID
show_tox_id() {
    if [ -f "${RATOX_DIR}/ratox.json" ]; then
        # Try to extract Tox ID from logs or use ratox-go command
        print_info "Your Tox ID will be displayed in the ratox-go output"
        print_info "Check the logs or run: ratox-go -v"
    else
        print_warning "ratox-go not initialized yet"
    fi
}

# Monitor all messages
monitor_messages() {
    if ! check_ratox_running; then
        print_error "ratox-go is not running. Start it first with: $0 start"
        return 1
    fi
    
    print_info "Monitoring messages from all friends (Ctrl+C to stop)..."
    
    # Find all friend directories and monitor their text_out files
    for friend_dir in "${RATOX_DIR}"/*/; do
        if [ -d "$friend_dir" ] && [ -p "${friend_dir}/text_out" ]; then
            friend_id=$(basename "$friend_dir")
            tail -f "${friend_dir}/text_out" | while read -r line; do
                echo -e "${GREEN}[$friend_id]${NC} $line"
            done &
        fi
    done
    
    # Also monitor friend requests
    if [ -p "${RATOX_DIR}/request_out" ]; then
        tail -f "${RATOX_DIR}/request_out" | while read -r line; do
            echo -e "${YELLOW}[FRIEND REQUEST]${NC} $line"
        done &
    fi
    
    # Wait for interrupt
    trap 'kill $(jobs -p) 2>/dev/null; exit 0' INT
    wait
}

# Send message to friend
send_message() {
    local friend_id="$1"
    local message="$2"
    
    if [ -z "$friend_id" ] || [ -z "$message" ]; then
        print_error "Usage: $0 send <friend_id> <message>"
        return 1
    fi
    
    local friend_dir="${RATOX_DIR}/${friend_id}"
    
    if [ ! -d "$friend_dir" ]; then
        print_error "Friend $friend_id not found"
        return 1
    fi
    
    if [ ! -p "${friend_dir}/text_in" ]; then
        print_error "text_in FIFO not found for friend $friend_id"
        return 1
    fi
    
    echo "$message" > "${friend_dir}/text_in"
    print_success "Message sent to $friend_id: $message"
}

# Add friend
add_friend() {
    local tox_id="$1"
    
    if [ -z "$tox_id" ]; then
        print_error "Usage: $0 add <tox_id>"
        return 1
    fi
    
    if [ ${#tox_id} -ne 76 ]; then
        print_error "Invalid Tox ID length. Should be 76 characters."
        return 1
    fi
    
    if [ ! -p "${RATOX_DIR}/request_in" ]; then
        print_error "request_in FIFO not found. Is ratox-go running?"
        return 1
    fi
    
    echo "$tox_id" > "${RATOX_DIR}/request_in"
    print_success "Friend request sent to $tox_id"
}

# Set name
set_name() {
    local name="$1"
    
    if [ -z "$name" ]; then
        print_error "Usage: $0 name <name>"
        return 1
    fi
    
    if [ ! -p "${RATOX_DIR}/name" ]; then
        print_error "name FIFO not found. Is ratox-go running?"
        return 1
    fi
    
    echo "$name" > "${RATOX_DIR}/name"
    print_success "Name set to: $name"
}

# Set status message
set_status() {
    local status="$1"
    
    if [ -z "$status" ]; then
        print_error "Usage: $0 status <status_message>"
        return 1
    fi
    
    if [ ! -p "${RATOX_DIR}/status_message" ]; then
        print_error "status_message FIFO not found. Is ratox-go running?"
        return 1
    fi
    
    echo "$status" > "${RATOX_DIR}/status_message"
    print_success "Status message set to: $status"
}

# List friends
list_friends() {
    print_info "Friends:"
    
    if [ ! -d "$RATOX_DIR" ]; then
        print_warning "ratox-go not initialized"
        return 1
    fi
    
    local count=0
    for friend_dir in "${RATOX_DIR}"/*/; do
        if [ -d "$friend_dir" ]; then
            friend_id=$(basename "$friend_dir")
            status="offline"
            
            # Try to read status
            if [ -p "${friend_dir}/status" ]; then
                status=$(timeout 1s cat "${friend_dir}/status" 2>/dev/null || echo "unknown")
            fi
            
            echo -e "  ${GREEN}$friend_id${NC} - $status"
            count=$((count + 1))
        fi
    done
    
    if [ $count -eq 0 ]; then
        print_info "No friends added yet"
    else
        print_info "Total friends: $count"
    fi
}

# Show help
show_help() {
    echo "ratox-go helper script"
    echo ""
    echo "Usage: $0 <command> [arguments]"
    echo ""
    echo "Commands:"
    echo "  start                 - Start ratox-go daemon"
    echo "  stop                  - Stop ratox-go daemon"
    echo "  status                - Show ratox-go status"
    echo "  monitor               - Monitor all messages (real-time)"
    echo "  send <id> <message>   - Send message to friend"
    echo "  add <tox_id>          - Add friend by Tox ID"
    echo "  friends               - List all friends"
    echo "  name <name>           - Set your display name"
    echo "  status <message>      - Set your status message"
    echo "  toxid                 - Show your Tox ID"
    echo "  help                  - Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 start"
    echo "  $0 add 1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234"
    echo "  $0 send ABCDEF1234567890 \"Hello, friend!\""
    echo "  $0 name \"My Name\""
    echo "  $0 status \"Available\""
    echo ""
    echo "Config directory: $RATOX_DIR"
}

# Main command handler
case "$1" in
    start)
        start_ratox
        ;;
    stop)
        stop_ratox
        ;;
    status)
        if check_ratox_running; then
            print_success "ratox-go is running"
        else
            print_warning "ratox-go is not running"
        fi
        ;;
    monitor)
        monitor_messages
        ;;
    send)
        send_message "$2" "$3"
        ;;
    add)
        add_friend "$2"
        ;;
    friends)
        list_friends
        ;;
    name)
        set_name "$2"
        ;;
    status)
        set_status "$2"
        ;;
    toxid)
        show_tox_id
        ;;
    help|--help|-h)
        show_help
        ;;
    "")
        print_error "No command specified"
        show_help
        exit 1
        ;;
    *)
        print_error "Unknown command: $1"
        show_help
        exit 1
        ;;
esac

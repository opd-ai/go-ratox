# ratox-go

A Go implementation of the ratox Tox chat client using the [opd-ai/toxcore](https://github.com/opd-ai/toxcore) pure Go library.

Ratox is a FIFO (named pipe) based Tox client that provides a filesystem interface for Tox messaging. This implementation replicates the original ratox functionality while leveraging Go's concurrency features and the pure Go Tox library.

## Features

- **FIFO-based filesystem interface** matching original ratox behavior  
- **Text messaging** with UTF-8 support
- **File transfers** up to 100MB (configurable)
- **Friend management** with request handling and friend deletion
- **Typing indicators** for real-time conversation awareness
- **Online/offline status management**
- **Concurrent handling** of multiple friends
- **Thread-safe operations** for all Tox interactions
- **Graceful error handling** and network interruption recovery
- **JSON configuration** with persistent settings
- **Automatic bootstrap** to Tox DHT nodes
- **Tor and I2P support** for anonymous networking
- **Async (offline) messaging** with message queuing
- **Conference/group chat support** (experimental, send-only)

## Installation

### Prerequisites

- Go 1.24 or later
- Linux/Unix system (for FIFO support)

### Build from source

```bash
git clone https://github.com/opd-ai/go-ratox.git
cd go-ratox
go mod tidy
go build -o ratox-go
```

### Install

```bash
go install github.com/opd-ai/go-ratox@latest
```

## Quick Start

```bash
# Start the client (uses ~/.config/ratox-go by default)
./ratox-go

# Start with debug logging
./ratox-go -debug

# Start with custom profile directory
./ratox-go -profile /path/to/profile

# Show help
./ratox-go -help
```

After starting, ratox-go creates a filesystem interface at `~/.config/ratox-go/`:

```
~/.config/ratox-go/
├── client/
│   ├── request_in      # Write friend requests here
│   ├── request_out     # Read incoming friend requests  
│   ├── name            # Write to change your name
│   ├── status_message  # Write to change status message
│   ├── conference_in   # Create conferences (experimental)
│   └── config.json     # Configuration file
├── FRIEND_ID/          # Directory for each friend
│   ├── text_in         # Write messages to send
│   ├── text_out        # Read received messages
│   ├── file_in         # Write file paths to send
│   ├── file_out        # Read incoming file info
│   ├── status          # Read friend's status
│   ├── typing          # Read friend's typing status
│   └── remove_in       # Write to remove friend
└── conferences/        # Conference directories (experimental)
    └── <id>/           # Per-conference FIFOs
        ├── text_in     # Send conference messages
        └── invite_in   # Invite friends
```

## Filesystem Interface

The client creates a directory structure that mirrors the original ratox:

```
~/.config/ratox-go/
├── client/
│   ├── request_in           # Accept friend requests (write-only)
│   ├── request_out          # Incoming friend requests (read-only)
│   ├── name                 # Your display name (write-only)
│   ├── status_message       # Your status message (write-only)
│   ├── conference_in        # Create new conferences (write-only)
│   ├── config.json          # Configuration file
│   └── ratox.tox           # Tox save data
├── <friend_id>/            # Directory for each friend
│   ├── text_in             # Send messages (write-only)
│   ├── text_out            # Receive messages (read-only)
│   ├── file_in             # Send files (write-only)
│   ├── file_out            # Receive files (read-only)
│   ├── status              # Friend status (read-only)
│   ├── typing              # Friend typing status (read-only)
│   └── remove_in           # Remove friend (write-only)
└── conferences/<conference_id>/  # Directory for each conference
    ├── text_in             # Send conference messages (write-only)
    └── invite_in           # Invite friends to conference (write-only)
```

### Basic Operations

#### Add a friend
```bash
echo "76_CHARACTER_TOX_ID_HERE" > ~/.config/ratox-go/client/request_in
```

#### Monitor incoming friend requests
```bash
tail -f ~/.config/ratox-go/client/request_out
```

#### Change your display name
```bash
echo "My New Name" > ~/.config/ratox-go/client/name
```

#### Change your status message
```bash
echo "Available for chat" > ~/.config/ratox-go/client/status_message
```

#### Send a message to a friend
```bash
echo "Hello, world!" > ~/.config/ratox-go/FRIEND_ID/text_in
```

#### Read incoming messages
```bash
tail -f ~/.config/ratox-go/FRIEND_ID/text_out
```

#### Send a file
```bash
echo "/path/to/file.txt" > ~/.config/ratox-go/FRIEND_ID/file_in
```

#### Monitor friend status
```bash
cat ~/.config/ratox-go/FRIEND_ID/status
```

#### Monitor friend typing status
```bash
# Check if friend is typing (shows "true" or "false")
cat ~/.config/ratox-go/FRIEND_ID/typing

# Monitor typing status in real-time
watch cat ~/.config/ratox-go/FRIEND_ID/typing
```

#### Remove a friend
```bash
# Remove a friend by writing "confirm" to their remove_in FIFO
echo "confirm" > ~/.config/ratox-go/FRIEND_ID/remove_in
# This will delete the friend from your contact list and clean up their directory
```

### Monitoring Multiple Friends

```bash
# Monitor all incoming messages
tail -f ~/.config/ratox-go/*/text_out

# Monitor all friend status changes
watch 'find ~/.config/ratox-go -name status -exec echo {} \; -exec cat {} \;'
```

### Conference/Group Chat (Experimental)

**Status:** Conference support is **experimental and send-only**. You can create conferences, send messages, and invite friends, but receiving messages is not yet supported due to toxcore API limitations.

#### Create a conference
```bash
# Write anything to conference_in to create a new conference
echo "create" > ~/.config/ratox-go/client/conference_in
# The conference ID will be logged in debug output
```

#### Send a message to a conference
```bash
# Replace <conference_id> with the actual numeric ID
echo "Hello everyone!" > ~/.config/ratox-go/conferences/<conference_id>/text_in
```

#### Invite a friend to a conference
```bash
# Replace <conference_id> and <friend_public_key> with actual values
echo "<friend_public_key>" > ~/.config/ratox-go/conferences/<conference_id>/invite_in
```

**Known Limitations:**
- **No incoming messages**: Conference messages sent by others cannot be received due to missing toxcore callbacks
- **No member list**: The API doesn't expose conference member enumeration
- **No invite acceptance**: Accepting conference invites from friends is not yet supported

These limitations will be resolved once the underlying toxcore library adds the necessary callback APIs (`OnConferenceMessage`, `OnConferenceInvite`, etc.).

## Configuration

The client automatically creates a configuration file (`config.json`) with the following options:

```json
{
  "debug": false,
  "name": "ratox-go user",
  "status_message": "Running ratox-go",
  "auto_accept_files": false,
  "max_file_size": 104857600,
  "bootstrap_nodes": [
    {
      "address": "nodes.tox.chat",
      "port": 33445,
      "public_key": "6FC41E2BD381D37E9748FC0E0328CE086AF9598BECC8FEB7DDF2E440475F300E"
    },
    {
      "address": "130.133.110.14",
      "port": 33445,
      "public_key": "461FA3776EF0FA655F1A05477DF1B3B614F7D6B124F7DB1DD4FE3C08B03B640F"
    }
  ]
}
```

### Configuration Options

- `debug`: Enable debug logging
- `name`: Your display name (max 128 characters)
- `status_message`: Your status message (max 1007 characters)
- `auto_accept_files`: Automatically accept incoming file transfers
- `max_file_size`: Maximum file size to accept in bytes (default: 100MB)
- `bootstrap_nodes`: List of DHT bootstrap nodes for network connection

### Updating Bootstrap Nodes

Bootstrap nodes are critical for connecting to the Tox DHT network. If the default nodes become unavailable, you can update them by editing `~/.config/ratox-go/client/config.json`.

#### Finding Current Bootstrap Nodes

The Tox community maintains updated lists of bootstrap nodes:

- **Tox Wiki**: https://wiki.tox.chat/users/nodes
- **nodes.tox.chat**: Tox's official node list service
- **Community Lists**: Check the [Tox project](https://tox.chat/) for the latest recommendations

#### Adding or Replacing Nodes

Edit your `config.json` to add new nodes or replace stale ones:

```json
{
  "bootstrap_nodes": [
    {
      "address": "nodes.tox.chat",
      "port": 33445,
      "public_key": "6FC41E2BD381D37E9748FC0E0328CE086AF9598BECC8FEB7DDF2E440475F300E"
    },
    {
      "address": "NEW_NODE_ADDRESS",
      "port": 33445,
      "public_key": "NEW_NODE_PUBLIC_KEY"
    }
  ]
}
```

**Important**: Each bootstrap node requires three fields:
- `address`: Domain name or IP address
- `port`: UDP port (usually 33445)
- `public_key`: 64-character hex public key

#### Verifying Connection

After updating nodes, restart ratox-go with debug mode to verify connectivity:

```bash
./ratox-go -debug
```

Look for log messages indicating successful bootstrap:
```
Bootstrapping to nodes.tox.chat:33445
DHT connection established
```

If connection fails with all nodes, check:
1. Network connectivity (firewall rules, UDP blocked?)
2. Node addresses are current (check Tox wiki)
3. Public keys are correct (typos will cause connection failure)

### Anonymous Networking (Tor/I2P)

ratox-go supports routing Tox traffic through Tor or I2P for enhanced privacy and anonymity.

#### Tor Configuration

To route traffic through Tor, enable Tor transport in your `config.json`:

```json
{
  "transport": {
    "tor_enabled": true,
    "tor_socks_addr": "127.0.0.1:9050"
  }
}
```

**Requirements:**
- Tor daemon running locally (usually on port 9050)
- Install Tor: `sudo apt install tor` (Debian/Ubuntu) or see [torproject.org](https://www.torproject.org/)

**Benefits:**
- Hides your IP address from peers and bootstrap nodes
- Strong anonymity through Tor's onion routing
- Bypass network restrictions and censorship

#### I2P Configuration

To route traffic through I2P, enable I2P transport in your `config.json`:

```json
{
  "transport": {
    "i2p_enabled": true,
    "i2p_sam_addr": "127.0.0.1:7656"
  }
}
```

**Requirements:**
- I2P router running with SAM bridge enabled (default port 7656)
- Install I2P: see [geti2p.net](https://geti2p.net/)

**Benefits:**
- Alternative anonymizing network using garlic routing
- Resistant to traffic analysis
- Decentralized peer-to-peer architecture

#### Using Both Tor and I2P

You can enable both transports simultaneously for maximum connectivity:

```json
{
  "transport": {
    "tor_enabled": true,
    "tor_socks_addr": "127.0.0.1:9050",
    "i2p_enabled": true,
    "i2p_sam_addr": "127.0.0.1:7656"
  }
}
```

The client will intelligently route traffic through the appropriate transport based on peer capabilities and network conditions.

### Command Line Options

```bash
./ratox-go [options]

Options:
  -help           Show help message
  -version        Show version information
  -debug          Enable debug logging
  -profile DIR    Configuration directory (default: ~/.config/ratox-go)
```

## Scripts and Automation

The FIFO interface makes ratox-go compatible with shell scripts and automation tools:

### Simple chat monitor
```bash
#!/bin/bash
# Monitor all friends for new messages
for friend_dir in ~/.config/ratox-go/*/; do
    if [ -d "$friend_dir" ] && [ "$(basename "$friend_dir")" != "client" ]; then
        friend_id=$(basename "$friend_dir")
        tail -f "$friend_dir/text_out" | while read line; do
            echo "[$friend_id] $line"
        done &
    fi
done
wait
```

### Auto-responder
```bash
#!/bin/bash
# Auto-respond to messages containing "ping"
tail -f ~/.config/ratox-go/*/text_out | while read line; do
    if echo "$line" | grep -qi "ping"; then
        # Extract friend directory from the file path
        friend_dir=$(echo "$line" | cut -d: -f1 | xargs dirname)
        echo "pong" > "$friend_dir/text_in"
    fi
done
```

### Message logger
```bash
#!/bin/bash
# Log all messages with timestamps
mkdir -p ~/.config/ratox-go/logs
tail -f ~/.config/ratox-go/*/text_out | while read line; do
    echo "$(date '+%Y-%m-%d %H:%M:%S') $line" >> ~/.config/ratox-go/logs/messages.log
done
```

## Architecture

### Project Structure

```
ratox-go/
├── main.go              # Entry point and CLI handling
├── client/              # Tox client implementation
│   ├── client.go        # Main client logic
│   ├── fifo.go          # FIFO management
│   └── handlers.go      # Message/file/request handlers
├── config/              # Configuration management
│   └── config.go        # Config loading and saving
├── go.mod               # Module definition
└── README.md            # This file
```

### Key Components

1. **Client**: Core Tox client managing connections and state
2. **FIFOManager**: Handles all named pipe operations
3. **Handlers**: Processes Tox events and callbacks (friend messages, file transfers, conferences)
4. **Config**: Configuration management and persistence

## Development

### Running Tests

```bash
go test ./...
```

### Code Quality

```bash
# Run linter
golangci-lint run

# Format code
go fmt ./...

# Vet code
go vet ./...
```

## Development

### Building

```bash
# Clone and build
git clone https://github.com/opd-ai/go-ratox.git
cd go-ratox
go mod tidy
go build -o ratox-go
```

### Testing

```bash
# Run unit tests
go test ./...

# Run functionality tests
./test_ratox.sh

# Run with race detection
go test -race ./...
```

### Code Quality

```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run linter (if available)
golangci-lint run
```

## Example Scripts

The `examples/` directory contains ready-to-use shell scripts demonstrating various ratox-go features:

### Basic Usage
- **`basic_usage.sh`** - Introduction to the FIFO interface and common operations
- **`message_logger.sh`** - Log all messages with timestamps to files
- **`autoresponder.sh`** - Automated bot that responds to specific messages

### Advanced Features
- **`conference_chat.sh`** - Create and use group chats (experimental)
- **`file_transfer_monitor.sh`** - Monitor and log all file transfer activity
- **`friend_deletion.sh`** - Safely remove friends from your contact list

### Running Examples

```bash
# Make scripts executable (if needed)
chmod +x examples/*.sh

# Run an example
./examples/conference_chat.sh
./examples/file_transfer_monitor.sh
./examples/friend_deletion.sh
```

Each script includes detailed comments and usage instructions. Check the script source for customization options.

## Troubleshooting

### Common Issues

1. **Permission Denied on FIFOs**: 
   - Check that the client is running
   - Verify directory permissions: `ls -la ~/.config/ratox-go/client/`

2. **FIFO Not Found**: 
   - Client recreates missing FIFOs automatically
   - Restart the client if FIFOs are corrupted

3. **Network Connection Issues**: 
   - Client automatically attempts reconnection
   - Check bootstrap nodes in configuration (see [Updating Bootstrap Nodes](#updating-bootstrap-nodes))
   - Verify UDP port 33445 is not blocked by firewall
   - Enable debug mode: `./ratox-go -debug`
   - If default nodes are offline, update to current nodes from [Tox wiki](https://wiki.tox.chat/users/nodes)

4. **Large Files Rejected**: 
   - Check `max_file_size` in configuration
   - Default limit is 100MB

### Debug Mode

Enable debug mode for detailed logging:

```bash
./ratox-go -debug
```

### Example Debug Output

```
2025/01/01 12:00:00 Tox client initialized. Tox ID: a1b2c3d4...
2025/01/01 12:00:00 Bootstrapping to nodes.tox.chat:33445
2025/01/01 12:00:00 Created FIFO: ~/.config/ratox-go/client/request_in
2025/01/01 12:00:01 Friend request received from: e5f6a7b8...
```

## Compatibility

This implementation maintains full compatibility with the original ratox core features:

- **FIFO Interface**: Identical file structure and behavior for friend operations
- **Message Format**: Compatible message encoding and formatting  
- **Friend Management**: Same friend request and management workflow
- **File Transfers**: Compatible file transfer protocol and handling
- **Status Updates**: Matching status information format and updates

Existing ratox scripts and automation should work without modification.

**Extension:** Conference/group chat support is an additional feature not present in the original ratox, enabled by the toxcore library's group chat API.

## Performance

The Go implementation offers several advantages:

- **Concurrent Operations**: Handle 100+ friends simultaneously
- **Memory Efficiency**: Optimized memory usage with garbage collection
- **Network Resilience**: Automatic reconnection and robust error recovery
- **FIFO Management**: Efficient named pipe handling with minimal latency

## Security

- **Safe Defaults**: Secure configuration and permissions out of the box
- **Input Validation**: All user inputs are validated and sanitized
- **Error Handling**: Graceful handling of network and filesystem errors
- **File Permissions**: Proper FIFO permissions (0600) for security

## License

This project is licensed under the same license as the original ratox client. See [LICENSE](LICENSE) for details.

## Acknowledgments

- Original [ratox](https://github.com/2f30/ratox) by 2f30
- [opd-ai/toxcore](https://github.com/opd-ai/toxcore) pure Go Tox library
- The Tox project and community

## Links

- [Tox Project](https://tox.chat/)
- [Original ratox](https://github.com/2f30/ratox)
- [toxcore Go library](https://github.com/opd-ai/toxcore)
- [Issue Tracker](https://github.com/opd-ai/go-ratox/issues)
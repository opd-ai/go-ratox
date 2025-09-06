# ratox-go

A Go implementation of the ratox Tox chat client using the [opd-ai/toxcore](https://github.com/opd-ai/toxcore) pure Go library.

Ratox is a FIFO (named pipe) based Tox client that provides a filesystem interface for Tox messaging. This implementation replicates the original ratox functionality while leveraging Go's concurrency features and the pure Go Tox library.

## Features

- **FIFO-based filesystem interface** matching original ratox behavior  
- **Text messaging** with UTF-8 support
- **File transfers** up to 4GB
- **Friend management** with request handling
- **Online/offline status management**
- **Concurrent handling** of multiple friends
- **Thread-safe operations** for all Tox interactions
- **Graceful error handling** and network interruption recovery
- **JSON configuration** with persistent settings
- **Automatic bootstrap** to Tox DHT nodes

## Installation

### Prerequisites

- Go 1.21 or later
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

After starting, ratox-go creates a filesystem interface at `~/.config/ratox-go/client/`:

```
~/.config/ratox-go/
├── client/
│   ├── request_in      # Write friend requests here
│   ├── request_out     # Read incoming friend requests  
│   ├── name            # Write to change your name
│   ├── status_message  # Write to change status message
│   └── config.json     # Configuration file
└── FRIEND_ID/          # Directory for each friend
    ├── text_in         # Write messages to send
    ├── text_out        # Read received messages
    ├── file_in         # Write file paths to send
    ├── file_out        # Read incoming file info
    └── status          # Read friend's status
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
│   ├── config.json          # Configuration file
│   └── ratox.tox           # Tox save data
└── <friend_id>/            # Directory for each friend
    ├── text_in             # Send messages (write-only)
    ├── text_out            # Receive messages (read-only)
    ├── file_in             # Send files (write-only)
    ├── file_out            # Receive files (read-only)
    └── status              # Friend status (read-only)
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

### Monitoring Multiple Friends

```bash
# Monitor all incoming messages
tail -f ~/.config/ratox-go/*/text_out

# Monitor all friend status changes
watch 'find ~/.config/ratox-go -name status -exec echo {} \; -exec cat {} \;'
```

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
3. **Handlers**: Processes Tox events and callbacks
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
   - Check bootstrap nodes in configuration
   - Enable debug mode: `./ratox-go -debug`

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

This implementation maintains full compatibility with the original ratox:

- **FIFO Interface**: Identical file structure and behavior
- **Message Format**: Compatible message encoding and formatting  
- **Friend Management**: Same friend request and management workflow
- **File Transfers**: Compatible file transfer protocol and handling
- **Status Updates**: Matching status information format and updates

Existing ratox scripts and automation should work without modification.

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
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

## Usage

### Basic Usage

```bash
# Start the client
./ratox-go -p ~/.config/ratox-go

# Start with debug logging
./ratox-go -d -p ~/.config/ratox-go
```

### Command Line Options

- `-p <path>`: Configuration directory path (default: `~/.config/ratox-go`)
- `-d`: Enable debug logging
- `-h`: Show help message
- `-v`: Show version

## Filesystem Interface

The client creates a directory structure that mirrors the original ratox:

```
~/.config/ratox-go/
├── <friend_id>/              # Directory for each friend
│   ├── text_in              # Send messages (write-only)
│   ├── text_out             # Receive messages (read-only)
│   ├── file_in              # Send files (write-only)
│   ├── file_out             # Receive files (read-only)
│   └── status               # Friend status (read-only)
├── request_in               # Accept friend requests (write-only)
├── request_out              # Incoming friend requests (read-only)
├── name                     # Your display name (write-only)
├── status_message           # Your status message (write-only)
├── ratox.json               # Configuration file
└── ratox.tox                # Tox save data
```

### Examples

#### Send a message to a friend
```bash
echo "Hello, world!" > ~/.config/ratox-go/<friend_id>/text_in
```

#### Read incoming messages
```bash
tail -f ~/.config/ratox-go/<friend_id>/text_out
```

#### Accept a friend request
```bash
echo "<76_character_tox_id>" > ~/.config/ratox-go/request_in
```

#### Monitor incoming friend requests
```bash
tail -f ~/.config/ratox-go/request_out
```

#### Send a file
```bash
echo "/path/to/file.txt" > ~/.config/ratox-go/<friend_id>/file_in
```

#### Change your display name
```bash
echo "My New Name" > ~/.config/ratox-go/name
```

#### Change your status message
```bash
echo "Busy coding..." > ~/.config/ratox-go/status_message
```

## Configuration

The client automatically creates a configuration file (`ratox.json`) with the following options:

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
    }
  ]
}
```

### Configuration Options

- `debug`: Enable debug logging
- `name`: Your display name
- `status_message`: Your status message
- `auto_accept_files`: Automatically accept incoming file transfers
- `max_file_size`: Maximum file size to accept (in bytes, 0 = no limit)
- `bootstrap_nodes`: List of DHT bootstrap nodes

## Scripts and Automation

The FIFO interface makes ratox-go compatible with shell scripts and automation tools:

### Simple chat monitor
```bash
#!/bin/bash
# Monitor all friends for new messages
for friend_dir in ~/.config/ratox-go/*/; do
    if [ -d "$friend_dir" ]; then
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
    if echo "$line" | grep -q "ping"; then
        friend_dir=$(dirname "$line")
        echo "pong" > "$friend_dir/text_in"
    fi
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

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## Compatibility

This implementation aims for 100% compatibility with the original ratox client:

- **FIFO Interface**: Identical file structure and naming
- **Message Format**: Compatible message formatting
- **Friend Management**: Same friend request workflow
- **File Transfers**: Compatible file transfer protocol
- **Status Updates**: Matching status information format

## Performance

The Go implementation offers several performance advantages:

- **Concurrent Operations**: Handle 100+ friends simultaneously
- **Memory Efficiency**: Lower memory footprint than C implementation
- **Network Resilience**: Automatic reconnection and error recovery
- **Garbage Collection**: Automatic memory management

## Security

- **Safe Defaults**: Secure configuration out of the box
- **Input Validation**: All user inputs are validated
- **Error Handling**: Graceful handling of network errors
- **File Permissions**: Proper FIFO permissions (read-only/write-only)

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure the configuration directory is writable
2. **FIFO Not Found**: Client will recreate missing FIFOs automatically
3. **Network Issues**: Client automatically attempts reconnection
4. **Large Files**: Check `max_file_size` configuration

### Debug Mode

Enable debug mode for detailed logging:

```bash
./ratox-go -d -p ~/.config/ratox-go
```

### Logs

Monitor system logs for additional information:

```bash
journalctl -f | grep ratox-go
```

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
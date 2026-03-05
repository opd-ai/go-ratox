# PLAN.md — go-ratox Implementation Completion Plan

This document provides a step-by-step plan to resolve all implementation gaps in go-ratox and complete the client, based on the toxcore API available at commit `521bbdf2a2a389ddc7933cd3898dd99fd1f5b28e`.

## Current State

go-ratox is a FIFO-based Tox client that exposes the Tox messaging protocol through a filesystem interface using named pipes. The core FIFO framework, friend management, text messaging, and basic file transfer initiation are implemented. The dependency has been updated to use `github.com/opd-ai/toxcore v0.0.0-20260305025602-521bbdf2a2a3`.

## Implementation Gaps

The following gaps have been identified by auditing the go-ratox client against the toxcore API surface:

### Gap 1: File Transfer — Incomplete Chunk Handling

**Status:** Partial — file send initiation works, but receive/send chunk handling is stubbed.

**Files:** `client/handlers.go`

**Details:**
- `handleFileReceiveChunk` only logs received chunks; it does not write data to disk.
- `handleFileChunkRequest` sends a nil chunk (signaling EOF) immediately instead of reading and sending the actual file data.
- No tracking of active outgoing file transfers (file path → transfer ID mapping).
- No tracking of active incoming file transfers (transfer ID → output file mapping).

### Gap 2: File Transfer — Rejection of Oversized Files

**Status:** Missing — the code logs "rejecting" for oversized files but does not call `FileControl` with `FileControlCancel`.

**Files:** `client/handlers.go:144`

### Gap 3: Friend Connection Status Tracking

**Status:** Missing — `OnFriendConnectionStatus` callback is not registered.

**Files:** `client/client.go`

**Details:**
- The toxcore API provides `OnFriendConnectionStatus(func(friendID uint32, connectionStatus ConnectionStatus))` which fires when a friend transitions between None/TCP/UDP.
- go-ratox only registers `OnFriendStatus` (away/busy/online), but does not track the actual connection status (online/offline).
- The `Friend.Online` field is never updated.

### Gap 4: Friend Deletion

**Status:** Missing — no filesystem interface to remove a friend.

**Files:** `client/client.go`, `client/fifo.go`

**Details:**
- The toxcore API provides `DeleteFriend(friendID uint32) error`.
- go-ratox has no FIFO or mechanism to trigger friend deletion.
- Deleting a friend should also clean up their FIFO directory.

### Gap 5: Typing Notifications

**Status:** Missing — `OnFriendTyping` callback not registered; no outgoing typing support.

**Files:** `client/client.go`, `client/handlers.go`, `client/fifo.go`

**Details:**
- The toxcore API provides `OnFriendTyping(func(friendID uint32, isTyping bool))` and `SetTyping(friendID uint32, isTyping bool) error`.
- go-ratox does not surface typing indicators to the filesystem.

### Gap 6: Friend Status Message Change Callback

**Status:** Missing — `OnFriendStatusMessage` callback not registered.

**Files:** `client/client.go`, `client/handlers.go`

**Details:**
- The toxcore API provides `OnFriendStatusMessage(func(friendID uint32, statusMessage string))`.
- Friend status message changes are not captured or written to the filesystem.

### Gap 7: Self Connection Status Callback

**Status:** Missing — `OnConnectionStatus` callback not registered.

**Files:** `client/client.go`

**Details:**
- The toxcore API provides `OnConnectionStatus(func(status ConnectionStatus))`.
- go-ratox polls `SelfGetConnectionStatus()` periodically instead of reacting to events.

### Gap 8: Use `NewFromSavedata` for Loading Saved State

**Status:** Suboptimal — go-ratox manually passes `SavedataData`/`SavedataType` in Options.

**Files:** `client/client.go`

**Details:**
- The toxcore API provides `NewFromSavedata(options *Options, savedata []byte)` which properly restores the full Tox state including friends list, name, and status message.
- go-ratox currently sets `options.SavedataType = toxcore.SaveDataTypeToxSave` and `options.SavedataData = saveData` then calls `New(options)`. This may not restore all state correctly with the new toxcore.

### Gap 9: Use `IterationInterval()` for Dynamic Tick Rate

**Status:** Suboptimal — hardcoded 50ms tick.

**Files:** `client/client.go:237`

**Details:**
- The toxcore API provides `IterationInterval() time.Duration` to get the recommended interval between `Iterate()` calls.
- go-ratox hardcodes `50 * time.Millisecond`.

### Gap 10: Conference/Group Chat Support

**Status:** Missing — not implemented at all.

**Files:** N/A (new feature)

**Details:**
- The toxcore API provides `ConferenceNew()`, `ConferenceInvite()`, `ConferenceSendMessage()`.
- go-ratox has no group chat support.

### Gap 11: Async Messaging (Offline Messages)

**Status:** Missing — not exposed through FIFO interface.

**Files:** N/A (new feature)

**Details:**
- The toxcore API provides `OnAsyncMessage` callback and `IsAsyncMessagingAvailable()`.
- Async messages sent to offline friends are queued and delivered when they come online.
- go-ratox does not register for or surface async message events.

### Gap 12: Friend `remove_in` FIFO for Deletion

**Status:** Missing — no way to remove friends through the FIFO interface.

**Files:** `client/fifo.go`

### Gap 13: Alternate Transport Support

**Status:** Missing — only UDP transport is enabled; TCP relay, encrypted transports, and anonymizing overlay networks are not used.

**Files:** `client/client.go`, `config/config.go`

**Details:**
- The toxcore API provides pluggable transports: UDP (`transport.NewUDPTransport()`), TCP relay support, Noise-IK encryption (`transport.NewNoiseTransport()`), and version-negotiating transport (`transport.NewNegotiatingTransport()` for automatic legacy/Noise-IK fallback).
- go-ratox currently only sets `options.UDPEnabled = true` and does not expose any configuration for TCP relay connections, encrypted transport layers, or anonymizing overlay networks.
- Enabling TCP relay support would allow connectivity through restrictive NATs and firewalls where UDP is blocked.
- Wrapping transports with Noise-IK provides forward secrecy beyond the standard Tox encryption.
- `NegotiatingTransport` enables seamless interoperability between legacy and Noise-IK peers.
- **Tor transport** is not supported. Routing Tox traffic through the Tor network (via a local SOCKS5 proxy) would hide the user's IP address from peers and bootstrap nodes, providing strong anonymity.
- **I2P transport** is not supported. Routing Tox traffic through the I2P network (via the SAM bridge or I2P SOCKS proxy) would provide an alternative anonymizing overlay with garlic routing, offering strong anonymity and resistance to traffic analysis.

### Gap 14: Bootstrap Node Mode

**Status:** Missing — go-ratox only connects to bootstrap nodes as a client; it cannot act as one.

**Files:** `client/client.go`, `config/config.go`, `main.go`

**Details:**
- go-ratox currently bootstraps by connecting to external DHT nodes listed in the configuration, but it does not expose functionality to operate as a DHT bootstrap or relay node itself.
- The toxcore API supports DHT node operation and TCP relay serving, which would allow a go-ratox instance to help other Tox clients join the network.
- Acting as a bootstrap node requires listening on a configurable address/port, advertising the node's public key, and serving DHT and relay requests.
- This would enable self-hosted Tox infrastructure without relying on third-party public bootstrap nodes.

---

## Step-by-Step Completion Plan

### Phase 1: Critical Fixes (Core Functionality)

These steps fix broken or incomplete core features.

#### Step 1.1: Complete File Receive Chunk Handling ✅

**Status:** COMPLETE

**Goal:** Write incoming file data to disk instead of just logging.

Implementation:
- ✅ Added `incomingTransfer` struct to track active incoming transfers
- ✅ Added `incomingTransfers` map to `Client` (key: "friendID:fileNumber")
- ✅ In `handleFileReceive`, create destination file and register transfer when auto-accepting
- ✅ In `handleFileReceiveChunk`, write chunk data to file at correct position
- ✅ Handle transfer completion (empty chunk) by closing file and notifying via `file_out` FIFO
- ✅ Refactored into helper functions to maintain low complexity

#### Step 1.2: Complete File Send Chunk Handling ✅

**Status:** COMPLETE

**Goal:** Read file data from disk and send chunks when requested.

Implementation:
- ✅ Added file opening and tracking in `handleFriendFileIn` after `FileSend` succeeds
- ✅ Registered outgoing transfer with file handle in `outgoingTransfers` map
- ✅ Implemented `handleFileChunkRequest` to read chunks from file at specified position
- ✅ Send chunks via `FileSendChunk` with proper error handling
- ✅ Added `completeFileSend` to clean up file handle and notify via FIFO
- ✅ Handle EOF properly by sending nil chunk to signal completion

#### Step 1.3: Implement File Transfer Rejection ✅

**Status:** COMPLETE

**Goal:** Properly reject oversized file transfers.

Implementation:
- ✅ In `handleFileReceive`, check file size against `MaxFileSize`
- ✅ Call `FileControl` with `FileControlCancel` to reject oversized files
- ✅ Log the rejection with file size details

### Phase 2: Connection & Status Tracking

These steps ensure go-ratox accurately tracks friend and self connection state.

#### Step 2.1: Register `OnFriendConnectionStatus` Callback ✅

**Status:** COMPLETE

**Goal:** Track when friends come online or go offline.

Implementation:
- ✅ Added `OnFriendConnectionStatus` callback in `setupCallbacks`
- ✅ Implemented `handleFriendConnectionStatusChange` in `handlers.go`
- ✅ Updates `friend.Online` based on `status != ConnectionNone`
- ✅ Writes connection status to friend's `status` FIFO with proper status strings (offline/online (TCP)/online (UDP))
- ✅ Logs connection status changes in debug mode

#### Step 2.2: Register `OnConnectionStatus` Callback ✅

**Status:** COMPLETE

**Goal:** React to self connection status changes instead of polling.

Implementation:
- ✅ In `setupCallbacks`, added `OnConnectionStatus` callback registration
- ✅ Implemented `handleSelfConnectionStatusChange` to update the connection status file immediately
- ✅ Handler logs status changes in debug mode
- ✅ Connection status file is now updated event-driven instead of polling-only

#### Step 2.3: Register `OnFriendStatusMessage` Callback ✅

**Status:** COMPLETE

**Goal:** Track when friends change their status message.

Implementation:
- ✅ Added `StatusMessage` field to `Friend` struct in `client.go`
- ✅ Added `FriendStatusMessage` FIFO constant for friend-specific status messages
- ✅ Implemented `handleFriendStatusMessageChange` handler in `handlers.go`
- ✅ Registered `OnFriendStatusMessage` callback in `setupCallbacks`
- ✅ Created `WriteFriendStatusMessage` method to write to friend's status_message FIFO
- ✅ Added friend status_message FIFO to `CreateFriendFIFOs` list

### Phase 3: Use Improved toxcore APIs

These steps adopt better APIs available in the updated toxcore.

#### Step 3.1: Use `NewFromSavedata` for State Restoration ✅

**Status:** COMPLETE

**Goal:** Properly restore full Tox state from save data.

1. Modify `initTox()` in `client.go`:
   ```go
   if saveData, err := os.ReadFile(c.config.SaveFile); err == nil {
       tox, err := toxcore.NewFromSavedata(options, saveData)
       if err != nil {
           return fmt.Errorf("failed to restore Tox from savedata: %w", err)
       }
       c.tox = tox
   } else {
       tox, err := toxcore.New(options)
       if err != nil {
           return fmt.Errorf("failed to create Tox instance: %w", err)
       }
       c.tox = tox
   }
   ```
2. After `NewFromSavedata`, the friend list, name, and status are already restored — skip manual re-setting if present.

#### Step 3.2: Use `IterationInterval()` for Dynamic Tick Rate ✅

**Status:** COMPLETE

**Goal:** Use toxcore's recommended iteration interval.

Implementation:
- ✅ Replaced hardcoded 50ms ticker with dynamic `IterationInterval()`
- ✅ Changed from ticker-based select to default case with sleep
- ✅ Maintains responsive shutdown behavior via select statement
- ✅ Tests pass with race detection

#### Step 3.3: Use `FriendByPublicKey` for Lookups ✅

**Status:** COMPLETE

**Goal:** Use toxcore's built-in friend lookup instead of iterating the map.

Implementation:
- ✅ Replaced manual friend lookups in `handleFriendTextIn` with `tox.FriendByPublicKey(publicKey)`
- ✅ Replaced manual friend lookups in `handleFriendFileIn` with `tox.FriendByPublicKey(publicKey)`
- ✅ Reduced cyclomatic complexity of `handleFriendTextIn` from 9 to 7 (22.2% improvement)
- ✅ Reduced cyclomatic complexity of `handleFriendFileIn` from 12 to 10 (16.7% improvement)
- ✅ Eliminated need to hold read lock while iterating friends map
- ✅ Tests pass with race detection

### Phase 4: New Features

These steps add new capabilities exposed by the updated toxcore.

#### Step 4.1: Friend Deletion Support ✅

**Status:** COMPLETE

**Goal:** Allow users to remove friends through the filesystem interface.

Implementation:
- ✅ Added `RemoveIn` constant to FIFO names
- ✅ Added `remove_in` FIFO to each friend's directory
- ✅ Monitor `remove_in` in `monitorFriendFIFOs` with dedicated goroutine
- ✅ Implemented `handleFriendRemoveIn` to validate confirmation ("confirm" or friend ID)
- ✅ Extract friend number using `FriendByPublicKey` toxcore API
- ✅ Call `tox.DeleteFriend(friendID)` to remove from Tox
- ✅ Remove friend from `c.friends` map
- ✅ Clean up friend's FIFO directory with `os.RemoveAll`
- ✅ Save Tox state after deletion
- ✅ Refactored into helper functions to maintain low complexity (main handler: 7, under threshold of 10)

#### Step 4.2: Typing Notifications

**Goal:** Surface typing indicators through the filesystem.

1. Register `OnFriendTyping` callback:
   ```go
   c.tox.OnFriendTyping(func(friendID uint32, isTyping bool) {
       c.handleFriendTyping(friendID, isTyping)
   })
   ```
2. Add a `typing` file to each friend's directory showing typing state.
3. Optionally send typing notifications when a user opens a friend's `text_in` FIFO.

#### Step 4.3: Conference/Group Chat Support

**Goal:** Add basic group chat via FIFO interface.

1. Add a `conference_in` global FIFO to create/join conferences.
2. For each active conference, create a directory:
   ```
   conferences/<conference_id>/
   ├── text_in       # Send messages
   ├── text_out      # Receive messages
   ├── invite_in     # Invite friends (write friend public key)
   └── members       # List of members (read-only)
   ```
3. Wire up `ConferenceNew`, `ConferenceInvite`, `ConferenceSendMessage`.

#### Step 4.4: Async (Offline) Messaging

**Goal:** Ensure offline messages are delivered and surfaced.

1. Register `OnAsyncMessage` callback:
   ```go
   c.tox.OnAsyncMessage(func(senderPK [32]byte, message string, messageType async.MessageType) {
       // Find friend by public key and write to their text_out
   })
   ```
2. Messages sent to offline friends are already queued by toxcore's async manager — no FIFO changes needed for outgoing.
3. Incoming async messages should appear in the same `text_out` FIFO as real-time messages, with a marker indicating they were delivered asynchronously.

#### Step 4.5: Alternate Transport Support

**Goal:** Enable TCP relay, encrypted transport layers, and anonymizing overlay networks (Tor, I2P) for improved connectivity, security, and privacy.

1. Add transport configuration fields to `Config`:
   ```go
   type TransportConfig struct {
       TCPEnabled       bool   `json:"tcp_enabled"`
       TCPPort          uint16 `json:"tcp_port"`
       NoiseEnabled     bool   `json:"noise_enabled"`
       NegotiateVersion bool   `json:"negotiate_version"`
       TorEnabled       bool   `json:"tor_enabled"`
       TorSOCKSAddr     string `json:"tor_socks_addr"`  // e.g. "127.0.0.1:9050"
       I2PEnabled       bool   `json:"i2p_enabled"`
       I2PSAMAddr       string `json:"i2p_sam_addr"`    // e.g. "127.0.0.1:7656"
   }
   ```
2. In `initTox()`, configure the transport based on settings:
   ```go
   if c.config.Transport.TCPEnabled {
       options.TCPEnabled = true
       options.TCPPort = c.config.Transport.TCPPort
   }
   ```
3. When `NoiseEnabled` is true, wrap the transport with `transport.NewNoiseTransport()` for Noise-IK forward secrecy.
4. When `NegotiateVersion` is true, use `transport.NewNegotiatingTransport()` to support automatic fallback between Noise-IK and legacy protocols.
5. When `TorEnabled` is true, route all Tox TCP traffic through a local Tor SOCKS5 proxy:
   - Use the configured `TorSOCKSAddr` (defaulting to `127.0.0.1:9050`) to establish a SOCKS5 connection.
   - Disable UDP when Tor is active (Tor only supports TCP streams), falling back to TCP relay mode automatically.
   - Wrap outgoing connections with `golang.org/x/net/proxy` or a SOCKS5 dialer to tunnel through Tor.
   - Support `.onion` addresses for bootstrap nodes and friends running Tor hidden services.
   - Ensure DNS resolution is performed through Tor to prevent DNS leaks.
6. When `I2PEnabled` is true, route all Tox traffic through the I2P network:
   - Connect to the local I2P router via the SAM (Simple Anonymous Messaging) bridge at `I2PSAMAddr` (defaulting to `127.0.0.1:7656`).
   - Create an I2P session and use it for all Tox connections, providing garlic-routed anonymity.
   - Disable UDP when I2P is active, using I2P streaming (TCP-like) connections instead.
   - Generate and expose a `.b32.i2p` destination address for the node so other I2P-based Tox peers can connect.
   - Support `.b32.i2p` and full base64 I2P destination addresses for bootstrap nodes and friends.
7. Add a global `transport` status file exposing the active transport type (UDP, TCP, Noise-IK, Tor, I2P).
8. Update default configuration to include transport options (all alternate transports disabled by default for backward compatibility).
9. Validate transport configuration at startup:
   - Tor and I2P are mutually exclusive (cannot be enabled simultaneously, as each requires exclusive control over the connection routing layer).
   - Tor and I2P require TCP mode (UDP is automatically disabled when either is active).
   - Warn and continue without the overlay network if the configured SOCKS5 or SAM address is unreachable, falling back to direct TCP/UDP connectivity.

#### Step 4.6: Bootstrap Node Mode

**Goal:** Allow go-ratox to act as a DHT bootstrap and relay node for other Tox clients.

1. Add bootstrap node configuration fields to `Config`:
   ```go
   type BootstrapServerConfig struct {
       Enabled    bool   `json:"enabled"`
       ListenAddr string `json:"listen_addr"`
       ListenPort uint16 `json:"listen_port"`
       Motd       string `json:"motd"`
       TCPRelay   bool   `json:"tcp_relay"`
   }
   ```
2. Implement a `BootstrapServer` component in a new `bootstrap/` package:
   - Listen on the configured address/port for incoming DHT requests.
   - Serve DHT node discovery responses to joining clients.
   - Optionally serve as a TCP relay for peers behind restrictive NATs.
3. Expose the node's public key and address for other clients to use:
   - Write to a `bootstrap_info` file in the config directory containing the node's address, port, and public key.
4. Add a CLI flag (`--bootstrap-node`) or config option to start in bootstrap node mode.
5. When running as a bootstrap node, the client can optionally also operate as a regular Tox client simultaneously.
6. Log bootstrap node activity (connections served, relay sessions) for monitoring.

### Phase 5: Robustness & Quality

#### Step 5.1: Add Client Package Tests

**Goal:** Add unit tests for client logic.

1. Create `client/client_test.go` with tests for:
   - `SendMessage` validation (empty, too long, UTF-8 byte counting)
   - Friend map operations (add, get, accept)
   - Name/status message updates
2. Create `client/fifo_test.go` with tests for:
   - FIFO path generation
   - Input parsing (Tox ID formats, message types)
   - Handler dispatching

#### Step 5.2: Improve Error Handling in File Transfers

**Goal:** Handle edge cases in file transfers.

1. Handle disk-full errors when writing received file chunks.
2. Handle file-not-found errors when reading chunks for sending.
3. Implement transfer timeout/cancellation for stalled transfers.
4. Clean up partial files on transfer failure.

#### Step 5.3: Add Integration Test

**Goal:** Test the full client lifecycle using toxcore's test infrastructure.

1. Use `toxcore.NewOptionsForTesting()` to create lightweight Tox instances.
2. Create two clients, add each other as friends, exchange messages, verify FIFO output.
3. Test file transfer end-to-end between two local clients.

### Phase 6: Documentation & Polish

#### Step 6.1: Update README.md

1. Document new features: friend deletion, typing indicators, conference support.
2. Update filesystem interface diagram with new FIFOs.
3. Update version to reflect new toxcore dependency.

#### Step 6.2: Update AUDIT.md

1. Document the toxcore dependency update.
2. Add entries for any new issues discovered during implementation.

#### Step 6.3: Add Examples

1. Add example scripts for conference chat.
2. Add example scripts for file transfer monitoring.
3. Add example for friend deletion.

---

## Priority Order

| Priority | Steps | Effort | Impact |
|----------|-------|--------|--------|
| P0 | 1.1, 1.2, 1.3 | Medium | Completes file transfer — a documented core feature |
| P1 | 2.1, 2.2, 2.3 | Low | Fixes friend online/offline tracking |
| P2 | 3.1, 3.2, 3.3 | Low | Adopts better APIs for correctness and performance |
| P3 | 4.1, 4.2 | Medium | Friend management and UX improvements |
| P4 | 5.1, 5.2, 5.3 | Medium | Quality and test coverage |
| P5 | 4.3, 4.4 | High | New features (group chat, offline messaging) |
| P6 | 4.5 | Medium | Alternate transports (TCP, Noise-IK, Tor, I2P) for connectivity, security, and anonymity |
| P7 | 4.6 | High | Bootstrap node mode for self-hosted Tox infrastructure |
| P8 | 6.1, 6.2, 6.3 | Low | Documentation updates |

## Verification

After each phase, verify:

```bash
# Build
go build ./...

# Unit tests
go test ./...

# Race detection
go test -race ./...

# Vet
go vet ./...

# Integration test (when implemented)
./test_ratox.sh
```

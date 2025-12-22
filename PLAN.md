# CDP Tool Implementation Plan

A context-efficient CLI tool for LLM agents to interact with the web via the Chrome DevTools Protocol.

## Project Overview

This tool manages local Chromium installations and provides raw CDP message exchange over WebSockets. It prioritizes minimal output suitable for LLM consumption while maintaining full CDP capability.

## Architecture

```
cdp (binary)
├── cmd/           # Cobra command implementations
├── chromium/      # Chromium download/install/process management
├── cdp/           # CDP WebSocket client and message handling
└── main.go        # Entry point
```

## Commands

### `cdp chromium install`

Downloads and installs a private local copy of Chromium for Testing.

**Flags:**
- `--channel` (`stable`|`beta`|`dev`|`canary`) - Release channel, default `stable`
- `--path` - Custom install location, default `~/.cdp/chromium`

**Behavior:**
1. Fetch version info from `https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json`
2. Detect platform (`darwin-arm64`, `darwin-x64`, `linux64`, `win64`)
3. Download appropriate `chrome` zip archive
4. Extract to install path with version subdirectory
5. Create `current` symlink pointing to installed version

**Output:** Path to installed binary, or error.

### `cdp chromium uninstall`

Removes installed Chromium.

**Flags:**
- `--version` - Specific version to remove, default removes all
- `--path` - Custom install location

**Output:** Confirmation or error.

### `cdp chromium upgrade`

Upgrades to latest version of the configured channel.

**Flags:**
- `--channel` - Release channel to upgrade to

**Behavior:**
1. Download new version (same as install)
2. Update `current` symlink
3. Optionally clean old versions

**Output:** New version path or "already up to date".

### `cdp browser start`

Starts a new Chromium process with remote debugging enabled.

**Flags:**
- `--name` - Instance identifier for later reference
- `--port` - Remote debugging port (0 = auto-assign)
- `--headless` - Run in headless mode
- `--user-data-dir` - Profile directory, default creates temp dir

**Launch flags applied:**
- `--remote-debugging-port=<port>`
- `--user-data-dir=<dir>` (required per Chrome 136+ security changes)
- `--no-first-run`
- `--remote-allow-origins=*`

**Behavior:**
1. Generate unique instance name if not provided
2. Create user data directory if needed
3. Launch Chromium as background process
4. Wait for debug port to become available
5. Store instance metadata (PID, port, ws URL, user data dir) in `~/.cdp/instances/<name>.json`

**Output:** JSON with `name`, `pid`, `port`, `wsUrl`.

### `cdp browser stop`

Stops a running Chromium instance.

**Flags:**
- `--name` - Instance name (required, or `--all`)
- `--all` - Stop all instances

**Behavior:**
1. Send graceful shutdown via CDP `Browser.close` command
2. Fall back to SIGTERM if needed
3. Clean up instance metadata file
4. Optionally remove temp user data dir

**Output:** Confirmation or error.

### `cdp browser list`

Lists running Chromium instances.

**Output:** JSON array of instances with `name`, `pid`, `port`, `wsUrl`, `started`.

### `cdp send`

Sends a raw CDP command and returns the response.

**Arguments:**
- `<method>` - CDP method (e.g., `Page.navigate`)

**Flags:**
- `--name` - Browser instance name (default: first available)
- `--target` - Target ID or `browser` for browser-level commands
- `--params` - JSON params (can also be piped via stdin)

**Behavior:**
1. Connect to browser's WebSocket
2. If target specified, attach to target session
3. Send CDP command with auto-generated ID
4. Wait for response with matching ID
5. Return result or error

**Output:** Raw CDP response JSON.

### `cdp listen`

Subscribes to CDP events and streams them.

**Arguments:**
- `<domain>` - CDP domain to enable (e.g., `Page`, `Network`)

**Flags:**
- `--name` - Browser instance name
- `--target` - Target ID
- `--filter` - Event name filter (e.g., `Page.loadEventFired`)
- `--count` - Exit after N events

**Behavior:**
1. Connect to target
2. Enable the specified domain
3. Stream matching events to stdout
4. Exit on count reached or interrupt

**Output:** Newline-delimited JSON events.

## CDP Message Format

Commands use JSON-RPC style:
```json
{"id": 1, "method": "Page.navigate", "params": {"url": "https://example.com"}}
```

Responses:
```json
{"id": 1, "result": {"frameId": "...", "loaderId": "..."}}
```

Events (no id):
```json
{"method": "Page.loadEventFired", "params": {"timestamp": 123.456}}
```

Session-scoped commands include `sessionId`:
```json
{"id": 2, "method": "Runtime.evaluate", "sessionId": "abc123", "params": {...}}
```

## Instance Metadata

Stored in `~/.cdp/instances/<name>.json`:
```json
{
  "name": "browser1",
  "pid": 12345,
  "port": 9222,
  "wsUrl": "ws://127.0.0.1:9222/devtools/browser/abc-123",
  "userDataDir": "/tmp/cdp-browser1-xyz",
  "started": "2024-01-15T10:30:00Z"
}
```

## Chromium Installation Layout

```
~/.cdp/
├── chromium/
│   ├── 131.0.6778.69/
│   │   └── chrome-mac-arm64/
│   │       └── Google Chrome for Testing.app/
│   ├── 132.0.6834.57/
│   │   └── ...
│   └── current -> 132.0.6834.57
└── instances/
    ├── browser1.json
    └── browser2.json
```

## Discovery Endpoints

When Chromium runs with `--remote-debugging-port`, it exposes:

- `GET http://127.0.0.1:<port>/json/version` - Browser version and ws URL
- `GET http://127.0.0.1:<port>/json/list` - Available debug targets
- `GET http://127.0.0.1:<port>/json/protocol` - Protocol schema

## Implementation Order

1. **Project scaffold** - Go module, Cobra root command, config paths
2. **Chromium install/uninstall** - Download, extract, symlink management
3. **Browser start/stop** - Process launch, port detection, metadata storage
4. **Browser list** - Instance enumeration with liveness check
5. **CDP send** - WebSocket connection, message exchange, response handling
6. **CDP listen** - Event subscription and streaming
7. **Chromium upgrade** - Version comparison, atomic upgrade

## Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/gorilla/websocket` - WebSocket client
- Standard library for HTTP, JSON, process management, archive extraction

## Error Handling

All commands exit with:
- `0` - Success
- `1` - User/input error (missing args, invalid flags)
- `2` - Runtime error (download failed, process died, CDP error)

Errors print to stderr as plain text. Successful output goes to stdout as JSON where applicable.

## Platform Support

- macOS arm64 (`mac-arm64`)
- macOS x64 (`mac-x64`)
- Linux x64 (`linux64`)
- Windows x64 (`win64`)

Platform detection uses `runtime.GOOS` and `runtime.GOARCH`.

# CDP Tool Improvement Plan

## Current State

All planned commands are implemented and functional (~1,100 lines across 7 files):

| Command | Status |
|---------|--------|
| `chromium install` | ✓ Complete |
| `chromium uninstall` | ✓ Complete |
| `chromium upgrade` | ✓ Complete |
| `browser start` | ✓ Complete |
| `browser stop` | ✓ Complete |
| `browser list` | ✓ Complete |
| `send` | ✓ Complete |
| `listen` | ✓ Complete |

---

## Completed Improvements

### 1. CDP Response Timeout ✓

**Problem**: `send` blocked indefinitely if Chromium never responded.

**Solution**: Added `--timeout` flag (default 30s) with `context.WithTimeout` and select-based response handling.

**Location**: `cmd/send.go`

---

### 2. Resource Leak on Startup Failure ✓

**Problem**: If browser started but metadata save failed, temp user-data-dir was orphaned.

**Solution**: Track temp dir creation with cleanup function, called on all error paths.

**Location**: `cmd/browser.go:147-225`

---

### 3. Unified CDP Connection Code ✓

**Problem**: `cdpConn` and `eventConn` were identical 40-line duplicates.

**Solution**: Extracted to `cmd/cdp.go` as shared `cdpConn` type with `dialCDP(url, withEvents)` constructor.

**Location**: `cmd/cdp.go`

---

### 4. Download Verification ✓

**Problem**: Chromium zip downloaded without integrity verification.

**Solution**: Added content-length verification to detect truncated downloads. Note: SHA256 checksums are not available from the Chrome for Testing API.

**Location**: `cmd/chromium.go:106-129`

---

### 5. Error Handling Improvements ✓

**Problem**: Some errors were discarded without reporting.

**Solution**:
- `stop --all`: Now tracks and reports failed instances
- `list`: Logs warning for stale instance cleanup failures

**Location**: `cmd/browser.go:231-256, 294-332`

---

### 6. Safe JSON Construction ✓

**Problem**: Target IDs were interpolated directly into JSON strings.

**Solution**: Use `json.Marshal` for all CDP message construction to prevent injection issues.

**Location**: `cmd/send.go`, `cmd/listen.go`

---

## Remaining Items

### 7. No Exit Code Differentiation

**Problem**: All errors exit with code 1. Original spec distinguishes user errors (1) from runtime errors (2).

**Tasks**:
- [ ] Define error types (user vs runtime)
- [ ] Configure Cobra exit codes accordingly

---

### 8. No Debug Mode

**Problem**: No visibility into WebSocket frames or HTTP requests for troubleshooting.

**Tasks**:
- [ ] Add `--verbose` global flag
- [ ] Log WebSocket traffic when enabled
- [ ] Log HTTP requests for version discovery

---

### 9. SIGTERM Instead of CDP Shutdown

**Problem**: `browser stop` uses SIGTERM. Original spec suggested `Browser.close` CDP method.

**Location**: `cmd/browser.go:258-283`

**Tasks**:
- [ ] Try `Browser.close` via WebSocket first
- [ ] Fall back to SIGTERM if WS unavailable
- [ ] Keep SIGKILL as final fallback

---

## Execution Order

| Priority | Item | Status |
|----------|------|--------|
| 1 | CDP timeout | ✓ Done |
| 2 | Resource leak fix | ✓ Done |
| 3 | Deduplicate CDP code | ✓ Done |
| 4 | Download verification | ✓ Done |
| 5 | Error handling fixes | ✓ Done |
| 6 | Safe JSON construction | ✓ Done |
| 7 | Exit code differentiation | Pending |
| 8 | Debug mode | Pending |
| 9 | CDP shutdown | Pending |

---

## Non-Goals

These are explicitly out of scope:
- Config file support (.cdp.yml)
- Additional CDP convenience commands
- Multi-architecture Windows/Linux
- Auto-update daemon
- WebDriver/Puppeteer compatibility

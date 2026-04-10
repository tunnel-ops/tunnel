# Testing Guide

This document covers all testing steps for `requests` — from automated tests through full end-to-end validation.

---

## Prerequisites

- Go 1.21+
- macOS (for keychain and launchd tests; Linux skips those steps)
- `cloudflared` installed (required for setup wizard end-to-end)
- A domain you control (e.g. `greennote.app`) with one of: Cloudflare, GoDaddy, Namecheap, or manual DNS access

---

## 1. Automated Tests

```bash
# Run all unit tests
go test ./...

# Run with verbose output
go test -v ./...

# Run a specific package
go test -v ./internal/dns/...
go test -v ./internal/proxy/...
go test -v ./internal/names/...
```

### What each package tests

| Package | Tests |
|---|---|
| `internal/dns` | Namecheap XML parsing (OK + ERROR), `splitDomain`, GoDaddy PUT request construction + auth header, public IP detection |
| `internal/proxy` | `ExtractPort` — valid ports, wrong domain, nested subdomain, out-of-range ports, non-numeric subdomain; default blocked ports |
| `internal/names` | Add/Lookup/Remove/List on a temp-file-backed store |
| `internal/appconfig` | `AutoUpdate` and `LastUpdateCheck` round-trip through JSON |
| `cmd/tunnel` | `parseTargets` — `--open` flag stripping from mixed arg lists; `isNewerVersion` — semver comparison including multi-digit components |

**Expected output:**
```
ok  github.com/tunnel-ops/tunnel/internal/dns
ok  github.com/tunnel-ops/tunnel/internal/proxy
ok  github.com/tunnel-ops/tunnel/internal/names
```

---

## 2. Build

```bash
make build
```

Produces `bin/tunnel` and `bin/requests-proxy`. Verify:

```bash
./bin/tunnel help
./bin/requests-proxy --help 2>&1 || true   # exits 1 but prints usage
```

---

## 3. tunnel CLI — Offline Commands

These work without a running proxy or domain.

### Help

```bash
./bin/tunnel help
./bin/tunnel -h
./bin/tunnel --help
```

Expected: prints usage text with all commands listed.

### Port validation

```bash
# Valid port — warns nothing is listening yet
DOMAIN=example.com ./bin/tunnel 8080

# Invalid ports
DOMAIN=example.com ./bin/tunnel 80      # error: out of range
DOMAIN=example.com ./bin/tunnel 0       # error: out of range
DOMAIN=example.com ./bin/tunnel 65536   # error: out of range
DOMAIN=example.com ./bin/tunnel abc     # error: not a number
```

### List (empty)

```bash
DOMAIN=example.com ./bin/tunnel list
```

Expected: `No active services found.`

---

## 4. tunnel CLI — Port Registration & Close

```bash
# Start a dummy server on 9999
python3 -m http.server 9999 &
SERVER_PID=$!

DOMAIN=example.com ./bin/tunnel 9999
# Expected: https://9999.example.com  (no warning — port is listening)

DOMAIN=example.com ./bin/tunnel list
# Expected: 9999 listed with ● active marker

DOMAIN=example.com ./bin/tunnel close 9999
# Expected: ○ https://9999.example.com  removed

DOMAIN=example.com ./bin/tunnel list
# Expected: nothing active (9999 removed from store)

kill $SERVER_PID
```

---

## 4a. tunnel CLI — Multi-Port Open & Close

```bash
python3 -m http.server 9001 &
python3 -m http.server 9002 &

DOMAIN=example.com ./bin/tunnel 9001 9002
# Expected: single branded header listing both URLs

DOMAIN=example.com ./bin/tunnel list
# Expected: both 9001 and 9002 listed with ● markers

DOMAIN=example.com ./bin/tunnel close 9001 9002
# Expected: single header with both entries removed

DOMAIN=example.com ./bin/tunnel list
# Expected: nothing active

kill %1 %2
```

Close with unknown key aborts before removing any:

```bash
DOMAIN=example.com ./bin/tunnel 9001
DOMAIN=example.com ./bin/tunnel close 9001 unknown-key
# Expected: error about "unknown-key", 9001 still in store

DOMAIN=example.com ./bin/tunnel close 9001
```

---

## 5. tunnel CLI — Named Routes

```bash
python3 -m http.server 8080 &
SERVER_PID=$!

DOMAIN=example.com ./bin/tunnel --name api 8080
# Expected: https://api.example.com

DOMAIN=example.com ./bin/tunnel list
# Expected: "Named routes:" with api → :8080

DOMAIN=example.com ./bin/tunnel close api
# Expected: closed: api.example.com

DOMAIN=example.com ./bin/tunnel close api
# Expected: error: no registered tunnel for "api"

# rm is an alias for close
DOMAIN=example.com ./bin/tunnel --name ui 3000
DOMAIN=example.com ./bin/tunnel rm ui

kill $SERVER_PID
```

---

## 5a. tunnel CLI — Block & Unblock

```bash
python3 -m http.server 9999 &
SERVER_PID=$!

# Block an active port — interactive prompt appears
DOMAIN=example.com ./bin/tunnel block 9999
# Select "Stop process and block"
# Expected: ✓ :9999 blocked

# Trying to register a blocked port shows a styled error
DOMAIN=example.com ./bin/tunnel 9999
# Expected: ⊘ port 9999 is blocked  (no registration)

# Named tunnel to a blocked port also errors
DOMAIN=example.com ./bin/tunnel --name svc 9999
# Expected: ⊘ port 9999 is blocked

# Multi-port with one blocked aborts early
DOMAIN=example.com ./bin/tunnel 8080 9999
# Expected: ⊘ port 9999 is blocked, nothing registered

# list -a shows Blocked section
DOMAIN=example.com ./bin/tunnel list -a
# Expected: "Blocked" section with ⊘ 9999 entry

# Unblock and register succeeds
DOMAIN=example.com ./bin/tunnel unblock 9999
# Expected: ✓ :9999 unblocked

DOMAIN=example.com ./bin/tunnel 9999
# Expected: https://9999.example.com

DOMAIN=example.com ./bin/tunnel close 9999
kill $SERVER_PID
```

---

## 5b. tunnel CLI — Watch (live request monitor)

```bash
python3 -m http.server 9000 &
DOMAIN=example.com ./bin/tunnel 9000

# In another terminal
DOMAIN=example.com ./bin/tunnel watch
# Expected: TUI showing live requests; press q or ctrl+c to exit

# Filter to one port
DOMAIN=example.com ./bin/tunnel watch 9000
# Expected: same TUI filtered to :9000 only

# Send a test request
curl -s -H "Host: 9000.example.com" http://localhost:7999/ > /dev/null
# Expected: request appears in watch TUI

kill %1
DOMAIN=example.com ./bin/tunnel close 9000
```

---

## 5c. tunnel CLI — Auto-update

```bash
# Check for updates and apply if available
./bin/tunnel update
# Expected: shows current version, "already up to date" if on latest,
#           or downloads and replaces binary

# Enable/disable automatic updates
./bin/tunnel update --enable
# Expected: ✓ auto-updates enabled

./bin/tunnel update --disable
# Expected: ✓ auto-updates disabled

# When enabled, running any tunnel command checks for updates once per day
# (throttled — check ~/.config/requests/config.json for lastUpdateCheck field)
```

---

## 6. Proxy — Local Routing

Start the proxy and a backend, then verify routing via curl.

```bash
# Terminal 1: start a test backend
python3 -m http.server 8080

# Terminal 2: start the proxy
DOMAIN=example.com ./bin/requests-proxy

# Terminal 3: send a request via the proxy, spoofing the Host header
curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: 8080.example.com" \
  http://localhost:7999/
# Expected: 200

# Wrong domain — rejected
curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: 8080.wrong.com" \
  http://localhost:7999/
# Expected: 403

# Blocked port
curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: 3306.example.com" \
  http://localhost:7999/
# Expected: 403

# Port not listening — bad gateway
curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: 9876.example.com" \
  http://localhost:7999/
# Expected: 502

# Nested subdomain — rejected
curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: sub.8080.example.com" \
  http://localhost:7999/
# Expected: 403
```

### Named route via proxy

```bash
DOMAIN=example.com ./bin/tunnel --name myapp 8080

curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: myapp.example.com" \
  http://localhost:7999/
# Expected: 200
```

---

## 7. Proxy — Body Size Limit

```bash
# Generate a file larger than the 10 MB default
dd if=/dev/urandom bs=1M count=11 of=/tmp/large.bin 2>/dev/null

curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: 8080.example.com" \
  -X POST --data-binary @/tmp/large.bin \
  http://localhost:7999/
# Expected: 413 or 502 (backend may reject, but proxy enforces the limit)
```

---

## 8. Proxy — Logs

The proxy logs JSON to stdout. Verify the fields are present:

```bash
DOMAIN=example.com ./bin/requests-proxy &
curl -s -H "Host: 8080.example.com" http://localhost:7999/ > /dev/null
# Expected log line contains: "method", "host", "port", "path", "status", "latency_ms"
```

---

## 9. setup Wizard — TUI Smoke Test

This tests the Bubble Tea TUI without performing actual Cloudflare operations.

```bash
./bin/tunnel setup
```

Walk through each screen manually:

| Step | Action | Expected |
|---|---|---|
| Provider picker | Press ↑↓ | Cursor moves between choices |
| Provider picker | Press Enter on "Cloudflare" | Advances to domain input |
| Domain input | Type `test.com`, press Enter | Advances to tunnel name input |
| Tunnel name | Press Enter (blank) | Uses default "dev", advances to confirm |
| Confirm screen | Press ↑↓ | Cursor moves between checkboxes |
| Confirm screen | Press Space | Toggles `[✓]` ↔ `[ ]` |
| Any screen | Press Ctrl+C | Exits cleanly, terminal restored |

---

## 10. setup Wizard — GoDaddy / Namecheap Credential Screen

```bash
# Clear any stored credentials first (if testing fresh)
security delete-generic-password -s requests-tunnel -a godaddy-key 2>/dev/null || true
security delete-generic-password -s requests-tunnel -a godaddy-secret 2>/dev/null || true

./bin/tunnel setup
# Select: GoDaddy
```

| Step | Action | Expected |
|---|---|---|
| Credentials screen | Type in "API Key" field | Characters visible |
| Credentials screen | Press Tab | Focus moves to "API Secret" |
| Credentials screen | Type in "API Secret" | Characters masked as `·` |
| Credentials screen | Press Shift+Tab | Focus returns to "API Key" |
| Credentials screen | Press Enter with empty field | Does not advance |
| Credentials screen | Fill both fields, Enter | Saves to keychain, advances to confirm |

Verify credentials were saved to keychain:

```bash
security find-generic-password -s requests-tunnel -a godaddy-key -w
security find-generic-password -s requests-tunnel -a godaddy-secret -w
```

Re-run setup — credential screen should be skipped:

```bash
./bin/tunnel setup
# Select: GoDaddy
# Expected: jumps straight to confirm screen (skips creds since already in keychain)
```

---

## 11. setup Wizard — Idempotency

After a successful `tunnel setup`, re-running it should skip all already-completed steps:

```bash
./bin/tunnel setup
```

In the running screen, steps should show:
- `–` (skipped) for: cloudflared check, Cloudflare auth, Create tunnel
- `✓` (done) for remaining steps

---

## 12. setup Wizard — Domain Persistence

After `tunnel setup` completes, `DOMAIN` env var should no longer be required:

```bash
# No DOMAIN set
./bin/tunnel list
# Expected: reads domain from ~/.config/requests/config.json — no error

./bin/tunnel 8080
# Expected: prints https://8080.your-domain.com
```

---

## 13. setup Wizard — Manual DNS Provider

```bash
./bin/tunnel setup
# Select: Manual
# Fill domain and tunnel name
# Complete Cloudflare auth and tunnel creation steps
```

At the DNS step, the TUI suspends and shows in the terminal:

```
  Create this DNS record:

    *.your-domain.com  CNAME  <tunnel-id>.cfargotunnel.com

  Press Enter once the record is live...
```

Press Enter → TUI resumes, remaining steps complete.

---

## 14. End-to-End (Full Public URL Test)

Prerequisites: `tunnel setup` completed successfully, `cloudflared` running.

```bash
# Start a backend
python3 -m http.server 5173

# Register the port
tunnel 5173
# Prints: https://5173.your-domain.com

# Verify the proxy is routing locally
curl -s -o /dev/null -w "%{http_code}" \
  -H "Host: 5173.your-domain.com" \
  http://localhost:7999/
# Expected: 200

# Test the public URL (requires cloudflared tunnel to be running)
curl -s -o /dev/null -w "%{http_code}" https://5173.your-domain.com
# Expected: 200

# Named route
tunnel --name frontend 5173 --open
# Expected: opens https://frontend.your-domain.com in browser

# Clean up
tunnel close 5173
tunnel close frontend
```

---

## 15. Install Script (dry run)

Verify the install script works against a tagged release:

```bash
# Inspect the script before running
less install.sh

# Dry-run: override INSTALL_DIR to avoid writing to /usr/local/bin
INSTALL_DIR=/tmp/requests-test VERSION=v0.1.0 bash install.sh

ls -la /tmp/requests-test/
# Expected: tunnel  requests-proxy  (both executable)

/tmp/requests-test/tunnel help
```

---

## 16. Config File Verification

After `tunnel setup`:

```bash
# App config (no secrets)
cat ~/.config/requests/config.json
# Expected fields: domain, provider, tunnelId, tunnelName, proxyPort
# Must NOT contain: api keys, secrets, passwords

# File permissions
stat -f "%Sp %N" ~/.config/requests/config.json
# Expected: -rw------- (0600)

# cloudflared config
cat ~/.cloudflared/config.yml
# Expected: tunnel ID, credentials-file path, wildcard ingress rule
```

---

## Checklist Summary

```
[ ] go test ./...                                   passes
[ ] make build                                      produces both binaries
[ ] tunnel help                                     displays usage with all commands
[ ] tunnel <port> (invalid)                         correct error messages
[ ] tunnel 9999 / list / close 9999                register, show, remove
[ ] tunnel 9001 9002 / close 9001 9002             multi-port open and close
[ ] tunnel close 9001 unknown — aborts before any removal
[ ] tunnel --name api 8080 / close api             named route lifecycle
[ ] tunnel block 9999                              interactive prompt, process killed, blocked
[ ] tunnel 9999 after block                        ⊘ styled error, not registered
[ ] tunnel --name svc 9999 after block             ⊘ styled error
[ ] tunnel list -a after block                     Blocked section visible
[ ] tunnel unblock 9999 / tunnel 9999              succeeds after unblock
[ ] tunnel watch                                   TUI renders, q exits cleanly
[ ] tunnel watch <port>                            TUI filters to port
[ ] tunnel update                                  shows version, applies if newer
[ ] tunnel update --enable / --disable             toggles autoUpdate in config.json
[ ] proxy routes valid Host headers                200
[ ] proxy rejects wrong domain / blocked ports     403
[ ] proxy returns 502 for dead backends            502
[ ] setup TUI — all screens render                 no panic, ctrl+c restores terminal
[ ] setup TUI — creds screen masks secrets         · characters shown
[ ] setup TUI — skip creds if already stored       no prompt on re-run
[ ] setup TUI — domain persisted                   no DOMAIN env needed after setup
[ ] setup TUI — idempotent re-run                  skipped steps shown with –
[ ] config.json has 0600 permissions               stat confirms
[ ] config.json contains no secrets               grep for key/secret is empty
[ ] full public URL resolves                       curl https://<port>.domain returns 200
```

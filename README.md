# tunnel

Expose any local port as a public URL in seconds.

```
tunnel 8080
# https://8080.yourdomain.com
```

`tunnel` runs a local reverse proxy and routes traffic through a [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/). Any subdomain matching `<port>.yourdomain.com` is forwarded to `localhost:<port>` — no port forwarding, no firewall rules.

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/bellamy/requests/main/install.sh | bash
```

Installs `tunnel` and `requests-proxy` to `/usr/local/bin`. macOS and Linux (amd64/arm64) are supported.

To install a specific version:

```bash
VERSION=v1.0.0 bash <(curl -fsSL https://raw.githubusercontent.com/bellamy/requests/main/install.sh)
```

---

## Quick start

### 1. Run the setup wizard

```bash
tunnel setup
```

The TUI wizard handles everything in one go:

- Installs `cloudflared` if missing (via Homebrew on macOS)
- Authenticates with Cloudflare
- Creates a named tunnel
- Configures the wildcard DNS record (`*.yourdomain.com`)
- Writes the cloudflared config
- Optionally installs both services to auto-start on login

Supported DNS providers: **Cloudflare**, **GoDaddy**, **Namecheap**, **Manual**.

API credentials for GoDaddy and Namecheap are stored in the macOS Keychain — never written to disk as plaintext.

### 2. Start your service and expose it

```bash
# Your dev server
npm run dev          # running on :5173

# Register and print the public URL
tunnel 5173
# https://5173.yourdomain.com
```

`tunnel` warns if nothing is listening on the port yet and auto-registers it so it shows up in `tunnel list`.

---

## Usage

```
tunnel welcome                      Show welcome screen and quick-start info
tunnel setup                        First-time configuration wizard
tunnel <port>                       Print the public URL for a numeric port
tunnel <port> --open                Print and open in browser
tunnel --name <name> <port>         Register a named subdomain and print URL
tunnel --name <name> <port> --open  Register, print, and open in browser
tunnel close <port|name>            Remove a registered tunnel
tunnel rm <name>                    Alias for close
tunnel list                         List registered tunnels and listening ports
tunnel help                         Show this help
```

### Examples

```bash
# Numeric port
tunnel 3000
# https://3000.yourdomain.com

# Named subdomain
tunnel --name api 8080
# https://api.yourdomain.com

# Open in browser immediately
tunnel --name app 5173 --open

# See everything running
tunnel list

# Remove when done
tunnel close api
tunnel close 3000
```

---

## How it works

```
browser → Cloudflare edge → cloudflared tunnel → requests-proxy (:7999) → localhost:<port>
```

- `**requests-proxy**` is a reverse proxy that listens on a single port (default `7999`). It reads the `Host` header, extracts the subdomain, resolves it to a local port, and forwards the request to `127.0.0.1:<port>`.
- `**cloudflared**` maintains the tunnel from Cloudflare's edge to `requests-proxy`, so inbound traffic never requires an open firewall port.
- Named subdomains (e.g. `api.yourdomain.com`) are registered in `~/.config/requests/names.json` and resolved by the proxy at request time.
- WebSocket connections are proxied transparently (HMR works out of the box).

### Security


| Concern              | Mitigation                                                                                                             |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| SSRF                 | Proxy always forwards to `127.0.0.1` — never follows arbitrary targets                                                 |
| Sensitive ports      | SSH (22), SMTP (25), MySQL (3306), PostgreSQL (5432), Redis (6379), MongoDB (27017+) and others are blocked by default |
| Subdomain validation | Only single-level subdomains matching the configured domain are accepted; nested subdomains return 403                 |
| Body size            | Requests over 10 MB are rejected (configurable via `MAX_BODY_MB`)                                                      |
| API credentials      | Stored in macOS Keychain via the `security` CLI; never written to disk                                                 |


---

## Configuration

After `tunnel setup`, the domain is saved to `~/.config/requests/config.json` and no environment variables are needed for daily use.

You can override any setting with environment variables:


| Variable        | Default         | Description                              |
| --------------- | --------------- | ---------------------------------------- |
| `DOMAIN`        | from config     | Your base domain (e.g. `yourdomain.com`) |
| `PROXY_PORT`    | `7999`          | Port `requests-proxy` listens on         |
| `MAX_BODY_MB`   | `10`            | Maximum request body size in MB          |
| `BLOCKED_PORTS` | (built-in list) | Comma-separated list of blocked ports    |
| `READ_TIMEOUT`  | `30s`           | HTTP read timeout                        |
| `WRITE_TIMEOUT` | `30s`           | HTTP write timeout                       |
| `IDLE_TIMEOUT`  | `120s`          | HTTP idle timeout                        |


---

## Build from source

```bash
git clone https://github.com/bellamy/requests
cd requests
make build
# → bin/tunnel
# → bin/requests-proxy
```

Requires Go 1.21+.

```bash
# Run tests
go test ./...

# Install as background services (after setup)
make install DOMAIN=yourdomain.com
```

---

## License

MIT — see [LICENSE](LICENSE).
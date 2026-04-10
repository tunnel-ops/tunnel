PROXY_BIN  := bin/requests-proxy
TUNNEL_BIN := bin/tunnel
PLIST_NAME := com.bellamy.requests-proxy
PLIST_PATH := $(HOME)/Library/LaunchAgents/$(PLIST_NAME).plist
PREFIX     := /usr/local/bin

VERSION    := $(shell git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
PROXY_LDFLAGS  := -ldflags="-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
TUNNEL_LDFLAGS := -ldflags="-X main.Version=$(VERSION)"

-include .env
export

.PHONY: build run install restart uninstall tunnel-install tunnel-uninstall setup clean

build:
	@mkdir -p bin
	go build $(PROXY_LDFLAGS) -o $(PROXY_BIN) ./cmd/proxy
	go build $(TUNNEL_LDFLAGS) -o $(TUNNEL_BIN) ./cmd/tunnel

run: build
	@if [ -z "$(DOMAIN)" ]; then \
		echo "error: DOMAIN is not set. Run: DOMAIN=your-domain.com make run"; \
		exit 1; \
	fi
	./$(PROXY_BIN)

install: build
	@if [ -z "$(DOMAIN)" ]; then \
		echo "error: DOMAIN is not set in environment or .env"; \
		exit 1; \
	fi
	@mkdir -p $(HOME)/Library/LaunchAgents
	@PROXY_PORT_VAL=$${PROXY_PORT:-7999}; \
	BINARY_ABS="$(PREFIX)/requests-proxy"; \
	python3 -c " \
import sys; \
tpl = '''<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n\
<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n\
<plist version=\"1.0\">\n\
<dict>\n\
  <key>Label</key><string>$(PLIST_NAME)</string>\n\
  <key>ProgramArguments</key>\n\
  <array><string>{binary}</string></array>\n\
  <key>EnvironmentVariables</key>\n\
  <dict>\n\
    <key>DOMAIN</key><string>{domain}</string>\n\
    <key>PROXY_PORT</key><string>{port}</string>\n\
  </dict>\n\
  <key>RunAtLoad</key><true/>\n\
  <key>KeepAlive</key>\n\
  <dict><key>SuccessfulExit</key><false/></dict>\n\
  <key>ThrottleInterval</key><integer>5</integer>\n\
  <key>StandardOutPath</key><string>$(HOME)/Library/Logs/requests-proxy.log</string>\n\
  <key>StandardErrorPath</key><string>$(HOME)/Library/Logs/requests-proxy.log</string>\n\
</dict>\n\
</plist>'''; \
print(tpl.format(binary=sys.argv[1], domain=sys.argv[2], port=sys.argv[3])) \
" "$$BINARY_ABS" "$(DOMAIN)" "$$PROXY_PORT_VAL" > $(PLIST_PATH)
	launchctl load -w $(PLIST_PATH)
	@echo "Proxy installed and started. Logs: $(HOME)/Library/Logs/requests-proxy.log"
	@echo "Installing binaries to $(PREFIX)..."
	@sudo cp $(PROXY_BIN) $(PREFIX)/requests-proxy
	@sudo cp $(TUNNEL_BIN) $(PREFIX)/tunnel
	@echo "requests-proxy and tunnel installed."

restart: build
	sudo cp $(PROXY_BIN) $(PREFIX)/requests-proxy
	sudo cp $(TUNNEL_BIN) $(PREFIX)/tunnel
	sudo launchctl bootout gui/$$(id -u $${SUDO_USER:-$$USER}) $(PLIST_PATH) 2>/dev/null || true
	sudo launchctl bootstrap gui/$$(id -u $${SUDO_USER:-$$USER}) $(PLIST_PATH)
	@echo "requests-proxy and tunnel deployed."

uninstall:
	@if [ -f $(PLIST_PATH) ]; then \
		launchctl unload -w $(PLIST_PATH) && rm -f $(PLIST_PATH); \
		echo "Proxy service removed."; \
	else \
		echo "Service not installed."; \
	fi
	@rm -f $(PREFIX)/requests-proxy && echo "requests-proxy removed." || true
	@rm -f $(PREFIX)/tunnel && echo "tunnel removed." || true

tunnel-install:
	@if [ ! -f "$(HOME)/.cloudflared/config.yml" ]; then \
		echo "error: ~/.cloudflared/config.yml not found."; \
		echo "Copy config/cloudflared.yml there and fill in your Tunnel ID first."; \
		exit 1; \
	fi
	cloudflared service install
	@echo "cloudflared installed as a system service."

tunnel-uninstall:
	cloudflared service uninstall

setup: build
	@./$(TUNNEL_BIN) setup

clean:
	rm -rf bin

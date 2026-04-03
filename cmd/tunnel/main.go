package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bellamy/requests/internal/appconfig"
	"github.com/bellamy/requests/internal/names"
	"github.com/bellamy/requests/internal/setup"
)

const usageText = `tunnel — expose local dev servers via <port>.your-domain.com

Usage:
  tunnel welcome                      Show welcome screen and quick-start info
  tunnel setup                        First-time configuration wizard
  tunnel <port>                       Print the public URL for a numeric port
  tunnel <port> --open                Print and open in browser
  tunnel --name <name> <port>         Register a named subdomain and print URL
  tunnel --name <name> <port> --open  Register, print, and open in browser
  tunnel close <port|name>            Remove a registered tunnel
  tunnel rm <name>                    Alias for close
  tunnel list                         List registered tunnels and listening ports
  tunnel block <port>                 Block a port from being exposed
  tunnel unblock <port>               Remove a port block
  tunnel help                         Show this help

Environment (optional after running setup):
  DOMAIN    Override the configured domain

Examples:
  tunnel setup
  tunnel 5173
  tunnel --name api 8080
  tunnel --name api 8080 --open
  tunnel close 5173
  tunnel close api
  tunnel list
`

func main() {
	args := os.Args[1:]

	if len(args) == 0 || isHelp(args[0]) {
		fmt.Print(usageText)
		return
	}

	// welcome and setup do not need domain or names store
	if args[0] == "welcome" {
		cmdWelcome()
		return
	}

	if args[0] == "setup" {
		if err := setup.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	domain := resolveDomain()

	store, err := names.New("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to open names store: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		cmdList(domain, store)

	case "block":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "error: tunnel block <port>")
			os.Exit(1)
		}
		port, err := parsePort(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		cmdBlock(port)

	case "unblock":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "error: tunnel unblock <port>")
			os.Exit(1)
		}
		port, err := parsePort(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		cmdUnblock(port)

	case "rm", "close":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "error: tunnel %s <port|name>\n", args[0])
			os.Exit(1)
		}
		cmdClose(args[1], domain, store)

	case "--name":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "error: tunnel --name <name> <port>")
			os.Exit(1)
		}
		name := args[1]
		port, err := parsePort(args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		open := len(args) >= 4 && args[3] == "--open"
		cmdNamed(name, port, domain, open, store)

	default:
		port, err := parsePort(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		open := len(args) >= 2 && args[1] == "--open"
		cmdPort(port, domain, open, store)
	}
}

// resolveDomain returns DOMAIN env var or falls back to appconfig.
func resolveDomain() string {
	if d := os.Getenv("DOMAIN"); d != "" {
		return d
	}
	if cfg, err := appconfig.Load(); err == nil && cfg.Domain != "" {
		return cfg.Domain
	}
	fmt.Fprintln(os.Stderr, "error: domain not configured — run 'tunnel setup' first, or set DOMAIN=your-domain.com")
	os.Exit(1)
	return ""
}

const proxyLaunchLabel = "com.bellamy.requests-proxy"

func ensureServicesUp(proxyPort int) string {
	var issues []string

	if !isListening(proxyPort) {
		_ = exec.Command("launchctl", "start", proxyLaunchLabel).Run()
		time.Sleep(800 * time.Millisecond)
		if !isListening(proxyPort) {
			issues = append(issues, fmt.Sprintf("requests-proxy not running on :%d", proxyPort))
		}
	}

	if !setup.IsCloudflaredRunning() {
		if err := setup.StartCloudflaredService(); err != nil {
			issues = append(issues, "could not start cloudflared: "+err.Error())
		} else {
			time.Sleep(1500 * time.Millisecond)
			if !setup.IsCloudflaredRunning() {
				issues = append(issues, "cloudflared tunnel not running — run 'tunnel setup'")
			}
		}
	}

	return strings.Join(issues, ", ")
}

func resolveProxyPort() int {
	cfg, _ := appconfig.Load()
	if cfg != nil && cfg.ProxyPort != 0 {
		return cfg.ProxyPort
	}
	return 7999
}

func cmdPort(port int, domain string, open bool, store *names.Store) {
	_ = store.Add(strconv.Itoa(port), port)
	serviceIssue := ensureServicesUp(resolveProxyPort())
	url := fmt.Sprintf("https://%d.%s", port, domain)
	showTunnelURL(url, strconv.Itoa(port), port, isListening(port), serviceIssue)
	if open && serviceIssue == "" {
		openBrowser(url)
	}
}

func cmdNamed(name string, port int, domain string, open bool, store *names.Store) {
	if err := store.Add(name, port); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not save mapping: %v\n", err)
		os.Exit(1)
	}
	serviceIssue := ensureServicesUp(resolveProxyPort())
	url := fmt.Sprintf("https://%s.%s", name, domain)
	showTunnelURL(url, name, port, isListening(port), serviceIssue)
	if open && serviceIssue == "" {
		openBrowser(url)
	}
}

func killPortProcess(port int) {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return
	}
	for _, pidStr := range strings.Fields(string(out)) {
		_ = exec.Command("kill", pidStr).Run()
	}
}

func cmdBlock(port int) {
	blocked, err := names.NewBlocked("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if isListening(port) {
		if !confirmBlock(port) {
			return
		}
		killPortProcess(port)
	}
	if err := blocked.Add(port); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	showBlocked(port)
}

func cmdUnblock(port int) {
	blocked, err := names.NewBlocked("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := blocked.Remove(port); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	showUnblocked(port)
}

// stopServicesIfIdle stops cloudflared and requests-proxy when no registered
// port has an active listener. Returns true if services were stopped.
func stopServicesIfIdle(store *names.Store) bool {
	for _, port := range store.List() {
		if isListening(port) {
			return false
		}
	}
	_ = exec.Command("launchctl", "stop", proxyLaunchLabel).Run()
	_ = setup.StopCloudflaredService()
	return true
}

func cmdClose(key string, domain string, store *names.Store) {
	if _, ok := store.Lookup(key); !ok {
		fmt.Fprintf(os.Stderr, "error: no registered tunnel for %q — use 'tunnel list' to see active tunnels\n", key)
		os.Exit(1)
	}
	if err := store.Remove(key); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	showClosed(key, domain)
	if stopServicesIfIdle(store) {
		showServicesStopped()
	}
}

func cmdList(domain string, store *names.Store) {
	all := store.List()

	var portKeys, nameKeys []string
	for k := range all {
		if _, err := strconv.Atoi(k); err == nil {
			portKeys = append(portKeys, k)
		} else {
			nameKeys = append(nameKeys, k)
		}
	}

	listeningSet := map[int]struct{}{}
	for _, p := range listeningPorts() {
		listeningSet[p] = struct{}{}
	}
	for _, k := range portKeys {
		delete(listeningSet, all[k])
	}

	unregistered := make([]int, 0, len(listeningSet))
	for p := range listeningSet {
		unregistered = append(unregistered, p)
	}

	showList(domain, nameKeys, portKeys, all, unregistered)
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("port must be a number, got %q", s)
	}
	if n < 1024 || n > 65535 {
		return 0, fmt.Errorf("port %d is out of allowed range [1024, 65535]", n)
	}
	return n, nil
}

func isHelp(s string) bool {
	return s == "help" || s == "-h" || s == "--help"
}

func isListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}


var portRe = regexp.MustCompile(`:(\d+)\s*\(LISTEN\)`)

func listeningPorts() []int {
	out, err := exec.Command("lsof", "-i", "-P", "-n", "-sTCP:LISTEN").Output()
	if err != nil {
		return nil
	}

	seen := map[int]struct{}{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		m := portRe.FindSubmatch(scanner.Bytes())
		if m == nil {
			continue
		}
		p, err := strconv.Atoi(string(m[1]))
		if err != nil || p < 1024 || p > 65535 {
			continue
		}
		seen[p] = struct{}{}
	}

	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		fmt.Fprintf(os.Stderr, "open not supported on %s — visit: %s\n", runtime.GOOS, url)
		return
	}
	_ = cmd.Start()
}

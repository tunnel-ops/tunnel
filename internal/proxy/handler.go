package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tunnel-ops/tunnel/internal/names"
)

// Config holds all runtime configuration for the proxy.
type Config struct {
	Domain       string
	ProxyPort    string
	HealthPort   string
	BlockedPorts map[int]struct{}
	MaxBodyBytes int64
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	Names        *names.Store
	Blocked      *names.Blocked
}

// Handler is the reverse proxy HTTP handler.
type Handler struct {
	cfg       Config
	transport *http.Transport
}

// New returns a new Handler with a tuned transport.
func New(cfg Config) *Handler {
	t := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}
	return &Handler{cfg: cfg, transport: t}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes)

	port, errMsg := h.resolvePort(r.Host)
	if errMsg != "" {
		slog.Warn("rejected request", "host", r.Host, "reason", errMsg)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if _, blocked := h.cfg.BlockedPorts[port]; blocked {
		slog.Warn("blocked port", "host", r.Host, "port", port)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if h.cfg.Blocked != nil && h.cfg.Blocked.Contains(port) {
		slog.Warn("user-blocked port", "host", r.Host, "port", port)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if isWebSocketUpgrade(r) {
		h.proxyWebSocket(w, r, port, start)
		return
	}

	// Strip port from Host (e.g. "9090.greennote.app:443" → "9090.greennote.app").
	publicHost := r.Host
	if h, _, err := net.SplitHostPort(publicHost); err == nil {
		publicHost = h
	}

	target := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", port),
	}

	proxy := &httputil.ReverseProxy{
		Transport: h.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host
			pr.Out.Header.Set("X-Forwarded-Proto", "https")
			pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
			pr.Out.Header.Set("X-Forwarded-For", pr.In.RemoteAddr)
			// Force uncompressed response so body rewriting works on plain text.
			// DisableCompression: true on the transport prevents Go from auto-adding
			// Accept-Encoding: gzip, but we also set identity explicitly here.
			pr.Out.Header.Set("Accept-Encoding", "identity")

			// Rewrite URL-valued query params that reference the public host back
			// to the local backend address. Backends like Keycloak validate
			// redirect_uri against their own registered URI whitelist, which only
			// knows about localhost — not the public proxy hostname.
			if pr.Out.URL.RawQuery != "" {
				if rewritten, ok := rewriteQueryScheme(pr.Out.URL.RawQuery, publicHost); ok {
					pr.Out.URL.RawQuery = rewritten
					slog.Debug("rewrote query params", "path", pr.Out.URL.Path)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Browser-level upgrade: catches any http:// URL constructed at JS runtime
			// that body rewriting might miss (e.g. from XHR-fetched config).
			resp.Header.Add("Content-Security-Policy", "upgrade-insecure-requests")

			// Rewrite server-side redirect Location headers.
			if loc := resp.Header.Get("Location"); strings.HasPrefix(loc, "http://") {
				resp.Header.Set("Location", "https://"+loc[len("http://"):])
			}

			// Rewrite http://host URLs embedded in text response bodies.
			if resp.Body == nil || publicHost == "" || !isTextContentType(resp.Header.Get("Content-Type")) {
				return nil
			}

			// Decompress if the backend ignored Accept-Encoding: identity.
			var reader io.Reader = resp.Body
			if resp.Header.Get("Content-Encoding") == "gzip" {
				gr, err := gzip.NewReader(resp.Body)
				if err != nil {
					return err
				}
				defer gr.Close()
				reader = gr
				resp.Header.Del("Content-Encoding")
			}

			body, err := io.ReadAll(reader)
			_ = resp.Body.Close()
			if err != nil {
				return err
			}

			rewritten := bytes.ReplaceAll(body,
				[]byte("http://"+publicHost),
				[]byte("https://"+publicHost),
			)
			resp.Body = io.NopCloser(bytes.NewReader(rewritten))
			resp.ContentLength = int64(len(rewritten))
			resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error", "port", port, "error", err)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}

	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	proxy.ServeHTTP(rw, r)

	slog.Info("request",
		"method", r.Method,
		"host", r.Host,
		"port", port,
		"path", r.URL.Path,
		"status", rw.status,
		"latency_ms", time.Since(start).Milliseconds(),
	)
}

// resolvePort resolves the target port from the Host header.
// It first tries to parse the subdomain as a numeric port; if that fails and
// a names store is configured, it looks up the subdomain as a named route.
func (h *Handler) resolvePort(host string) (int, string) {
	hostname := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		hostname = hh
	}

	suffix := "." + h.cfg.Domain
	if !strings.HasSuffix(hostname, suffix) {
		return 0, fmt.Sprintf("host %q does not match domain %q", hostname, h.cfg.Domain)
	}

	sub := strings.TrimSuffix(hostname, suffix)
	if strings.Contains(sub, ".") {
		return 0, fmt.Sprintf("nested subdomain not allowed: %q", sub)
	}

	// Try numeric port first.
	if port, err := strconv.Atoi(sub); err == nil {
		if port < 1024 || port > 65535 {
			return 0, fmt.Sprintf("port %d out of allowed range [1024, 65535]", port)
		}
		return port, ""
	}

	// Fall back to named route lookup.
	if h.cfg.Names != nil {
		if port, ok := h.cfg.Names.Lookup(sub); ok {
			return port, ""
		}
		return 0, fmt.Sprintf("unknown named route %q", sub)
	}

	return 0, fmt.Sprintf("non-numeric subdomain %q and no names store configured", sub)
}

func isTextContentType(ct string) bool {
	ct = strings.ToLower(strings.SplitN(ct, ";", 2)[0])
	ct = strings.TrimSpace(ct)
	switch ct {
	case "text/html", "text/css", "text/javascript", "text/plain", "text/xml",
		"application/javascript", "application/json", "application/xml":
		return true
	}
	return strings.HasPrefix(ct, "text/")
}

// ExtractPort is the pure parsing function exposed for testing.
func ExtractPort(host, domain string) (int, string) {
	h := &Handler{cfg: Config{Domain: domain}}
	return h.resolvePort(host)
}

// rewriteQueryScheme rewrites URL-valued query parameters whose hostname matches
// publicHost and whose scheme is "https" to use "http" instead. This is required
// for OAuth redirect_uri parameters: the proxy forwards requests to backends over
// plain HTTP with the original Host header, so backends compute their own URL as
// http://publicHost. Valid redirect URI patterns therefore use the http scheme, and
// the browser-supplied https scheme must be downgraded before forwarding.
//
// Returns the rewritten query string and true if any values were changed.
func rewriteQueryScheme(rawQuery, publicHost string) (string, bool) {
	if rawQuery == "" || publicHost == "" {
		return rawQuery, false
	}
	params, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery, false
	}
	changed := false
	for key, vals := range params {
		for i, v := range vals {
			if u, err := url.Parse(v); err == nil && u.Hostname() == publicHost && u.Scheme == "https" {
				u.Scheme = "http"
				vals[i] = u.String()
				changed = true
			}
		}
		params[key] = vals
	}
	if !changed {
		return rawQuery, false
	}
	return params.Encode(), true
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func (h *Handler) proxyWebSocket(w http.ResponseWriter, r *http.Request, port int, start time.Time) {
	dst, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 10*time.Second)
	if err != nil {
		slog.Error("websocket dial failed", "port", port, "error", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer dst.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	src, buf, err := hijacker.Hijack()
	if err != nil {
		slog.Error("websocket hijack failed", "error", err)
		return
	}
	defer src.Close()

	if err := r.Write(dst); err != nil {
		slog.Error("websocket request write failed", "error", err)
		return
	}

	if buf.Reader.Buffered() > 0 {
		buffered := make([]byte, buf.Reader.Buffered())
		if _, err := buf.Read(buffered); err == nil {
			_, _ = dst.Write(buffered)
		}
	}

	slog.Info("websocket",
		"host", r.Host,
		"port", port,
		"path", r.URL.Path,
		"latency_ms", time.Since(start).Milliseconds(),
	)

	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go cp(dst, src)
	go cp(src, dst)
	<-done
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

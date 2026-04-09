package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tunnel-ops/tunnel/internal/names"
	"github.com/tunnel-ops/tunnel/internal/proxy"
)

const maxRequests = 200

// statusResponse mirrors the JSON returned by /api/status.
type statusResponse struct {
	Status    string         `json:"status"`
	Version   string         `json:"version"`
	Domain    string         `json:"domain"`
	ProxyPort string         `json:"proxy_port"`
	Uptime    string         `json:"uptime"`
	Ports     map[string]int `json:"ports"`
}

// sseReader holds an open SSE HTTP connection.
type sseReader struct {
	resp    *http.Response
	scanner *bufio.Scanner
}

// Tea messages
type statusMsg struct{ resp statusResponse }
type statusErrMsg struct{ err error }
type sseConnectedMsg struct{ resp *http.Response }
type requestEventMsg struct{ event proxy.RequestEvent }
type sseErrMsg struct{ err error }
type retrySSEMsg struct{}
type retryStatusMsg struct{}

// watchModel is the Bubble Tea model for tunnel watch.
type watchModel struct {
	adminURL   string
	portFilter int // 0 = all ports
	status     *statusResponse
	requests   []proxy.RequestEvent
	sseReader  *sseReader
	err        error
	store      *names.Store
	// open port prompt
	openPrompt bool
	openInput  textinput.Model
	// close port mode
	closeMode   bool
	closeCursor int
	closePorts  []string // sorted keys from status.Ports
}

var (
	boldStyle = lipgloss.NewStyle().Bold(true)
	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func statusCodeStyle(code int) lipgloss.Style {
	switch {
	case code >= 200 && code < 300:
		return okStyle
	case code >= 400:
		return errStyle
	default:
		return dimStyle
	}
}

func (m watchModel) Init() tea.Cmd {
	return tea.Batch(fetchStatus(m.adminURL), startSSEConnect(m.adminURL))
}

func (m watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if m.openPrompt {
			switch msg.Type {
			case tea.KeyEsc:
				m.openPrompt = false
				m.openInput.Reset()
				return m, nil
			case tea.KeyEnter:
				portStr := strings.TrimSpace(m.openInput.Value())
				port, err := strconv.Atoi(portStr)
				if err == nil && port >= 1024 && port <= 65535 {
					_ = m.store.Add(portStr, port)
					if m.status != nil {
						if m.status.Ports == nil {
							m.status.Ports = make(map[string]int)
						}
						m.status.Ports[portStr] = port
						m.closePorts = sortedKeys(m.status.Ports)
					}
				}
				m.openPrompt = false
				m.openInput.Reset()
				return m, nil
			default:
				var cmd tea.Cmd
				m.openInput, cmd = m.openInput.Update(msg)
				return m, cmd
			}
		}

		if m.closeMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.closeMode = false
				return m, nil
			case tea.KeyUp:
				if m.closeCursor > 0 {
					m.closeCursor--
				}
				return m, nil
			case tea.KeyDown:
				if m.closeCursor < len(m.closePorts)-1 {
					m.closeCursor++
				}
				return m, nil
			case tea.KeyEnter:
				if len(m.closePorts) > 0 && m.closeCursor < len(m.closePorts) {
					key := m.closePorts[m.closeCursor]
					_ = m.store.Remove(key)
					if m.status != nil {
						delete(m.status.Ports, key)
						m.closePorts = sortedKeys(m.status.Ports)
						if m.closeCursor >= len(m.closePorts) && m.closeCursor > 0 {
							m.closeCursor--
						}
					}
					m.closeMode = false
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "o":
			if m.portFilter == 0 {
				m.openPrompt = true
				m.openInput.Focus()
			}
			return m, nil
		case "c":
			if m.portFilter == 0 && len(m.closePorts) > 0 {
				m.closeMode = true
				m.closeCursor = 0
			}
			return m, nil
		case "ctrl+c", "q":
			if m.sseReader != nil {
				m.sseReader.resp.Body.Close()
			}
			return m, tea.Quit
		}

	case statusMsg:
		m.status = &msg.resp
		m.closePorts = sortedKeys(msg.resp.Ports)
		return m, nil

	case statusErrMsg:
		m.err = fmt.Errorf("proxy offline — is requests-proxy running? (%w)", msg.err)
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return retryStatusMsg{} })

	case retryStatusMsg:
		m.err = nil
		return m, tea.Batch(fetchStatus(m.adminURL), startSSEConnect(m.adminURL))

	case sseConnectedMsg:
		r := &sseReader{
			resp:    msg.resp,
			scanner: bufio.NewScanner(msg.resp.Body),
		}
		m.sseReader = r
		return m, waitForSSEEvent(r)

	case requestEventMsg:
		if m.portFilter == 0 || msg.event.Port == m.portFilter {
			m.requests = append([]proxy.RequestEvent{msg.event}, m.requests...)
			if len(m.requests) > maxRequests {
				m.requests = m.requests[:maxRequests]
			}
		}
		return m, waitForSSEEvent(m.sseReader)

	case sseErrMsg:
		m.sseReader = nil
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return retrySSEMsg{} })

	case retrySSEMsg:
		return m, startSSEConnect(m.adminURL)
	}

	return m, nil
}

func (m watchModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  %s\n\n  %s\n",
			boldStyle.Render("tunnel watch"),
			dimStyle.Render(m.err.Error()+" — retrying in 3s  (q to quit)"),
		)
	}

	var b strings.Builder
	hr := strings.Repeat("─", 72)

	b.WriteString(boldStyle.Render("tunnel watch"))
	if m.portFilter != 0 {
		b.WriteString(boldStyle.Render(fmt.Sprintf(" %d", m.portFilter)))
	}
	b.WriteString(dimStyle.Render("                                    (Ctrl+C to quit)") + "\n\n")

	if m.status != nil {
		b.WriteString(fmt.Sprintf("Session Status    %s\n", m.status.Status))
		b.WriteString(fmt.Sprintf("Domain            %s\n", m.status.Domain))
		b.WriteString(fmt.Sprintf("Proxy             http://localhost:%s\n", m.status.ProxyPort))
		b.WriteString(fmt.Sprintf("Uptime            %s\n", m.status.Uptime))
	} else {
		b.WriteString("Loading...\n")
	}

	b.WriteString("\nPorts\n" + hr + "\n")
	if m.status != nil {
		keys := m.closePorts
		if m.portFilter != 0 {
			keys = []string{strconv.Itoa(m.portFilter)}
		}
		domain := strings.TrimPrefix(m.status.Domain, "*.")
		for i, k := range keys {
			port := m.status.Ports[k]
			prefix := "  "
			if m.closeMode && i == m.closeCursor {
				prefix = boldStyle.Render("> ")
			}
			b.WriteString(fmt.Sprintf("%s%-8s https://%s.%s → http://localhost:%d\n",
				prefix, k, k, domain, port))
		}
	}

	if m.portFilter == 0 {
		switch {
		case m.openPrompt:
			b.WriteString(fmt.Sprintf("\nOpen port: %s\n", m.openInput.View()))
		case m.closeMode:
			b.WriteString("\n[↑↓] Select   [Enter] Confirm   [Esc] Cancel\n")
		default:
			b.WriteString("\n[o] Open port   [c] Close port\n")
		}
	}

	b.WriteString("\nHTTP Requests\n" + hr + "\n")

	switch {
	case m.sseReader == nil && len(m.requests) == 0:
		b.WriteString(dimStyle.Render("reconnecting...") + "\n")
	case len(m.requests) == 0:
		b.WriteString(dimStyle.Render("Waiting for requests...") + "\n")
	default:
		for _, r := range m.requests {
			code := statusCodeStyle(r.StatusCode).Render(strconv.Itoa(r.StatusCode))
			path := r.Path
			if len(path) > 40 {
				path = path[:37] + "..."
			}
			b.WriteString(fmt.Sprintf("%s  %-5s %-6d %-40s %s\n",
				r.Timestamp.Format("15:04:05"),
				r.Method,
				r.Port,
				path,
				code,
			))
		}
	}

	return b.String()
}

func fetchStatus(adminURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(adminURL + "/api/status")
		if err != nil {
			return statusErrMsg{err}
		}
		defer resp.Body.Close()
		var s statusResponse
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return statusErrMsg{err}
		}
		return statusMsg{resp: s}
	}
}

func startSSEConnect(adminURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(adminURL + "/api/events")
		if err != nil {
			return sseErrMsg{err}
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return sseErrMsg{fmt.Errorf("SSE endpoint returned %d", resp.StatusCode)}
		}
		return sseConnectedMsg{resp: resp}
	}
}

// waitForSSEEvent reads one SSE event from r and returns it as requestEventMsg.
// Blocks until an event arrives or the connection closes.
func waitForSSEEvent(r *sseReader) tea.Cmd {
	return func() tea.Msg {
		var dataLine string
		for r.scanner.Scan() {
			line := r.scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			} else if line == "" && dataLine != "" {
				var e proxy.RequestEvent
				if json.Unmarshal([]byte(dataLine), &e) == nil {
					return requestEventMsg{e}
				}
				dataLine = ""
			}
		}
		if err := r.scanner.Err(); err != nil {
			return sseErrMsg{err}
		}
		return sseErrMsg{io.EOF}
	}
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cmdWatch(portFilter int) {
	store, err := names.New("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	adminPort := os.Getenv("HEALTH_PORT")
	if adminPort == "" {
		adminPort = "7998"
	}

	ti := textinput.New()
	ti.Placeholder = "3000"
	ti.CharLimit = 5
	ti.Width = 10

	m := watchModel{
		adminURL:   "http://localhost:" + adminPort,
		portFilter: portFilter,
		store:      store,
		openInput:  ti,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

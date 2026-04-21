// Package setup provides the interactive first-time setup wizard (Bubble Tea TUI).
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tunnel-ops/tunnel/internal/appconfig"
	"github.com/tunnel-ops/tunnel/internal/dns"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const defaultProxyPort = 7999
const defaultTunnelName = "dev"

// providerHelp shows contextual notes when a provider is highlighted in the picker.
var providerHelp = map[string]string{
	"cloudflare": "Uses cloudflared CLI auth — no API key needed.",
	"godaddy":    "Requires a GoDaddy developer API key.\n  → https://developer.godaddy.com/keys",
	"namecheap":  "Requires API access enabled on your account ($50+ balance or 20+ domains).\n  → https://ap.www.namecheap.com/settings/tools/apiaccess/",
	"manual":     "You will add the DNS record yourself — no API needed.",
}

// prevState maps each wizard state to the one before it for esc/back navigation.
var prevState = map[wizardState]wizardState{
	stateInputDomain: statePickProvider,
	stateInputTunnel: stateInputDomain,
	stateInputCreds:  stateInputTunnel,
	stateConfirm:     stateInputCreds,
}

// Step indices for stateRunning.
const (
	stepCloudflaredCheck = iota
	stepCloudflareAuth
	stepTunnelCreate
	stepDNS
	stepWriteConfig
	stepSaveSettings
	stepSvcProxy
	stepSvcCloudflared
	numSteps
)

// ── State types ───────────────────────────────────────────────────────────────

type wizardState int

const (
	statePickProvider wizardState = iota
	stateInputDomain
	stateInputTunnel
	stateInputCreds
	stateConfirm
	stateRunning
	stateTunnelConflict
	stateManualDNS
	stateDone
	stateErr
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepSkipped
	stepFailed
)

// ── Messages ──────────────────────────────────────────────────────────────────

type stepDoneMsg struct {
	idx     int
	err     error
	payload string // tunnel ID returned by stepTunnelCreate
}

type execDoneMsg struct {
	idx int
	err error
}

// tunnelConflictMsg is sent when a tunnel with the requested name already exists in Cloudflare.
type tunnelConflictMsg struct {
	name string
	id   string
}

// takenNamesMsg carries the full set of existing tunnel names after the user chooses to rename.
type takenNamesMsg struct {
	names map[string]bool
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	subtleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	doneStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	labelStyle    = lipgloss.NewStyle().Width(12)
)

// ── Model ─────────────────────────────────────────────────────────────────────

type stepStatus struct {
	label string
	state stepState
}

type model struct {
	state wizardState
	cfg   *appconfig.Config

	// statePickProvider
	providerChoices []string
	providerKeys    []string
	cursor          int

	// stateInputDomain / stateInputTunnel
	textInput textinput.Model

	// stateInputCreds
	credLabels []string
	credFields []textinput.Model
	credFocus  int

	// stateConfirm
	installProxy       bool
	installCloudflared bool
	confirmCursor      int // 0 = proxy checkbox, 1 = cloudflared checkbox

	// stateRunning
	steps      []stepStatus
	activeStep int
	spinner    spinner.Model
	tunnelID   string // populated after stepTunnelCreate completes

	// stateTunnelConflict
	existingTunnelName string
	existingTunnelID   string
	conflictChoice     int          // 0 = use it, 1 = change name
	takenTunnelNames   map[string]bool

	// input validation (stateInputTunnel)
	inputError string

	err error
}

func initialModel(cfg *appconfig.Config) model {
	ti := textinput.New()
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	steps := []stepStatus{
		{label: "cloudflared installed"},
		{label: "Authenticated with Cloudflare"},
		{label: "Create tunnel"},
		{label: "Configure DNS"},
		{label: "Write cloudflared config"},
		{label: "Save settings"},
		{label: "Install proxy service"},
		{label: "Install cloudflared service"},
	}

	return model{
		state:              statePickProvider,
		cfg:                cfg,
		providerChoices:    []string{"Cloudflare", "GoDaddy", "Namecheap", "Manual"},
		providerKeys:       []string{"cloudflare", "godaddy", "namecheap", "manual"},
		textInput:          ti,
		installProxy:       true,
		installCloudflared: true,
		steps:              steps,
		spinner:            sp,
	}
}

// ── Entry point ───────────────────────────────────────────────────────────────

// Run launches the interactive setup TUI.
func Run() error {
	cfg, err := appconfig.Load()
	if err != nil || cfg == nil {
		cfg = &appconfig.Config{ProxyPort: defaultProxyPort}
	}
	if cfg.ProxyPort == 0 {
		cfg.ProxyPort = defaultProxyPort
	}

	p := tea.NewProgram(initialModel(cfg), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(model); ok && fm.err != nil {
		return fm.err
	}
	return nil
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {
		case statePickProvider:
			return m.updatePickProvider(msg)
		case stateInputDomain:
			return m.updateInputText(msg, func(v string) (model, tea.Cmd) {
				m.cfg.Domain = v
				return m.transitionTo(stateInputTunnel)
			})
		case stateInputTunnel:
			return m.updateInputTunnel(msg)
		case stateInputCreds:
			return m.updateInputCreds(msg)
		case stateConfirm:
			return m.updateConfirm(msg)
		case stateTunnelConflict:
			return m.updateTunnelConflict(msg)
		case stateManualDNS:
			return m.updateManualDNS(msg)
		case stateRunning:
			// ignore keypresses while steps are executing
		case stateDone:
			if isQuit(msg.String()) {
				return m, tea.Quit
			}
		case stateErr:
			return m.updateErr(msg)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case stepDoneMsg:
		return m.handleStepDone(msg)

	case execDoneMsg:
		return m.handleExecDone(msg)

	case tunnelConflictMsg:
		m.steps[stepTunnelCreate].state = stepPending
		m.existingTunnelName = msg.name
		m.existingTunnelID = msg.id
		m.conflictChoice = 0
		m.state = stateTunnelConflict
		return m, nil

	case takenNamesMsg:
		m.takenTunnelNames = msg.names
		m.cfg.TunnelName = ""
		m.cfg.TunnelID = ""
		m.tunnelID = ""
		return m.transitionTo(stateInputTunnel)
	}

	return m, nil
}

func isQuit(key string) bool {
	return key == "q" || key == "enter" || key == "ctrl+c"
}

// goBack transitions to the previous wizard state. No-op when already at the first screen.
func (m model) goBack() (tea.Model, tea.Cmd) {
	prev, ok := prevState[m.state]
	if !ok {
		return m, nil
	}
	// Cloudflare and Manual have no creds screen, so skip it when going back from confirm.
	if m.state == stateConfirm && (m.cfg.Provider == "cloudflare" || m.cfg.Provider == "manual") {
		prev = stateInputTunnel
	}
	return m.transitionTo(prev)
}

// updatePickProvider handles key events on the provider selection screen.
func (m model) updatePickProvider(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.providerChoices)-1 {
			m.cursor++
		}
	case "enter":
		m.cfg.Provider = m.providerKeys[m.cursor]
		return m.transitionTo(stateInputDomain)
	case "esc":
		// already at first screen — no-op
	}
	return m, nil
}

// updateInputText handles key events for a single text input screen.
// onConfirm is called with the trimmed value when Enter is pressed.
func (m model) updateInputText(msg tea.KeyMsg, onConfirm func(string) (model, tea.Cmd)) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.goBack()
	case "enter":
		v := strings.TrimSpace(m.textInput.Value())
		return onConfirm(v)
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updateInputCreds handles key events on the credentials input screen.
func (m model) updateInputCreds(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.goBack()

	case "tab", "down":
		m.credFields[m.credFocus].Blur()
		m.credFocus = (m.credFocus + 1) % len(m.credFields)
		m.credFields[m.credFocus].Focus()
		return m, textinput.Blink

	case "shift+tab", "up":
		m.credFields[m.credFocus].Blur()
		m.credFocus = (m.credFocus - 1 + len(m.credFields)) % len(m.credFields)
		m.credFields[m.credFocus].Focus()
		return m, textinput.Blink

	case "enter":
		for _, f := range m.credFields {
			if strings.TrimSpace(f.Value()) == "" {
				return m, nil // require all fields
			}
		}
		var saveErr error
		switch m.cfg.Provider {
		case "godaddy":
			saveErr = dns.SaveGoDaddyCredentials(m.credFields[0].Value(), m.credFields[1].Value())
		case "namecheap":
			saveErr = dns.SaveNamecheapCredentials(m.credFields[0].Value(), m.credFields[1].Value())
		}
		// Zero credential values in the model regardless of outcome
		for i := range m.credFields {
			m.credFields[i].SetValue("")
		}
		if saveErr != nil {
			m.err = saveErr
			m.state = stateErr
			return m, nil
		}
		return m.transitionTo(stateConfirm)
	}

	// Delegate typing to the active field only
	var cmd tea.Cmd
	m.credFields[m.credFocus], cmd = m.credFields[m.credFocus].Update(msg)
	return m, cmd
}

// updateConfirm handles key events on the confirm screen.
func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.goBack()
	case "up", "k":
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}
	case "down", "j":
		if m.confirmCursor < 1 {
			m.confirmCursor++
		}
	case " ":
		switch m.confirmCursor {
		case 0:
			m.installProxy = !m.installProxy
		case 1:
			m.installCloudflared = !m.installCloudflared
		}
	case "enter":
		return m.transitionTo(stateRunning)
	}
	return m, nil
}

// updateInputTunnel handles key events on the tunnel name screen.
// It validates the entered name against any known-taken tunnel names.
func (m model) updateInputTunnel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		v := strings.TrimSpace(m.textInput.Value())
		if v == "" {
			v = defaultTunnelName
		}
		if m.takenTunnelNames[v] {
			m.inputError = fmt.Sprintf("%q is already taken — choose a different name", v)
			return m, nil
		}
		m.inputError = ""
		m.cfg.TunnelName = v
		return m.transitionTo(stateInputCreds)
	default:
		m.inputError = "" // clear stale error as soon as user types
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updateTunnelConflict handles key events on the tunnel-conflict resolution screen.
func (m model) updateTunnelConflict(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.conflictChoice > 0 {
			m.conflictChoice--
		}
	case "down", "j":
		if m.conflictChoice < 1 {
			m.conflictChoice++
		}
	case "enter":
		if m.conflictChoice == 0 {
			// Reuse the existing tunnel — resume running from the DNS step.
			m.cfg.TunnelID = m.existingTunnelID
			m.tunnelID = m.existingTunnelID
			m.steps[stepTunnelCreate].state = stepSkipped
			m.state = stateRunning
			return startStep(m, stepDNS)
		}
		// Change name — fetch all taken names so the input screen can validate.
		return m, func() tea.Msg {
			names, err := ListTunnelNames()
			if err != nil {
				// Fall back to only blocking the one name we know about.
				names = map[string]bool{m.existingTunnelName: true}
			}
			return takenNamesMsg{names: names}
		}
	}
	return m, nil
}

// updateManualDNS handles key events on the manual DNS record screen.
func (m model) updateManualDNS(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.steps[stepDNS].state = stepDone
		m.state = stateRunning
		return startStep(m, stepWriteConfig)
	}
	return m, nil
}

// updateErr handles key events on the error screen. Pressing 'r' retries the
// failed step from where setup left off.
func (m model) updateErr(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.err = nil
		m.steps[m.activeStep].state = stepPending
		m.state = stateRunning
		return startStep(m, m.activeStep)
	case "q", "ctrl+c", "enter":
		return m, tea.Quit
	}
	return m, nil
}

// handleStepDone processes the result of a background (non-exec) step.
func (m model) handleStepDone(msg stepDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.steps[msg.idx].state = stepFailed
		m.err = msg.err
		m.state = stateErr
		return m, nil
	}
	m.steps[msg.idx].state = stepDone
	if msg.payload != "" {
		m.tunnelID = msg.payload
		m.cfg.TunnelID = msg.payload
	}
	return startStep(m, msg.idx+1)
}

// handleExecDone processes the result of a tea.ExecProcess step.
func (m model) handleExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.steps[msg.idx].state = stepFailed
		m.err = msg.err
		m.state = stateErr
		return m, nil
	}
	m.steps[msg.idx].state = stepDone
	return startStep(m, msg.idx+1)
}

// transitionTo sets up model state when moving to a new screen.
func (m model) transitionTo(s wizardState) (model, tea.Cmd) {
	m.state = s
	switch s {
	case stateInputDomain:
		m.textInput = textinput.New()
		m.textInput.Placeholder = "example.com"
		m.textInput.SetValue(m.cfg.Domain)
		m.textInput.Focus()
		return m, textinput.Blink

	case stateInputTunnel:
		m.inputError = ""
		m.textInput = textinput.New()
		m.textInput.Placeholder = defaultTunnelName
		m.textInput.SetValue(m.cfg.TunnelName)
		m.textInput.Focus()
		return m, textinput.Blink

	case stateInputCreds:
		switch m.cfg.Provider {
		case "cloudflare", "manual":
			return m.transitionTo(stateConfirm)
		case "godaddy":
			if dns.HasGoDaddyCredentials() {
				return m.transitionTo(stateConfirm)
			}
			m.credLabels = []string{"API Key", "API Secret"}
			m.credFields = makeCredFields([]bool{false, true})
		case "namecheap":
			if dns.HasNamecheapCredentials() {
				return m.transitionTo(stateConfirm)
			}
			m.credLabels = []string{"Username", "API Key"}
			m.credFields = makeCredFields([]bool{false, true})
		}
		m.credFocus = 0
		m.credFields[0].Focus()
		return m, textinput.Blink

	case stateRunning:
		return startStep(m, stepCloudflaredCheck)
	}
	return m, nil
}

// makeCredFields creates textinput models; isPassword controls echo mode.
func makeCredFields(isPassword []bool) []textinput.Model {
	fields := make([]textinput.Model, len(isPassword))
	for i, pwd := range isPassword {
		f := textinput.New()
		if pwd {
			f.EchoMode = textinput.EchoPassword
			f.EchoCharacter = '·'
		}
		fields[i] = f
	}
	return fields
}

// startStep marks step idx as running and returns the appropriate command.
// Steps skip or chain synchronously when they detect already-done conditions.
func startStep(m model, idx int) (model, tea.Cmd) {
	if idx >= numSteps {
		m.state = stateDone
		return m, nil
	}
	m.activeStep = idx
	m.steps[idx].state = stepRunning

	switch idx {
	case stepCloudflaredCheck:
		if IsCloudflaredInstalled() {
			m.steps[idx].state = stepDone
			return startStep(m, idx+1)
		}
		if runtime.GOOS != "darwin" {
			m.steps[idx].state = stepFailed
			m.err = fmt.Errorf("cloudflared not found — install from https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
			m.state = stateErr
			return m, nil
		}
		if _, err := exec.LookPath("brew"); err != nil {
			m.steps[idx].state = stepFailed
			m.err = fmt.Errorf("cloudflared not found and Homebrew is not installed\n  Install Homebrew: https://brew.sh\n  Or install cloudflared directly: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
			m.state = stateErr
			return m, nil
		}
		return m, func() tea.Msg {
			cmd := exec.Command("brew", "install", "cloudflare/cloudflare/cloudflared")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return stepDoneMsg{idx: stepCloudflaredCheck,
					err: fmt.Errorf("brew install failed: %w\n%s", err, strings.TrimSpace(string(out)))}
			}
			return stepDoneMsg{idx: stepCloudflaredCheck}
		}

	case stepCloudflareAuth:
		if IsAuthenticated() {
			m.steps[idx].state = stepSkipped
			return startStep(m, idx+1)
		}
		return m, func() tea.Msg {
			cmd := exec.Command("cloudflared", "tunnel", "login")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return stepDoneMsg{idx: stepCloudflareAuth,
					err: fmt.Errorf("cloudflared login failed: %w\n%s", err, strings.TrimSpace(string(out)))}
			}
			return stepDoneMsg{idx: stepCloudflareAuth}
		}

	case stepTunnelCreate:
		if m.cfg.TunnelID != "" {
			m.tunnelID = m.cfg.TunnelID
			m.steps[idx].state = stepSkipped
			return startStep(m, idx+1)
		}
		tunnelName := m.cfg.TunnelName
		return m, func() tea.Msg {
			id, err := CreateTunnel(tunnelName)
			if err != nil && strings.Contains(err.Error(), "already exists") {
				existingID, _ := TunnelExists(tunnelName)
				return tunnelConflictMsg{name: tunnelName, id: existingID}
			}
			return stepDoneMsg{idx: stepTunnelCreate, err: err, payload: id}
		}

	case stepDNS:
		if m.cfg.Provider == "manual" {
			m.state = stateManualDNS
			return m, nil
		}
		provider := m.cfg.Provider
		tunnelName := m.cfg.TunnelName
		domain := m.cfg.Domain
		target := m.cfg.TunnelID + ".cfargotunnel.com"
		return m, func() tea.Msg {
			p, err := buildProvider(provider, tunnelName)
			if err != nil {
				return stepDoneMsg{idx: stepDNS, err: err}
			}
			return stepDoneMsg{idx: stepDNS, err: p.SetupWildcard(domain, target)}
		}

	case stepWriteConfig:
		tunnelID := m.cfg.TunnelID
		tunnelName := m.cfg.TunnelName
		domain := m.cfg.Domain
		proxyPort := m.cfg.ProxyPort
		return m, func() tea.Msg {
			return stepDoneMsg{
				idx: stepWriteConfig,
				err: WriteCloudflaredConfig(tunnelID, tunnelName, domain, proxyPort),
			}
		}

	case stepSaveSettings:
		cfg := m.cfg
		return m, func() tea.Msg {
			return stepDoneMsg{idx: stepSaveSettings, err: appconfig.Save(cfg)}
		}

	case stepSvcProxy:
		if !m.installProxy {
			m.steps[idx].state = stepSkipped
			return startStep(m, idx+1)
		}
		cfg := m.cfg
		return m, func() tea.Msg {
			return stepDoneMsg{idx: stepSvcProxy, err: installProxy(cfg)}
		}

	case stepSvcCloudflared:
		if !m.installCloudflared {
			m.steps[idx].state = stepSkipped
			return startStep(m, idx+1)
		}
		return m, func() tea.Msg {
			return stepDoneMsg{idx: stepSvcCloudflared, err: installCloudflared()}
		}
	}

	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	switch m.state {
	case statePickProvider:
		return m.viewPickProvider()
	case stateInputDomain:
		return m.viewTextInput("Your domain")
	case stateInputTunnel:
		return m.viewTextInput("Tunnel name")
	case stateInputCreds:
		return m.viewCreds()
	case stateConfirm:
		return m.viewConfirm()
	case stateRunning:
		return m.viewRunning()
	case stateTunnelConflict:
		return m.viewTunnelConflict()
	case stateManualDNS:
		return m.viewManualDNS()
	case stateDone:
		return m.viewDone()
	case stateErr:
		return m.viewErr()
	}
	return ""
}

func (m model) viewPickProvider() string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("tunnel setup") + "\n\n")
	b.WriteString("  Which DNS provider manages your domain?\n\n")
	for i, c := range m.providerChoices {
		if i == m.cursor {
			b.WriteString("  " + selectedStyle.Render("> "+c) + "\n")
		} else {
			b.WriteString("    " + c + "\n")
		}
	}
	if help, ok := providerHelp[m.providerKeys[m.cursor]]; ok {
		b.WriteString("\n")
		for _, line := range strings.Split(help, "\n") {
			b.WriteString("  " + subtleStyle.Render(line) + "\n")
		}
	}
	b.WriteString("\n  " + subtleStyle.Render("↑↓ move  enter select  ctrl+c quit") + "\n")
	return b.String()
}

func (m model) viewTextInput(label string) string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render(label) + "\n")
	b.WriteString("  " + m.textInput.View() + "\n")
	if m.inputError != "" {
		b.WriteString("  " + errorStyle.Render(m.inputError) + "\n")
	}
	b.WriteString("\n  " + subtleStyle.Render("enter confirm  esc back  ctrl+c quit") + "\n")
	return b.String()
}

func (m model) viewCreds() string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render(capitalize(m.cfg.Provider)+" API credentials") + "\n\n")
	for i, f := range m.credFields {
		b.WriteString("  " + labelStyle.Render(m.credLabels[i]) + "  " + f.View() + "\n")
	}
	b.WriteString("\n  " + subtleStyle.Render("tab next field  enter confirm  esc back  ctrl+c quit") + "\n")
	return b.String()
}

func (m model) viewConfirm() string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("Ready to set up tunnel") + "\n\n")
	b.WriteString("  " + labelStyle.Render("Provider") + "  " + m.cfg.Provider + "\n")
	b.WriteString("  " + labelStyle.Render("Domain") + "  " + m.cfg.Domain + "\n")
	b.WriteString("  " + labelStyle.Render("Tunnel") + "  " + m.cfg.TunnelName + "\n\n")

	b.WriteString("  " + checkBox(m.installProxy, m.confirmCursor == 0) + "  Install proxy as background service\n")
	b.WriteString("  " + checkBox(m.installCloudflared, m.confirmCursor == 1) + "  Install cloudflared as background service\n")
	b.WriteString("\n  " + subtleStyle.Render("↑↓ move  space toggle  enter begin  esc back  ctrl+c quit") + "\n")
	return b.String()
}

func checkBox(checked, focused bool) string {
	var s string
	if checked {
		s = "[✓]"
	} else {
		s = "[ ]"
	}
	if focused {
		return selectedStyle.Render(s)
	}
	return s
}

func (m model) viewRunning() string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("Setting up your tunnel...") + "\n\n")
	for i, step := range m.steps {
		b.WriteString("  " + stepIcon(m, i, step) + "  " + step.label + "\n")
	}
	if m.activeStep == stepCloudflareAuth && m.steps[stepCloudflareAuth].state == stepRunning {
		b.WriteString("\n  " + subtleStyle.Render("→ A browser window should open — log in with your Cloudflare account.") + "\n")
	}
	return b.String()
}

func stepIcon(m model, idx int, step stepStatus) string {
	switch step.state {
	case stepDone:
		return doneStyle.Render("✓")
	case stepSkipped:
		return dimStyle.Render("–")
	case stepFailed:
		return errorStyle.Render("✗")
	case stepRunning:
		if idx == m.activeStep {
			return m.spinner.View()
		}
	}
	return dimStyle.Render("·")
}

func (m model) viewTunnelConflict() string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("Tunnel already exists") + "\n\n")
	b.WriteString(fmt.Sprintf("  Tunnel %q already exists in your Cloudflare account.\n\n", m.existingTunnelName))

	items := []string{"Use it", "Choose a different name"}
	for i, item := range items {
		if i == m.conflictChoice {
			b.WriteString("  " + selectedStyle.Render("> "+item) + "\n")
		} else {
			b.WriteString("    " + item + "\n")
		}
	}
	b.WriteString("\n  " + subtleStyle.Render("↑↓ move  enter select  ctrl+c quit") + "\n")
	return b.String()
}

func (m model) viewManualDNS() string {
	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("Add a DNS record") + "\n\n")
	b.WriteString("  Create this record in your DNS provider:\n\n")
	b.WriteString("    Type:   CNAME\n")
	b.WriteString(fmt.Sprintf("    Name:   *.%s\n", m.cfg.Domain))
	b.WriteString(fmt.Sprintf("    Value:  %s.cfargotunnel.com\n", m.cfg.TunnelID))
	b.WriteString("\n  Once the record is live, press Enter to continue.\n")
	b.WriteString("\n  " + subtleStyle.Render("enter continue  ctrl+c quit") + "\n")
	return b.String()
}

func (m model) viewDone() string {
	var b strings.Builder
	b.WriteString("\n  " + doneStyle.Render("✓  Setup complete!") + "\n\n")
	b.WriteString("  Run: tunnel 8080\n\n")
	b.WriteString("  " + subtleStyle.Render("press any key to exit") + "\n")
	return b.String()
}

func (m model) viewErr() string {
	var b strings.Builder
	b.WriteString("\n  " + errorStyle.Render("✗  Setup failed") + "\n\n")
	if m.err != nil {
		b.WriteString("  " + m.err.Error() + "\n\n")
	}
	b.WriteString("  " + subtleStyle.Render("r retry  q quit") + "\n")
	return b.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func buildProvider(providerName, tunnelName string) (dns.Provider, error) {
	switch providerName {
	case "cloudflare":
		return &dns.CloudflareProvider{TunnelName: tunnelName}, nil
	case "godaddy":
		return &dns.GoDaddyProvider{}, nil
	case "namecheap":
		return &dns.NamecheapProvider{}, nil
	case "manual":
		return &dns.ManualProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q", providerName)
	}
}

func installProxy(cfg *appconfig.Config) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("launchd service install is macOS-only; start requests-proxy manually")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	proxyBin := filepath.Join(filepath.Dir(self), "requests-proxy")
	if _, err := os.Stat(proxyBin); err != nil {
		return fmt.Errorf("requests-proxy binary not found at %s", proxyBin)
	}

	home, _ := os.UserHomeDir()
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	_ = os.MkdirAll(plistDir, 0o755)

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.bellamy.requests-proxy</string>
  <key>ProgramArguments</key>
  <array><string>%s</string></array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>DOMAIN</key><string>%s</string>
    <key>PROXY_PORT</key><string>%d</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s/Library/Logs/requests-proxy.log</string>
  <key>StandardErrorPath</key><string>%s/Library/Logs/requests-proxy.log</string>
</dict>
</plist>`, proxyBin, cfg.Domain, cfg.ProxyPort, home, home)

	plistPath := filepath.Join(plistDir, "com.bellamy.requests-proxy.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	// Unload first so re-running setup always applies the latest config.
	_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
	return exec.Command("launchctl", "load", "-w", plistPath).Run()
}

func installCloudflared() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("launchd service install is macOS-only; start cloudflared manually")
	}
	cloudflaredBin, err := exec.LookPath("cloudflared")
	if err != nil {
		return fmt.Errorf("cloudflared binary not found on PATH")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".cloudflared", "config.yml")

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	_ = os.MkdirAll(plistDir, 0o755)

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.cloudflare.cloudflared</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>tunnel</string>
    <string>--config</string>
    <string>%s</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s/Library/Logs/cloudflared.log</string>
  <key>StandardErrorPath</key><string>%s/Library/Logs/cloudflared.log</string>
</dict>
</plist>`, cloudflaredBin, configPath, home, home)

	plistPath := filepath.Join(plistDir, "com.cloudflare.cloudflared.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
	return exec.Command("launchctl", "load", "-w", plistPath).Run()
}

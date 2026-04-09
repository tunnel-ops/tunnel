package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	urlStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4C618F"))
	liveStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3DD68C"))
	deadStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666677"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3DD68C")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F4A261"))
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#888899"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555566"))
	hintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555566"))
)

func gradientTunnel() string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4C618F")).Render("t") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5B5F9E")).Render("u") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6A5CAC")).Render("n") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7958B8")).Render("n") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8855C3")).Render("e") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9751CC")).Render("l")
}

func statusDot(listening bool) string {
	if listening {
		return liveStyle.Render("●")
	}
	return deadStyle.Render("○")
}

func showTunnelURL(url, closeKey string, port int, listening bool, serviceIssue string) {
	hr := dimStyle.Render(strings.Repeat("─", 55))

	var dot, statusText string
	switch {
	case serviceIssue != "":
		dot = warnStyle.Render("⚠")
		statusText = warnStyle.Render(serviceIssue)
	case listening:
		dot = liveStyle.Render("●")
		statusText = liveStyle.Render("live")
	default:
		dot = deadStyle.Render("○")
		statusText = warnStyle.Render("nothing listening yet")
	}

	fmt.Println()
	fmt.Printf("  %s\n", gradientTunnel()+boldStyle.Render("  "+closeKey))
	fmt.Printf("  %s\n", hr)
	fmt.Printf("  %s  %s  %s  %s\n",
		dot,
		urlStyle.Render(url),
		dimStyle.Render("→ :"+strconv.Itoa(port)),
		statusText,
	)
	fmt.Printf("  %s\n", hr)
	fmt.Println()
	fmt.Printf("  %s\n", hintStyle.Render("tunnel close "+closeKey+"  to remove"))
	fmt.Println()
}

func showClosed(key, domain string) {
	hr := dimStyle.Render(strings.Repeat("─", 55))
	url := "https://" + key + "." + domain

	fmt.Println()
	fmt.Printf("  %s\n", gradientTunnel()+boldStyle.Render("  close"))
	fmt.Printf("  %s\n", hr)
	fmt.Printf("  %s  %s\n", deadStyle.Render("○"), dimStyle.Render(url+"  removed"))
	fmt.Printf("  %s\n", hr)
	fmt.Println()
}

func showServicesStopped() {
	fmt.Printf("  %s  %s\n", dimStyle.Render("◼"), dimStyle.Render("no active ports — background services stopped"))
	fmt.Println()
}

func showList(domain string, nameKeys, portKeys []string, all map[string]int, unregistered []int, showAll bool) {
	hr := dimStyle.Render(strings.Repeat("─", 55))

	suffix := "  list"
	if showAll {
		suffix = "  list -a"
	}

	fmt.Println()
	fmt.Printf("  %s\n", gradientTunnel()+boldStyle.Render(suffix))

	if showAll {
		if len(nameKeys) == 0 && len(portKeys) == 0 && len(unregistered) == 0 {
			fmt.Printf("  %s\n", hr)
			fmt.Printf("  %s\n", dimStyle.Render("nothing registered"))
			fmt.Printf("  %s\n", hr)
			fmt.Println()
			return
		}

		if len(nameKeys) > 0 {
			fmt.Println()
			fmt.Printf("  %s\n", sectionStyle.Render("Named routes"))
			fmt.Printf("  %s\n", hr)
			sorted := make([]string, len(nameKeys))
			copy(sorted, nameKeys)
			sort.Strings(sorted)
			for _, k := range sorted {
				fmt.Printf("  %s  %-14s  %s\n",
					statusDot(isListening(all[k])),
					k,
					urlStyle.Render("https://"+k+"."+domain),
				)
			}
		}

		if len(portKeys) > 0 {
			fmt.Println()
			fmt.Printf("  %s\n", sectionStyle.Render("Registered ports"))
			fmt.Printf("  %s\n", hr)
			sorted := make([]string, len(portKeys))
			copy(sorted, portKeys)
			sort.Slice(sorted, func(i, j int) bool {
				a, _ := strconv.Atoi(sorted[i])
				b, _ := strconv.Atoi(sorted[j])
				return a < b
			})
			for _, k := range sorted {
				p := all[k]
				fmt.Printf("  %s  %-14s  %s\n",
					statusDot(isListening(p)),
					k,
					urlStyle.Render("https://"+strconv.Itoa(p)+"."+domain),
				)
			}
		}

		if len(unregistered) > 0 {
			fmt.Println()
			fmt.Printf("  %s\n", sectionStyle.Render("Other listening"))
			fmt.Printf("  %s\n", hr)
			sortedU := make([]int, len(unregistered))
			copy(sortedU, unregistered)
			sort.Ints(sortedU)
			for _, p := range sortedU {
				fmt.Printf("  %s  %-14s  %s\n",
					liveStyle.Render("●"),
					strconv.Itoa(p),
					urlStyle.Render("https://"+strconv.Itoa(p)+"."+domain),
				)
			}
			fmt.Println()
			fmt.Printf("  %s\n", hintStyle.Render("run 'tunnel <port>' to register an untracked port"))
		}

		fmt.Println()
		return
	}

	// Default: active-only flat view
	fmt.Printf("  %s\n", hr)

	count := 0

	sortedNames := make([]string, 0)
	for _, k := range nameKeys {
		if isListening(all[k]) {
			sortedNames = append(sortedNames, k)
		}
	}
	sort.Strings(sortedNames)
	for _, k := range sortedNames {
		fmt.Printf("  %s  %-14s  %s\n",
			liveStyle.Render("●"),
			k,
			urlStyle.Render("https://"+k+"."+domain),
		)
		count++
	}

	sortedPorts := make([]string, 0)
	for _, k := range portKeys {
		if isListening(all[k]) {
			sortedPorts = append(sortedPorts, k)
		}
	}
	sort.Slice(sortedPorts, func(i, j int) bool {
		a, _ := strconv.Atoi(sortedPorts[i])
		b, _ := strconv.Atoi(sortedPorts[j])
		return a < b
	})
	for _, k := range sortedPorts {
		p := all[k]
		fmt.Printf("  %s  %-14s  %s\n",
			liveStyle.Render("●"),
			k,
			urlStyle.Render("https://"+strconv.Itoa(p)+"."+domain),
		)
		count++
	}

	if count == 0 {
		fmt.Printf("  %s\n", dimStyle.Render("nothing active"))
	}

	fmt.Printf("  %s\n", hr)
	fmt.Println()

	hasInactive := false
	for _, k := range nameKeys {
		if !isListening(all[k]) {
			hasInactive = true
			break
		}
	}
	if !hasInactive {
		for _, k := range portKeys {
			if !isListening(all[k]) {
				hasInactive = true
				break
			}
		}
	}
	if hasInactive {
		fmt.Printf("  %s\n", hintStyle.Render("tunnel list -a  to show all registered"))
		fmt.Println()
	}
}

// ── Block confirmation prompt ─────────────────────────────────────────────────

type blockConfirmModel struct {
	port   int
	cursor int // 0 = stop & block, 1 = cancel
	choice int // -1 = undecided, 0 = confirmed, 1 = cancelled
}

var (
	promptTitleStyle  = lipgloss.NewStyle().Bold(true)
	choiceActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4C618F")).Bold(true)
	choiceStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#555566"))
)

func (m blockConfirmModel) Init() tea.Cmd { return nil }

func (m blockConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < 1 {
				m.cursor++
			}
		case "enter", " ":
			m.choice = m.cursor
			return m, tea.Quit
		case "ctrl+c", "q", "esc":
			m.choice = 1 // cancel
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m blockConfirmModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s\n\n", gradientTunnel()+boldStyle.Render("  block"))
	fmt.Fprintf(&b, "  %s\n\n", promptTitleStyle.Render(fmt.Sprintf(":%d is currently listening.", m.port)))
	choices := []string{"Stop process and block", "Cancel"}
	for i, c := range choices {
		cursor := "  "
		var label string
		if i == m.cursor {
			cursor = warnStyle.Render("❯ ")
			label = choiceActiveStyle.Render(c)
		} else {
			label = choiceStyle.Render(c)
		}
		fmt.Fprintf(&b, "  %s%s\n", cursor, label)
	}
	fmt.Fprintf(&b, "\n  %s\n", hintStyle.Render("↑↓ move  enter confirm"))
	return b.String()
}

// confirmBlock shows an interactive prompt and returns true if the user chose
// "Stop process and block".
func confirmBlock(port int) bool {
	m := blockConfirmModel{port: port, choice: -1}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return false
	}
	if fm, ok := final.(blockConfirmModel); ok {
		return fm.choice == 0
	}
	return false
}

func showBlocked(port int) {
	fmt.Println()
	fmt.Printf("  %s  %s\n", doneStyle.Render("✓"), dimStyle.Render(fmt.Sprintf(":%d blocked", port)))
	fmt.Println()
}

func showUnblocked(port int) {
	fmt.Println()
	fmt.Printf("  %s  %s\n", doneStyle.Render("✓"), dimStyle.Render(fmt.Sprintf(":%d unblocked", port)))
	fmt.Println()
}

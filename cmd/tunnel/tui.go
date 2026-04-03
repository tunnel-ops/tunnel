package main

import (
	"fmt"
	"sort"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	urlStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4C8FFF"))
	liveStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3DD68C"))
	deadStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666677"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3DD68C")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F4A261"))
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#888899"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555566"))
	hintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555566"))
)

func statusDot(listening bool) string {
	if listening {
		return liveStyle.Render("●")
	}
	return deadStyle.Render("○")
}

func showTunnelURL(url, closeKey string, port int, listening bool, serviceIssue string) {
	fmt.Println()
	fmt.Printf("  %s\n", urlStyle.Render(url))
	fmt.Println()

	if serviceIssue != "" {
		fmt.Printf("  %s  %s\n",
			warnStyle.Render("⚠"),
			warnStyle.Render(serviceIssue+" — run 'tunnel setup' or check launchctl"),
		)
	} else if listening {
		fmt.Printf("  %s  %s\n", liveStyle.Render("●"), liveStyle.Render("live"))
	} else {
		fmt.Printf("  %s  %s\n", deadStyle.Render("○"), warnStyle.Render("nothing listening on :"+strconv.Itoa(port)+" yet"))
	}

	fmt.Println()
	fmt.Printf("  %s\n", hintStyle.Render("tunnel close "+closeKey+"  to remove"))
	fmt.Println()
}

func showClosed(key, domain string) {
	fmt.Println()
	fmt.Printf("  %s  %s\n", doneStyle.Render("✓"), dimStyle.Render("closed: "+key+"."+domain))
	fmt.Println()
}

func showServicesStopped() {
	fmt.Printf("  %s  %s\n", dimStyle.Render("◼"), dimStyle.Render("no active ports — background services stopped"))
	fmt.Println()
}

func showList(domain string, nameKeys, portKeys []string, all map[string]int, unregistered []int) {
	hasAny := len(nameKeys) > 0 || len(portKeys) > 0 || len(unregistered) > 0

	fmt.Println()

	if !hasAny {
		fmt.Printf("  %s\n", dimStyle.Render("No active services found."))
		fmt.Println()
		return
	}

	if len(nameKeys) > 0 {
		fmt.Printf("  %s\n", sectionStyle.Render("Named routes"))
		fmt.Println()
		sorted := make([]string, len(nameKeys))
		copy(sorted, nameKeys)
		sort.Strings(sorted)
		for _, k := range sorted {
			dot := statusDot(isListening(all[k]))
			fmt.Printf("  %s  %-30s  %s\n",
				dot,
				urlStyle.Render("https://"+k+"."+domain),
				dimStyle.Render(":"+strconv.Itoa(all[k])),
			)
		}
		fmt.Println()
	}

	if len(portKeys) > 0 {
		fmt.Printf("  %s\n", sectionStyle.Render("Registered ports"))
		fmt.Println()
		sorted := make([]string, len(portKeys))
		copy(sorted, portKeys)
		sort.Slice(sorted, func(i, j int) bool {
			a, _ := strconv.Atoi(sorted[i])
			b, _ := strconv.Atoi(sorted[j])
			return a < b
		})
		for _, k := range sorted {
			p := all[k]
			dot := statusDot(isListening(p))
			fmt.Printf("  %s  %s\n",
				dot,
				urlStyle.Render("https://"+strconv.Itoa(p)+"."+domain),
			)
		}
		fmt.Println()
	}

	if len(unregistered) > 0 {
		fmt.Printf("  %s\n", sectionStyle.Render("Other listening ports"))
		fmt.Println()
		sortedPorts := make([]int, len(unregistered))
		copy(sortedPorts, unregistered)
		sort.Ints(sortedPorts)
		for _, p := range sortedPorts {
			fmt.Printf("  %s  %-30s  %s\n",
				liveStyle.Render("●"),
				urlStyle.Render("https://"+strconv.Itoa(p)+"."+domain),
				hintStyle.Render("run 'tunnel "+strconv.Itoa(p)+"' to register"),
			)
		}
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
	choiceActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4C8FFF")).Bold(true)
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
	s := fmt.Sprintf("\n  %s\n\n",
		promptTitleStyle.Render(fmt.Sprintf(":%d is currently listening.", m.port)),
	)
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
		s += fmt.Sprintf("  %s%s\n", cursor, label)
	}
	s += fmt.Sprintf("\n  %s\n", hintStyle.Render("↑↓ move  enter confirm"))
	return s
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

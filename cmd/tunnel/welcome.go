package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func cmdWelcome() {
	var (
		c1 = lipgloss.NewStyle().Foreground(lipgloss.Color("#4C8FFF"))
		c2 = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B72FF"))
		c3 = lipgloss.NewStyle().Foreground(lipgloss.Color("#7E65FF"))
		c4 = lipgloss.NewStyle().Foreground(lipgloss.Color("#9148FF"))

		subtleSt = lipgloss.NewStyle().Foreground(lipgloss.Color("#666677"))
		boldSt  = lipgloss.NewStyle().Bold(true)
		dimSt   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888899"))
		labelSt = lipgloss.NewStyle().Bold(true).Width(10)
	)

	// ── Logo + branding ───────────────────────────────────────────────────────
	// Text is aligned to column 34 (visible chars). Spacing is computed per row:
	//   row 1:  "    ::::"        = 8 visible  → 26 spaces to col 34
	//   row 3:  "        ::::::::::" = 18 visible → 16 spaces to col 34
	//   row 4:  "      ::::::"    = 12 visible  → 22 spaces to col 34

	fmt.Println()
	fmt.Println("  " + c1.Render("::"))
	// "tunnel" rendered letter-by-letter across the blue→purple gradient
	name := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4C8FFF")).Render("t") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6270FF")).Render("u") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7360FF")).Render("n") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8050FF")).Render("n") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8E40FF")).Render("e") +
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9B30FF")).Render("l")

	fmt.Println("  " + "  " + c2.Render("::::") + "                          " + name)
	fmt.Println("  " + "    " + c3.Render("::::::"))
	fmt.Println("  " + "      " + c4.Render("::::::::::") + "                " + subtleSt.Render("expose local ports as public URLs"))
	fmt.Println("  " + "    " + c3.Render("::::::") + "                      " + subtleSt.Render("zero config · cloudflare powered"))
	fmt.Println("  " + "  " + c2.Render("::::"))
	fmt.Println("  " + c1.Render("::"))
	fmt.Println()

	// ── Links ─────────────────────────────────────────────────────────────────
	fmt.Printf("  %s  %s\n",
		labelSt.Render("GitHub"),
		dimSt.Render("https://github.com/tunnel-ops/tunnel  (please leave a star ⭐)"),
	)
	fmt.Println()

	// ── Quick commands ────────────────────────────────────────────────────────
	fmt.Printf("  %s %s %s\n", dimSt.Render("Run"), boldSt.Render("tunnel --help"), dimSt.Render("for all commands."))
	fmt.Printf("  %s %s %s\n", dimSt.Render("Run"), boldSt.Render("tunnel setup"), dimSt.Render("to configure your domain."))
	fmt.Printf("  %s %s %s\n", dimSt.Render("Run"), boldSt.Render("tunnel list"), dimSt.Render("to see active tunnels."))
	fmt.Println()
}

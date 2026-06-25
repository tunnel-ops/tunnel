package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func cmdHelp() {
	hr := dimStyle.Render(strings.Repeat("─", 55))

	cmd := func(c, desc string) {
		fmt.Printf("  %s  %s\n", boldStyle.Render(fmt.Sprintf("%-34s", c)), dimStyle.Render(desc))
	}

	fmt.Println()
	fmt.Printf("  %s\n", gradientTunnel()+boldStyle.Render("  help"))
	fmt.Printf("  %s\n", hr)

	cmd("tunnel <port>", "print the public URL")
	cmd("tunnel <port> --open", "open in browser")
	cmd("tunnel --name <name> <port>", "register a named subdomain")
	cmd("tunnel close <port|name>", "remove a registered tunnel")
	cmd("tunnel list", "show active tunnels")
	cmd("tunnel list -a", "show all registered tunnels")
	cmd("tunnel watch", "live request monitor · all ports")
	cmd("tunnel watch <port>", "live request monitor · one port")
	cmd("tunnel block <port>", "block a port from being exposed")
	cmd("tunnel unblock <port>", "remove a port block")
	cmd("tunnel update", "check for a newer release and apply it")
	cmd("tunnel update --enable", "enable automatic updates")
	cmd("tunnel update --disable", "disable automatic updates")
	cmd("tunnel setup", "first-time configuration wizard")
	cmd("tunnel welcome", "show welcome screen")

	fmt.Printf("  %s\n", hr)
	fmt.Println()

	fmt.Printf("  %s\n", sectionStyle.Render("Examples"))
	fmt.Printf("  %s\n", dimStyle.Render("tunnel 5173"))
	fmt.Printf("  %s\n", dimStyle.Render("tunnel --name api 8080 --open"))
	fmt.Printf("  %s\n", dimStyle.Render("tunnel close api"))
	fmt.Printf("  %s\n", dimStyle.Render("tunnel watch 3000"))
	fmt.Println()
}

func cmdWelcome() {
	var (
		c1 = lipgloss.NewStyle().Foreground(lipgloss.Color("#4C618F"))
		c2 = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B5F9E"))
		c3 = lipgloss.NewStyle().Foreground(lipgloss.Color("#6A5CAC"))
		c4 = lipgloss.NewStyle().Foreground(lipgloss.Color("#7958B8"))

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
	name := gradientTunnel()

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

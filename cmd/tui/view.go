package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const bgColor = "#0f1117"

var (
	// row-level styles (applied to the full line)
	rowStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(bgColor)).
			Foreground(lipgloss.Color("#e4e4e7"))

	rowAltStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#151821")).
			Foreground(lipgloss.Color("#e4e4e7"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2e3039")).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#e4e4e7")).
			Background(lipgloss.Color("#1e2030"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ca3af")).
			Background(lipgloss.Color("#1a1b26"))

	emptyStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(bgColor))

	// ANSI color codes for inline cell coloring (no background set, row style handles bg)
	ansiFg     = "\033[22;38;2;228;228;231m" // not bold, fg #e4e4e7 - matches row foreground
	ansiBold   = "\033[1m"
	ansiRed    = "\033[38;2;239;68;68m"
	ansiGreen  = "\033[38;2;34;197;94m"
	ansiYellow = "\033[38;2;234;179;8m"
	ansiIndigo = "\033[38;2;99;102;241m"
	ansiMuted  = "\033[38;2;107;114;128m"

	sortIndicators = []string{"POD", "CONTAINER", "SECRET", "READS/SEC", "CACHED", "LAST READ"}
)

// colWidths returns responsive pod and container column widths based on terminal width
// and user-adjustable ratio (< and > keys).
func (m model) colWidths() (podW, contW int) {
	// Fixed: padding(2) + gaps(5 spaces) + secret(14) + reads(10) + cached(10) + lastRead(9) + tag(6)
	const fixed = 2 + 5 + 14 + 10 + 10 + 9 + 6 // = 56
	flex := m.width - fixed
	if flex < 30 {
		flex = 30
	}
	podW = flex * m.colRatio / 100
	contW = flex - podW
	return
}

func (m model) View() string {
	switch m.phase {
	case phaseSplash:
		return m.viewSplash()
	case phaseGoodbye:
		return m.viewGoodbye()
	}
	return m.viewDashboard()
}

func (m model) viewSplash() string {
	W := ansiFg  // white/default
	I := ansiIndigo
	Y := ansiYellow
	M := ansiMuted
	B := ansiBold

	raccoon := []string{
		"",
		I + "          /\\___/\\" + W,
		I + "         / " + W + "=o_{" + Y + "o" + W + "}=" + I + "" + W,
		I + "        (" + W + "    ^  \\|" + I + " )" + W,
		I + "         \\" + W + "  ~~~" + I + "  \\/" + W,
		I + "         /" + W + "       " + I + "\\" + W,
		I + "        ( ( " + W + "| |" + I + " ) )" + W,
		I + "         \\_\\|" + W + " " + I + "|/_/" + W,
		"",
		B + I + "       secrets-snitcher" + W,
		M + "       eBPF-powered K8s secret monitor" + W,
		"",
		M + "       by " + B + W + "Michael Ridner" + M + " - 2026" + W,
		M + "       github.com/pizzabits/secrets-snitcher" + W,
		"",
		M + "       press any key..." + W,
	}

	var lines []string
	// center vertically
	topPad := 0
	if m.height > len(raccoon) {
		topPad = (m.height - len(raccoon)) / 2
	}
	for i := 0; i < topPad; i++ {
		lines = append(lines, m.fullLine("", emptyStyle))
	}
	for _, line := range raccoon {
		// center horizontally
		centered := line
		if m.width > 0 {
			visLen := len([]rune(stripAnsi(line)))
			pad := (m.width - visLen) / 2
			if pad > 0 {
				centered = strings.Repeat(" ", pad) + line
			}
		}
		lines = append(lines, m.fullLine(centered, emptyStyle))
	}
	for len(lines) < m.height {
		lines = append(lines, m.fullLine("", emptyStyle))
	}
	return strings.Join(lines, "\n")
}

func (m model) viewGoodbye() string {
	W := ansiFg
	I := ansiIndigo
	G := ansiGreen
	Y := ansiYellow
	M := ansiMuted
	B := ansiBold
	R := ansiRed

	_ = Y // reserved for future use
	bye := []string{
		"",
		I + "          /\\___/\\" + W,
		I + "         / " + W + "[o_o]" + I + " \\" + W,
		I + "        (" + W + "    .   " + I + " )" + W,
		I + "         \\" + W + "  ---" + I + "  /" + W,
		I + "         /" + W + "       " + I + "\\" + W,
		I + "        { { " + W + "(_)" + I + " } }" + W,
		I + "         \\ \\" + W + " | " + I + "/ /" + W,
		I + "          \\ \\|/" + W + " " + I + "/" + W,
		I + "           \\_|_/" + W,
		"",
		B + G + "       Secrets are safe... for now." + W,
		"",
		I + "       ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~" + W,
		"",
		M + "       Made with " + R + B + "<3" + M + " by " + B + W + "Michael Ridner" + W,
		M + "       pizzabits / secrets-snitcher" + W,
		M + "       " + I + "github.com/pizzabits/secrets-snitcher" + W,
	}

	var lines []string
	topPad := 0
	if m.height > len(bye) {
		topPad = (m.height - len(bye)) / 2
	}
	for i := 0; i < topPad; i++ {
		lines = append(lines, m.fullLine("", emptyStyle))
	}
	for _, line := range bye {
		centered := line
		if m.width > 0 {
			visLen := len([]rune(stripAnsi(line)))
			pad := (m.width - visLen) / 2
			if pad > 0 {
				centered = strings.Repeat(" ", pad) + line
			}
		}
		lines = append(lines, m.fullLine(centered, emptyStyle))
	}
	for len(lines) < m.height {
		lines = append(lines, m.fullLine("", emptyStyle))
	}
	return strings.Join(lines, "\n")
}

// stripAnsi removes ANSI escape sequences for width calculation
func stripAnsi(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// CSI sequence: skip until letter
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

func (m model) viewDashboard() string {
	var lines []string
	podW, contW := m.colWidths()

	// title
	title := "  " + ansiBold + ansiIndigo + "secrets-snitcher" + ansiFg + "  "
	if m.connected {
		title += ansiGreen + "● connected" + ansiFg
	} else if m.err != nil {
		title += ansiRed + "● disconnected" + ansiFg
	} else {
		title += ansiMuted + "● connecting..." + ansiFg
	}
	lines = append(lines, m.fullLine(title, emptyStyle))
	lines = append(lines, m.fullLine("", emptyStyle))

	// anomaly banner
	entries := m.filteredEntries()
	hasAnomaly := false
	for _, e := range entries {
		if !e.Cached && e.ReadPerSec > 5 {
			hasAnomaly = true
			break
		}
	}
	if hasAnomaly {
		banner := "  " + ansiBold + ansiRed + "!! ANOMALY DETECTED" + ansiFg + " - suspicious secret access pattern"
		lines = append(lines, m.fullLine(banner, emptyStyle))
		lines = append(lines, m.fullLine("", emptyStyle))
	}

	// table header
	hfmt := fmt.Sprintf("  %%-%ds %%-%ds %%-14s %%10s  %%-10s  %%-9s", podW, contW)
	header := fmt.Sprintf(hfmt,
		m.sortLabel(sortPod), m.sortLabel(sortContainer), m.sortLabel(sortSecret),
		m.sortLabel(sortReads), m.sortLabel(sortCached), m.sortLabel(sortLastRead))
	lines = append(lines, m.fullLine(header, headerStyle))

	// rows
	if len(entries) == 0 {
		empty := "  " + ansiMuted + "No secret access detected. Watching..." + ansiFg
		lines = append(lines, m.fullLine(empty, emptyStyle))
	}

	rfmt := fmt.Sprintf("  %%-%ds %%-%ds %%-14s %%s  %%s  %%s%%s", podW, contW)
	for i, e := range entries {
		secret := filepath.Base(e.SecretPath)
		if len(secret) > 14 {
			secret = secret[:11] + "..."
		}

		pod := e.Pod
		if len(pod) > podW {
			pod = pod[:podW-3] + "..."
		}

		container := e.Container
		if len(container) > contW {
			container = container[:contW-3] + "..."
		}

		var readsStr, cachedStr string
		isAlert := !e.Cached && e.ReadPerSec > 5
		if isAlert {
			readsStr = ansiRed + ansiBold + fmt.Sprintf("%10.1f", e.ReadPerSec) + ansiFg
			cachedStr = ansiRed + ansiBold + fmt.Sprintf("%-10s", "ACTIVE") + ansiFg
		} else if e.Cached {
			readsStr = ansiGreen + fmt.Sprintf("%10.2f", e.ReadPerSec) + ansiFg
			cachedStr = ansiGreen + fmt.Sprintf("%-10s", "cached") + ansiFg
		} else {
			readsStr = fmt.Sprintf("%10.2f", e.ReadPerSec)
			cachedStr = fmt.Sprintf("%-10s", "open")
		}

		lastRead := "         "
		if len(e.LastRead) >= 19 {
			lastRead = e.LastRead[11:19] + " "
		}

		tag := "      "
		if firstSeen, ok := m.podFirstSeen[e.Pod]; ok && time.Since(firstSeen) < 30*time.Second {
			tag = ansiYellow + ansiBold + " NEW  " + ansiFg
		}

		row := fmt.Sprintf(rfmt, pod, container, secret, readsStr, cachedStr, lastRead, tag)

		var style lipgloss.Style
		if i == m.cursor {
			style = selectedStyle
		} else if i%2 == 0 {
			style = rowStyle
		} else {
			style = rowAltStyle
		}
		lines = append(lines, m.fullLine(row, style))
	}

	// fill remaining space (reserve 2 lines for status + help)
	usedLines := len(lines) + 2
	if m.height > 0 {
		for i := usedLines; i < m.height; i++ {
			lines = append(lines, m.fullLine("", emptyStyle))
		}
	}

	// status bar
	status := fmt.Sprintf("  %d entries", len(entries))
	if m.lastRefresh.IsZero() {
		status += " | waiting for data..."
	} else {
		status += fmt.Sprintf(" | last refresh: %s", m.lastRefresh.Format("15:04:05"))
	}
	if m.search != "" {
		status += fmt.Sprintf(" | filter: %s", m.search)
	}
	if m.searching {
		status += " | type to search, enter/esc to finish"
	}
	lines = append(lines, m.fullLine(status, statusBarStyle))

	// help
	help := "  j/k:navigate  /:search  s:sort  S:order  </>/arrows:resize cols  q:quit"
	help = ansiMuted + help + ansiFg
	lines = append(lines, m.fullLine(help, emptyStyle))

	return strings.Join(lines, "\n")
}

func (m model) fullLine(content string, style lipgloss.Style) string {
	if m.width > 0 {
		return style.Width(m.width).Render(content)
	}
	return style.Render(content)
}

func (m model) sortLabel(col sortColumn) string {
	label := sortIndicators[col]
	if m.sortCol == col {
		if m.sortAsc {
			return label + " ▲"
		}
		return label + " ▼"
	}
	return label
}
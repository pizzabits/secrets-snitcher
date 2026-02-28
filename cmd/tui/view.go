package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#6366f1"))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#e4e4e7")).
			Background(lipgloss.Color("#2e3039")).
			Padding(0, 1)

	rowStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#2e3039"))

	alertStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ef4444"))

	safeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e"))

	newTagStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#eab308"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6b7280"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ca3af")).
			Padding(0, 1)

	sortIndicators = []string{"POD", "CONTAINER", "SECRET", "READS/SEC", "CACHED", "LAST READ"}
)

func (m model) View() string {
	var b strings.Builder

	// title
	b.WriteString(titleStyle.Render("  secrets-snitcher") + "  ")
	if m.connected {
		b.WriteString(safeStyle.Render("● connected"))
	} else if m.err != nil {
		b.WriteString(alertStyle.Render("● disconnected"))
	} else {
		b.WriteString(mutedStyle.Render("● connecting..."))
	}
	b.WriteString("\n\n")

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
		b.WriteString(alertStyle.Render("  !! ANOMALY DETECTED - suspicious secret access pattern"))
		b.WriteString("\n\n")
	}

	// table header
	header := fmt.Sprintf("  %-25s %-18s %-14s %10s  %-10s  %-6s",
		m.sortLabel(sortPod), m.sortLabel(sortContainer), m.sortLabel(sortSecret),
		m.sortLabel(sortReads), m.sortLabel(sortCached), m.sortLabel(sortLastRead))
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// rows
	if len(entries) == 0 {
		b.WriteString(mutedStyle.Render("\n  No secret access detected. Watching...\n"))
	}

	for i, e := range entries {
		secret := filepath.Base(e.SecretPath)
		if len(secret) > 14 {
			secret = secret[:11] + "..."
		}

		pod := e.Pod
		if len(pod) > 25 {
			pod = pod[:22] + "..."
		}

		container := e.Container
		if len(container) > 18 {
			container = container[:15] + "..."
		}

		// status coloring
		var readsStr, cachedStr string
		if !e.Cached && e.ReadPerSec > 5 {
			readsStr = alertStyle.Render(fmt.Sprintf("%10.1f", e.ReadPerSec))
			cachedStr = alertStyle.Render("ACTIVE")
		} else if e.Cached {
			readsStr = safeStyle.Render(fmt.Sprintf("%10.2f", e.ReadPerSec))
			cachedStr = safeStyle.Render("cached")
		} else {
			readsStr = fmt.Sprintf("%10.2f", e.ReadPerSec)
			cachedStr = "open"
		}

		// last read - time only
		lastRead := ""
		if len(e.LastRead) >= 19 {
			lastRead = e.LastRead[11:19]
		}

		// new tag
		tag := "      "
		if m.newPods[e.Pod] {
			tag = newTagStyle.Render(" NEW  ")
		}

		row := fmt.Sprintf("  %-25s %-18s %-14s %s  %-10s  %s%s",
			pod, container, secret, readsStr, cachedStr, lastRead, tag)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(rowStyle.Render(row))
		}
		b.WriteString("\n")
	}

	// status bar
	b.WriteString("\n")
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
	b.WriteString(statusBarStyle.Render(status))

	// help
	b.WriteString("\n")
	help := "  j/k:navigate  /:search  s:sort column  S:toggle order  q:quit"
	b.WriteString(mutedStyle.Render(help))

	return b.String()
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

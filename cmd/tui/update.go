package main

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always handle window size and phase transitions
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case splashDoneMsg:
		m.phase = phaseDashboard
		return m, nil
	case goodbyeDoneMsg:
		return m, tea.Quit
	}

	// During splash, only q/ctrl+c can exit, skip everything else
	if m.phase == phaseSplash {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			default:
				m.phase = phaseDashboard
				return m, nil
			}
		}
		// Still process data in background during splash
		return m.handleDataMsg(msg)
	}

	// During goodbye, ignore all input
	if m.phase == phaseGoodbye {
		return m, nil
	}

	// Dashboard phase
	switch msg := msg.(type) {
	case tickMsg:
		return m, tea.Batch(
			fetchData(m.client),
			tickCmd(m.interval),
		)
	case dataMsg:
		return m.handleDataUpdate(msg)
	case tea.KeyMsg:
		if m.searching {
			return m.handleSearchKey(msg)
		}
		return m.handleNormalKey(msg)
	}

	return m, nil
}

func (m model) handleDataMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tea.Batch(
			fetchData(m.client),
			tickCmd(m.interval),
		)
	case dataMsg:
		return m.handleDataUpdate(msg)
	}
	return m, nil
}

func (m model) handleDataUpdate(msg dataMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.connected = false
		return m, nil
	}
	m.err = nil
	m.connected = true
	m.lastRefresh = time.Now()

	// detect new pods (track first-seen time, show NEW for 30s)
	now := time.Now()
	currentPods := make(map[string]bool)
	for _, e := range msg.response.Entries {
		currentPods[e.Pod] = true
		if !m.prevPods[e.Pod] {
			m.podFirstSeen[e.Pod] = now
		}
	}
	// clean up pods that disappeared
	for pod := range m.podFirstSeen {
		if !currentPods[pod] {
			delete(m.podFirstSeen, pod)
		}
	}
	m.prevPods = currentPods
	m.entries = msg.response.Entries
	m.sortEntries()

	if m.cursor >= len(m.entries) && len(m.entries) > 0 {
		m.cursor = len(m.entries) - 1
	}
	return m, nil
}

func (m model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.phase = phaseGoodbye
		return m, goodbyeTimer()
	case "j", "down":
		if m.cursor < len(m.filteredEntries())-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		entries := m.filteredEntries()
		if len(entries) > 0 {
			m.cursor = len(entries) - 1
		}
	case "/":
		m.searching = true
		m.search = ""
		m.cursor = 0
	case "s":
		m.sortCol = (m.sortCol + 1) % 6
		m.sortEntries()
	case "S":
		m.sortAsc = !m.sortAsc
		m.sortEntries()
	case "<", "left":
		if m.colRatio > 20 {
			m.colRatio -= 5
		}
	case ">", "right":
		if m.colRatio < 80 {
			m.colRatio += 5
		}
	case "esc":
		m.search = ""
		m.cursor = 0
	}
	return m, nil
}

func (m model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.searching = false
	case "backspace":
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.search += msg.String()
		}
	}
	m.cursor = 0
	return m, nil
}

func (m *model) sortEntries() {
	sort.Slice(m.entries, func(i, j int) bool {
		var less bool
		switch m.sortCol {
		case sortPod:
			less = m.entries[i].Pod < m.entries[j].Pod
		case sortContainer:
			less = m.entries[i].Container < m.entries[j].Container
		case sortSecret:
			less = m.entries[i].SecretPath < m.entries[j].SecretPath
		case sortReads:
			less = m.entries[i].ReadPerSec < m.entries[j].ReadPerSec
		case sortCached:
			less = !m.entries[i].Cached && m.entries[j].Cached
		case sortLastRead:
			less = m.entries[i].LastRead < m.entries[j].LastRead
		}
		if m.sortAsc {
			return less
		}
		return !less
	})
}

func (m model) filteredEntries() []Entry {
	if m.search == "" {
		return m.entries
	}
	var filtered []Entry
	q := strings.ToLower(m.search)
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.Pod), q) ||
			strings.Contains(strings.ToLower(e.Container), q) ||
			strings.Contains(strings.ToLower(e.SecretPath), q) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type phase int

const (
	phaseSplash phase = iota
	phaseDashboard
	phaseGoodbye
)

type tickMsg time.Time
type splashDoneMsg struct{}
type goodbyeDoneMsg struct{}

type dataMsg struct {
	response *APIResponse
	err      error
}

type sortColumn int

const (
	sortPod sortColumn = iota
	sortContainer
	sortSecret
	sortReads
	sortCached
	sortLastRead
)

type model struct {
	phase        phase
	client       *Client
	entries      []Entry
	prevPods     map[string]bool
	podFirstSeen map[string]time.Time
	cursor       int
	width        int
	height       int
	err          error
	connected    bool
	lastRefresh  time.Time
	interval     time.Duration
	search       string
	searching    bool
	sortCol      sortColumn
	sortAsc      bool
	colRatio     int // percentage of flex space for pod column (0-100)
}

func initialModel(apiURL string, interval time.Duration) model {
	return model{
		phase:        phaseSplash,
		client:       NewClient(apiURL),
		prevPods:     make(map[string]bool),
		podFirstSeen: make(map[string]time.Time),
		interval:     interval,
		sortCol:      sortReads,
		sortAsc:      false,
		colRatio:     55,
	}
}

func splashTimer() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return splashDoneMsg{}
	})
}

func goodbyeTimer() tea.Cmd {
	return tea.Tick(1700*time.Millisecond, func(t time.Time) tea.Msg {
		return goodbyeDoneMsg{}
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		splashTimer(),
		fetchData(m.client),
		tickCmd(m.interval),
	)
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchData(c *Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.FetchEntries()
		return dataMsg{response: resp, err: err}
	}
}
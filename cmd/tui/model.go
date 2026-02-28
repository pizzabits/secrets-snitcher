package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type tickMsg time.Time

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
	client       *Client
	entries      []Entry
	prevPods     map[string]bool
	newPods      map[string]bool
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
}

func initialModel(apiURL string, interval time.Duration) model {
	return model{
		client:   NewClient(apiURL),
		prevPods: make(map[string]bool),
		newPods:  make(map[string]bool),
		interval: interval,
		sortCol:  sortReads,
		sortAsc:  false,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
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

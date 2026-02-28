package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateWindowSize(t *testing.T) {
	m := testModel(nil)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	result := updated.(model)

	if result.width != 200 || result.height != 50 {
		t.Errorf("got %dx%d, want 200x50", result.width, result.height)
	}
	if cmd != nil {
		t.Error("WindowSizeMsg should return nil cmd")
	}
}

func TestUpdateTickReturnsFetchAndTick(t *testing.T) {
	m := testModel(nil)
	_, cmd := m.Update(tickMsg(time.Now()))

	if cmd == nil {
		t.Fatal("tickMsg should return a batch cmd")
	}
}

func TestUpdateDataMsgSuccess(t *testing.T) {
	m := testModel(nil)
	m.connected = false

	resp := &APIResponse{
		Timestamp:             "2025-01-15T13:53:57Z",
		ObservationWindowSecs: 60,
		Entries:               sampleEntries(),
	}

	updated, _ := m.Update(dataMsg{response: resp})
	result := updated.(model)

	if !result.connected {
		t.Error("should be connected after successful data")
	}
	if result.err != nil {
		t.Errorf("err should be nil, got %v", result.err)
	}
	if len(result.entries) != 3 {
		t.Errorf("got %d entries, want 3", len(result.entries))
	}
	if result.lastRefresh.IsZero() {
		t.Error("lastRefresh should be set")
	}
}

func TestUpdateDataMsgError(t *testing.T) {
	m := testModel(sampleEntries())
	m.connected = true

	updated, _ := m.Update(dataMsg{err: errTest})
	result := updated.(model)

	if result.connected {
		t.Error("should be disconnected on error")
	}
	if result.err == nil {
		t.Error("err should be set")
	}
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }

func TestUpdateDataMsgDetectsNewPods(t *testing.T) {
	m := testModel(nil)
	m.prevPods = map[string]bool{"old-pod": true}

	entries := []Entry{
		{Pod: "old-pod", Container: "c1", SecretPath: "/s1"},
		{Pod: "new-pod", Container: "c2", SecretPath: "/s2"},
	}
	resp := &APIResponse{Entries: entries}

	updated, _ := m.Update(dataMsg{response: resp})
	result := updated.(model)

	if _, ok := result.podFirstSeen["new-pod"]; !ok {
		t.Error("new-pod should be detected as new")
	}
	if _, ok := result.podFirstSeen["old-pod"]; ok {
		t.Error("old-pod should NOT be marked as new")
	}
}

func TestUpdateDataMsgClampsCursor(t *testing.T) {
	m := testModel(nil)
	m.cursor = 10

	resp := &APIResponse{Entries: []Entry{{Pod: "p", Container: "c", SecretPath: "/s"}}}
	updated, _ := m.Update(dataMsg{response: resp})
	result := updated.(model)

	if result.cursor != 0 {
		t.Errorf("cursor should clamp to 0 (1 entry), got %d", result.cursor)
	}
}

func TestNavigationKeys(t *testing.T) {
	entries := sampleEntries()
	m := testModel(entries)
	m.cursor = 0

	// j moves down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	result := updated.(model)
	if result.cursor != 1 {
		t.Errorf("j: cursor=%d, want 1", result.cursor)
	}

	// k moves up
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	result = updated.(model)
	if result.cursor != 0 {
		t.Errorf("k: cursor=%d, want 0", result.cursor)
	}

	// k at top stays at 0
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	result = updated.(model)
	if result.cursor != 0 {
		t.Errorf("k at top: cursor=%d, want 0", result.cursor)
	}

	// G jumps to end
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	result = updated.(model)
	if result.cursor != len(entries)-1 {
		t.Errorf("G: cursor=%d, want %d", result.cursor, len(entries)-1)
	}

	// g jumps to start
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	result = updated.(model)
	if result.cursor != 0 {
		t.Errorf("g: cursor=%d, want 0", result.cursor)
	}
}

func TestNavigationAtBounds(t *testing.T) {
	m := testModel(sampleEntries())
	m.cursor = len(m.entries) - 1

	// j at bottom stays at bottom
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	result := updated.(model)
	if result.cursor != len(m.entries)-1 {
		t.Errorf("j at bottom: cursor=%d, want %d", result.cursor, len(m.entries)-1)
	}
}

func TestSearchMode(t *testing.T) {
	m := testModel(sampleEntries())

	// / enters search mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	result := updated.(model)
	if !result.searching {
		t.Error("/ should enter search mode")
	}
	if result.search != "" {
		t.Error("search should be empty on enter")
	}

	// typing adds characters
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	result = updated.(model)
	if result.search != "a" {
		t.Errorf("search=%q, want 'a'", result.search)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	result = updated.(model)
	if result.search != "ap" {
		t.Errorf("search=%q, want 'ap'", result.search)
	}

	// backspace removes character
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	result = updated.(model)
	if result.search != "a" {
		t.Errorf("backspace: search=%q, want 'a'", result.search)
	}

	// enter exits search mode
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result = updated.(model)
	if result.searching {
		t.Error("enter should exit search mode")
	}
	if result.search != "a" {
		t.Error("search text should persist after enter")
	}
}

func TestSearchEscExits(t *testing.T) {
	m := testModel(sampleEntries())
	m.searching = true
	m.search = "test"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	result := updated.(model)

	if result.searching {
		t.Error("esc should exit search mode")
	}
}

func TestSearchBackspaceOnEmpty(t *testing.T) {
	m := testModel(sampleEntries())
	m.searching = true
	m.search = ""

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	result := updated.(model)

	if result.search != "" {
		t.Errorf("backspace on empty should stay empty, got %q", result.search)
	}
}

func TestFilteredEntries(t *testing.T) {
	entries := []Entry{
		{Pod: "payment-svc", Container: "pay", SecretPath: "/token"},
		{Pod: "frontend", Container: "nginx", SecretPath: "/cert"},
		{Pod: "vault-agent", Container: "vault", SecretPath: "/db-creds"},
	}
	m := testModel(entries)

	// No filter returns all
	if len(m.filteredEntries()) != 3 {
		t.Errorf("no filter: got %d, want 3", len(m.filteredEntries()))
	}

	// Filter by pod
	m.search = "pay"
	filtered := m.filteredEntries()
	if len(filtered) != 1 || filtered[0].Pod != "payment-svc" {
		t.Errorf("filter 'pay': got %d entries", len(filtered))
	}

	// Filter by container
	m.search = "nginx"
	filtered = m.filteredEntries()
	if len(filtered) != 1 || filtered[0].Container != "nginx" {
		t.Errorf("filter 'nginx': got %d entries", len(filtered))
	}

	// Filter by secret path
	m.search = "creds"
	filtered = m.filteredEntries()
	if len(filtered) != 1 || filtered[0].Pod != "vault-agent" {
		t.Errorf("filter 'creds': got %d entries", len(filtered))
	}

	// Case insensitive
	m.search = "VAULT"
	filtered = m.filteredEntries()
	if len(filtered) != 1 {
		t.Errorf("case insensitive filter: got %d, want 1", len(filtered))
	}

	// No match
	m.search = "nonexistent"
	if len(m.filteredEntries()) != 0 {
		t.Error("should return empty for no match")
	}
}

func TestSortEntries(t *testing.T) {
	entries := []Entry{
		{Pod: "b-pod", Container: "b-cont", SecretPath: "/b", ReadPerSec: 10, Cached: false, LastRead: "2025-01-15T13:00:00Z"},
		{Pod: "a-pod", Container: "a-cont", SecretPath: "/a", ReadPerSec: 5, Cached: true, LastRead: "2025-01-15T14:00:00Z"},
		{Pod: "c-pod", Container: "c-cont", SecretPath: "/c", ReadPerSec: 20, Cached: false, LastRead: "2025-01-15T12:00:00Z"},
	}

	tests := []struct {
		col     sortColumn
		asc     bool
		firstPod string
	}{
		{sortPod, true, "a-pod"},
		{sortPod, false, "c-pod"},
		{sortReads, true, "a-pod"},    // 5 first
		{sortReads, false, "c-pod"},   // 20 first
		{sortContainer, true, "a-pod"},
		{sortSecret, true, "a-pod"},
		{sortLastRead, true, "c-pod"}, // 12:00 first
	}

	for _, tt := range tests {
		m := testModel(nil)
		// Make a copy of entries so sorts don't interfere
		m.entries = make([]Entry, len(entries))
		copy(m.entries, entries)
		m.sortCol = tt.col
		m.sortAsc = tt.asc
		m.sortEntries()

		if m.entries[0].Pod != tt.firstPod {
			t.Errorf("sort col=%d asc=%v: first pod=%q, want %q",
				tt.col, tt.asc, m.entries[0].Pod, tt.firstPod)
		}
	}
}

func TestSortCycleKey(t *testing.T) {
	m := testModel(sampleEntries())
	m.sortCol = sortPod

	// s cycles to next column
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	result := updated.(model)
	if result.sortCol != sortContainer {
		t.Errorf("s: sortCol=%d, want %d", result.sortCol, sortContainer)
	}

	// S toggles direction
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	result = updated.(model)
	if !result.sortAsc {
		t.Error("S should toggle sortAsc to true")
	}
}

func TestColResizeKeys(t *testing.T) {
	m := testModel(nil)
	m.colRatio = 55

	// < shrinks pod column
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	result := updated.(model)
	if result.colRatio != 50 {
		t.Errorf("<: colRatio=%d, want 50", result.colRatio)
	}

	// > grows pod column
	result.colRatio = 55
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	result = updated.(model)
	if result.colRatio != 60 {
		t.Errorf(">: colRatio=%d, want 60", result.colRatio)
	}

	// Can't go below 20
	result.colRatio = 20
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	result = updated.(model)
	if result.colRatio != 20 {
		t.Errorf("< at min: colRatio=%d, want 20", result.colRatio)
	}

	// Can't go above 80
	result.colRatio = 80
	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	result = updated.(model)
	if result.colRatio != 80 {
		t.Errorf("> at max: colRatio=%d, want 80", result.colRatio)
	}
}

func TestEscClearsSearch(t *testing.T) {
	m := testModel(sampleEntries())
	m.search = "test"
	m.cursor = 2

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	result := updated.(model)

	if result.search != "" {
		t.Errorf("esc should clear search, got %q", result.search)
	}
	if result.cursor != 0 {
		t.Errorf("esc should reset cursor, got %d", result.cursor)
	}
}

func TestQuitKey(t *testing.T) {
	m := testModel(nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("q should return quit cmd")
	}
}

func TestViewAnomalyBanner(t *testing.T) {
	// Entry with high reads and not cached triggers anomaly
	entries := []Entry{
		{Pod: "bad-pod", Container: "evil", SecretPath: "/token", ReadPerSec: 100, Cached: false, LastRead: "2025-01-15T13:53:57Z"},
	}
	m := testModel(entries)
	output := m.View()

	if !strings.Contains(output, "ANOMALY") {
		t.Error("should show anomaly banner for high uncached reads")
	}
}

func TestViewNoAnomalyForCached(t *testing.T) {
	// High reads but cached should NOT trigger anomaly
	entries := []Entry{
		{Pod: "ok-pod", Container: "app", SecretPath: "/token", ReadPerSec: 100, Cached: true, LastRead: "2025-01-15T13:53:57Z"},
	}
	m := testModel(entries)
	output := m.View()

	if strings.Contains(output, "ANOMALY") {
		t.Error("cached high reads should not trigger anomaly banner")
	}
}

func TestViewNoAnomalyForLowReads(t *testing.T) {
	// Low uncached reads should NOT trigger anomaly
	entries := []Entry{
		{Pod: "ok-pod", Container: "app", SecretPath: "/token", ReadPerSec: 3, Cached: false, LastRead: "2025-01-15T13:53:57Z"},
	}
	m := testModel(entries)
	output := m.View()

	if strings.Contains(output, "ANOMALY") {
		t.Error("low uncached reads should not trigger anomaly banner")
	}
}

func TestViewSearchStatus(t *testing.T) {
	m := testModel(sampleEntries())
	m.search = "payment"
	output := m.View()

	if !strings.Contains(output, "filter: payment") {
		t.Error("should show active filter in status bar")
	}
}

func TestViewSearchingPrompt(t *testing.T) {
	m := testModel(sampleEntries())
	m.searching = true
	output := m.View()

	if !strings.Contains(output, "type to search") {
		t.Error("should show search prompt when in search mode")
	}
}

func TestSortLabelIndicator(t *testing.T) {
	m := testModel(nil)
	m.sortCol = sortReads
	m.sortAsc = false

	label := m.sortLabel(sortReads)
	if !strings.HasSuffix(label, "▼") {
		t.Errorf("active desc sort should show ▼, got %q", label)
	}

	m.sortAsc = true
	label = m.sortLabel(sortReads)
	if !strings.HasSuffix(label, "▲") {
		t.Errorf("active asc sort should show ▲, got %q", label)
	}

	// Inactive column has no indicator
	label = m.sortLabel(sortPod)
	if strings.Contains(label, "▲") || strings.Contains(label, "▼") {
		t.Errorf("inactive column should have no indicator, got %q", label)
	}
}
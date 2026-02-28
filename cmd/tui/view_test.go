package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func testModel(entries []Entry) model {
	m := initialModel("http://localhost:9100", 2*time.Second)
	m.phase = phaseDashboard
	m.width = 120
	m.height = 25
	m.connected = true
	m.lastRefresh = time.Now()
	m.entries = entries
	return m
}

func sampleEntries() []Entry {
	return []Entry{
		{Pod: "totally-legit-app", Container: "definitely-not-suspicious", SecretPath: "/var/run/secrets/token", ReadPerSec: 4872.3, LastRead: "2025-01-15T13:53:57Z", Cached: false},
		{Pod: "totally-legit-app", Container: "definitely-not-suspicious", SecretPath: "/var/run/secrets/ca.crt", ReadPerSec: 4871.1, LastRead: "2025-01-15T13:53:57Z", Cached: false},
		{Pod: "totally-legit-app", Container: "definitely-not-suspicious", SecretPath: "/var/run/secrets/namespace", ReadPerSec: 4870.8, LastRead: "2025-01-15T13:53:57Z", Cached: false},
	}
}

func TestViewFillsTerminalHeight(t *testing.T) {
	m := testModel(sampleEntries())
	output := m.View()
	lines := strings.Split(output, "\n")

	if len(lines) != m.height {
		t.Errorf("View() produced %d lines, want %d (terminal height)", len(lines), m.height)
	}
}

func TestViewFillsTerminalHeightEmpty(t *testing.T) {
	m := testModel(nil)
	output := m.View()
	lines := strings.Split(output, "\n")

	if len(lines) != m.height {
		t.Errorf("View() with no entries produced %d lines, want %d", len(lines), m.height)
	}
}

func TestViewLinesFullWidth(t *testing.T) {
	m := testModel(sampleEntries())
	output := m.View()
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		w := lipgloss.Width(line)
		if w != m.width {
			t.Errorf("line %d: visible width %d, want %d", i, w, m.width)
		}
	}
}

func TestViewNoOSCEscapes(t *testing.T) {
	m := testModel(sampleEntries())
	output := m.View()

	if strings.Contains(output, "\033]11;") {
		t.Error("View() contains OSC 11 escape sequence - this corrupts terminal colors on exit")
	}
}

func TestViewNoSGRFullReset(t *testing.T) {
	m := testModel(sampleEntries())
	output := m.View()

	// Row content should use ansiFg (explicit #e4e4e7) not \033[0m or \033[22;39m
	// which reset to terminal default foreground instead of our row color
	if strings.Contains(output, "\033[22;39m") {
		t.Error("View() contains \\033[22;39m - use ansiFg to preserve row foreground color")
	}
}

func TestViewDisconnected(t *testing.T) {
	m := testModel(nil)
	m.connected = false
	m.lastRefresh = time.Time{}
	output := m.View()
	lines := strings.Split(output, "\n")

	if len(lines) != m.height {
		t.Errorf("disconnected View() produced %d lines, want %d", len(lines), m.height)
	}
}

func TestColWidthsResponsive(t *testing.T) {
	tests := []struct {
		width   int
		minPod  int
		minCont int
	}{
		{120, 30, 25},
		{80, 10, 8},
		{200, 50, 40},
	}
	for _, tt := range tests {
		m := testModel(nil)
		m.width = tt.width
		podW, contW := m.colWidths()
		if podW < tt.minPod {
			t.Errorf("width=%d: podW=%d, want >= %d", tt.width, podW, tt.minPod)
		}
		if contW < tt.minCont {
			t.Errorf("width=%d: contW=%d, want >= %d", tt.width, contW, tt.minCont)
		}
	}
}

func TestColWidthsNarrowTerminal(t *testing.T) {
	m := testModel(nil)
	m.width = 40 // very narrow
	podW, contW := m.colWidths()

	// Should not panic or return negative values
	if podW < 1 || contW < 1 {
		t.Errorf("narrow terminal: podW=%d, contW=%d - must be positive", podW, contW)
	}
}

func TestColWidthsUserAdjustable(t *testing.T) {
	m := testModel(nil)
	m.width = 120

	// Default ratio (55)
	podDefault, contDefault := m.colWidths()

	// Shift toward container (lower ratio = narrower pod)
	m.colRatio = 30
	podSmall, contLarge := m.colWidths()

	if podSmall >= podDefault {
		t.Errorf("lowering ratio should shrink pod: got %d >= %d", podSmall, podDefault)
	}
	if contLarge <= contDefault {
		t.Errorf("lowering ratio should grow container: got %d <= %d", contLarge, contDefault)
	}

	// Shift toward pod (higher ratio = wider pod)
	m.colRatio = 80
	podLarge, contSmall := m.colWidths()

	if podLarge <= podDefault {
		t.Errorf("raising ratio should grow pod: got %d <= %d", podLarge, podDefault)
	}
	if contSmall >= contDefault {
		t.Errorf("raising ratio should shrink container: got %d >= %d", contSmall, contDefault)
	}
}
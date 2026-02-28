package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSetTerminalBgOutput(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	setTerminalBg("#0f1117")

	os.Stdout = old
	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	got := buf.String()

	want := "\033]11;#0f1117\007"
	if got != want {
		t.Errorf("setTerminalBg output = %q, want %q", got, want)
	}
}

func TestResetTerminalBgOutput(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	resetTerminalBg()

	os.Stdout = old
	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	got := buf.String()

	want := "\033]111\007"
	if got != want {
		t.Errorf("resetTerminalBg output = %q, want %q", got, want)
	}
}

func TestSetTerminalBgRestoresOriginal(t *testing.T) {
	// Simulate: if we captured "rgb:ffff/ffff/dddd", restore should emit that exact color
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	origBg := "rgb:ffff/ffff/dddd"
	setTerminalBg(origBg)

	os.Stdout = old
	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	got := buf.String()

	if !strings.Contains(got, origBg) {
		t.Errorf("restore should contain original color %q, got %q", origBg, got)
	}
}

func TestQueryTerminalBgNonTerminal(t *testing.T) {
	// When stdin is not a terminal (e.g. in CI), query should return empty gracefully
	result := queryTerminalBg()
	if result != "" {
		t.Logf("queryTerminalBg returned %q (running in a real terminal)", result)
	}
}
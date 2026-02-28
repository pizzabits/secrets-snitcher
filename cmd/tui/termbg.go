package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// queryTerminalBg queries the terminal for its current background color via OSC 11.
// Returns the color string (e.g. "rgb:ffff/ffff/dddd") or empty if unsupported.
func queryTerminalBg() string {
	fd := int(os.Stdin.Fd())

	// Save current terminal state and switch to raw mode for reading the response
	orig, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return ""
	}

	raw := *orig
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1 // 100ms timeout in deciseconds
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &raw); err != nil {
		return ""
	}
	defer func() { _ = unix.IoctlSetTermios(fd, ioctlSetTermios, orig) }()

	// Send OSC 11 query
	fmt.Print("\033]11;?\033\\")

	// Read response with timeout
	buf := make([]byte, 128)
	var resp strings.Builder
	deadline := time.Now().Add(200 * time.Millisecond)

	for time.Now().Before(deadline) {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			s := resp.String()
			if strings.Contains(s, "\007") || strings.Contains(s, "\033\\") {
				break
			}
		}
		if err != nil {
			break
		}
	}

	// Parse: expect \033]11;rgb:RRRR/GGGG/BBBB\033\\ or \007
	s := resp.String()
	if idx := strings.Index(s, "rgb:"); idx >= 0 {
		end := len(s)
		for i := idx; i < len(s); i++ {
			if s[i] == '\007' || s[i] == '\033' {
				end = i
				break
			}
		}
		return s[idx:end]
	}
	return ""
}

// setTerminalBg sets the terminal background color via OSC 11.
func setTerminalBg(color string) {
	fmt.Printf("\033]11;%s\007", color)
}

// resetTerminalBg restores the terminal background to its default.
func resetTerminalBg() {
	fmt.Print("\033]111\007")
}
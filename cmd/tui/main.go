package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func main() {
	var apiURL string
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Terminal UI for secrets-snitcher",
		Long:  "Interactive terminal dashboard for monitoring Kubernetes secret access detected by secrets-snitcher's eBPF probe.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Save original terminal bg, set ours, restore on exit
			origBg := queryTerminalBg()
			setTerminalBg("#0f1117")
			defer func() {
				if origBg != "" {
					setTerminalBg(origBg)
				} else {
					resetTerminalBg()
				}
			}()

			m := initialModel(apiURL, interval)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	cmd.Flags().StringVar(&apiURL, "api", "http://localhost:9100", "secrets-snitcher API endpoint")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "polling interval")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
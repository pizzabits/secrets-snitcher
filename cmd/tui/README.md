# secrets-snitcher TUI

Interactive terminal dashboard for monitoring Kubernetes secret access detected by secrets-snitcher's eBPF probe.

## Quick start

```bash
# Build
make tui

# Run against a live cluster (requires port-forward to secrets-snitcher service)
kubectl -n secrets-snitcher port-forward svc/secrets-snitcher 9100:9100
./secrets-snitcher-tui --api http://localhost:9100

# Or run with the demo mock API
make mock-api                    # Terminal 1
curl localhost:9100/toggle       # Terminal 2 - activate sample data
./secrets-snitcher-tui           # Terminal 3 (defaults to localhost:9100)
```

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--api` | `http://localhost:9100` | secrets-snitcher API endpoint |
| `--interval` | `2s` | Polling interval |

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| `j` / `k` or arrows | Navigate rows |
| `g` / `G` | Jump to first / last row |
| `/` | Search (filters by pod, container, or secret) |
| `s` | Cycle sort column |
| `S` | Toggle sort direction |
| `<` / `>` or left/right | Resize pod/container columns |
| `q` or Ctrl+C | Quit |

## What you see

- **ANOMALY DETECTED** banner when a pod reads secrets at high frequency without caching (reads/sec > 5, not cached)
- **NEW** tag on pods that appeared since the last poll
- Color-coded read rates: red for anomalies, green for cached, white for normal
- Connection status indicator (green = connected, red = disconnected)

## Architecture

For a deep dive into the code with C/C++ comparisons, see [DEVGUIDE.md](DEVGUIDE.md).

The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) using The Elm Architecture pattern:

```
Init() - start polling timer + first API fetch
  |
  v
Model ---> View() ---> terminal output
  ^
  |
  +--- Update(msg) <--- keyboard input / timer tick / API response
```

| File | Role |
|------|------|
| `main.go` | CLI flags, terminal background management, program entry |
| `model.go` | State definition, initialization, timer/fetch commands |
| `update.go` | Event handling, keyboard input, sorting, filtering |
| `view.go` | Rendering with lipgloss styles and ANSI color codes |
| `client.go` | HTTP client - fetches from `/api/v1/secret-access` |
| `termbg.go` | Terminal background detection/restore via OSC 11 |

## Demo mock API

`demo/mock-api.py` serves realistic data matching the real eBPF probe output. Endpoints:

- `GET /api/v1/secret-access` - secret access entries (empty until toggled)
- `GET /toggle` - switch between empty and realistic cluster data
- `GET /healthz` - health check

## Running tests

```bash
go test ./cmd/tui/ -v
```

## Acknowledgments

The TUI was inspired by [kostyay/netmon](https://github.com/kostyay/netmon) - a beautifully built Bubble Tea dashboard for network monitoring. 
His project showed what a polished terminal UI can look like for system-level tooling and motivated this implementation. No code was taken from netmon, but the idea of building a live-updating Bubble Tea TUI for infrastructure monitoring came directly from seeing his work. 
Thank you [Kostya](https://github.com/kostyay)!

# secrets-snitcher

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

eBPF-powered Kubernetes secret access monitor.

Watches which pods read secret files, how often, and whether they cache values in memory or re-read from disk on every request. Catches suspicious access patterns like a compromised pod hammering service account tokens.

> **NEW** - [Web dashboard](#web-dashboard) with live anomaly timeline, sparklines, and per-pod bar charts. [Prometheus /metrics](#prometheus-metrics) endpoint for Grafana integration. [Interactive TUI](#terminal-ui-tui) for terminal-native monitoring.

## How it works

```
  ┌──────────────────┐       ┌──────────────┐       ┌──────────────────┐
  │ Linux Kernel     │       │ secrets-     │       │ You / your tools │
  │                  │       │ snitcher     │       │                  │
  │ openat() syscall │──────>│ aggregator   │──────>│ GET :9100        │
  │ on secret paths  │ eBPF  │ (60s window) │ HTTP  │ /api/v1/         │
  │                  │       │              │       │ secret-access    │
  └──────────────────┘       └──────────────┘       └──────────────────┘
```

~50 lines of BPF C sit inside the kernel, filtering at the syscall level before anything reaches userspace. Zero overhead for non-secret file access.

1. **eBPF tracepoint** hooks `sys_enter_openat` - the syscall every file open goes through
2. **Kernel-side path filter** checks if the filename starts with a known secret mount path. Non-matching opens are dropped inside the kernel, never copied to userspace
3. **Perf buffer** streams matching events (pid, process name, filename, timestamp) to the Python aggregator
4. **Rolling window aggregator** tracks per-pod read frequency over 60 seconds, resolves pod names via `/proc/{pid}/environ`
5. **HTTP API** on port 9100 serves the current state as JSON

![Demo](/demo/demo.gif)

### What it watches

| Mount path | Source |
|---|---|
| `/var/run/secrets/kubernetes.io/serviceaccount/` | Default K8s service account tokens |
| `/var/secrets/` | Custom secret volume mounts |
| `/mnt/secrets-store/` | CSI Secrets Store driver (Azure Key Vault, AWS Secrets Manager, HashiCorp Vault) |
| `/run/secrets/` | Docker secrets / alternative mounts |

### The `cached` field

A service with `reads_per_sec >= 1` is actively opening the secret file on every request - **not cached**. If you rotate or delete that secret, the service will immediately see the change (or break). A service with `reads_per_sec < 1` has likely read the secret once and cached the value in memory. Nothing in Kubernetes tells you which behavior you're dealing with. This tool does.

## Quick start

No Docker build required. The probe runs as a pod using a public Ubuntu image with BCC installed at runtime.

```bash
# 1. Create namespace + RBAC
kubectl apply -f k8s/rbac.yaml

# 2. Deploy the probe (installs BCC, mounts code as ConfigMap)
kubectl apply -f k8s/pod-inline.yaml

# 3. Wait for it to be ready (~30s for apt-get)
kubectl -n secrets-snitcher wait --for=condition=Ready pod/secrets-snitcher --timeout=120s

# 4. Port-forward and query
kubectl -n secrets-snitcher port-forward svc/secrets-snitcher 9100:9100 &
curl http://localhost:9100/api/v1/secret-access | jq
```

Or use the one-liner:

```bash
curl -sL https://raw.githubusercontent.com/pizzabits/secrets-snitcher/main/install.sh | bash
```

## Testing with a malicious pod

The `demo/` directory includes a deliberately suspicious pod for testing:

```bash
# Deploy test secrets + a pod that hammers them
kubectl apply -f demo/sample-secrets.yaml
kubectl apply -f demo/malicious-pod.yaml

# Wait a few seconds, then query the API
curl http://localhost:9100/api/v1/secret-access | jq

# You should see "totally-legit-app" with very high reads_per_sec

# Clean up
kubectl delete -f demo/malicious-pod.yaml
kubectl delete -f demo/sample-secrets.yaml
```

The `totally-legit-app` pod runs a tight loop reading service account tokens as fast as possible. It will light up in the API as a clear outlier compared to normal workloads.

## API

### `GET /api/v1/secret-access`

Returns all secret file access observed in the rolling window.

```json
{
  "timestamp": "2026-02-14T12:00:00+00:00",
  "observation_window_seconds": 60,
  "entries": [
    {
      "pod": "totally-legit-app",
      "container": "sh",
      "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
      "reads_per_sec": 4872.3,
      "last_read": "2026-02-14T11:59:59+00:00",
      "cached": false
    },
    {
      "pod": "auth-service-7x8d",
      "container": "auth-svc",
      "secret_path": "/var/secrets/db-password",
      "reads_per_sec": 0.02,
      "last_read": "2026-02-14T11:59:30+00:00",
      "cached": true
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `pod` | Pod name (resolved from `/proc/{pid}/environ`) |
| `container` | Process name from the kernel (`comm`) |
| `secret_path` | File path that was accessed |
| `reads_per_sec` | Access frequency over the observation window |
| `cached` | `true` if `reads_per_sec < 1` (likely cached in memory) |

### `GET /healthz`

```json
{
  "status": "ok",
  "ebpf_attached": true
}
```

## Terminal UI (TUI)

A live dashboard for watching secret access in real time. Built with Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea).

![TUI Dashboard](/demo/tui-demo.gif)

```bash
# Build
make tui

# Run (with port-forward active)
./secrets-snitcher-tui --api http://localhost:9100

# Or try with the mock API for a quick demo
make mock-api            # Terminal 1
curl localhost:9100/toggle  # Terminal 2
./secrets-snitcher-tui      # Terminal 3
```

Features: anomaly detection banner, color-coded read rates, NEW pod badges, vim-style navigation, search, sortable columns, resizable layout.

See [cmd/tui/README.md](cmd/tui/README.md) for full keyboard shortcuts and options, and [cmd/tui/DEVGUIDE.md](cmd/tui/DEVGUIDE.md) for an architecture walkthrough aimed at C/C++ developers.

## Web Dashboard

An embedded web dashboard served directly from the snitcher pod - no extra dependencies, no CDN, no build step.

```bash
# With port-forward active, open in browser:
open http://localhost:9100

# Or try with the mock API:
python3 demo/mock-api.py    # Terminal 1
curl localhost:9100/toggle   # Terminal 2 (enable mock data)
open http://localhost:9100   # Browser
```

Features:
- Dark theme with color-coded anomaly/active/cached status
- Anomaly timeline chart (built from client-side history buffer)
- Per-pod horizontal bar chart with log-scale reads/sec
- Per-entry sparklines showing read rate trends
- Configurable client-side history buffer (5min - unlimited)
- Live updating every 2 seconds with connection status indicator
- Pulsing anomaly banner when suspicious access is detected

The dashboard stores history in browser memory while the tab is open. Use the buffer dropdown to control retention. Data resets when you close the tab.

To disable: set `SNITCHER_DASHBOARD_ENABLED=false`.

## Prometheus Metrics

A `/metrics` endpoint exposes secret access data in Prometheus text format for Grafana integration.

```bash
curl http://localhost:9100/metrics
```

```
# HELP snitcher_secret_reads_per_second Current read rate over the observation window.
# TYPE snitcher_secret_reads_per_second gauge
snitcher_secret_reads_per_second{pod="totally-legit-app",container="sh",secret_path="/var/run/secrets/kubernetes.io/serviceaccount/token"} 4872.3
```

Exposed metrics:

| Metric | Type | Labels |
|--------|------|--------|
| `snitcher_secret_reads_per_second` | gauge | pod, container, secret_path |
| `snitcher_secret_reads_total` | gauge | pod, container, secret_path |
| `snitcher_secret_cached` | gauge | pod, container, secret_path |
| `snitcher_secret_last_read_timestamp_seconds` | gauge | pod, container, secret_path |
| `snitcher_observation_window_seconds` | gauge | - |
| `snitcher_tracked_secrets` | gauge | - |
| `snitcher_ebpf_attached` | gauge | - |

Prometheus scrapes these gauges every 15-30 seconds and stores them in its own time-series database, giving you full historical data in Grafana even though secrets-snitcher only keeps a 60-second rolling window in memory.

To disable: set `SNITCHER_METRICS_ENABLED=false`.

## Configuration

Both the dashboard and metrics endpoint are enabled by default and can be independently toggled via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SNITCHER_DASHBOARD_ENABLED` | `true` | Serve web dashboard at `/` |
| `SNITCHER_METRICS_ENABLED` | `true` | Serve Prometheus metrics at `/metrics` |
| `SECRETS_SNITCHER_NO_TELEMETRY` | unset | Set to `1` to disable anonymous telemetry |

Set these in the pod spec or pass as environment variables when running standalone.

## Makefile targets

```bash
make deploy     # kubectl apply rbac + pod-inline, wait for ready
make undeploy   # remove everything
make demo       # deploy a suspicious test pod
make demo-clean # remove the test pod
make logs       # tail the snitcher logs
make test       # pytest tests/ -v
make tui        # build the terminal UI binary
make mock-api   # run the mock API for TUI development
```

## Running tests

```bash
pip install pytest
pytest tests/ -v
```

## Compatibility

Requires Linux nodes with kernel headers and BCC support:

| Platform | Status | Notes |
|---|---|---|
| **AKS** | Works | Ubuntu node images have kernel headers |
| **EKS** | Works | Amazon Linux 2 / Ubuntu AMIs |
| **GKE** | Partial | Ubuntu node images only. COS nodes lack kernel headers |
| **K3s** | [Tested](k3s/) | Ubuntu 24.04 + kernel 6.x verified. See [platform guide](k3s/) |
| **Bare metal / kubeadm** | Works | If kernel headers are installed |
| **kind / minikube** | Works | For local testing |

Requires privileged containers (`CAP_BPF` + `CAP_SYS_ADMIN` + `hostPID: true`).

## Architecture

```
k8s/pod-inline.yaml
├── ConfigMap (secrets-snitcher-code)
│   ├── api.py          # All-in-one: BPF loader + aggregator + HTTP server
│   ├── live.py         # Terminal monitor (port-forward + curses-style refresh)
│   └── dashboard.html  # Embedded web dashboard (single-file, no dependencies)
├── Pod (privileged, hostPID: true)
│   ├── mounts /proc as /host-proc (read-only, for pod name resolution)
│   ├── mounts /sys/kernel/debug (required by BCC)
│   ├── mounts /lib/modules (read-only, kernel module symbols)
│   ├── mounts /usr/src (read-only, kernel headers for BPF compilation)
│   └── installs python3 + BCC at startup from ubuntu:22.04
└── Service (ClusterIP :9100)
```

Everything ships as a single YAML file. No Docker build. The Python code lives in a ConfigMap, the pod installs BCC at startup from the ubuntu base image, and the BPF program compiles in-place on the node using the node's own kernel headers.

**Why BCC instead of libbpf/CO-RE:** BCC compiles the BPF C at runtime using Clang, which means it works on any kernel version without pre-compiled bytecode. The tradeoff is startup time (~30s for apt-get + compile) and requiring kernel headers on the node. A production hardened version would use libbpf with CO-RE for faster startup and smaller footprint.

## Platform guides

| Platform | Guide |
|---|---|
| K3s | [k3s/README.md](k3s/) - tested on Ubuntu 24.04 + kernel 6.x, includes verified output |

More platforms coming. If you've tested on a platform not listed here, open a PR with a guide under `<platform>/README.md`.

## Contributing

PRs welcome. Before submitting:

1. **Run tests:** `pytest tests/ -v` - all must pass
2. **Test on a real cluster** if your change touches `k8s/` manifests or the BPF program. The BPF C compiles at runtime on the node, so YAML-level correctness isn't enough.
3. **One concern per PR.** Don't bundle unrelated changes.
4. **Platform guides** go in `<platform>/README.md` with: prerequisites, deploy steps, verified output showing real data, and known issues.

## Telemetry

secrets-snitcher sends a single anonymous ping when the probe starts (at most once per 24 hours). It reports: tool version, kernel version, CPU architecture, Python version, and whether it's running as a DaemonSet or standalone.

No IP addresses, hostnames, secret paths, or cluster information is collected. To opt out: set `SECRETS_SNITCHER_NO_TELEMETRY=1`.

### Why we collect this

This is a solo open source project. Telemetry is the only way to know if anyone is actually using it, what kernels and platforms to support, and whether to keep investing time in it. Without it, the project is built blind.

### What we send

| Field | Example | Why |
|-------|---------|-----|
| tool | secrets-snitcher | Which tool sent the ping |
| version | 0.4.0 | Know which versions are in the wild |
| kernel | 6.17.0-1008-gcp | Know which kernel offsets to support |
| arch | x86_64 | Know if ARM support matters |
| python | 3.10.12 | Know minimum Python version to target |
| deployment_type | pod / daemonset / standalone | Know how people deploy |
| uptime_hours | 48 | Distinguish "tried once" from "running in prod" |
| distinct_id | a1b2c3... (SHA-256 hash) | Count unique installs without identifying anyone |

### How the install ID works

To count unique installations without collecting identifiable information, secrets-snitcher reads your cluster's `kube-system` namespace UID (a UUID that Kubernetes assigns when the cluster is created). This is why `rbac.yaml` includes a ClusterRole with read access to the `kube-system` namespace - it's only used to generate the telemetry hash.

The raw UID never leaves your cluster. It's hashed with SHA-256 before sending. The hash cannot be reversed. On standalone installs (no Kubernetes), `/etc/machine-id` is used instead.

## Limitations

This is a weekend project / proof of concept, not production-hardened. Known gaps:

- No persistence -- data is lost on pod restart
- No authentication on the HTTP API
- BCC requires kernel headers installed on every node
- Rolling window is in-memory only (no cross-node aggregation)
- Pod name resolution reads `/proc` which may not work in all container runtimes


## License

[MIT](LICENSE) - Copyright (c) 2026 Michael Ridner

# secrets-snitcher

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

eBPF-powered Kubernetes secret access monitor.

Watches which pods read secret files, how often, and whether they cache values in memory or re-read from disk on every request. Catches suspicious access patterns like a compromised pod hammering service account tokens.

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

1. **eBPF tracepoint** hooks `sys_enter_openat` — the syscall every file open goes through
2. **Kernel-side path filter** checks if the filename starts with a known secret mount path. Non-matching opens are dropped inside the kernel, never copied to userspace
3. **Perf buffer** streams matching events (pid, process name, filename, timestamp) to the Python aggregator
4. **Rolling window aggregator** tracks per-pod read frequency over 60 seconds, resolves pod names via `/proc/{pid}/environ`
5. **HTTP API** on port 9100 serves the current state as JSON

![Demo](snitcher-video.mov)  

### What it watches

| Mount path | Source |
|---|---|
| `/var/run/secrets/kubernetes.io/serviceaccount/` | Default K8s service account tokens |
| `/var/secrets/` | Custom secret volume mounts |
| `/mnt/secrets-store/` | CSI Secrets Store driver (Azure Key Vault, AWS Secrets Manager, HashiCorp Vault) |
| `/run/secrets/` | Docker secrets / alternative mounts |

### The `cached` field

A service with `reads_per_sec >= 1` is actively opening the secret file on every request — **not cached**. If you rotate or delete that secret, the service will immediately see the change (or break). A service with `reads_per_sec < 1` has likely read the secret once and cached the value in memory. Nothing in Kubernetes tells you which behavior you're dealing with. This tool does.

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

## Makefile targets

```bash
make deploy     # kubectl apply rbac + pod-inline, wait for ready
make undeploy   # remove everything
make demo       # deploy a suspicious test pod
make demo-clean # remove the test pod
make logs       # tail the snitcher logs
make test       # pytest tests/ -v
```

## Running tests

```bash
pip install pytest
pytest tests/ -v
```

## Compatibility

Requires Linux nodes with kernel headers and BCC support:

- **AKS** — works (Ubuntu node images have kernel headers)
- **EKS** — works (Amazon Linux 2 / Ubuntu AMIs)
- **GKE** — Ubuntu node images only. COS (Container-Optimized OS) nodes do not include kernel headers and are not supported by BCC
- **Bare metal, k3s, kubeadm** — works if kernel headers are installed
- **kind, minikube** — works for local testing

Requires privileged containers (`CAP_BPF` + `CAP_SYS_ADMIN` + `hostPID: true`).

## Architecture

```
k8s/pod-inline.yaml
├── ConfigMap (secrets-snitcher-code)
│   ├── api.py          # All-in-one: BPF loader + aggregator + HTTP server
│   └── live.py         # Terminal UI (port-forward + curses-style refresh)
├── Pod (privileged, hostPID: true)
│   ├── mounts /proc as /host-proc (read-only, for pod name resolution)
│   ├── mounts /sys/kernel/debug (required by BCC)
│   └── installs python3 + BCC at startup from ubuntu:22.04
└── Service (ClusterIP :9100)
```

Everything ships as a single YAML file. No Docker build. The Python code lives in a ConfigMap, the pod installs BCC at startup from the ubuntu base image, and the BPF program compiles in-place on the node using the node's own kernel headers.

**Why BCC instead of libbpf/CO-RE:** BCC compiles the BPF C at runtime using Clang, which means it works on any kernel version without pre-compiled bytecode. The tradeoff is startup time (~30s for apt-get + compile) and requiring kernel headers on the node. A production hardened version would use libbpf with CO-RE for faster startup and smaller footprint.

## Limitations

This is a weekend project / proof of concept, not production-hardened. Known gaps:

- No persistence -- data is lost on pod restart
- No authentication on the HTTP API
- BCC requires kernel headers installed on every node
- Rolling window is in-memory only (no cross-node aggregation)
- Pod name resolution reads `/proc` which may not work in all container runtimes

## License

[MIT](LICENSE)

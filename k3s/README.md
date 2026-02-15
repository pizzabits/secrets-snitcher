# K3s Platform Guide

secrets-snitcher has been tested on K3s single-node clusters running Ubuntu 24.04 LTS with kernel 6.x.

## Prerequisites

K3s nodes need kernel headers and BCC installed:

```bash
apt-get install -y linux-headers-$(uname -r) bpfcc-tools python3-bpfcc
```

The pod-inline.yaml handles this automatically at startup, but having headers pre-installed speeds up boot time.

## Deploy

```bash
kubectl apply -f k8s/rbac.yaml
kubectl apply -f k8s/pod-inline.yaml
kubectl -n secrets-snitcher wait --for=condition=Ready pod/secrets-snitcher --timeout=120s
```

## K3s-specific volume mounts

K3s nodes store kernel headers in `/usr/src` and modules in `/lib/modules`. The pod mounts both read-only so BCC can compile against the running kernel:

```yaml
volumeMounts:
  - name: modules
    mountPath: /lib/modules
    readOnly: true
  - name: usr-src
    mountPath: /usr/src
    readOnly: true
```

Without these mounts, BCC fails to compile the BPF program because it can't find the kernel headers.

## What runs inside K3s

```
K3s cluster (single node)
│
├── kube-system namespace
│   ├── coredns
│   ├── metrics-server ← secrets-snitcher sees this reading SA tokens
│   └── local-path-provisioner
│
├── secrets-snitcher namespace
│   ├── Pod: secrets-snitcher (privileged, hostPID)
│   │   ├── mounts: /host-proc, /sys/kernel/debug, /lib/modules, /usr/src
│   │   ├── installs BCC at startup from ubuntu:22.04
│   │   ├── compiles BPF C against host kernel headers
│   │   └── hooks tracepoint: syscalls:sys_enter_openat
│   └── Service: ClusterIP :9100
│
└── demo namespace (optional)
    ├── Deployment: secret-reader-cached  ← reads token once, caches
    └── Deployment: secret-reader-live    ← re-reads token every request
```

## Verified output

After deploying on a fresh K3s cluster, `curl localhost:9100/api/v1/secret-access | jq` shows K3s system components reading service account tokens:

```json
{
  "timestamp": "2026-02-15T...",
  "observation_window_seconds": 10,
  "entries": [
    {
      "pod": "metrics-server-...",
      "container": "metrics-server",
      "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
      "reads_per_sec": 0.2,
      "cached": true
    },
    {
      "pod": "coredns-...",
      "container": "coredns",
      "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
      "reads_per_sec": 0.1,
      "cached": true
    }
  ]
}
```

K3s system pods (metrics-server, coredns) read their service account tokens infrequently and cache them — exactly what you'd expect from well-behaved components.

## Troubleshooting

**BPF compilation fails:** Check that `linux-headers-$(uname -r)` is installed on the node. K3s doesn't ship kernel headers by default.

**Pod stuck in init:** The `apt-get install` step can take 30-60 seconds depending on network speed. The `--timeout=120s` in the deploy command accounts for this.

**No entries in the API:** Ensure `hostPID: true` is set on the pod spec. Without it, the BPF program can't see processes outside its own namespace.

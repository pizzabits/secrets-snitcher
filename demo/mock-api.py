"""Mock secrets-snitcher API for TUI development and demo recording.

Serves realistic responses matching what the real eBPF probe returns,
including mixed cached/active entries, varied read rates, and realistic
Kubernetes pod/container names.

Usage:
    python3 demo/mock-api.py
    # In another terminal: ./secrets-snitcher-tui --api http://localhost:9100
"""

import json
import random
import signal
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

# Simulates a real cluster with mixed workloads
REALISTIC_ENTRIES = [
    {
        "pod": "payment-service-7f8b9c6d4-xk2mn",
        "container": "payment-svc",
        "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
        "reads_per_sec": 4872.3,
        "cached": False,
    },
    {
        "pod": "payment-service-7f8b9c6d4-xk2mn",
        "container": "payment-svc",
        "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
        "reads_per_sec": 0.12,
        "cached": True,
    },
    {
        "pod": "frontend-deployment-with-very-long-name-abc123-9z8y7",
        "container": "nginx-ingress",
        "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/namespace",
        "reads_per_sec": 0.03,
        "cached": True,
    },
    {
        "pod": "vault-agent-injector-6b4f5c8d9-2plqr",
        "container": "vault-agent",
        "secret_path": "/var/secrets/db-credentials",
        "reads_per_sec": 2.45,
        "cached": False,
    },
    {
        "pod": "pid-31337",
        "container": "cryptominer",
        "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
        "reads_per_sec": 9999.9,
        "cached": False,
    },
    {
        "pod": "monitoring-prometheus-stack-kube-state-metrics-5f7d8c",
        "container": "kube-state-met",
        "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
        "reads_per_sec": 0.5,
        "cached": True,
    },
    {
        "pod": "argocd-application-controller-0",
        "container": "controller",
        "secret_path": "/mnt/secrets-store/github-token",
        "reads_per_sec": 1.2,
        "cached": False,
    },
    {
        "pod": "cert-manager-webhook-7b9c4d-lm8x2",
        "container": "webhook",
        "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
        "reads_per_sec": 0.01,
        "cached": True,
    },
]

_malicious_deployed = False


def _now():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def _jitter(val):
    """Add small random jitter to reads_per_sec for realism."""
    if val > 10:
        return round(val + random.uniform(-50, 50), 1)
    elif val > 1:
        return round(val + random.uniform(-0.3, 0.3), 2)
    else:
        return round(max(0, val + random.uniform(-0.01, 0.01)), 2)


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        global _malicious_deployed
        if self.path == "/api/v1/secret-access":
            now = _now()
            if _malicious_deployed:
                entries = []
                for e in REALISTIC_ENTRIES:
                    entry = e.copy()
                    entry["reads_per_sec"] = _jitter(e["reads_per_sec"])
                    entry["last_read"] = now
                    entries.append(entry)
                resp = {
                    "timestamp": now,
                    "observation_window_seconds": 60,
                    "entries": entries,
                }
            else:
                resp = {
                    "timestamp": now,
                    "observation_window_seconds": 60,
                    "entries": [],
                }
            self._json(resp)
        elif self.path == "/healthz":
            self._json({"status": "ok", "ebpf_attached": True})
        elif self.path == "/toggle":
            _malicious_deployed = not _malicious_deployed
            state = "ON - returning realistic cluster data" if _malicious_deployed else "OFF - returning empty"
            self._json({"malicious_deployed": _malicious_deployed, "state": state})
        else:
            self.send_error(404)

    def _json(self, data):
        body = json.dumps(data, indent=2).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", len(body))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        pass  # silent


def _signal_handler(sig, frame):
    print("\nMock API stopped.")
    sys.exit(0)


if __name__ == "__main__":
    signal.signal(signal.SIGINT, _signal_handler)
    signal.signal(signal.SIGTERM, _signal_handler)

    print("secrets-snitcher mock API on :9100")
    print("  GET /api/v1/secret-access  - returns empty until toggled")
    print("  GET /healthz               - health check")
    print("  GET /toggle                - switch between empty/realistic responses")
    print()
    print("Demo flow:")
    print("  1. ./secrets-snitcher-tui                       (empty, watching)")
    print("  2. curl localhost:9100/toggle                    (simulate cluster)")
    print("  3. TUI shows mixed cached/active/anomaly entries")
    print()
    print("Ctrl+C to stop.")

    HTTPServer(("", 9100), Handler).serve_forever()
"""Flask HTTP API for the secrets-snitcher eBPF agent."""

import os
import sys
from datetime import datetime, timezone

from flask import Flask, jsonify

from agent.aggregator import SecretAccessAggregator

app = Flask(__name__)
aggregator = SecretAccessAggregator()
probe = None


@app.route("/api/v1/secret-access")
def secret_access():
    return jsonify({
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "observation_window_seconds": aggregator.WINDOW_SECONDS,
        "entries": aggregator.get_summary(),
    })


@app.route("/healthz")
def healthz():
    return jsonify({
        "status": "ok",
        "ebpf_attached": probe.attached if probe else False,
    })


def main():
    global probe

    # Only attach eBPF on Linux with BCC available
    try:
        from agent.probe import SecretAccessProbe

        print("", file=sys.stderr)
        print("  secrets-snitcher v0.1.0", file=sys.stderr)
        print("  eBPF-powered Kubernetes secret access monitor", file=sys.stderr)
        print("", file=sys.stderr)
        print("[ebpf] Compiling BPF program...", file=sys.stderr)
        probe = SecretAccessProbe(aggregator)
        probe.attach()
        probe.start_background()
        print("[ebpf] BPF program loaded into kernel", file=sys.stderr)
        print("[ebpf] Attached tracepoint: syscalls:sys_enter_openat", file=sys.stderr)
        print("[ebpf] Attached tracepoint: syscalls:sys_exit_read", file=sys.stderr)
        print("[ebpf] Perf buffer open — streaming events", file=sys.stderr)
        print("", file=sys.stderr)
        print("[watch] /var/run/secrets/**", file=sys.stderr)
        print("[watch] /var/secrets/**", file=sys.stderr)
        print("[watch] /mnt/secrets-store/**", file=sys.stderr)
        print("[watch] /run/secrets/**", file=sys.stderr)
        print("", file=sys.stderr)
        print("[api] GET /api/v1/secret-access", file=sys.stderr)
        print("[api] GET /healthz", file=sys.stderr)
        print("", file=sys.stderr)
        print("[ready] Watching for secret access...", file=sys.stderr)
    except Exception as e:
        print(f"[secrets-snitcher] eBPF probe not available (running without tracing): {e}", file=sys.stderr)

    port = int(os.environ.get("PORT", "9100"))
    print(f"[api] Listening on :{port}", file=sys.stderr)
    app.run(host="0.0.0.0", port=port)


if __name__ == "__main__":
    main()

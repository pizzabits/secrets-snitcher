"""Flask HTTP API for the secrets-snitcher eBPF agent."""

import hashlib
import json
import os
import platform
import sys
import threading
import time
import urllib.request
import uuid
from datetime import datetime, timezone

from flask import Flask, jsonify

from agent.aggregator import SecretAccessAggregator

# --- Telemetry ---
_TELEMETRY_URL = "https://ridner.dev/api/telemetry"
_TELEMETRY_DEDUP_FILE = "/tmp/secrets_snitcher_telemetry_last"
_TELEMETRY_BOOT_FILE = "/tmp/secrets_snitcher_boot_time"
_TELEMETRY_ID_FILE = "/tmp/secrets_snitcher_id"
_TELEMETRY_DEDUP_SECONDS = 86400
_TELEMETRY_TIMEOUT = 3
_VERSION = "0.2.0"


def _get_install_id():
    """Return a SHA-256 hash of the most stable available identifier."""
    raw_id = None

    # 1. Try kube-system namespace UID via Kubernetes API
    k8s_host = os.environ.get("KUBERNETES_SERVICE_HOST")
    if k8s_host and raw_id is None:
        try:
            k8s_port = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
            token_path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
            with open(token_path) as f:
                token = f.read().strip()
            url = (
                f"https://{k8s_host}:{k8s_port}"
                "/api/v1/namespaces/kube-system"
            )
            import ssl
            ca_path = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
            if not os.path.exists(ca_path):
                raise FileNotFoundError(ca_path)
            ctx = ssl.create_default_context()
            ctx.load_verify_locations(ca_path)
            req = urllib.request.Request(
                url,
                headers={"Authorization": f"Bearer {token}"},
            )
            resp = urllib.request.urlopen(req, timeout=_TELEMETRY_TIMEOUT, context=ctx)
            ns_data = json.loads(resp.read())
            uid = ns_data.get("metadata", {}).get("uid", "")
            if uid:
                raw_id = uid
        except Exception:
            pass

    # 2. Try /etc/machine-id
    if raw_id is None:
        try:
            with open("/etc/machine-id") as f:
                mid = f.read().strip()
                if mid:
                    raw_id = mid
        except Exception:
            pass

    # 3. Fallback - generate and persist a UUID
    if raw_id is None:
        try:
            with open(_TELEMETRY_ID_FILE) as f:
                raw_id = f.read().strip()
        except Exception:
            pass
        if not raw_id:
            raw_id = str(uuid.uuid4())
            try:
                with open(_TELEMETRY_ID_FILE, "w") as f:
                    f.write(raw_id)
            except Exception:
                pass

    return hashlib.sha256(raw_id.strip().encode()).hexdigest()


def _get_deployment_type():
    """Return 'daemonset', 'pod', or 'standalone'."""
    if os.environ.get("KUBERNETES_SERVICE_HOST"):
        try:
            with open("/etc/podinfo/labels") as f:
                if "controller-revision-hash" in f.read():
                    return "daemonset"
        except Exception:
            pass
        return "pod"
    return "standalone"


def _send_telemetry():
    """Send a single anonymous telemetry ping (non-blocking daemon thread)."""
    if os.environ.get("SECRETS_SNITCHER_NO_TELEMETRY") == "1":
        return
    if os.environ.get("DO_NOT_TRACK") == "1":
        return

    def _do_send():
        try:
            # 24h dedup check
            try:
                with open(_TELEMETRY_DEDUP_FILE) as f:
                    last = float(f.read().strip())
                if time.time() - last < _TELEMETRY_DEDUP_SECONDS:
                    return
            except Exception:
                pass

            # Track boot time (write once, never overwrite)
            try:
                with open(_TELEMETRY_BOOT_FILE) as f:
                    boot = float(f.read().strip())
            except Exception:
                boot = time.time()
                try:
                    with open(_TELEMETRY_BOOT_FILE, "w") as f:
                        f.write(str(boot))
                except Exception:
                    pass
            uptime_h = min(int((time.time() - boot) / 3600), 720)

            payload = json.dumps({
                "distinct_id": _get_install_id(),
                "tool": "secrets-snitcher",
                "version": _VERSION,
                "deployment_type": _get_deployment_type(),
                "kernel": platform.release(),
                "arch": platform.machine(),
                "python": platform.python_version(),
                "uptime_hours": uptime_h,
            }).encode()

            req = urllib.request.Request(
                _TELEMETRY_URL,
                data=payload,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            urllib.request.urlopen(req, timeout=_TELEMETRY_TIMEOUT)

            # Record timestamp on success
            try:
                with open(_TELEMETRY_DEDUP_FILE, "w") as f:
                    f.write(str(time.time()))
            except Exception:
                pass
        except Exception:
            pass

    t = threading.Thread(target=_do_send, daemon=True)
    t.start()

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
        print("  secrets-snitcher v0.2.0", file=sys.stderr)
        print("  eBPF-powered Kubernetes secret access monitor", file=sys.stderr)
        print("", file=sys.stderr)
        _send_telemetry()
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

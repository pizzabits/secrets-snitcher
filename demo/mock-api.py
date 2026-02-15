"""Mock secrets-snitcher API for demo recording.

Run this locally, then curl it while recording your terminal.
It serves realistic responses that match what the real probe returns.

Usage:
    python3 demo/mock-api.py
    # In another terminal: curl localhost:9100/api/v1/secret-access | jq
"""

import json
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

EMPTY_RESPONSE = {
    "timestamp": "",
    "observation_window_seconds": 60,
    "entries": [],
}

CAUGHT_RESPONSE = {
    "timestamp": "",
    "observation_window_seconds": 60,
    "entries": [
        {
            "pod": "totally-legit-app",
            "container": "definitely-not-mining",
            "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
            "reads_per_sec": 4872.3,
            "last_read": "",
            "cached": False,
        },
        {
            "pod": "totally-legit-app",
            "container": "definitely-not-mining",
            "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
            "reads_per_sec": 4871.1,
            "last_read": "",
            "cached": False,
        },
        {
            "pod": "totally-legit-app",
            "container": "definitely-not-mining",
            "secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/namespace",
            "reads_per_sec": 4870.8,
            "last_read": "",
            "cached": False,
        },
    ],
}

# After malicious pod is "deployed", switch to caught response
_malicious_deployed = False


def _now():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        global _malicious_deployed
        if self.path == "/api/v1/secret-access":
            if _malicious_deployed:
                resp = CAUGHT_RESPONSE.copy()
                resp["timestamp"] = _now()
                for e in resp["entries"]:
                    e["last_read"] = _now()
            else:
                resp = EMPTY_RESPONSE.copy()
                resp["timestamp"] = _now()
            self._json(resp)
        elif self.path == "/healthz":
            self._json({"status": "ok", "ebpf_attached": True})
        elif self.path == "/toggle":
            _malicious_deployed = not _malicious_deployed
            state = "ON — returning caught data" if _malicious_deployed else "OFF — returning empty"
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


if __name__ == "__main__":
    print("secrets-snitcher mock API on :9100")
    print("  GET /api/v1/secret-access  — returns empty until toggled")
    print("  GET /healthz               — health check")
    print("  GET /toggle                — switch between empty/caught responses")
    print()
    print("Demo flow:")
    print("  1. curl localhost:9100/api/v1/secret-access   (empty)")
    print("  2. curl localhost:9100/toggle                  (simulate malicious pod)")
    print("  3. curl localhost:9100/api/v1/secret-access   (caught!)")
    HTTPServer(("", 9100), Handler).serve_forever()

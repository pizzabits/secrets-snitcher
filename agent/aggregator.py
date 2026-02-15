"""In-memory aggregation of secret file access events."""

import os
import threading
import time
from collections import defaultdict
from datetime import datetime, timezone
from typing import List


class SecretAccessAggregator:
    """Maintains rolling window of secret file access events."""

    WINDOW_SECONDS = 60
    CLEANUP_INTERVAL = 30

    def __init__(self):
        self._lock = threading.Lock()
        # key: (pid, filepath) -> list of timestamp_ns
        self._events = defaultdict(list)
        # key: pid -> comm (process name)
        self._comms = {}
        self._start_cleanup_thread()

    def record_access(self, pid: int, comm: str, filepath: str, timestamp: int):
        with self._lock:
            key = (pid, filepath)
            self._events[key].append(timestamp)
            self._comms[pid] = comm

    def get_summary(self) -> List[dict]:
        now_ns = time.time_ns()
        cutoff_ns = now_ns - (self.WINDOW_SECONDS * 1_000_000_000)
        results = []

        with self._lock:
            for (pid, filepath), accesses in list(self._events.items()):
                # Filter to rolling window
                recent = [ts for ts in accesses if ts >= cutoff_ns]
                if not recent:
                    continue

                comm = self._comms.get(pid, "unknown")
                reads_per_sec = len(recent) / self.WINDOW_SECONDS
                last_ts = max(recent)
                last_read_dt = datetime.fromtimestamp(
                    last_ts / 1_000_000_000, tz=timezone.utc
                )

                results.append({
                    "pod": self._resolve_pod_name(pid),
                    "container": comm,
                    "secret_path": filepath,
                    "reads_per_sec": round(reads_per_sec, 2),
                    "last_read": last_read_dt.isoformat(),
                    "cached": reads_per_sec < 1,
                })

        return results

    def _resolve_pod_name(self, pid: int) -> str:
        """Read /proc/{pid}/environ for HOSTNAME (K8s sets this to pod name)."""
        try:
            environ_path = f"/proc/{pid}/environ"
            with open(environ_path, "rb") as f:
                data = f.read()
            for entry in data.split(b"\x00"):
                if entry.startswith(b"HOSTNAME="):
                    return entry.split(b"=", 1)[1].decode("utf-8", errors="replace")
        except (FileNotFoundError, PermissionError):
            pass
        return f"pid-{pid}"

    def _cleanup_dead_pids(self):
        with self._lock:
            dead_keys = []
            for key in self._events:
                try:
                    os.kill(key[0], 0)
                except ProcessLookupError:
                    dead_keys.append(key)
                except PermissionError:
                    pass  # PID exists but we lack permission -- keep it
            for key in dead_keys:
                del self._events[key]
                self._comms.pop(key[0], None)

    def _start_cleanup_thread(self):
        def loop():
            while True:
                time.sleep(self.CLEANUP_INTERVAL)
                self._cleanup_dead_pids()

        t = threading.Thread(target=loop, daemon=True)
        t.start()

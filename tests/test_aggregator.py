"""Unit tests for SecretAccessAggregator."""

import os
import time
from unittest.mock import patch

import pytest
from agent.aggregator import SecretAccessAggregator


@pytest.fixture
def agg():
    return SecretAccessAggregator()


def _now_ns():
    return time.time_ns()


class TestRecordAndSummary:
    def test_empty_summary(self, agg):
        assert agg.get_summary() == []

    def test_single_access(self, agg):
        ts = _now_ns()
        agg.record_access(pid=100, comm="auth-svc", filepath="/var/secrets/db-pass", timestamp=ts)

        summary = agg.get_summary()
        assert len(summary) == 1
        entry = summary[0]
        assert entry["container"] == "auth-svc"
        assert entry["secret_path"] == "/var/secrets/db-pass"

    def test_multiple_accesses_same_file(self, agg):
        ts = _now_ns()
        for i in range(10):
            agg.record_access(pid=100, comm="auth-svc", filepath="/var/secrets/db-pass", timestamp=ts + i)

        summary = agg.get_summary()
        assert len(summary) == 1
        assert summary[0]["reads_per_sec"] == round(10 / 60, 2)

    def test_different_files_different_entries(self, agg):
        ts = _now_ns()
        agg.record_access(pid=100, comm="auth-svc", filepath="/var/secrets/db-pass", timestamp=ts)
        agg.record_access(pid=100, comm="auth-svc", filepath="/var/secrets/api-key", timestamp=ts)

        summary = agg.get_summary()
        assert len(summary) == 2
        paths = {e["secret_path"] for e in summary}
        assert paths == {"/var/secrets/db-pass", "/var/secrets/api-key"}


class TestCachedFlag:
    def test_low_frequency_is_cached(self, agg):
        ts = _now_ns()
        agg.record_access(pid=200, comm="web-app", filepath="/mnt/secrets-store/cert", timestamp=ts)

        summary = agg.get_summary()
        assert summary[0]["cached"] is True

    def test_high_frequency_is_not_cached(self, agg):
        ts = _now_ns()
        for i in range(120):
            agg.record_access(pid=200, comm="web-app", filepath="/mnt/secrets-store/cert", timestamp=ts + i)

        summary = agg.get_summary()
        assert summary[0]["cached"] is False
        assert summary[0]["reads_per_sec"] == 2.0


class TestPodNameResolution:
    def test_resolves_from_proc_environ(self, agg):
        mock_environ = b"PATH=/usr/bin\x00HOSTNAME=auth-service-7x8d\x00HOME=/root\x00"
        with patch("builtins.open", create=True) as mock_open:
            mock_open.return_value.__enter__ = lambda s: s
            mock_open.return_value.__exit__ = lambda s, *a: None
            mock_open.return_value.read = lambda: mock_environ

            result = agg._resolve_pod_name(12345)
            assert result == "auth-service-7x8d"

    def test_fallback_on_missing_proc(self, agg):
        result = agg._resolve_pod_name(999999999)
        assert result == "pid-999999999"


class TestDeadPidCleanup:
    def test_removes_dead_pids(self, agg):
        ts = _now_ns()
        agg.record_access(pid=999999999, comm="gone", filepath="/var/secrets/x", timestamp=ts)
        assert len(agg._events) == 1

        agg._cleanup_dead_pids()
        assert len(agg._events) == 0

    def test_keeps_live_pids(self, agg):
        ts = _now_ns()
        my_pid = os.getpid()
        agg.record_access(pid=my_pid, comm="python", filepath="/var/secrets/x", timestamp=ts)

        agg._cleanup_dead_pids()
        assert len(agg._events) == 1


class TestRollingWindow:
    def test_old_events_excluded(self, agg):
        now = _now_ns()
        old = now - (90 * 1_000_000_000)  # 90 seconds ago

        agg.record_access(pid=100, comm="svc", filepath="/var/secrets/old", timestamp=old)
        agg.record_access(pid=100, comm="svc", filepath="/var/secrets/new", timestamp=now)

        summary = agg.get_summary()
        paths = {e["secret_path"] for e in summary}
        assert "/var/secrets/new" in paths

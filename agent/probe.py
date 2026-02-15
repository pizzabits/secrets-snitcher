"""BPF program and BCC loader for tracing secret file access."""

import ctypes
import threading

BPF_PROGRAM = r"""
#include <uapi/linux/ptrace.h>
#include <linux/sched.h>

struct event_t {
    u32 pid;
    char comm[16];
    char filename[256];
    u64 timestamp;
};

BPF_PERF_OUTPUT(events);

static inline bool is_secret_path(const char *path) {
    char buf[20];
    bpf_probe_read_user_str(buf, sizeof(buf), path);

    // /var/run/secrets/ (K8s service account tokens)
    if (buf[0]=='/' && buf[1]=='v' && buf[2]=='a' && buf[3]=='r' &&
        buf[4]=='/' && buf[5]=='r' && buf[6]=='u' && buf[7]=='n' &&
        buf[8]=='/' && buf[9]=='s' && buf[10]=='e' && buf[11]=='c' &&
        buf[12]=='r' && buf[13]=='e' && buf[14]=='t' && buf[15]=='s' &&
        buf[16]=='/')
        return true;

    // /var/secrets/
    if (buf[0]=='/' && buf[1]=='v' && buf[2]=='a' && buf[3]=='r' &&
        buf[4]=='/' && buf[5]=='s' && buf[6]=='e' && buf[7]=='c' &&
        buf[8]=='r' && buf[9]=='e' && buf[10]=='t' && buf[11]=='s' &&
        buf[12]=='/')
        return true;

    // /mnt/secrets-store/
    if (buf[0]=='/' && buf[1]=='m' && buf[2]=='n' && buf[3]=='t' &&
        buf[4]=='/' && buf[5]=='s' && buf[6]=='e' && buf[7]=='c')
        return true;

    // /run/secrets/
    if (buf[0]=='/' && buf[1]=='r' && buf[2]=='u' && buf[3]=='n' &&
        buf[4]=='/' && buf[5]=='s' && buf[6]=='e' && buf[7]=='c' &&
        buf[8]=='r' && buf[9]=='e' && buf[10]=='t' && buf[11]=='s' &&
        buf[12]=='/')
        return true;

    return false;
}

TRACEPOINT_PROBE(syscalls, sys_enter_openat) {
    const char *fname = args->filename;
    if (!is_secret_path(fname))
        return 0;

    struct event_t evt = {};
    evt.pid = bpf_get_current_pid_tgid() >> 32;
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));
    bpf_probe_read_user_str(evt.filename, sizeof(evt.filename), fname);
    evt.timestamp = bpf_ktime_get_ns();
    events.perf_submit(args, &evt, sizeof(evt));
    return 0;
}

"""


class Event(ctypes.Structure):
    _fields_ = [
        ("pid", ctypes.c_uint32),
        ("comm", ctypes.c_char * 16),
        ("filename", ctypes.c_char * 256),
        ("timestamp", ctypes.c_uint64),
    ]


class SecretAccessProbe:
    """Loads BPF program and streams secret-access events to an aggregator."""

    def __init__(self, aggregator):
        self.aggregator = aggregator
        self._bpf = None
        self._thread = None
        self._running = False

    def attach(self):
        from bcc import BPF

        self._bpf = BPF(text=BPF_PROGRAM)
        self._bpf["events"].open_perf_buffer(self._handle_event)
        self._running = True

    def _handle_event(self, cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(Event)).contents
        self.aggregator.record_access(
            pid=event.pid,
            comm=event.comm.decode("utf-8", errors="replace").rstrip("\x00"),
            filepath=event.filename.decode("utf-8", errors="replace").rstrip("\x00"),
            timestamp=event.timestamp,
        )

    def poll_loop(self):
        while self._running:
            self._bpf.perf_buffer_poll(timeout=100)

    def start_background(self):
        self._thread = threading.Thread(target=self.poll_loop, daemon=True)
        self._thread.start()

    def stop(self):
        self._running = False
        if self._thread:
            self._thread.join(timeout=2)

    @property
    def attached(self):
        return self._bpf is not None and self._running

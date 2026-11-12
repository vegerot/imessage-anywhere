"""Record one real `imsg rpc` session as JSONL (both directions, timestamped).

Usage: python3 record-rpc-transcript.py REQUESTS.jsonl RAW_OUT.jsonl

REQUESTS.jsonl holds one JSON-RPC request per line ({"id", "method", "params"}). Each is sent
after the previous one's response arrives; notifications are recorded as they come. The
child runs with SSH_CONNECTION set, like the Proxy runs it, so contact names are included.
The raw output holds personal data: keep it out of the repo and run
sanitize-rpc-transcript.py on it.
"""
import json
import os
import subprocess
import sys
import threading
import time

requests_path, out_path = sys.argv[1], sys.argv[2]
start = time.monotonic()
log = open(out_path, "w")
lock = threading.Lock()
responses = {}


def record(direction, line):
    with lock:
        log.write(json.dumps({"t": round(time.monotonic() - start, 3), "dir": direction, "line": line}) + "\n")
        log.flush()


env = dict(os.environ, SSH_CONNECTION="local 0 local 0")
proc = subprocess.Popen(["imsg", "rpc"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1, env=env)


def reader():
    for line in proc.stdout:
        line = line.rstrip("\n")
        record("out", line)
        msg = json.loads(line)
        if "id" in msg:
            responses[msg["id"]] = msg


threading.Thread(target=reader, daemon=True).start()
for raw in open(requests_path):
    if not raw.strip():
        continue
    request = dict(json.loads(raw), jsonrpc="2.0")
    line = json.dumps(request, ensure_ascii=False)
    record("in", line)
    proc.stdin.write(line + "\n")
    proc.stdin.flush()
    deadline = time.monotonic() + 30
    while request["id"] not in responses and time.monotonic() < deadline:
        time.sleep(0.05)
time.sleep(3)  # let late notifications arrive
proc.stdin.close()
proc.wait(timeout=20)
record("exit", str(proc.returncode))

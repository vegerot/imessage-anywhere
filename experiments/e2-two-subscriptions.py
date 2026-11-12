"""E2: do two watch.subscribe calls on ONE imsg rpc child each get their own stream?

Both subscribe to chat 242 from a cursor a few rows back, so imsg replays existing rows;
nothing is sent. Prints only subscription IDs and row IDs, no content.
"""
import json
import subprocess
import threading
import time

proc = subprocess.Popen(["imsg", "rpc"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)
seen = []


def reader():
    for line in proc.stdout:
        msg = json.loads(line)
        if msg.get("method") == "message":
            seen.append((msg["params"]["subscription"], msg["params"]["message"]["id"]))
        elif "id" in msg:
            print("response", msg["id"], msg.get("result") or msg.get("error"))


threading.Thread(target=reader, daemon=True).start()
for n, since in ((1, 183572), (2, 183574)):
    proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": f"sub{n}", "method": "watch.subscribe",
                                 "params": {"chat_id": 242, "since_rowid": since}}) + "\n")
    proc.stdin.flush()
time.sleep(3)
proc.stdin.close()
proc.wait(timeout=20)
for sub in sorted({s for s, _ in seen}):
    print("subscription", sub, "rows", [r for s, r in seen if s == sub])

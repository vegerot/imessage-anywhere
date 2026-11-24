#!/usr/bin/env python3
r"""Record a real `imsg rpc` session and strip the personal data from it, for use as a test fixture.

Usage: python3 rpc-transcript.py record-rpc-transcript REQUESTS.jsonl RAW_OUT.jsonl
       python3 rpc-transcript.py sanitize-rpc-transcript RAW.jsonl > SANITIZED.jsonl

Subcommands:

  record-rpc-transcript    Record one real `imsg rpc` session as JSONL (both directions, timestamped).
  sanitize-rpc-transcript  Replace personal data in a recorded transcript with consistent fakes.
"""
import json
import os
import re
import subprocess
import sys
import threading
import time
import uuid


def record_rpc_transcript():
    """Record one real `imsg rpc` session as JSONL (both directions, timestamped).

    Arguments: REQUESTS.jsonl RAW_OUT.jsonl

    REQUESTS.jsonl holds one JSON-RPC request per line ({"id", "method", "params"}). Each is sent
    after the previous one's response arrives; notifications are recorded as they come. The
    child runs with SSH_CONNECTION set, like the Proxy runs it, so contact names are included.
    The raw output holds personal data: keep it out of the repo and run
    rpc-transcript.py sanitize-rpc-transcript on it.
    """
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


# What sanitize-rpc-transcript keeps, and how it fakes the rest.

KEEP = {
    "jsonrpc", "id", "method", "error", "message", "data", "supported_methods", "methods",
    "version", "service", "transport", "send_state", "uti", "mime_type", "kind", "type",
    "last_message_at", "created_at", "checked_at", "date_read", "date_delivered", "delivered_at",
    "edited_at", "unsent_at", "emoji", "reaction_emoji", "reaction_type", "balloon_bundle_id", "bundle_id", "event", "event_type",
}
HANDLE_KEYS = {
    "identifier", "participants", "last_addressed_handle", "sender", "destination_caller_id",
    "chat_identifier", "name", "display_name", "chat_name", "account_login", "to",
    "contact_name", "sender_name", "participant",
}
GUID_KEYS = {
    "guid", "chat_guid", "message_id", "account_id", "reply_to_guid", "thread_originator_guid",
    "poll_guid", "original_guid", "reacted_to_guid",
}
UUID_SHAPE = re.compile(r"[0-9A-Fa-f]{8}(-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}")
replaced = set()  # every real string we rewrote, for the leak check at the end
fakes = {}


def fake(kind, real, make):
    """The fake for `real`, made by `make(n)` for the n-th distinct value of its kind."""
    key = (kind, real)
    if key not in fakes:
        fakes[key] = make(sum(1 for k in fakes if k[0] == kind))
    return fakes[key]


def fake_handle(value):
    if value == "":
        return value
    if value.startswith("E:"):  # account_login is "E:<email>" or bare "E:"
        return "E:" + fake_handle(value[2:]) if value[2:] else value
    if "@" in value:
        return fake("email", value.lower(), lambda n: f"person{n}@example.com")
    digits = re.sub(r"\D", "", value)
    if digits and len(digits) <= 6:  # SMS short code
        return fake("short", digits, lambda n: str(50000 + n))
    if digits:
        normalized = fake("phone", digits[-10:], lambda n: f"+1555010{n:04d}")
        return normalized if value.startswith("+") else normalized[1:]
    return fake("name", value, lambda n: f"Synthetic Name {n}")


def fake_guid(value):
    match = re.fullmatch(r"(any|iMessage|SMS|RCS);([-+]);(.+)", value)
    if match:
        service, kind, rest = match.groups()
        tail = fake_handle(rest) if kind == "-" else fake("chat", rest, lambda n: f"chat{n:06d}")
        return f"{service};{kind};{tail}"
    return fake("uuid", value, lambda n: str(uuid.UUID(int=n + 1)).upper())


def fake_path(value):
    if value.endswith("chat.db"):
        return "/Users/me/Library/Messages/chat.db"
    last = value.rsplit("/", 1)[-1]
    ext = "." + last.rsplit(".", 1)[-1] if "." in last else ""  # on-disk paths are often extensionless
    # Keep the path's form: imsg's original_path is absolute, the database's filename is "~/…".
    # Both forms of one file get the same fake, under a synthetic home /Users/me.
    relative = re.sub(r"^(~|/Users/[^/]+)/", "", value)
    home = "~" if value.startswith("~") else "/Users/me"
    return home + "/" + fake("path", relative, lambda n: f"Library/Messages/Attachments/00/00/{n:04d}/attachment-{n}{ext}")


OBJECT_REPLACEMENT = "\ufffc"  # where Messages puts an attachment inside a Message's text


def fake_text(value):
    if value.startswith("🧪"):
        return value
    # Keep attachment markers: they're structure, not content, and the page relies on them.
    markers = OBJECT_REPLACEMENT * value.count(OBJECT_REPLACEMENT)
    if not value.replace(OBJECT_REPLACEMENT, "").strip():
        return value
    return fake("text", value, lambda n: f"Synthetic message {n}") + markers


def clean(value, key=None):
    """Rewrite every string in a JSON value, choosing the kind of fake from the key it sits under."""
    if isinstance(value, dict):
        return {k: clean(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [clean(v, key) for v in value]
    if key == "id" and isinstance(value, str) and UUID_SHAPE.fullmatch(value):
        return fake_guid(value)  # e.g. a poll option's id, which option_id elsewhere refers to
    if not isinstance(value, str) or key in KEEP:
        return value
    if len(value) >= 5 and not value.startswith("🧪"):
        replaced.add(value)
    if key in HANDLE_KEYS:
        return fake_handle(value)
    if key in GUID_KEYS:
        return fake_guid(value)
    if key in {"path", "original_path", "filename"}:
        return fake_path(value)
    if key == "transfer_name":
        return fake_path(value).rsplit("/", 1)[-1]
    if key == "text":
        return fake_text(value)
    return fake("other", value, lambda n: f"synthetic-{n}")


def sanitize_rpc_transcript():
    """Replace personal data in a recorded `imsg rpc` JSONL transcript with consistent fakes.

    Arguments: RAW.jsonl. The sanitized transcript goes to stdout.

    Allowlist approach: only the keys in KEEP pass through untouched; every other string is
    rewritten. The same real value always maps to the same fake, so IDs still line up across
    requests, responses, and notifications. Test Messages ("🧪 …") are kept verbatim.
    """
    out = []
    for raw in open(sys.argv[1]):
        record = json.loads(raw)
        if record["dir"] in ("in", "out"):
            record["line"] = json.dumps(clean(json.loads(record["line"])), ensure_ascii=False)
        out.append(json.dumps(record, ensure_ascii=False))
    result = "\n".join(out) + "\n"

    # Leak check: no rewritten real value may survive anywhere in the output (e.g. inside a
    # field we forgot to list). Path fragments like "chat.db" are expected and harmless.
    leaks = sorted(s for s in replaced if s in result and s != "chat.db")
    if leaks:
        sys.exit(f"sanitize: {len(leaks)} real value(s) leaked into the output; refusing to write it")
    sys.stdout.write(result)


COMMANDS = {
    "record-rpc-transcript": record_rpc_transcript,
    "sanitize-rpc-transcript": sanitize_rpc_transcript,
}


if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in COMMANDS:
        raise SystemExit(__doc__)
    command = sys.argv.pop(1)
    sys.argv[0] += " " + command  # so that usage messages name the subcommand
    sys.exit(COMMANDS[command]())

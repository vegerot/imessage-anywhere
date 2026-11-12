"""Replace personal data in a recorded `imsg rpc` JSONL transcript with consistent fakes.

Usage: python3 sanitize-rpc-transcript.py RAW.jsonl > SANITIZED.jsonl

Allowlist approach: only the keys in KEEP pass through untouched; every other string is
rewritten. The same real value always maps to the same fake, so IDs still line up across
requests, responses, and notifications. Test Messages ("🧪 …") are kept verbatim.
"""
import json
import re
import sys
import uuid

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
    key = (kind, real)
    if key not in fakes:
        fakes[key] = make(sum(1 for k in fakes if k[0] == kind))
    return fakes[key]


def handle(value):
    if value == "":
        return value
    if value.startswith("E:"):  # account_login is "E:<email>" or bare "E:"
        return "E:" + handle(value[2:]) if value[2:] else value
    if "@" in value:
        return fake("email", value.lower(), lambda n: f"person{n}@example.com")
    digits = re.sub(r"\D", "", value)
    if digits and len(digits) <= 6:  # SMS short code
        return fake("short", digits, lambda n: str(50000 + n))
    if digits:
        normalized = fake("phone", digits[-10:], lambda n: f"+1555010{n:04d}")
        return normalized if value.startswith("+") else normalized[1:]
    return fake("name", value, lambda n: f"Synthetic Name {n}")


def guid(value):
    match = re.fullmatch(r"(any|iMessage|SMS|RCS);([-+]);(.+)", value)
    if match:
        service, kind, rest = match.groups()
        tail = handle(rest) if kind == "-" else fake("chat", rest, lambda n: f"chat{n:06d}")
        return f"{service};{kind};{tail}"
    return fake("uuid", value, lambda n: str(uuid.UUID(int=n + 1)).upper())


def path(value):
    if value.endswith("chat.db"):
        return "/Users/me/Library/Messages/chat.db"
    last = value.rsplit("/", 1)[-1]
    ext = "." + last.rsplit(".", 1)[-1] if "." in last else ""  # on-disk paths are often extensionless
    return fake("path", value, lambda n: f"~/Library/Messages/Attachments/00/00/{n:04d}/attachment-{n}{ext}")


OBJECT_REPLACEMENT = "\ufffc"  # where Messages puts an attachment inside a Message's text


def text(value):
    if value.startswith("🧪"):
        return value
    # Keep attachment markers: they're structure, not content, and the page relies on them.
    markers = OBJECT_REPLACEMENT * value.count(OBJECT_REPLACEMENT)
    if not value.replace(OBJECT_REPLACEMENT, "").strip():
        return value
    return fake("text", value, lambda n: f"Synthetic message {n}") + markers


def clean(value, key=None):
    if isinstance(value, dict):
        return {k: clean(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [clean(v, key) for v in value]
    if key == "id" and isinstance(value, str) and UUID_SHAPE.fullmatch(value):
        return guid(value)  # e.g. a poll option's id, which option_id elsewhere refers to
    if not isinstance(value, str) or key in KEEP:
        return value
    if len(value) >= 5 and not value.startswith("🧪"):
        replaced.add(value)
    if key in HANDLE_KEYS:
        return handle(value)
    if key in GUID_KEYS:
        return guid(value)
    if key in {"path", "original_path", "filename"}:
        return path(value)
    if key == "transfer_name":
        return path(value).rsplit("/", 1)[-1]
    if key == "text":
        return text(value)
    return fake("other", value, lambda n: f"synthetic-{n}")


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

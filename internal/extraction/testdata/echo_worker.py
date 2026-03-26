"""Test helper: persistent worker that echoes requests back as responses."""
import json
import sys

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
    except json.JSONDecodeError:
        sys.stdout.write(json.dumps({"status": "error", "error": "bad json"}) + "\n")
        sys.stdout.flush()
        continue

    if req.get("command") == "shutdown":
        break

    # Echo the request back with status=ok
    resp = {"status": "ok", **req}
    sys.stdout.write(json.dumps(resp) + "\n")
    sys.stdout.flush()

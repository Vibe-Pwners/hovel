"""Frozen pre-1.0 wire consumer.

This file intentionally has no dependency on the live SDK. It captures the
minimum protocol spoken by modules already deployed at the 0.3.2 compatibility
floor, so a runtime change cannot make the fixture silently follow it.
"""

from __future__ import annotations

import json
import sys


def read_message():
    header = sys.stdin.buffer.readline()
    if not header:
        return None
    if not header.lower().startswith(b"content-length:"):
        raise RuntimeError("missing Content-Length")
    length = int(header.split(b":", 1)[1])
    if sys.stdin.buffer.readline() != b"\r\n":
        raise RuntimeError("malformed frame")
    return json.loads(sys.stdin.buffer.read(length))


def write_message(message):
    body = json.dumps(message, separators=(",", ":")).encode()
    sys.stdout.buffer.write(f"Content-Length: {len(body)}\r\n\r\n".encode() + body)
    sys.stdout.buffer.flush()


while request := read_message():
    method = request.get("method")
    if method == "handshake":
        result = {
            "name": "contract-probe",
            "version": "v0.3.2-compat",
            "moduleType": "survey",
            "summary": "Deterministic SDK compatibility probe.",
            "description": "",
            "tags": ["compat", "legacy"],
        }
    elif method == "schema":
        result = {
            "chainConfig": [],
            "targetConfig": [
                {
                    "key": "target.host",
                    "type": "host",
                    "required": True,
                    "default": "",
                    "description": "Target host.",
                    "allowed": [],
                    "secret": False,
                }
            ],
            "outputs": {},
        }
    elif method == "step.describe":
        result = {"steps": []}
    elif method == "execute":
        params = request.get("params", {})
        result = {
            "status": "succeeded",
            "summary": "probe complete",
            "outputs": {
                "echo": params.get("inputs", {}).get("probe.value", "default"),
                "target": params.get("target", ""),
            },
            "findings": [],
            "artifacts": [],
            "sessions": [],
            "payloads": [],
            "agentHints": [],
        }
    elif method == "shutdown":
        result = {"status": "ok"}
    else:
        write_message(
            {
                "jsonrpc": "2.0",
                "id": request.get("id"),
                "error": {"code": -32000, "message": f"unknown method {method}"},
            }
        )
        continue
    write_message({"jsonrpc": "2.0", "id": request.get("id"), "result": result})
    if method == "shutdown":
        break

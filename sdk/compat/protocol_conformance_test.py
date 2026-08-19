from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path


def resolve(path: str) -> Path:
    candidate = Path(path)
    if candidate.exists():
        return candidate.resolve()
    for root_name in ("RUNFILES_DIR", "TEST_SRCDIR"):
        root = os.environ.get(root_name)
        if not root:
            continue
        for prefix in ("", "_main", "hovel"):
            candidate = Path(root) / prefix / path
            if candidate.exists():
                return candidate.resolve()
    raise AssertionError(f"runfile not found: {path}")


def encode(message: dict) -> bytes:
    body = json.dumps(message, separators=(",", ":")).encode()
    return f"Content-Length: {len(body)}\r\n\r\n".encode() + body


def decode_all(data: bytes) -> list[dict]:
    messages = []
    while data:
        header, data = data.split(b"\r\n\r\n", 1)
        length = int(header.split(b":", 1)[1])
        body, data = data[:length], data[length:]
        messages.append(json.loads(body))
    return messages


def normalize(result: dict) -> dict:
    result = dict(result)
    if "tags" in result:
        result["tags"] = [tag for tag in result["tags"] if tag == "compat"]
    # SDKs historically differed on whether empty optional collections were
    # emitted. Their semantic contract is an empty collection either way.
    for key in ("findings", "artifacts", "sessions", "payloads", "agentHints"):
        if not result.get(key):
            result.pop(key, None)
    return result


def run_probe(binary: Path, requests: list[dict]) -> dict[int, dict]:
    completed = subprocess.run(
        [str(binary)],
        input=b"".join(encode(request) for request in requests),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        timeout=15,
    )
    if completed.returncode != 0:
        raise AssertionError(f"{binary} exited {completed.returncode}: {completed.stderr.decode(errors='replace')}")
    responses = {}
    for message in decode_all(completed.stdout):
        if "id" in message:
            responses[int(message["id"])] = message
    return responses


def main() -> int:
    contract = json.loads(resolve("sdk/compat/protocol_contract_v1.json").read_text())
    requests = contract["requests"]
    binaries = [resolve(arg) for arg in sys.argv[1:]]
    if len(binaries) != 4:
        raise AssertionError("expected Python, Go, Rust, and frozen legacy probes")
    observed = [run_probe(binary, requests) for binary in binaries]

    expected_handshake = {
        "name": "contract-probe",
        "version": "v0.3.2-compat",
        "moduleType": "survey",
        "summary": "Deterministic SDK compatibility probe.",
        "description": "",
        "tags": ["compat"],
    }
    expected_outputs = {"echo": "fixture", "target": "mock://compat"}
    for binary, responses in zip(binaries, observed, strict=True):
        assert normalize(responses[1]["result"]) == expected_handshake, binary
        schema = responses[2]["result"]
        assert schema["targetConfig"][0]["key"] == "target.host", binary
        execute = responses[3]["result"]
        assert execute["status"] == "succeeded", (binary, execute)
        assert execute["summary"] == "probe complete", binary
        assert execute["outputs"] == expected_outputs, binary
        error = responses[4]["error"]
        assert error["code"] == -32000 and "unknown method" in error["message"], (binary, error)
        assert responses[5]["result"]["status"] == "ok", binary
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Enforce readable pacing for every checked-in VHS demo."""

from __future__ import annotations

import re
from pathlib import Path


TYPING_RE = re.compile(r"^Set TypingSpeed (\d+) ms$")
SLEEP_RE = re.compile(r"^Sleep (\d+) ms$")


def main() -> int:
    tapes = sorted(Path("docs/demo").glob("tapes*/*.tape"))
    if not tapes:
        raise AssertionError("no VHS tapes found")

    failures: list[str] = []
    for tape in tapes:
        lines = [line.strip() for line in tape.read_text(encoding="utf-8").splitlines()]
        typing = [int(match.group(1)) for line in lines if (match := TYPING_RE.match(line))]
        sleeps = [int(match.group(1)) for line in lines if (match := SLEEP_RE.match(line))]
        if not typing or min(typing) < 20:
            failures.append(f"{tape}: typing speed must be at least 20 ms")
        if not sleeps or sleeps[-1] < 4000:
            failures.append(f"{tape}: final visible hold must be at least 4000 ms")

    if failures:
        raise AssertionError("\n".join(failures))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

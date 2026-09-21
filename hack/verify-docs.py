#!/usr/bin/env python3
"""Reject broken local Markdown links without fetching external resources."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
broken = []
for document in [root / "README.md", *sorted((root / "docs").rglob("*.md"))]:
    text = document.read_text()
    for target in re.findall(r"(?<!!)\[[^]]+\]\(([^)]+)\)", text):
        target = target.split("#", 1)[0]
        if not target or "://" in target or target.startswith("mailto:"):
            continue
        resolved = (document.parent / target).resolve()
        if not resolved.exists():
            broken.append(f"{document.relative_to(root)}: {target}")
if broken:
    print("broken local documentation links:", *broken, sep="\n", file=sys.stderr)
    sys.exit(1)

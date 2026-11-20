#!/usr/bin/env python3
"""Validate outlines against .agents/outline-method.md without executing them.

Hard errors (always fail):
  * a relative markdown link that does not resolve from the file's own directory
  * a backticked repo path that does not exist and is not marked (planned)
  * a canonical policy threshold pasted as a literal instead of cited by key name

Warnings (fail only with --strict): a missing or unrecognised provenance header. The
backlog is clear, so CI runs this strict; the split remains so a bulk documentation
import can be triaged before it blocks a build.
"""

import argparse
import re
import sys
from pathlib import Path

import yaml

OUTLINE_ROOTS = (".agents", "plans", "spec")
EXTRA_FILES = ("CLAUDE.md", "README.md")
SKIP_PARTS = {".cache", ".venv", ".worktrees", "node_modules", "__pycache__", ".git"}
STATUSES = {"authoritative", "proposed", "superseded", "active", "draft", "accepted"}
POLICY_FILE = Path("orchestrator/policy.yaml")

LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
STATUS = re.compile(r"(?im)^(?:\*\*Status:\*\*|-\s*Status:)\s*([A-Za-z]+)")
BACKTICK = re.compile(r"`([^`\n]+)`")
CODE_FENCE = re.compile(r"^\s*```")
SUFFIXES = (".go", ".py", ".json", ".yaml", ".yml", ".md", ".sh", ".toml", ".html")
EXEMPT_MARKERS = ("(planned)", "(absent)", "(superseded)")

# ADRs necessarily name paths that were rejected, renamed or removed; recording history
# is their purpose, so they are exempt from the path-existence check only.
PATH_CHECK_EXEMPT = ("spec/DECISIONS",)

# A document describing a design that was never built, or one that has been replaced,
# must be free to name the paths that design would have had. Marking it proposed or
# superseded is the declaration that its contents are not instructions.
HISTORICAL = {"proposed", "superseded"}


def iter_outlines(root: Path) -> list[Path]:
    found: list[Path] = []
    for sub in OUTLINE_ROOTS:
        base = root / sub
        if not base.is_dir():
            continue
        found.extend(
            path
            for path in base.rglob("*.md")
            if not SKIP_PARTS.intersection(path.parts)
        )
    found.extend(root / name for name in EXTRA_FILES if (root / name).is_file())
    return sorted(set(found))


def distinctive_thresholds(root: Path) -> dict[str, str]:
    """Canonical policy values distinctive enough to flag when pasted into prose.

    Small integers such as 2 or 24 are deliberately excluded: they collide with ordinary
    prose ("4-round Feistel", "24 hours") far too often to gate on.
    """
    path = root / POLICY_FILE
    if not path.is_file():
        return {}
    config = yaml.safe_load(path.read_text()) or {}
    values: dict[str, str] = {}
    for key, value in config.items():
        if isinstance(value, bool) or not isinstance(value, int | float):
            continue
        if isinstance(value, float) or value >= 1000:
            values[str(value)] = key
    return values


def code_line_numbers(text: str) -> set[int]:
    """Line numbers inside fenced code blocks, which are examples rather than claims."""
    inside = False
    fenced: set[int] = set()
    for number, line in enumerate(text.splitlines(), start=1):
        if CODE_FENCE.match(line):
            inside = not inside
            fenced.add(number)
        elif inside:
            fenced.add(number)
    return fenced


def looks_like_path(candidate: str) -> bool:
    """A backticked token specific enough to be checkable.

    Requires a directory separator, so bare filenames such as `policy.yaml` stay
    ambiguous and unchecked. Globs and placeholder templates are excluded:
    `schemas/*.json` and `results/<phase>.json` are patterns, not paths, and resolving
    them is not this gate's job.
    """
    if "/" not in candidate or " " in candidate:
        return False
    if any(character in candidate for character in "*<>"):
        return False
    if candidate.startswith(("http", "#", "@", "-")):
        return False
    return candidate.endswith("/") or candidate.endswith(SUFFIXES)


def check_links(path: Path, text: str, relative: str) -> list[str]:
    errors = []
    for match in LINK.finditer(text):
        target = match.group(1).split("#")[0].strip()
        if not target or target.startswith(("http://", "https://", "mailto:")):
            continue
        if not (path.parent / target).resolve().exists():
            errors.append(f"{relative}: dead link {target!r}")
    return errors


def check_paths(root: Path, text: str, relative: str, status: str | None = None) -> list[str]:
    if relative.startswith(PATH_CHECK_EXEMPT) or status in HISTORICAL:
        return []
    fenced = code_line_numbers(text)
    errors = []
    for number, line in enumerate(text.splitlines(), start=1):
        if number in fenced or any(marker in line for marker in EXEMPT_MARKERS):
            continue
        for candidate in BACKTICK.findall(line):
            candidate = candidate.strip()
            if not looks_like_path(candidate):
                continue
            if not (root / candidate.rstrip("/")).exists():
                errors.append(
                    f"{relative}:{number}: path {candidate!r} does not exist; "
                    f"mark it (planned) or correct it"
                )
    return errors


def check_thresholds(text: str, relative: str, thresholds: dict[str, str]) -> list[str]:
    fenced = code_line_numbers(text)
    errors = []
    for number, line in enumerate(text.splitlines(), start=1):
        if number in fenced:
            continue
        for literal, key in thresholds.items():
            if re.search(rf"(?<![\w.]){re.escape(literal)}(?![\w.])", line):
                errors.append(
                    f"{relative}:{number}: threshold {literal} pasted; "
                    f"cite policy.yaml:{key} by name instead"
                )
    return errors


def check_provenance(text: str, relative: str) -> list[str]:
    match = STATUS.search(text)
    if match is None:
        return [f"{relative}: missing provenance header (**Status:** ...)"]
    value = match.group(1).lower()
    if value not in STATUSES:
        return [f"{relative}: unrecognised status {value!r}; expected one of {sorted(STATUSES)}"]
    return []


def validate_tree(root: Path) -> tuple[list[str], list[str]]:
    outlines = iter_outlines(root)
    if not outlines:
        return [f"{root}: no outlines found"], []
    thresholds = distinctive_thresholds(root)
    errors: list[str] = []
    warnings: list[str] = []
    for path in outlines:
        relative = str(path.relative_to(root))
        text = path.read_text()
        declared = STATUS.search(text)
        status = declared.group(1).lower() if declared else None
        errors.extend(check_links(path, text, relative))
        errors.extend(check_paths(root, text, relative, status))
        errors.extend(check_thresholds(text, relative, thresholds))
        warnings.extend(check_provenance(text, relative))
    return errors, warnings


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path, nargs="?", default=Path("."))
    parser.add_argument(
        "--strict", action="store_true", help="treat provenance warnings as failures"
    )
    args = parser.parse_args()
    errors, warnings = validate_tree(args.root)
    for warning in warnings:
        print(f"warning: {warning}", file=sys.stderr)
    for error in errors:
        print(error, file=sys.stderr)
    if warnings:
        print(f"{len(warnings)} provenance warning(s) outstanding", file=sys.stderr)
    return int(bool(errors) or (args.strict and bool(warnings)))


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Validate the repository's small, composable skill set without executing it."""

import argparse
import re
import sys
from pathlib import Path

NAME = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
MAX_SKILLS = 5
CALLS = re.compile(r"(?im)^Calls:\s*([a-z0-9 -]+?)\s*$")


def validate_text(relative_path: str, text: str) -> list[str]:
    errors = []
    lines = text.splitlines()
    if not lines or lines[0] != "---":
        return [f"{relative_path}: missing YAML frontmatter"]
    try:
        end = lines[1:].index("---") + 1
    except ValueError:
        return [f"{relative_path}: unterminated YAML frontmatter"]
    fields = {}
    for line in lines[1:end]:
        if not line.strip():
            continue
        if line.startswith((" ", "\t")) or ":" not in line:
            errors.append(f"{relative_path}: unsupported frontmatter line {line!r}")
            continue
        key, value = line.split(":", 1)
        fields[key.strip()] = value.strip()
    name = fields.get("name", "")
    description = fields.get("description", "")
    if not NAME.fullmatch(name):
        errors.append(f"{relative_path}: name must use lowercase kebab-case")
    directory = relative_path.removesuffix("/SKILL.md")
    if name != directory:
        errors.append(f"{relative_path}: name must match directory {directory!r}")
    if len(description) < 40:
        errors.append(f"{relative_path}: description must contain at least 40 characters")
    if not text[end + 1 :].strip():
        errors.append(f"{relative_path}: skill body is empty")
    return errors


def validate_tree(root: Path) -> list[str]:
    paths = sorted(root.glob("*/SKILL.md"))
    errors = []
    if len(paths) > MAX_SKILLS:
        errors.append(f"{root}: at most {MAX_SKILLS} modular skills are allowed")
    if not paths:
        errors.append(f"{root}: no skills found")
    graph = {}
    for path in paths:
        text = path.read_text()
        errors.extend(validate_text(str(path.relative_to(root)), text))
        match = CALLS.search(text)
        graph[path.parent.name] = set(match.group(1).split()) if match else set()
    known = set(graph)
    for name, calls in graph.items():
        for called in calls:
            if called not in known:
                errors.append(f"{name}: calls unknown skill {called!r}")
    visiting = set()
    visited = set()

    def visit(name):
        if name in visiting:
            errors.append(f"skill call graph contains a cycle at {name!r}")
            return
        if name in visited:
            return
        visiting.add(name)
        for called in graph.get(name, ()) & known:
            visit(called)
        visiting.remove(name)
        visited.add(name)

    for name in graph:
        visit(name)
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    args = parser.parse_args()
    errors = validate_tree(args.root)
    for error in errors:
        print(error, file=sys.stderr)
    return int(bool(errors))


if __name__ == "__main__":
    raise SystemExit(main())

from __future__ import annotations

import argparse
import os
import shlex
import subprocess
import sys
from pathlib import Path
from typing import Sequence

from . import __version__


def main(argv: Sequence[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if not args:
        print_usage(sys.stderr)
        return 2

    command, rest = args[0], args[1:]
    if command == "scan":
        return scan(rest)
    if command == "serve":
        return run_core(["serve"])
    if command in {"version", "--version", "-v"}:
        print(__version__)
        return 0
    if command in {"help", "--help", "-h"}:
        print_usage(sys.stderr)
        return 0

    print(f"unknown command: {command}", file=sys.stderr)
    print_usage(sys.stderr)
    return 2


def scan(argv: Sequence[str]) -> int:
    parser = argparse.ArgumentParser(
        prog="java-process-mapper scan",
        description="Run the mapping pipeline locally and print a JSON summary.",
    )
    parser.add_argument("--root", required=True, help="path to Java repository root")
    parser.add_argument("--out", default="", help="output directory")
    parser.add_argument(
        "--addons",
        default="spring",
        help="comma-separated addons; default spring, use javaee for Java EE/Jakarta EE",
    )
    parser.add_argument(
        "--java-version",
        default="",
        help="override Java source version, for example 8, 11, 17 or 21",
    )
    parser.add_argument("--include-tests", action="store_true", help="include test source folders")
    parsed = parser.parse_args(list(argv))

    core_args = [
        "scan",
        "--root",
        parsed.root,
        "--addons",
        parsed.addons,
    ]
    if parsed.out:
        core_args.extend(["--out", parsed.out])
    if parsed.java_version:
        core_args.extend(["--java-version", parsed.java_version])
    if parsed.include_tests:
        core_args.append("--include-tests")
    return run_core(core_args)


def run_core(args: Sequence[str]) -> int:
    command = core_command()
    repo_root = repository_root()
    completed = subprocess.run([*command, *args], cwd=repo_root)
    return completed.returncode


def core_command() -> list[str]:
    configured = os.environ.get("JAVA_PROCESS_MAPPER_CORE_COMMAND", "").strip()
    if configured:
        return shlex.split(configured, posix=os.name != "nt")
    return ["go", "run", "./cmd/java-process-mapper-core"]


def repository_root() -> Path:
    configured = os.environ.get("JAVA_PROCESS_MAPPER_REPO_ROOT", "").strip()
    if configured:
        return Path(configured).resolve()
    return Path(__file__).resolve().parents[1]


def print_usage(stream) -> None:
    print(
        f"""java-process-mapper {__version__}

Usage:
  java-process-mapper serve
  java-process-mapper scan --root <path> --out <path> --addons spring
  java-process-mapper scan --root <path> --addons javaee --java-version 8

Commands:
  serve   Start MCP server over stdio.
  scan    Run the mapping pipeline locally and print a JSON summary.
""",
        file=stream,
        end="",
    )

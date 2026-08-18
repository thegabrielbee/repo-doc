from __future__ import annotations

import argparse
import json
import sys
from typing import Sequence

from . import __version__
from .mcp import serve_stdio
from .pipeline import PipelineError, run_scan


def main(argv: Sequence[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if not args:
        print_usage(sys.stderr)
        return 2

    command, rest = args[0], args[1:]
    if command == "scan":
        return scan(rest)
    if command == "serve":
        return serve_stdio()
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

    try:
        result = run_scan(
            root=parsed.root,
            output_dir=parsed.out or None,
            addons=parsed.addons,
            java_version=parsed.java_version or None,
            include_tests=parsed.include_tests,
        )
    except PipelineError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


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

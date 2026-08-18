from __future__ import annotations

from pathlib import Path
from typing import Any

from .addons import apply_addons, resolve_addons
from .discovery import dedupe_project, discover_project
from .flow import build_graph, build_traces
from .java_parser import parse_project_sources
from .model import refresh_summary
from .output import write_outputs


class PipelineError(Exception):
    pass


def run_scan(
    *,
    root: str,
    output_dir: str | None = None,
    addons: str | list[str] | None = None,
    java_version: str | None = None,
    include_tests: bool = False,
) -> dict[str, Any]:
    try:
        addon_list = resolve_addons(addons)
        root_path = Path(root).resolve()
        out = Path(output_dir).resolve() if output_dir else root_path / "out" / "java-process-mapper"
        project = discover_project(root_path, addons=addon_list, java_version=java_version, include_tests=include_tests)
        parse_project_sources(project)
        apply_addons(project, addon_list)
        dedupe_project(project)
        traces = build_traces(project)
        build_graph(project, traces)
        refresh_summary(project)
        artifacts = write_outputs(project, traces, out)
    except Exception as exc:  # noqa: BLE001 - CLI/MCP should report a stable pipeline failure.
        raise PipelineError(f"scan failed: {exc}") from exc

    return {
        "status": "completed",
        "projectId": project["id"],
        "rootPath": str(root_path),
        "outputDir": str(out),
        "artifacts": artifacts,
        "summary": project["summary"],
    }

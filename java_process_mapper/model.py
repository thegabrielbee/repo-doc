from __future__ import annotations

from pathlib import Path
from typing import Any

from .utils import stable_id


def evidence(
    path: str | Path = "",
    line: int = 0,
    symbol: str = "",
    kind: str = "",
    annotation: str = "",
    property: str = "",
) -> dict[str, Any]:
    item: dict[str, Any] = {}
    if path:
        item["path"] = str(path)
    if line:
        item["line"] = line
    if symbol:
        item["symbol"] = symbol
    if kind:
        item["kind"] = kind
    if annotation:
        item["annotation"] = annotation
    if property:
        item["property"] = property
    return item


def entrypoint(
    *,
    framework: str,
    kind: str,
    name: str,
    module_id: str,
    module_name: str,
    path: str = "",
    http_method: str = "",
    class_id: str = "",
    method_id: str = "",
    evidence_item: dict[str, Any] | None = None,
) -> dict[str, Any]:
    entry_id = stable_id("entrypoint", framework, kind, path, class_id, method_id, name)
    return {
        "id": entry_id,
        "framework": framework,
        "kind": kind,
        "name": name,
        "moduleId": module_id,
        "moduleName": module_name,
        "path": path,
        "httpMethod": http_method,
        "classId": class_id,
        "methodId": method_id,
        "confidence": "high" if evidence_item else "medium",
        "source": "found",
        "evidence": [evidence_item] if evidence_item else [],
    }


def dependency(
    *,
    kind: str,
    name: str,
    module_id: str,
    module_name: str,
    detail: str = "",
    class_id: str = "",
    method_id: str = "",
    target: str = "",
    evidence_item: dict[str, Any] | None = None,
    confidence: str = "medium",
) -> dict[str, Any]:
    dep_id = stable_id("dependency", kind, name, detail, class_id, method_id, target)
    return {
        "id": dep_id,
        "kind": kind,
        "name": name,
        "moduleId": module_id,
        "moduleName": module_name,
        "detail": detail,
        "target": target,
        "classId": class_id,
        "methodId": method_id,
        "confidence": confidence,
        "source": "found" if evidence_item else "inferred",
        "evidence": [evidence_item] if evidence_item else [],
    }


def refresh_summary(project: dict[str, Any]) -> None:
    project["summary"] = {
        "modules": len(project.get("modules", [])),
        "sourceFiles": len(project.get("sourceFiles", [])),
        "types": len(project.get("types", [])),
        "entryPoints": len(project.get("entryPoints", [])),
        "dependencies": len(project.get("dependencies", [])),
    }

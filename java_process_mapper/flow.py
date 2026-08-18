from __future__ import annotations

import fnmatch
from typing import Any


def build_traces(project: dict[str, Any]) -> dict[str, dict[str, Any]]:
    method_index = method_index_by_name(project)
    traces: dict[str, dict[str, Any]] = {}
    for entry in project.get("entryPoints", []):
        steps = []
        deps = []
        method = method_by_id(project, entry.get("methodId", ""))
        if method:
            steps.append(step_from_method(method, "entrypoint"))
            for call in method.get("calls", []):
                called_method = method_index.get(call.get("name", ""))
                if called_method:
                    steps.append(step_from_method(called_method, f"call at line {call.get('line', 0)}"))
        deps.extend(dependencies_for_entry(project, entry, method))
        traces[entry["id"]] = {
            "entryPointId": entry["id"],
            "entryPoint": entry,
            "steps": dedupe_steps(steps),
            "dependencies": dedupe_deps(deps),
            "evidence": entry.get("evidence", []),
        }
    return traces


def build_graph(project: dict[str, Any], traces: dict[str, dict[str, Any]]) -> None:
    nodes: list[dict[str, Any]] = []
    edges: list[dict[str, Any]] = []
    nodes.append({"id": project["id"], "label": project["name"], "kind": "project"})
    for module in project.get("modules", []):
        nodes.append({"id": module["id"], "label": module["name"], "kind": "module"})
        edges.append({"from": project["id"], "to": module["id"], "kind": "contains"})
    for entry in project.get("entryPoints", []):
        nodes.append({"id": entry["id"], "label": entry["name"], "kind": f"entrypoint:{entry['kind']}"})
        edges.append({"from": entry["moduleId"], "to": entry["id"], "kind": "exposes"})
    for dep in project.get("dependencies", []):
        nodes.append({"id": dep["id"], "label": dep["name"], "kind": f"dependency:{dep['kind']}"})
        if dep.get("moduleId"):
            edges.append({"from": dep["moduleId"], "to": dep["id"], "kind": "uses"})
    for trace in traces.values():
        for dep in trace.get("dependencies", []):
            edges.append({"from": trace["entryPointId"], "to": dep["id"], "kind": "touches"})
    project["graph"] = {"nodes": unique_nodes(nodes), "edges": unique_edges(edges)}


def dependencies_for_entry(project: dict[str, Any], entry: dict[str, Any], method: dict[str, Any] | None) -> list[dict[str, Any]]:
    deps: list[dict[str, Any]] = []
    entry_path = entry.get("path", "")
    for dep in project.get("dependencies", []):
        if dep.get("methodId") and dep.get("methodId") == entry.get("methodId"):
            deps.append(dep)
            continue
        if dep.get("classId") and dep.get("classId") == entry.get("classId") and dep.get("kind") not in {"table"}:
            deps.append(dep)
            continue
        if dep.get("kind") == "http_filter" and entry.get("kind") in {"http", "servlet"} and filter_matches(dep.get("detail", ""), entry_path):
            deps.append(dep)
            continue
        if dep.get("kind") in {"ui_api_call", "ui_websocket"} and dep.get("detail") == entry_path:
            deps.append(dep)
            continue
    if method:
        call_names = {call.get("name", "") for call in method.get("calls", [])}
        for dep in project.get("dependencies", []):
            if dep.get("name") in call_names and dep not in deps:
                deps.append(dep)
    return deps


def filter_matches(patterns: str, path: str) -> bool:
    if not patterns:
        return True
    if not path:
        return False
    for pattern in [item.strip() for item in patterns.split(",") if item.strip()]:
        servlet_pattern = pattern.replace("*", "*")
        if pattern == "/*" or fnmatch.fnmatch(path, servlet_pattern):
            return True
        if pattern.endswith("/*") and path.startswith(pattern[:-1]):
            return True
    return False


def method_by_id(project: dict[str, Any], method_id: str) -> dict[str, Any] | None:
    if not method_id:
        return None
    for type_item in project.get("types", []):
        for method in type_item.get("methods", []):
            if method.get("id") == method_id:
                return method
    return None


def method_index_by_name(project: dict[str, Any]) -> dict[str, dict[str, Any]]:
    index: dict[str, dict[str, Any]] = {}
    for type_item in project.get("types", []):
        for method in type_item.get("methods", []):
            index.setdefault(method.get("name", ""), method)
    return index


def step_from_method(method: dict[str, Any], reason: str) -> dict[str, Any]:
    return {
        "id": method.get("id", ""),
        "name": method.get("qualifiedName", method.get("name", "")),
        "kind": "method",
        "reason": reason,
        "file": method.get("file", ""),
        "line": method.get("line", 0),
        "conditions": method.get("conditions", []),
    }


def dedupe_steps(steps: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[str] = set()
    result: list[dict[str, Any]] = []
    for step in steps:
        key = step.get("id", "")
        if key in seen:
            continue
        seen.add(key)
        result.append(step)
    return result


def dedupe_deps(deps: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[str] = set()
    result: list[dict[str, Any]] = []
    for dep in deps:
        key = dep.get("id", "")
        if key in seen:
            continue
        seen.add(key)
        result.append(dep)
    return result


def unique_nodes(nodes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[str] = set()
    result: list[dict[str, Any]] = []
    for node in nodes:
        if node["id"] in seen:
            continue
        seen.add(node["id"])
        result.append(node)
    return result


def unique_edges(edges: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[tuple[str, str, str]] = set()
    result: list[dict[str, Any]] = []
    for edge in edges:
        key = (edge["from"], edge["to"], edge["kind"])
        if key in seen:
            continue
        seen.add(key)
        result.append(edge)
    return result

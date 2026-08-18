from __future__ import annotations

import json
import sys
import uuid
from pathlib import Path
from typing import Any

from .output import process_markdown
from .pipeline import PipelineError, run_scan
from .utils import stable_id

JOBS: dict[str, dict[str, Any]] = {}


def serve_stdio() -> int:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
            response = handle_request(request)
        except Exception as exc:  # noqa: BLE001 - stdio server must not crash on malformed input.
            response = {"jsonrpc": "2.0", "id": None, "error": {"code": -32603, "message": str(exc)}}
        if response is not None:
            print(json.dumps(response, ensure_ascii=False), flush=True)
    return 0


def handle_request(request: dict[str, Any]) -> dict[str, Any] | None:
    method = request.get("method", "")
    request_id = request.get("id")
    params = request.get("params") or {}
    if method in {"initialize", "mcp.initialize"}:
        return ok(request_id, {"serverInfo": {"name": "java-process-mapper", "version": "0.1.0"}, "capabilities": {"tools": {}}})
    if method in {"notifications/initialized", "initialized"}:
        return None
    if method in {"tools/list", "mcp.tools.list"}:
        return ok(request_id, {"tools": tools()})
    if method in {"tools/call", "mcp.tools.call"}:
        name = params.get("name")
        arguments = params.get("arguments") or {}
        return ok(request_id, {"content": [{"type": "text", "text": json.dumps(call_tool(name, arguments), ensure_ascii=False)}]})
    if method in {"start_mapping", "get_mapping_status", "get_mapping_result", "get_next_mapping_item", "get_mapping_item", "mark_mapping_item_mapped"}:
        return ok(request_id, call_tool(method, params))
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": -32601, "message": f"unknown method: {method}"}}


def call_tool(name: str, args: dict[str, Any]) -> dict[str, Any]:
    if name == "start_mapping":
        root = args.get("rootPath") or args.get("root")
        if not root:
            raise ValueError("rootPath is required")
        job_id = str(uuid.uuid4())
        try:
            result = run_scan(
                root=root,
                output_dir=args.get("outputDir") or args.get("out"),
                addons=args.get("addons"),
                java_version=args.get("javaVersion"),
                include_tests=bool(args.get("includeTests", False)),
            )
            JOBS[job_id] = {"jobId": job_id, "status": "completed", "result": result}
        except PipelineError as exc:
            JOBS[job_id] = {"jobId": job_id, "status": "failed", "error": str(exc)}
        return JOBS[job_id]
    if name == "get_mapping_status":
        mapping_job = lookup_job(args)
        return {key: value for key, value in mapping_job.items() if key != "result"}
    if name == "get_mapping_result":
        return lookup_job(args)
    if name == "get_next_mapping_item":
        return get_next_mapping_item(args)
    if name == "get_mapping_item":
        return get_mapping_item(args)
    if name == "mark_mapping_item_mapped":
        return mark_mapping_item_mapped(args)
    raise ValueError(f"unknown tool: {name}")


def lookup_job(args: dict[str, Any]) -> dict[str, Any]:
    job_id = args.get("jobId")
    if not job_id or job_id not in JOBS:
        raise ValueError("unknown jobId")
    return JOBS[job_id]


def get_next_mapping_item(args: dict[str, Any]) -> dict[str, Any]:
    job = lookup_job(args)
    for state_item in load_state(job).get("items", []):
        if state_item.get("status") == "mapped":
            continue
        return mapping_item_response(job, state_item["entryPointId"], include_markdown=bool(args.get("includeMechanicalMarkdown")))
    return {"status": "empty", "done": True, "item": None}


def get_mapping_item(args: dict[str, Any]) -> dict[str, Any]:
    job = lookup_job(args)
    selector_keys = ("entryPointId", "entryPointName", "title", "query", "documentPath")
    selector = next((key for key in selector_keys if str(args.get(key, "")).strip()), "")
    if not selector and "index" not in args:
        raise ValueError("one selector is required: entryPointId, entryPointName, title, query, documentPath or index")

    state = load_state(job)
    traces = load_traces(job)
    state_items = state.get("items", [])

    if "index" in args:
        index = int(args["index"])
        if index < 0 or index >= len(state_items):
            return {"status": "not_found", "item": None}
        return mapping_item_response(job, state_items[index]["entryPointId"], include_markdown=bool(args.get("includeMechanicalMarkdown")))

    value = str(args.get(selector, "")).strip()
    matches = matching_items(job, state_items, traces, selector, value)
    if not matches:
        return {"status": "not_found", "item": None}
    if selector == "query" and len(matches) > 1:
        return {"status": "ambiguous", "matches": [item_summary(job, item["entryPointId"]) for item in matches]}
    return mapping_item_response(job, matches[0]["entryPointId"], include_markdown=bool(args.get("includeMechanicalMarkdown")))


def mark_mapping_item_mapped(args: dict[str, Any]) -> dict[str, Any]:
    job = lookup_job(args)
    entry_point_id = str(args.get("entryPointId", "")).strip()
    if not entry_point_id:
        raise ValueError("entryPointId is required")

    state_path = artifact_path(job, "state")
    state = load_state(job)
    traces = load_traces(job)
    trace = traces.get(entry_point_id)
    if not trace:
        raise ValueError(f"unknown entryPointId: {entry_point_id}")

    title = str(args.get("title") or trace["entryPoint"].get("name", "")).strip()
    final_doc_path = str(args.get("finalDocPath") or default_final_doc_path(job, title))
    markdown = str(args.get("markdown") or "")
    if markdown:
        doc_path = Path(final_doc_path)
        doc_path.parent.mkdir(parents=True, exist_ok=True)
        doc_path.write_text(markdown, encoding="utf-8")

    for item in state.get("items", []):
        if item.get("entryPointId") == entry_point_id:
            item["status"] = "mapped"
            item["title"] = title
            item["notes"] = str(args.get("notes") or "")
            item["finalDocPath"] = final_doc_path
            break
    else:
        state.setdefault("items", []).append(
            {
                "entryPointId": entry_point_id,
                "status": "mapped",
                "title": title,
                "notes": str(args.get("notes") or ""),
                "finalDocPath": final_doc_path,
            }
        )
    write_json(state_path, state)
    return {"status": "mapped", "entryPointId": entry_point_id, "finalDocPath": final_doc_path}


def matching_items(job: dict[str, Any], state_items: list[dict[str, Any]], traces: dict[str, Any], selector: str, value: str) -> list[dict[str, Any]]:
    normalized = value.casefold()
    result: list[dict[str, Any]] = []
    for state_item in state_items:
        entry_point_id = state_item.get("entryPointId", "")
        trace = traces.get(entry_point_id, {})
        entry = trace.get("entryPoint", {})
        mechanical_path = process_doc_path(job, entry)
        candidates = {
            "entryPointId": [entry_point_id],
            "entryPointName": [entry.get("name", "")],
            "title": [state_item.get("title", ""), entry.get("name", "")],
            "documentPath": [state_item.get("finalDocPath", ""), Path(str(state_item.get("finalDocPath", ""))).name, mechanical_path, Path(mechanical_path).name],
            "query": [
                entry_point_id,
                entry.get("name", ""),
                entry.get("kind", ""),
                entry.get("framework", ""),
                entry.get("path", ""),
                state_item.get("title", ""),
                state_item.get("finalDocPath", ""),
                mechanical_path,
            ],
        }
        values = [str(candidate) for candidate in candidates.get(selector, []) if candidate]
        if selector == "query":
            if any(normalized in candidate.casefold() for candidate in values):
                result.append(state_item)
        elif any(candidate.casefold() == normalized for candidate in values):
            result.append(state_item)
    return result


def mapping_item_response(job: dict[str, Any], entry_point_id: str, *, include_markdown: bool) -> dict[str, Any]:
    state = load_state(job)
    state_item = next((item for item in state.get("items", []) if item.get("entryPointId") == entry_point_id), {})
    traces = load_traces(job)
    trace = traces.get(entry_point_id)
    if not trace:
        return {"status": "not_found", "item": None}
    item = {
        "entryPointId": entry_point_id,
        "status": state_item.get("status", "pending"),
        "title": state_item.get("title") or trace["entryPoint"].get("name", ""),
        "entryPoint": trace["entryPoint"],
        "trace": trace,
        "processDocPath": process_doc_path(job, trace["entryPoint"]),
        "finalDocPath": state_item.get("finalDocPath", ""),
    }
    response = {"status": "found", "done": False, "item": item}
    if include_markdown:
        response["mechanicalMarkdown"] = process_markdown(trace)
    return response


def item_summary(job: dict[str, Any], entry_point_id: str) -> dict[str, Any]:
    traces = load_traces(job)
    trace = traces.get(entry_point_id, {})
    entry = trace.get("entryPoint", {})
    return {
        "entryPointId": entry_point_id,
        "title": entry.get("name", ""),
        "framework": entry.get("framework", ""),
        "kind": entry.get("kind", ""),
        "path": entry.get("path", ""),
    }


def load_state(job: dict[str, Any]) -> dict[str, Any]:
    return read_json(artifact_path(job, "state"))


def load_traces(job: dict[str, Any]) -> dict[str, Any]:
    return read_json(artifact_path(job, "traces"))


def artifact_path(job: dict[str, Any], key: str) -> Path:
    result = job.get("result") or {}
    path = (result.get("artifacts") or {}).get(key)
    if not path:
        raise ValueError(f"job has no artifact path for {key}")
    return Path(path)


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")


def process_doc_path(job: dict[str, Any], entry: dict[str, Any]) -> str:
    if not entry:
        return ""
    result = job.get("result") or {}
    output_dir = Path(result.get("outputDir", ".")).resolve()
    filename = f"{stable_id(entry.get('kind', ''), entry.get('name', ''))}.md"
    return str(output_dir / "docs" / "processes" / filename)


def default_final_doc_path(job: dict[str, Any], title: str) -> str:
    result = job.get("result") or {}
    output_dir = Path(result.get("outputDir", ".")).resolve()
    return str(output_dir / "docs" / "mapped" / f"{stable_id(title or 'processo')}.md")


def ok(request_id: Any, result: Any) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def tools() -> list[dict[str, Any]]:
    return [
        {
            "name": "start_mapping",
            "description": "Run a static Java process mapping scan.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "rootPath": {"type": "string"},
                    "outputDir": {"type": "string"},
                    "addons": {"type": "array", "items": {"type": "string"}},
                    "javaVersion": {"type": "string"},
                    "includeTests": {"type": "boolean"},
                },
                "required": ["rootPath"],
            },
        },
        {"name": "get_mapping_status", "description": "Return status for a mapping job.", "inputSchema": {"type": "object", "properties": {"jobId": {"type": "string"}}, "required": ["jobId"]}},
        {"name": "get_mapping_result", "description": "Return artifacts for a completed mapping job.", "inputSchema": {"type": "object", "properties": {"jobId": {"type": "string"}}, "required": ["jobId"]}},
        {
            "name": "get_next_mapping_item",
            "description": "Return next pending mapping item.",
            "inputSchema": {
                "type": "object",
                "properties": {"jobId": {"type": "string"}, "includeMechanicalMarkdown": {"type": "boolean"}},
                "required": ["jobId"],
            },
        },
        {
            "name": "get_mapping_item",
            "description": "Return a specific mapping item by id, name, title, query, document path, or index.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "jobId": {"type": "string"},
                    "entryPointId": {"type": "string"},
                    "entryPointName": {"type": "string"},
                    "title": {"type": "string"},
                    "query": {"type": "string"},
                    "documentPath": {"type": "string"},
                    "index": {"type": "integer"},
                    "includeMechanicalMarkdown": {"type": "boolean"},
                },
                "required": ["jobId"],
            },
        },
        {
            "name": "mark_mapping_item_mapped",
            "description": "Mark a mapping item as documented.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "jobId": {"type": "string"},
                    "entryPointId": {"type": "string"},
                    "markdown": {"type": "string"},
                    "title": {"type": "string"},
                    "notes": {"type": "string"},
                    "finalDocPath": {"type": "string"},
                },
                "required": ["jobId", "entryPointId"],
            },
        },
    ]

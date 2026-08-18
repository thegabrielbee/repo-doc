from __future__ import annotations

import json
import sys
import uuid
from typing import Any

from .pipeline import PipelineError, run_scan

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
    if method in {"start_mapping", "get_mapping_status", "get_mapping_result", "get_next_mapping_item", "mark_mapping_item_mapped"}:
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
        return {"item": None, "status": "empty"}
    if name == "mark_mapping_item_mapped":
        return {"status": "mapped", "entryPointId": args.get("entryPointId")}
    raise ValueError(f"unknown tool: {name}")


def lookup_job(args: dict[str, Any]) -> dict[str, Any]:
    job_id = args.get("jobId")
    if not job_id or job_id not in JOBS:
        raise ValueError("unknown jobId")
    return JOBS[job_id]


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
        {"name": "get_next_mapping_item", "description": "Return next pending mapping item.", "inputSchema": {"type": "object", "properties": {"jobId": {"type": "string"}}, "required": ["jobId"]}},
        {"name": "mark_mapping_item_mapped", "description": "Mark a mapping item as documented.", "inputSchema": {"type": "object", "properties": {"jobId": {"type": "string"}, "entryPointId": {"type": "string"}}, "required": ["jobId", "entryPointId"]}},
    ]

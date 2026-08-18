from __future__ import annotations

import re
from pathlib import Path
from typing import Any

from .utils import clean_literal, line_number_at, read_text, split_top_level, stable_id

TYPE_RE = re.compile(
    r"(?P<prefix>(?:@\w[\w.]*(?:\s*\([^@{};]*?\))?\s*)*)"
    r"(?P<mods>(?:(?:public|protected|private|abstract|final|static|strictfp)\s+)*)"
    r"(?P<kind>class|interface|enum|record)\s+"
    r"(?P<name>[A-Za-z_$][\w$]*)"
    r"(?P<tail>[^{;]*)\{",
    re.S,
)

METHOD_RE = re.compile(
    r"(?P<prefix>(?:@\w[\w.]*(?:\s*\([^@{};]*?\))?\s*)*)"
    r"(?P<mods>(?:(?:public|protected|private|static|final|abstract|synchronized|native|default|strictfp)\s+)*)"
    r"(?:(?:<[^;{}()]+>)\s*)?"
    r"(?P<return>[A-Za-z_$][\w$<>\[\].?,\s]*?)\s+"
    r"(?P<name>[A-Za-z_$][\w$]*)\s*"
    r"\((?P<params>[^{};]*)\)\s*"
    r"(?P<throws>throws\s+[^{;]+)?"
    r"(?P<body>[{;])",
    re.S,
)

ANNOTATION_RE = re.compile(r"@(?P<name>[A-Za-z_$][\w$.]*)(?:\s*\((?P<args>.*?)\))?", re.S)
CALL_RE = re.compile(r"(?<![\w$])(?P<target>[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)?)\s*\(")

CONTROL_WORDS = {
    "catch",
    "for",
    "if",
    "return",
    "switch",
    "synchronized",
    "try",
    "while",
}


def parse_project_sources(project: dict[str, Any]) -> None:
    types: list[dict[str, Any]] = []
    for source_file in project.get("sourceFiles", []):
        types.extend(parse_java_file(Path(source_file["path"]), source_file["moduleId"], source_file["moduleName"]))
    project["types"] = sorted(types, key=lambda item: (item.get("file", ""), item.get("line", 0), item.get("name", "")))


def parse_java_file(path: Path, module_id: str, module_name: str) -> list[dict[str, Any]]:
    text = read_text(path)
    package = match_text(r"\bpackage\s+([\w.]+)\s*;", text)
    imports = re.findall(r"\bimport\s+(?:static\s+)?([\w.*]+)\s*;", text)
    result: list[dict[str, Any]] = []
    for match in TYPE_RE.finditer(text):
        name = match.group("name")
        body_start = match.end() - 1
        body_end = find_matching_brace(text, body_start)
        if body_end <= body_start:
            continue
        tail = match.group("tail") or ""
        line = line_number_at(text, match.start("name"))
        fqn = f"{package}.{name}" if package else name
        type_id = stable_id("type", path, fqn, line)
        annotations = parse_annotations(match.group("prefix") or annotation_segment(text, match.start()), text, path, match.start("prefix"))
        body = text[body_start + 1 : body_end]
        result.append(
            {
                "id": type_id,
                "name": name,
                "package": package,
                "qualifiedName": fqn,
                "kind": match.group("kind"),
                "file": str(path),
                "line": line,
                "moduleId": module_id,
                "moduleName": module_name,
                "annotations": annotations,
                "imports": imports,
                "extends": parse_extends(tail),
                "implements": parse_implements(tail),
                "fields": parse_fields(body, text, body_start + 1, type_id, fqn, path),
                "methods": parse_methods(body, text, body_start + 1, type_id, fqn, path),
            }
        )
    return result


def parse_annotations(segment: str, whole_text: str, path: Path, base_index: int | None = None) -> list[dict[str, Any]]:
    annotations: list[dict[str, Any]] = []
    base = whole_text.find(segment) if base_index is None else base_index
    if base < 0:
        base = 0
    for match in ANNOTATION_RE.finditer(segment):
        name = match.group("name").split(".")[-1]
        args = (match.group("args") or "").strip()
        annotations.append(
            {
                "name": name,
                "qualifiedName": match.group("name"),
                "raw": args,
                "values": parse_annotation_values(args),
                "line": line_number_at(whole_text, base + match.start()),
                "path": str(path),
            }
        )
    return annotations


def parse_annotation_values(args: str) -> dict[str, str]:
    values: dict[str, str] = {}
    if not args:
        return values
    parts = split_top_level(args)
    if len(parts) == 1 and "=" not in parts[0]:
        values["value"] = clean_literal(parts[0])
        return values
    for part in parts:
        if "=" not in part:
            continue
        key, value = part.split("=", 1)
        values[key.strip()] = clean_literal(value)
    return values


def annotation_segment(text: str, position: int, *, stop_at_open_brace: bool = False) -> str:
    stops = [text.rfind("\n\n", 0, position), text.rfind(";", 0, position)]
    block_closings = list(re.finditer(r"(?m)^\s*}\s*$", text[:position]))
    if block_closings:
        stops.append(block_closings[-1].end())
    if stop_at_open_brace:
        block_openings = list(re.finditer(r"\{\s*\n", text[:position]))
        if block_openings:
            stops.append(block_openings[-1].start())
    start = max(stops)
    segment = text[start + 1 : position]
    if len(segment) > 2000:
        segment = segment[-2000:]
    return segment


def parse_methods(body: str, whole_text: str, body_absolute_start: int, type_id: str, fqn: str, path: Path) -> list[dict[str, Any]]:
    methods: list[dict[str, Any]] = []
    for match in METHOD_RE.finditer(body):
        name = match.group("name")
        if name in CONTROL_WORDS:
            continue
        absolute = body_absolute_start + match.start()
        prefix = match.group("prefix") or annotation_segment(whole_text, absolute, stop_at_open_brace=True)
        base = body_absolute_start + match.start("prefix") if match.group("prefix") else None
        annotations = parse_annotations(prefix, whole_text, path, base)
        mods = (match.group("mods") or "").split()
        body_token = match.group("body")
        method_body = ""
        end_line = 0
        if body_token == "{":
            opening = body_absolute_start + match.end("body") - 1
            closing = find_matching_brace(whole_text, opening)
            if closing > opening:
                method_body = whole_text[opening + 1 : closing]
                end_line = line_number_at(whole_text, closing)
        line = line_number_at(whole_text, absolute + match.start("name"))
        method_id = stable_id("method", path, fqn, name, line)
        methods.append(
            {
                "id": method_id,
                "name": name,
                "qualifiedName": f"{fqn}.{name}",
                "returnType": clean_type(match.group("return")),
                "modifiers": mods,
                "parameters": parse_parameters(match.group("params"), whole_text, body_absolute_start + match.start("params"), path),
                "annotations": annotations,
                "calls": parse_calls(method_body, whole_text, body_absolute_start + match.end(), path),
                "conditions": parse_conditions(method_body, whole_text, body_absolute_start + match.end(), path),
                "line": line,
                "endLine": end_line,
                "classId": type_id,
                "file": str(path),
            }
        )
    return methods


def parse_fields(body: str, whole_text: str, body_absolute_start: int, type_id: str, fqn: str, path: Path) -> list[dict[str, Any]]:
    fields: list[dict[str, Any]] = []
    scrubbed = strip_method_bodies(body)
    field_re = re.compile(
        r"(?P<prefix>(?:@\w[\w.]*(?:\s*\([^;{}]*?\))?\s*)*)"
        r"(?P<mods>(?:(?:public|protected|private|static|final|volatile|transient)\s+)*)"
        r"(?P<type>[A-Za-z_$][\w$<>\[\].?,\s]*?)\s+"
        r"(?P<name>[A-Za-z_$][\w$]*)\s*(?:=[^;]*)?;",
        re.S,
    )
    for match in field_re.finditer(scrubbed):
        name = match.group("name")
        if name in CONTROL_WORDS:
            continue
        absolute = body_absolute_start + match.start("name")
        line = line_number_at(whole_text, absolute)
        fields.append(
            {
                "id": stable_id("field", path, fqn, name, line),
                "name": name,
                "type": clean_type(match.group("type")),
                "annotations": parse_annotations(
                    match.group("prefix") or annotation_segment(whole_text, body_absolute_start + match.start(), stop_at_open_brace=True),
                    whole_text,
                    path,
                    body_absolute_start + match.start("prefix") if match.group("prefix") else None,
                ),
                "line": line,
                "classId": type_id,
                "file": str(path),
            }
        )
    return fields


def parse_parameters(params: str, whole_text: str, absolute_start: int, path: Path) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for part in split_top_level(params):
        if not part:
            continue
        annotations = parse_annotations(part, whole_text, path)
        cleaned = ANNOTATION_RE.sub("", part).replace("final ", "").strip()
        tokens = cleaned.split()
        if not tokens:
            continue
        name = tokens[-1].replace("...", "[]")
        param_type = clean_type(" ".join(tokens[:-1]))
        result.append({"name": name, "type": param_type, "annotations": annotations})
    return result


def parse_calls(method_body: str, whole_text: str, absolute_start: int, path: Path) -> list[dict[str, Any]]:
    calls: list[dict[str, Any]] = []
    seen: set[tuple[str, int]] = set()
    for match in CALL_RE.finditer(method_body):
        target = match.group("target")
        name = target.split(".")[-1]
        if name in CONTROL_WORDS or name in {"new", "super", "this"}:
            continue
        line = line_number_at(whole_text, absolute_start + match.start())
        key = (target, line)
        if key in seen:
            continue
        seen.add(key)
        calls.append({"target": target, "name": name, "line": line, "file": str(path)})
    return calls


def parse_conditions(method_body: str, whole_text: str, absolute_start: int, path: Path) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for match in re.finditer(r"\b(if|else\s+if|switch|catch|for|while)\s*\((.*?)\)", method_body, re.S):
        result.append(
            {
                "kind": re.sub(r"\s+", " ", match.group(1)),
                "expression": re.sub(r"\s+", " ", match.group(2)).strip(),
                "line": line_number_at(whole_text, absolute_start + match.start()),
                "file": str(path),
            }
        )
    return result


def find_matching_brace(text: str, opening_index: int) -> int:
    if opening_index < 0 or opening_index >= len(text) or text[opening_index] != "{":
        return -1
    depth = 0
    quote = ""
    escape = False
    in_line_comment = False
    in_block_comment = False
    index = opening_index
    while index < len(text):
        char = text[index]
        next_char = text[index + 1] if index + 1 < len(text) else ""
        if in_line_comment:
            if char == "\n":
                in_line_comment = False
            index += 1
            continue
        if in_block_comment:
            if char == "*" and next_char == "/":
                in_block_comment = False
                index += 2
                continue
            index += 1
            continue
        if quote:
            if escape:
                escape = False
            elif char == "\\":
                escape = True
            elif char == quote:
                quote = ""
            index += 1
            continue
        if char == "/" and next_char == "/":
            in_line_comment = True
            index += 2
            continue
        if char == "/" and next_char == "*":
            in_block_comment = True
            index += 2
            continue
        if char in {"'", '"'}:
            quote = char
            index += 1
            continue
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return index
        index += 1
    return -1


def strip_method_bodies(body: str) -> str:
    chars = list(body)
    for match in METHOD_RE.finditer(body):
        if match.group("body") != "{":
            continue
        opening = match.end("body") - 1
        closing = find_matching_brace(body, opening)
        if closing > opening:
            for index in range(match.start(), closing + 1):
                chars[index] = "\n" if chars[index] == "\n" else " "
    return "".join(chars)


def parse_extends(tail: str) -> str:
    match = re.search(r"\bextends\s+([A-Za-z_$][\w$.]*)", tail)
    return match.group(1) if match else ""


def parse_implements(tail: str) -> list[str]:
    match = re.search(r"\bimplements\s+([^{]+)", tail)
    if not match:
        return []
    return [part.strip().split("<", 1)[0] for part in match.group(1).split(",") if part.strip()]


def match_text(pattern: str, text: str) -> str:
    match = re.search(pattern, text)
    return match.group(1).strip() if match else ""


def clean_type(value: str) -> str:
    return re.sub(r"\s+", " ", value or "").strip()


def annotations_by_name(item: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {annotation["name"]: annotation for annotation in item.get("annotations", [])}


def has_annotation(item: dict[str, Any], *names: str) -> bool:
    annotation_names = {annotation["name"] for annotation in item.get("annotations", [])}
    return any(name in annotation_names for name in names)


def annotation(item: dict[str, Any], *names: str) -> dict[str, Any] | None:
    lookup = annotations_by_name(item)
    for name in names:
        if name in lookup:
            return lookup[name]
    return None

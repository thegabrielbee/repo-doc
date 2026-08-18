from __future__ import annotations

import hashlib
import re
from pathlib import Path
from typing import Any, Iterable

JAVA_VERSION_UNKNOWN = "unknown"


def stable_id(*parts: object) -> str:
    raw = ":".join(str(part) for part in parts if part is not None and str(part) != "")
    folded = re.sub(r"[^A-Za-z0-9]+", "-", raw.lower()).strip("-")
    if not folded:
        folded = "item"
    if len(folded) <= 140:
        return folded
    digest = hashlib.sha1(raw.encode("utf-8", "ignore")).hexdigest()[:12]
    return f"{folded[:120].rstrip('-')}-{digest}"


def normalize_java_version(value: object | None) -> str:
    if value is None:
        return JAVA_VERSION_UNKNOWN
    text = str(value).strip().strip('"').strip("'")
    if not text:
        return JAVA_VERSION_UNKNOWN
    text = text.replace("JavaVersion.VERSION_", "").replace("VERSION_", "")
    text = text.replace("java-", "").replace("Java ", "").replace("_", ".")
    if text in {"1.8", "1.8.0"}:
        return "8"
    match = re.search(r"(\d+)", text)
    if not match:
        return JAVA_VERSION_UNKNOWN
    number = match.group(1)
    if number == "1" and "8" in text:
        return "8"
    return number


def is_empty(value: Any) -> bool:
    return value is None or value == "" or value == [] or value == {}


def compact(value: Any) -> Any:
    if isinstance(value, dict):
        result: dict[str, Any] = {}
        for key, item in value.items():
            clean = compact(item)
            if not is_empty(clean):
                result[key] = clean
        return result
    if isinstance(value, list):
        return [compact(item) for item in value if not is_empty(compact(item))]
    return value


def unique_by_id(items: Iterable[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[str] = set()
    result: list[dict[str, Any]] = []
    for item in items:
        item_id = str(item.get("id", ""))
        if item_id in seen:
            continue
        seen.add(item_id)
        result.append(item)
    return result


def read_text(path: Path) -> str:
    for encoding in ("utf-8-sig", "utf-8", "latin-1"):
        try:
            return path.read_text(encoding=encoding)
        except UnicodeDecodeError:
            continue
    return path.read_text(errors="ignore")


def rel_path(path: Path, root: Path) -> str:
    try:
        return path.resolve().relative_to(root.resolve()).as_posix()
    except ValueError:
        return path.resolve().as_posix()


def line_number_at(text: str, index: int) -> int:
    return text.count("\n", 0, max(index, 0)) + 1


def split_top_level(text: str, delimiter: str = ",") -> list[str]:
    parts: list[str] = []
    start = 0
    depth = 0
    quote = ""
    escape = False
    for index, char in enumerate(text):
        if escape:
            escape = False
            continue
        if quote:
            if char == "\\":
                escape = True
            elif char == quote:
                quote = ""
            continue
        if char in {"'", '"'}:
            quote = char
            continue
        if char in "({[":
            depth += 1
            continue
        if char in ")}]":
            depth = max(0, depth - 1)
            continue
        if char == delimiter and depth == 0:
            parts.append(text[start:index].strip())
            start = index + 1
    tail = text[start:].strip()
    if tail:
        parts.append(tail)
    return parts


def clean_literal(value: object | None) -> str:
    if value is None:
        return ""
    text = str(value).strip()
    if text.startswith("{") and text.endswith("}"):
        text = text[1:-1].strip()
    return text.strip('"').strip("'")


def first_value(values: dict[str, str], *keys: str) -> str:
    for key in keys:
        if key in values and clean_literal(values[key]):
            return clean_literal(values[key])
    return ""


def join_paths(*parts: str) -> str:
    cleaned: list[str] = []
    absolute = False
    for part in parts:
        if not part:
            continue
        for chunk in str(part).split(","):
            chunk = clean_literal(chunk).strip()
            if not chunk:
                continue
            absolute = absolute or chunk.startswith("/")
            cleaned.append(chunk.strip("/"))
    if not cleaned:
        return ""
    path = "/".join(item for item in cleaned if item)
    if absolute:
        path = "/" + path
    return re.sub(r"/+", "/", path) or "/"


def string_list(value: object | None) -> list[str]:
    text = clean_literal(value)
    if not text:
        return []
    return [clean_literal(part) for part in split_top_level(text) if clean_literal(part)]


def redact_if_sensitive(value: str) -> str:
    lowered = value.lower()
    if any(token in lowered for token in ("password", "secret", "token", "credential", "passwd")):
        return "redacted"
    if "${" in value or "$(" in value:
        return "defined_externally"
    return value

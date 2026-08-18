from __future__ import annotations

import re
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any

from .model import refresh_summary
from .utils import JAVA_VERSION_UNKNOWN, normalize_java_version, read_text, rel_path, stable_id, unique_by_id

SKIP_DIRS = {
    ".git",
    ".gradle",
    ".idea",
    ".mvn",
    ".vscode",
    "__pycache__",
    "bin",
    "build",
    "dist",
    "node_modules",
    "out",
    "target",
}

DESCRIPTOR_NAMES = {
    "application.xml",
    "beans.xml",
    "ejb-jar.xml",
    "faces-config.xml",
    "jboss-deployment-structure.xml",
    "jboss-ejb3.xml",
    "jboss-web.xml",
    "persistence.xml",
    "web-fragment.xml",
    "web.xml",
    "webservices.xml",
}

UI_SUFFIXES = {".xhtml", ".jsp", ".html", ".htm"}


def discover_project(
    root: str | Path,
    *,
    addons: list[str],
    java_version: str | None = None,
    include_tests: bool = False,
) -> dict[str, Any]:
    root_path = Path(root).resolve()
    if not root_path.exists() or not root_path.is_dir():
        raise ValueError(f"root path not found or not a directory: {root_path}")

    module_roots = discover_module_roots(root_path)
    modules = [
        discover_module(root_path, module_root, module_roots, java_version=java_version, include_tests=include_tests)
        for module_root in module_roots
    ]
    source_files = []
    for module in modules:
        source_files.extend(module["sourceFiles"])

    module_versions = {module.get("javaVersion", JAVA_VERSION_UNKNOWN) for module in modules}
    if java_version:
        project_java_version = normalize_java_version(java_version)
    elif len(module_versions) == 1:
        project_java_version = next(iter(module_versions))
    elif module_versions:
        project_java_version = "mixed"
    else:
        project_java_version = JAVA_VERSION_UNKNOWN

    project = {
        "id": stable_id("project", root_path),
        "name": root_path.name,
        "rootPath": str(root_path),
        "addons": addons,
        "javaVersion": project_java_version,
        "modules": modules,
        "sourceFiles": source_files,
        "types": [],
        "entryPoints": [],
        "dependencies": [],
        "graph": {"nodes": [], "edges": []},
    }
    refresh_summary(project)
    return project


def discover_module_roots(root: Path) -> list[Path]:
    candidates: list[Path] = []
    for path in walk_files(root):
        if path.name == "pom.xml" or path.name in {"build.gradle", "build.gradle.kts"}:
            candidates.append(path.parent.resolve())
    if not candidates:
        return [root.resolve()]
    candidates = sorted(set(candidates), key=lambda item: (len(item.parts), str(item).lower()))
    selected: list[Path] = []
    for candidate in candidates:
        if any(candidate == parent for parent in selected):
            continue
        selected.append(candidate)
    return selected


def discover_module(root: Path, module_root: Path, all_module_roots: list[Path], *, java_version: str | None, include_tests: bool) -> dict[str, Any]:
    build_file = find_build_file(module_root)
    build_tool = build_tool_for(build_file)
    build_text = read_text(build_file) if build_file else ""
    packaging = infer_packaging(build_file, build_text, module_root)
    inferred_java = normalize_java_version(java_version) if java_version else infer_java_version(module_root, root)

    module_id = stable_id("module", rel_path(module_root, root) or module_root.name)
    source_files: list[dict[str, Any]] = []
    config_files: list[str] = []
    descriptor_files: list[str] = []
    ui_files: list[str] = []
    migrations: list[str] = []

    for path in walk_files(module_root):
        if belongs_to_nested_module(path, module_root, all_module_roots):
            continue
        if not include_tests and is_test_path(path, module_root):
            continue
        relative = rel_path(path, root)
        suffix = path.suffix.lower()
        if suffix == ".java":
            source_files.append({"path": str(path), "relativePath": relative, "moduleId": module_id, "moduleName": module_root.name})
        elif suffix in {".properties", ".yml", ".yaml", ".xml", ".wsdl"}:
            config_files.append(str(path))
            if path.name in DESCRIPTOR_NAMES or suffix == ".wsdl":
                descriptor_files.append(str(path))
            if is_migration(path):
                migrations.append(str(path))
        elif suffix in UI_SUFFIXES:
            ui_files.append(str(path))

    return {
        "id": module_id,
        "name": module_root.name,
        "rootPath": str(module_root),
        "relativePath": rel_path(module_root, root),
        "buildTool": build_tool,
        "buildFile": str(build_file) if build_file else "",
        "packaging": packaging,
        "javaVersion": inferred_java,
        "sourceFiles": sorted(source_files, key=lambda item: item["path"].lower()),
        "configFiles": sorted(config_files, key=str.lower),
        "descriptorFiles": sorted(set(descriptor_files), key=str.lower),
        "uiFiles": sorted(set(ui_files), key=str.lower),
        "migrations": sorted(set(migrations), key=str.lower),
    }


def belongs_to_nested_module(path: Path, module_root: Path, all_module_roots: list[Path]) -> bool:
    resolved = path.resolve()
    for other in all_module_roots:
        other = other.resolve()
        if other == module_root.resolve():
            continue
        try:
            other.relative_to(module_root.resolve())
            resolved.relative_to(other)
            return True
        except ValueError:
            continue
    return False


def walk_files(root: Path) -> list[Path]:
    result: list[Path] = []
    stack = [root]
    while stack:
        current = stack.pop()
        try:
            children = sorted(current.iterdir(), key=lambda item: item.name.lower())
        except OSError:
            continue
        for child in children:
            if child.is_dir():
                if child.name in SKIP_DIRS:
                    continue
                stack.append(child)
            elif child.is_file():
                result.append(child)
    return result


def is_test_path(path: Path, module_root: Path) -> bool:
    relative = rel_path(path, module_root).lower()
    wrapped = f"/{relative}/"
    return (
        relative.startswith("src/test/")
        or "/src/test/" in wrapped
        or "/test/" in wrapped
        or "/tests/" in wrapped
    )


def find_build_file(module_root: Path) -> Path | None:
    for name in ("pom.xml", "build.gradle", "build.gradle.kts"):
        path = module_root / name
        if path.exists():
            return path
    return None


def build_tool_for(path: Path | None) -> str:
    if path is None:
        return "unknown"
    if path.name == "pom.xml":
        return "maven"
    if path.name.startswith("build.gradle"):
        return "gradle"
    return "unknown"


def infer_packaging(build_file: Path | None, build_text: str, module_root: Path) -> str:
    if build_file and build_file.name == "pom.xml":
        try:
            root = ET.fromstring(build_text)
            packaging = find_xml_text(root, "packaging")
            if packaging:
                return packaging
        except ET.ParseError:
            match = re.search(r"<packaging>\s*([^<]+)\s*</packaging>", build_text)
            if match:
                return match.group(1).strip()
        return "jar"
    if build_file and build_file.name.startswith("build.gradle"):
        lowered = build_text.lower()
        if re.search(r"\bwar\b", lowered):
            return "war"
        if re.search(r"\bear\b", lowered):
            return "ear"
        return "jar"
    if (module_root / "src" / "main" / "webapp").exists():
        return "war"
    return "unknown"


def infer_java_version(module_root: Path, repo_root: Path) -> str:
    current = module_root
    while True:
        for name in ("pom.xml", "build.gradle", "build.gradle.kts"):
            candidate = current / name
            if candidate.exists():
                version = infer_java_version_from_build(candidate)
                if version != JAVA_VERSION_UNKNOWN:
                    return version
        if current == repo_root or current.parent == current:
            break
        current = current.parent
    return JAVA_VERSION_UNKNOWN


def infer_java_version_from_build(path: Path) -> str:
    text = read_text(path)
    if path.name == "pom.xml":
        for pattern in (
            r"<maven\.compiler\.release>\s*([^<]+)\s*</maven\.compiler\.release>",
            r"<maven\.compiler\.source>\s*([^<]+)\s*</maven\.compiler\.source>",
            r"<java\.version>\s*([^<]+)\s*</java\.version>",
            r"<source>\s*([^<]+)\s*</source>",
            r"<release>\s*([^<]+)\s*</release>",
        ):
            match = re.search(pattern, text)
            if match:
                return normalize_java_version(match.group(1))
    for pattern in (
        r"sourceCompatibility\s*=\s*['\"]?([^'\"\s]+)",
        r"targetCompatibility\s*=\s*['\"]?([^'\"\s]+)",
        r"JavaVersion\.VERSION_([A-Za-z0-9_]+)",
        r"languageVersion\s*=\s*JavaLanguageVersion\.of\((\d+)\)",
    ):
        match = re.search(pattern, text)
        if match:
            return normalize_java_version(match.group(1))
    return JAVA_VERSION_UNKNOWN


def find_xml_text(root: ET.Element, local_name: str) -> str:
    for element in root.iter():
        if element.tag.split("}")[-1] == local_name and element.text:
            return element.text.strip()
    return ""


def is_migration(path: Path) -> bool:
    lowered = path.as_posix().lower()
    name = path.name.lower()
    return "db/migration" in lowered or (name.startswith(("v", "r")) and "__" in name and path.suffix.lower() in {".sql", ".xml"})


def module_for_path(project: dict[str, Any], path: str | Path) -> dict[str, Any]:
    resolved = Path(path).resolve()
    modules = sorted(project.get("modules", []), key=lambda item: len(str(item.get("rootPath", ""))), reverse=True)
    for module in modules:
        module_root = Path(module["rootPath"]).resolve()
        try:
            resolved.relative_to(module_root)
            return module
        except ValueError:
            continue
    return project.get("modules", [{}])[0]


def dedupe_project(project: dict[str, Any]) -> None:
    project["entryPoints"] = sorted(
        unique_by_id(project.get("entryPoints", [])),
        key=lambda item: (item.get("framework", ""), item.get("kind", ""), item.get("name", "")),
    )
    project["dependencies"] = sorted(
        unique_by_id(project.get("dependencies", [])),
        key=lambda item: (item.get("kind", ""), item.get("name", "")),
    )

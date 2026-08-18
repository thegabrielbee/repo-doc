from __future__ import annotations

import re
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any

from .discovery import module_for_path
from .java_parser import annotation, has_annotation
from .model import dependency, entrypoint, evidence
from .utils import clean_literal, first_value, join_paths, read_text, redact_if_sensitive, stable_id, string_list

SUPPORTED_ADDONS = {"spring", "javaee"}

HTTP_METHOD_ANNOTATIONS = {
    "GET": "GET",
    "POST": "POST",
    "PUT": "PUT",
    "DELETE": "DELETE",
    "PATCH": "PATCH",
    "HEAD": "HEAD",
    "OPTIONS": "OPTIONS",
}

SPRING_HTTP_ANNOTATIONS = {
    "GetMapping": "GET",
    "PostMapping": "POST",
    "PutMapping": "PUT",
    "DeleteMapping": "DELETE",
    "PatchMapping": "PATCH",
    "RequestMapping": "",
}

CDI_EXTENSION_EVENT_TYPES = {
    "BeforeBeanDiscovery",
    "AfterBeanDiscovery",
    "AfterDeploymentValidation",
    "BeforeShutdown",
    "ProcessAnnotatedType",
    "ProcessBean",
    "ProcessBeanAttributes",
    "ProcessInjectionPoint",
    "ProcessInjectionTarget",
    "ProcessManagedBean",
    "ProcessObserverMethod",
    "ProcessProducer",
    "ProcessProducerField",
    "ProcessProducerMethod",
    "ProcessSessionBean",
    "ProcessSyntheticAnnotatedType",
    "ProcessSyntheticBean",
    "ProcessSyntheticObserverMethod",
}


def resolve_addons(raw: str | list[str] | None) -> list[str]:
    if raw is None or raw == "":
        return ["spring"]
    if isinstance(raw, str):
        names = [part.strip().lower() for part in raw.split(",") if part.strip()]
    else:
        names = [str(part).strip().lower() for part in raw if str(part).strip()]
    if not names:
        return ["spring"]
    unknown = [name for name in names if name not in SUPPORTED_ADDONS]
    if unknown:
        raise ValueError(f"unknown addon(s): {', '.join(unknown)}")
    result: list[str] = []
    for name in names:
        if name not in result:
            result.append(name)
    return result


def apply_addons(project: dict[str, Any], addons: list[str]) -> None:
    if "spring" in addons:
        detect_spring(project)
    if "javaee" in addons:
        detect_javaee(project)


def detect_spring(project: dict[str, Any]) -> None:
    for type_item in project.get("types", []):
        type_path = first_annotation_value(type_item, "RequestMapping")
        is_controller = has_annotation(type_item, "RestController", "Controller") or bool(type_path)
        for method in type_item.get("methods", []):
            method_ann, http_method = first_spring_http_annotation(method)
            if is_controller and method_ann:
                path = join_paths(type_path, path_from_annotation(method_ann))
                project["entryPoints"].append(
                    entrypoint(
                        framework="spring",
                        kind="http",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=path,
                        http_method=http_method or method_from_request_mapping(method_ann),
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", method_ann["name"]),
                    )
                )
            if has_annotation(method, "Scheduled"):
                project["entryPoints"].append(
                    entrypoint(
                        framework="spring",
                        kind="scheduler",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", "Scheduled"),
                    )
                )
            listener = annotation(method, "KafkaListener", "RabbitListener", "JmsListener")
            if listener:
                project["entryPoints"].append(
                    entrypoint(
                        framework="spring",
                        kind="message_listener",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=first_value(listener.get("values", {}), "topics", "queues", "destination", "value"),
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", listener["name"]),
                    )
                )
            if has_annotation(method, "EventListener"):
                project["entryPoints"].append(
                    entrypoint(
                        framework="spring",
                        kind="event_listener",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", "EventListener"),
                    )
                )
    detect_shared_dependencies(project, "spring")


def detect_javaee(project: dict[str, Any]) -> None:
    named_beans = named_bean_index(project)
    filters = []
    for type_item in project.get("types", []):
        type_path = first_annotation_value(type_item, "Path")
        managed = is_container_managed(type_item)

        servlet = annotation(type_item, "WebServlet")
        if servlet:
            paths = web_paths(servlet)
            project["entryPoints"].append(
                entrypoint(
                    framework="javaee",
                    kind="servlet",
                    name=type_item["name"],
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    path=", ".join(paths),
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "annotation", "WebServlet"),
                )
            )

        filter_annotation = annotation(type_item, "WebFilter")
        if filter_annotation:
            filters.append(
                dependency(
                    kind="http_filter",
                    name=type_item["name"],
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    detail=", ".join(web_paths(filter_annotation)),
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "annotation", "WebFilter"),
                    confidence="high",
                )
            )

        if has_annotation(type_item, "WebListener"):
            project["entryPoints"].append(
                entrypoint(
                    framework="javaee",
                    kind="listener",
                    name=type_item["name"],
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "annotation", "WebListener"),
                )
            )

        if has_annotation(type_item, "Startup") and not any(has_annotation(method, "PostConstruct") for method in type_item.get("methods", [])):
            project["entryPoints"].append(
                entrypoint(
                    framework="javaee",
                    kind="startup",
                    name=type_item["name"],
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "annotation", "Startup"),
                )
            )

        message_driven = annotation(type_item, "MessageDriven")
        server_endpoint = annotation(type_item, "ServerEndpoint")
        web_service = annotation(type_item, "WebService")

        for method in type_item.get("methods", []):
            http_annotation, http_method = first_javaee_http_annotation(method)
            if type_path and http_annotation:
                path = join_paths(type_path, path_from_annotation(annotation(method, "Path") or {}))
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="http",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=path,
                        http_method=http_method,
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", http_annotation["name"]),
                    )
                )

            web_method = annotation(method, "WebMethod")
            if web_service and should_map_web_method(method, web_method):
                operation = first_value((web_method or {}).get("values", {}), "operationName", "value") or method["name"]
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="soap",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=operation,
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", (web_method or web_service)["name"]),
                    )
                )

            scheduler_annotation = annotation(method, "Schedule", "Schedules", "Timeout")
            if scheduler_annotation:
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="scheduler",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=cronish_detail(scheduler_annotation),
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", scheduler_annotation["name"]),
                    )
                )

            if managed and has_annotation(method, "PostConstruct"):
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="startup",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", "PostConstruct"),
                    )
                )

            if message_driven and method["name"] == "onMessage":
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="message_listener",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=message_destination(message_driven),
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", "MessageDriven"),
                    )
                )

            websocket_annotation = annotation(method, "OnOpen", "OnMessage", "OnClose", "OnError")
            if server_endpoint and websocket_annotation:
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="websocket",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        path=path_from_annotation(server_endpoint),
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", websocket_annotation["name"]),
                    )
                )

            event_annotation = cdi_observer_annotation(method)
            if event_annotation and not observes_cdi_extension_event(method):
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="event_listener",
                        name=f"{type_item['name']}.{method['name']}",
                        module_id=type_item["moduleId"],
                        module_name=type_item["moduleName"],
                        class_id=type_item["id"],
                        method_id=method["id"],
                        evidence_item=evidence(method["file"], method["line"], method["qualifiedName"], "annotation", event_annotation),
                    )
                )

    project["dependencies"].extend(filters)
    detect_javaee_descriptors(project)
    detect_ui_entrypoints(project, named_beans)
    detect_shared_dependencies(project, "javaee")


def detect_shared_dependencies(project: dict[str, Any], framework: str) -> None:
    for type_item in project.get("types", []):
        if has_annotation(type_item, "Entity"):
            table = annotation(type_item, "Table")
            table_name = first_value((table or {}).get("values", {}), "name", "value") or type_item["name"]
            project["dependencies"].append(
                dependency(
                    kind="table",
                    name=table_name,
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    detail=type_item.get("qualifiedName", type_item["name"]),
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "annotation", "Entity"),
                    confidence="high",
                )
            )
        if any(name.endswith("Repository") for name in [type_item["name"], type_item.get("extends", "")]) or "Repository" in " ".join(type_item.get("implements", [])):
            project["dependencies"].append(
                dependency(
                    kind="database_repository",
                    name=type_item["name"],
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "type"),
                )
            )
        if "LoginModule" in type_item.get("implements", []) or type_item.get("extends", "").endswith("LoginModule"):
            project["dependencies"].append(
                dependency(
                    kind="auth_provider",
                    name=type_item["name"],
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    detail="JAAS LoginModule",
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "type"),
                    confidence="high",
                )
            )
        imports = " ".join(type_item.get("imports", []))
        if "EntityManager" in imports or any(field.get("type", "").endswith("EntityManager") for field in type_item.get("fields", [])):
            project["dependencies"].append(
                dependency(
                    kind="database_client",
                    name=f"{type_item['name']} EntityManager",
                    module_id=type_item["moduleId"],
                    module_name=type_item["moduleName"],
                    detail="JPA EntityManager",
                    class_id=type_item["id"],
                    evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "import"),
                )
            )
        if "javax.mail" in imports or "jakarta.mail" in imports:
            project["dependencies"].append(simple_class_dep(type_item, "mail_server", "JavaMail SMTP", "JavaMail"))
        if "Jedis" in imports or "redis" in imports.lower():
            project["dependencies"].append(simple_class_dep(type_item, "cache", "Redis/Jedis", "Redis"))
        if "AmazonS3" in imports or "S3Client" in imports:
            project["dependencies"].append(simple_class_dep(type_item, "s3", "AWS S3", "AWS S3"))
        if "AmazonSQS" in imports or "SqsClient" in imports:
            project["dependencies"].append(simple_class_dep(type_item, "queue", "AWS SQS", "AWS SQS"))
        if "FTPClient" in imports or "ChannelSftp" in imports:
            project["dependencies"].append(simple_class_dep(type_item, "ftp_endpoint", "FTP/SFTP", "FTP/SFTP"))

        for method in type_item.get("methods", []):
            for call in method.get("calls", []):
                target = call.get("target", "")
                lowered = target.lower()
                if any(token in lowered for token in ("persist", "merge", "remove", "find", "save", "delete")):
                    project["dependencies"].append(
                        dependency(
                            kind="repository_call",
                            name=target,
                            module_id=type_item["moduleId"],
                            module_name=type_item["moduleName"],
                            class_id=type_item["id"],
                            method_id=method["id"],
                            evidence_item=evidence(call["file"], call["line"], method["qualifiedName"], "call"),
                        )
                    )
                if any(token in target for token in ("getForObject", "getForEntity", "postForObject", "postForEntity", "execute", "exchange", "openConnection", "newCall")):
                    project["dependencies"].append(
                        dependency(
                            kind="external_api",
                            name=target,
                            module_id=type_item["moduleId"],
                            module_name=type_item["moduleName"],
                            class_id=type_item["id"],
                            method_id=method["id"],
                            evidence_item=evidence(call["file"], call["line"], method["qualifiedName"], "call"),
                        )
                    )
                if any(token in lowered for token in ("send", "publish", "convertandsend")):
                    project["dependencies"].append(
                        dependency(
                            kind="message_publish",
                            name=target,
                            module_id=type_item["moduleId"],
                            module_name=type_item["moduleName"],
                            class_id=type_item["id"],
                            method_id=method["id"],
                            evidence_item=evidence(call["file"], call["line"], method["qualifiedName"], "call"),
                        )
                    )

    for module in project.get("modules", []):
        for migration in module.get("migrations", []):
            project["dependencies"].append(
                dependency(
                    kind="database_migration",
                    name=Path(migration).name,
                    module_id=module["id"],
                    module_name=module["name"],
                    detail=migration,
                    evidence_item=evidence(migration, 1, Path(migration).name, "file"),
                )
            )


def detect_javaee_descriptors(project: dict[str, Any]) -> None:
    for module in project.get("modules", []):
        for descriptor in module.get("descriptorFiles", []):
            name = Path(descriptor).name
            if name == "web.xml":
                detect_web_xml(project, module, Path(descriptor))
            elif name == "persistence.xml":
                detect_persistence_xml(project, module, Path(descriptor))


def detect_web_xml(project: dict[str, Any], module: dict[str, Any], path: Path) -> None:
    try:
        root = ET.fromstring(read_text(path))
    except ET.ParseError:
        return
    servlets = {}
    for servlet in elements(root, "servlet"):
        servlet_name = child_text(servlet, "servlet-name")
        servlet_class = child_text(servlet, "servlet-class")
        if servlet_name:
            servlets[servlet_name] = servlet_class or servlet_name
    for mapping in elements(root, "servlet-mapping"):
        servlet_name = child_text(mapping, "servlet-name")
        url = child_text(mapping, "url-pattern")
        if not servlet_name or not url:
            continue
        name = servlets.get(servlet_name, servlet_name)
        project["entryPoints"].append(
            entrypoint(
                framework="javaee",
                kind="servlet",
                name=name,
                module_id=module["id"],
                module_name=module["name"],
                path=url,
                evidence_item=evidence(path, 1, servlet_name, "xml", "servlet-mapping"),
            )
        )
    filters = {}
    for filter_element in elements(root, "filter"):
        filter_name = child_text(filter_element, "filter-name")
        filter_class = child_text(filter_element, "filter-class")
        if filter_name:
            filters[filter_name] = filter_class or filter_name
    for mapping in elements(root, "filter-mapping"):
        filter_name = child_text(mapping, "filter-name")
        url = child_text(mapping, "url-pattern")
        if not filter_name:
            continue
        project["dependencies"].append(
            dependency(
                kind="http_filter",
                name=filters.get(filter_name, filter_name),
                module_id=module["id"],
                module_name=module["name"],
                detail=url,
                evidence_item=evidence(path, 1, filter_name, "xml", "filter-mapping"),
                confidence="high",
            )
        )
    for resource in elements(root, "resource-ref"):
        res_name = child_text(resource, "res-ref-name")
        if res_name:
            project["dependencies"].append(
                dependency(
                    kind="queue" if "queue" in res_name.lower() else "external_dependency",
                    name=res_name,
                    module_id=module["id"],
                    module_name=module["name"],
                    detail="JNDI resource-ref",
                    evidence_item=evidence(path, 1, res_name, "xml", "resource-ref"),
                )
            )


def detect_persistence_xml(project: dict[str, Any], module: dict[str, Any], path: Path) -> None:
    try:
        root = ET.fromstring(read_text(path))
    except ET.ParseError:
        return
    for unit in elements(root, "persistence-unit"):
        name = unit.attrib.get("name", "")
        if not name:
            continue
        project["dependencies"].append(
            dependency(
                kind="persistence_unit",
                name=name,
                module_id=module["id"],
                module_name=module["name"],
                detail=child_text(unit, "jta-data-source") or child_text(unit, "non-jta-data-source"),
                evidence_item=evidence(path, 1, name, "xml", "persistence-unit"),
                confidence="high",
            )
        )


def detect_ui_entrypoints(project: dict[str, Any], named_beans: dict[str, dict[str, Any]]) -> None:
    for module in project.get("modules", []):
        for ui_file in module.get("uiFiles", []):
            path = Path(ui_file)
            text = read_text(path)
            expressions = re.findall(r"#\{\s*([A-Za-z_$][\w$]*)\.([A-Za-z_$][\w$]*)", text)
            if not expressions:
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="ui_page",
                        name=path.name,
                        module_id=module["id"],
                        module_name=module["name"],
                        path=str(path),
                        evidence_item=evidence(path, 1, path.name, "ui"),
                    )
                )
            for bean, method_name in expressions:
                bean_type = named_beans.get(bean)
                method = find_method(bean_type, method_name) if bean_type else None
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="ui_page",
                        name=f"{path.name} -> {bean}.{method_name}",
                        module_id=module["id"],
                        module_name=module["name"],
                        path=str(path),
                        class_id=(bean_type or {}).get("id", ""),
                        method_id=(method or {}).get("id", ""),
                        evidence_item=evidence(path, line_for(text, bean), f"{bean}.{method_name}", "ui", "EL"),
                    )
                )
            for api_call in ui_api_calls(path, text):
                project["entryPoints"].append(
                    entrypoint(
                        framework="javaee",
                        kind="ui_page",
                        name=f"{path.name} -> {api_call['target']}",
                        module_id=module["id"],
                        module_name=module["name"],
                        path=api_call["target"],
                        http_method=api_call.get("method", ""),
                        evidence_item=evidence(api_call["file"], api_call["line"], api_call["target"], "ui", api_call["kind"]),
                    )
                )
                project["dependencies"].append(
                    dependency(
                        kind="ui_websocket" if api_call["kind"] == "WebSocket" else "ui_api_call",
                        name=api_call["target"],
                        module_id=module["id"],
                        module_name=module["name"],
                        detail=api_call["target"],
                        evidence_item=evidence(api_call["file"], api_call["line"], api_call["target"], "ui", api_call["kind"]),
                        confidence="high",
                    )
                )


def ui_api_calls(path: Path, text: str) -> list[dict[str, Any]]:
    calls: list[dict[str, Any]] = []
    scripts = [path]
    for src in re.findall(r"<script[^>]+src=[\"']([^\"']+)[\"']", text, re.I):
        if src.startswith(("http://", "https://", "//")):
            continue
        candidate = (path.parent / src.lstrip("/")).resolve()
        if candidate.exists():
            scripts.append(candidate)
    for script in scripts:
        script_text = read_text(script)
        for match in re.finditer(r"fetch\s*\(\s*[\"']([^\"']+)[\"']", script_text):
            calls.append({"kind": "fetch", "target": match.group(1), "method": "GET", "file": str(script), "line": line_for(script_text, match.group(1))})
        for match in re.finditer(r"\.open\s*\(\s*[\"']([A-Z]+)[\"']\s*,\s*[\"']([^\"']+)[\"']", script_text):
            calls.append({"kind": "XMLHttpRequest", "target": match.group(2), "method": match.group(1), "file": str(script), "line": line_for(script_text, match.group(2))})
        for match in re.finditer(r"new\s+WebSocket\s*\(\s*[\"']([^\"']+)[\"']", script_text):
            calls.append({"kind": "WebSocket", "target": match.group(1), "method": "WEBSOCKET", "file": str(script), "line": line_for(script_text, match.group(1))})
    return calls


def named_bean_index(project: dict[str, Any]) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for type_item in project.get("types", []):
        bean_annotation = annotation(type_item, "Named", "ManagedBean")
        if not bean_annotation:
            continue
        explicit = first_value(bean_annotation.get("values", {}), "value", "name")
        name = explicit or lower_first(type_item["name"])
        result[name] = type_item
    return result


def is_container_managed(type_item: dict[str, Any]) -> bool:
    return has_annotation(
        type_item,
        "ApplicationScoped",
        "Dependent",
        "ManagedBean",
        "Named",
        "RequestScoped",
        "SessionScoped",
        "Singleton",
        "Stateful",
        "Stateless",
        "Startup",
    )


def first_javaee_http_annotation(method: dict[str, Any]) -> tuple[dict[str, Any] | None, str]:
    for annotation_item in method.get("annotations", []):
        if annotation_item["name"] in HTTP_METHOD_ANNOTATIONS:
            return annotation_item, HTTP_METHOD_ANNOTATIONS[annotation_item["name"]]
    return None, ""


def first_spring_http_annotation(method: dict[str, Any]) -> tuple[dict[str, Any] | None, str]:
    for annotation_item in method.get("annotations", []):
        name = annotation_item["name"]
        if name in SPRING_HTTP_ANNOTATIONS:
            return annotation_item, SPRING_HTTP_ANNOTATIONS[name]
    return None, ""


def first_annotation_value(item: dict[str, Any], name: str) -> str:
    annotation_item = annotation(item, name)
    if not annotation_item:
        return ""
    return path_from_annotation(annotation_item)


def path_from_annotation(annotation_item: dict[str, Any]) -> str:
    values = annotation_item.get("values", {})
    return first_value(values, "value", "path", "urlPatterns", "urlPattern") or ""


def web_paths(annotation_item: dict[str, Any]) -> list[str]:
    values = annotation_item.get("values", {})
    path = first_value(values, "urlPatterns", "urlPattern", "value")
    paths = string_list(path)
    return paths or ([path] if path else [])


def method_from_request_mapping(annotation_item: dict[str, Any]) -> str:
    values = annotation_item.get("values", {})
    method_value = first_value(values, "method")
    match = re.search(r"RequestMethod\.([A-Z]+)", method_value)
    return match.group(1) if match else ""


def should_map_web_method(method: dict[str, Any], web_method: dict[str, Any] | None) -> bool:
    if "private" in method.get("modifiers", []) or "static" in method.get("modifiers", []):
        return False
    if web_method and first_value(web_method.get("values", {}), "exclude").lower() == "true":
        return False
    return bool(web_method)


def cronish_detail(annotation_item: dict[str, Any]) -> str:
    values = annotation_item.get("values", {})
    parts = []
    for key in ("second", "minute", "hour", "dayOfMonth", "month", "dayOfWeek", "timezone"):
        value = first_value(values, key)
        if value:
            parts.append(f"{key}={value}")
    return ", ".join(parts)


def message_destination(annotation_item: dict[str, Any]) -> str:
    raw = annotation_item.get("raw", "")
    matches = re.findall(r"propertyName\s*=\s*\"([^\"]+)\"\s*,\s*propertyValue\s*=\s*\"([^\"]+)\"", raw)
    values = {name: value for name, value in matches}
    return values.get("destinationLookup") or values.get("destination") or values.get("destinationType") or ""


def cdi_observer_annotation(method: dict[str, Any]) -> str:
    for parameter in method.get("parameters", []):
        for annotation_item in parameter.get("annotations", []):
            if annotation_item["name"] in {"Observes", "ObservesAsync"}:
                return annotation_item["name"]
    return ""


def observes_cdi_extension_event(method: dict[str, Any]) -> bool:
    for parameter in method.get("parameters", []):
        param_type = parameter.get("type", "").split(".")[-1].split("<", 1)[0]
        if param_type in CDI_EXTENSION_EVENT_TYPES:
            return True
    return False


def simple_class_dep(type_item: dict[str, Any], kind: str, name: str, detail: str) -> dict[str, Any]:
    return dependency(
        kind=kind,
        name=name,
        module_id=type_item["moduleId"],
        module_name=type_item["moduleName"],
        detail=detail,
        class_id=type_item["id"],
        evidence_item=evidence(type_item["file"], type_item["line"], type_item["qualifiedName"], "import"),
    )


def elements(root: ET.Element, local_name: str) -> list[ET.Element]:
    return [element for element in root.iter() if element.tag.split("}")[-1] == local_name]


def child_text(element: ET.Element, local_name: str) -> str:
    for child in list(element):
        if child.tag.split("}")[-1] == local_name and child.text:
            return child.text.strip()
    return ""


def find_method(type_item: dict[str, Any] | None, method_name: str) -> dict[str, Any] | None:
    if not type_item:
        return None
    for method in type_item.get("methods", []):
        if method.get("name") == method_name:
            return method
    return None


def lower_first(value: str) -> str:
    return value[:1].lower() + value[1:] if value else value


def line_for(text: str, needle: str) -> int:
    index = text.find(needle)
    if index < 0:
        return 1
    return text.count("\n", 0, index) + 1

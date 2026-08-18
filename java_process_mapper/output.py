from __future__ import annotations

import html
import json
from pathlib import Path
from typing import Any

from .utils import compact, stable_id


def write_outputs(project: dict[str, Any], traces: dict[str, dict[str, Any]], output_dir: str | Path) -> dict[str, str]:
    out = Path(output_dir).resolve()
    docs = out / "docs"
    processes = docs / "processes"
    features = docs / "features"
    processes.mkdir(parents=True, exist_ok=True)
    features.mkdir(parents=True, exist_ok=True)

    artifacts = {
        "findings": str(out / "findings.json"),
        "traces": str(out / "traces.json"),
        "graph": str(out / "graph.json"),
        "state": str(out / "mapping-state.json"),
        "docsIndex": str(docs / "index.md"),
        "gaps": str(docs / "gaps.md"),
        "visualization": str(docs / "visualization.html"),
    }
    write_json(out / "findings.json", project)
    write_json(out / "traces.json", traces)
    write_json(out / "graph.json", project.get("graph", {}))
    write_json(out / "mapping-state.json", mapping_state(project, traces))
    (docs / "index.md").write_text(index_markdown(project), encoding="utf-8")
    (docs / "gaps.md").write_text(gaps_markdown(project), encoding="utf-8")
    for trace in traces.values():
        entry = trace["entryPoint"]
        filename = f"{stable_id(entry['kind'], entry['name'])}.md"
        (processes / filename).write_text(process_markdown(trace), encoding="utf-8")
    (docs / "visualization.html").write_text(visualization_html(project, traces), encoding="utf-8")
    return artifacts


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(compact(data), ensure_ascii=False, indent=2), encoding="utf-8")


def mapping_state(project: dict[str, Any], traces: dict[str, dict[str, Any]]) -> dict[str, Any]:
    return {
        "projectId": project["id"],
        "status": "ready",
        "items": [
            {
                "entryPointId": trace["entryPointId"],
                "status": "pending",
                "title": trace["entryPoint"].get("name", ""),
            }
            for trace in traces.values()
        ],
    }


def index_markdown(project: dict[str, Any]) -> str:
    lines = [
        f"# {project.get('name', 'Projeto')}",
        "",
        f"- Add-ons: {', '.join(project.get('addons', [])) or 'spring'}",
        f"- Java version: {project.get('javaVersion', 'unknown')}",
        f"- Modules: {len(project.get('modules', []))}",
        f"- Entry points: {len(project.get('entryPoints', []))}",
        f"- Dependencies: {len(project.get('dependencies', []))}",
        "",
        "## Entry points",
        "",
    ]
    if not project.get("entryPoints"):
        lines.append("Nenhum entrypoint identificado.")
    for entry in project.get("entryPoints", []):
        detail = entry.get("path") or entry.get("httpMethod") or entry.get("moduleName", "")
        lines.append(f"- `{entry.get('framework')}/{entry.get('kind')}` {entry.get('name')} {detail}".rstrip())
    lines.append("")
    return "\n".join(lines)


def gaps_markdown(project: dict[str, Any]) -> str:
    if project.get("entryPoints"):
        return "# Gaps\n\nNenhum gap bloqueante identificado pela analise estatica.\n"
    return "# Gaps\n\nNenhum entrypoint identificado. Revise add-ons, raiz informada e fontes incluidas.\n"


def process_markdown(trace: dict[str, Any]) -> str:
    entry = trace["entryPoint"]
    lines = [
        f"# {entry.get('name', 'Processo')}",
        "",
        f"- Framework: `{entry.get('framework', '')}`",
        f"- Tipo: `{entry.get('kind', '')}`",
    ]
    if entry.get("httpMethod") or entry.get("path"):
        lines.append(f"- Rota: `{entry.get('httpMethod', '')} {entry.get('path', '')}`".strip())
    lines.extend(["", "## Fluxo", ""])
    if not trace.get("steps"):
        lines.append("Fluxo nao resolvido alem do entrypoint identificado.")
    for step in trace.get("steps", []):
        lines.append(f"- {step.get('name')} ({step.get('file')}:{step.get('line')})")
    lines.extend(["", "## Dependencias", ""])
    if not trace.get("dependencies"):
        lines.append("Nenhuma dependencia direta identificada.")
    for dep in trace.get("dependencies", []):
        lines.append(f"- `{dep.get('kind')}` {dep.get('name')} {dep.get('detail', '')}".rstrip())
    lines.append("")
    return "\n".join(lines)


def visualization_html(project: dict[str, Any], traces: dict[str, dict[str, Any]]) -> str:
    cards = {}
    processes = []
    for trace in traces.values():
        entry = trace["entryPoint"]
        html_id = html_anchor(entry["id"])
        processes.append(
            {
                "id": html_id,
                "title": entry.get("name", ""),
                "kind": entry.get("kind", ""),
                "framework": entry.get("framework", ""),
                "path": entry.get("path", ""),
                "method": entry.get("httpMethod", ""),
            }
        )
        cards[html_id] = render_process_card(trace)
    processes.sort(key=lambda item: (item["framework"], item["kind"], item["title"]))
    payload = json.dumps({"processes": processes, "cards": cards}, ensure_ascii=False).replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")
    title = html.escape(project.get("name", "java-process-mapper"))
    return f"""<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{title} - java-process-mapper</title>
  <style>
    :root {{ color-scheme: light; font-family: Inter, Segoe UI, Arial, sans-serif; background: #f7f8fa; color: #20242a; }}
    body {{ margin: 0; }}
    header {{ padding: 20px 24px 12px; border-bottom: 1px solid #d9dde3; background: #ffffff; position: sticky; top: 0; z-index: 2; }}
    h1 {{ margin: 0 0 12px; font-size: 22px; letter-spacing: 0; }}
    main {{ display: grid; grid-template-columns: minmax(280px, 360px) 1fr; min-height: calc(100vh - 89px); }}
    aside {{ border-right: 1px solid #d9dde3; background: #ffffff; padding: 14px; overflow: auto; }}
    section {{ padding: 20px 24px; overflow: auto; }}
    input {{ width: 100%; box-sizing: border-box; padding: 10px 12px; border: 1px solid #b9c0ca; border-radius: 6px; font: inherit; }}
    .summary {{ color: #596170; font-size: 13px; margin-top: 8px; }}
    .process-list {{ list-style: none; padding: 0; margin: 14px 0 0; display: grid; gap: 8px; }}
    .process-button {{ width: 100%; text-align: left; border: 1px solid #d8dde5; background: #fbfcfd; border-radius: 6px; padding: 10px; cursor: pointer; }}
    .process-button:hover, .process-button.active {{ border-color: #2f6fed; background: #eef4ff; }}
    .badge {{ display: inline-block; border-radius: 999px; padding: 2px 7px; background: #e7ebf0; color: #343b45; font-size: 12px; margin-right: 4px; }}
    .empty {{ color: #657083; padding: 24px 0; }}
    article.process {{ background: #ffffff; border: 1px solid #d9dde3; border-radius: 8px; padding: 18px; max-width: 1100px; }}
    article.process h2 {{ margin-top: 0; font-size: 20px; }}
    article.process h3 {{ margin-bottom: 6px; }}
    code {{ background: #eef1f5; padding: 1px 4px; border-radius: 4px; }}
    @media (max-width: 800px) {{ main {{ grid-template-columns: 1fr; }} aside {{ border-right: 0; border-bottom: 1px solid #d9dde3; }} }}
  </style>
</head>
<body>
  <header>
    <h1>{title}</h1>
    <input data-process-search-input type="search" placeholder="Filtrar entrypoints">
    <div class="summary">{len(processes)} entrypoints identificados</div>
  </header>
  <main>
    <aside>
      <ul class="process-list" data-process-list></ul>
    </aside>
    <section data-process-detail>
      <div class="empty">Selecione um entrypoint na lista para ver o fluxo.</div>
    </section>
  </main>
  <script type="application/json" id="process-data">{payload}</script>
  <script>
    const data = JSON.parse(document.getElementById('process-data').textContent);
    const list = document.querySelector('[data-process-list]');
    const detail = document.querySelector('[data-process-detail]');
    const input = document.querySelector('[data-process-search-input]');
    let selectedId = location.hash ? location.hash.slice(1) : '';

    function normalizeSearchText(value) {{
      return (value || '').normalize('NFD').replace(/[\\u0300-\\u036f]/g, '').toLowerCase();
    }}

    function processCardText(process) {{
      return normalizeSearchText([process.title, process.framework, process.kind, process.path, process.method].join(' '));
    }}

    function renderList() {{
      const terms = normalizeSearchText(input.value).split(/\\s+/).filter(Boolean);
      const matches = data.processes.filter((process) => terms.every((term) => processCardText(process).includes(term)));
      list.innerHTML = matches.map((process) => `
        <li>
          <button class="process-button ${{process.id === selectedId ? 'active' : ''}}" data-process-id="${{process.id}}">
            <span class="badge">${{process.framework}}</span><span class="badge">${{process.kind}}</span><br>
            <strong>${{escapeHtml(process.title)}}</strong><br>
            <small>${{escapeHtml([process.method, process.path].filter(Boolean).join(' '))}}</small>
          </button>
        </li>`).join('');
      if (!matches.length) list.innerHTML = '<li class="empty">Nenhum resultado para o filtro.</li>';
    }}

    function renderSelectedProcess(id) {{
      selectedId = id;
      if (!id || !data.cards[id]) {{
        detail.innerHTML = '<div class="empty">Selecione um entrypoint na lista para ver o fluxo.</div>';
        renderList();
        return;
      }}
      detail.innerHTML = data.cards[id];
      history.replaceState(null, '', '#' + id);
      renderList();
    }}

    function escapeHtml(value) {{
      return String(value || '').replace(/[&<>"']/g, (char) => ({{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}}[char]));
    }}

    list.addEventListener('click', (event) => {{
      const button = event.target.closest('[data-process-id]');
      if (button) renderSelectedProcess(button.dataset.processId);
    }});
    input.addEventListener('input', renderList);
    window.addEventListener('hashchange', () => renderSelectedProcess(location.hash.slice(1)));
    renderList();
    if (selectedId) renderSelectedProcess(selectedId);
  </script>
</body>
</html>
"""


def render_process_card(trace: dict[str, Any]) -> str:
    entry = trace["entryPoint"]
    lines = [
        f'<article class="process" id="{html.escape(html_anchor(entry["id"]))}">',
        f"<h2>{html.escape(entry.get('name', 'Processo'))}</h2>",
        f"<p><span class=\"badge\">{html.escape(entry.get('framework', ''))}</span><span class=\"badge\">{html.escape(entry.get('kind', ''))}</span></p>",
    ]
    if entry.get("httpMethod") or entry.get("path"):
        lines.append(f"<p><code>{html.escape((entry.get('httpMethod', '') + ' ' + entry.get('path', '')).strip())}</code></p>")
    lines.append("<h3>Fluxo</h3>")
    if not trace.get("steps"):
        lines.append("<p>Fluxo nao resolvido alem do entrypoint identificado.</p>")
    else:
        lines.append("<ol>")
        for step in trace.get("steps", []):
            lines.append(f"<li>{html.escape(step.get('name', ''))} <small>{html.escape(step.get('file', ''))}:{step.get('line', 0)}</small></li>")
        lines.append("</ol>")
    lines.append("<h3>Dependencias</h3>")
    if not trace.get("dependencies"):
        lines.append("<p>Nenhuma dependencia direta identificada.</p>")
    else:
        lines.append("<ul>")
        for dep in trace.get("dependencies", []):
            detail = dep.get("detail", "")
            lines.append(f"<li><code>{html.escape(dep.get('kind', ''))}</code> {html.escape(dep.get('name', ''))} {html.escape(detail)}</li>")
        lines.append("</ul>")
    lines.append("</article>")
    return "\n".join(lines)


def html_anchor(value: str) -> str:
    return "p-" + stable_id(value)

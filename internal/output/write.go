package output

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bee/java-process-mapper/internal/flow"
	"github.com/bee/java-process-mapper/internal/model"
)

type Artifacts struct {
	OutputDir     string   `json:"outputDir"`
	Graph         string   `json:"graph"`
	Findings      string   `json:"findings"`
	Traces        string   `json:"traces"`
	Index         string   `json:"index"`
	Visualization string   `json:"visualization"`
	Gaps          string   `json:"gaps"`
	Docs          []string `json:"docs"`
}

func Write(project *model.Project, outputDir string) (Artifacts, error) {
	outputDir = filepath.Clean(outputDir)
	docsDir := filepath.Join(outputDir, "docs")
	processesDir := filepath.Join(docsDir, "processes")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
		return Artifacts{}, err
	}

	traces := map[string]flow.Trace{}
	for _, entry := range project.EntryPoints {
		traces[entry.ID] = flow.Build(project, entry)
	}

	artifacts := Artifacts{
		OutputDir:     outputDir,
		Graph:         filepath.Join(outputDir, "graph.json"),
		Findings:      filepath.Join(outputDir, "findings.json"),
		Traces:        filepath.Join(outputDir, "traces.json"),
		Index:         filepath.Join(docsDir, "index.md"),
		Visualization: filepath.Join(docsDir, "visualization.html"),
		Gaps:          filepath.Join(docsDir, "gaps.md"),
		Docs:          []string{},
	}

	if err := writeJSON(artifacts.Graph, project.Graph); err != nil {
		return Artifacts{}, err
	}
	if err := writeJSON(artifacts.Findings, project); err != nil {
		return Artifacts{}, err
	}
	if err := writeJSON(artifacts.Traces, traces); err != nil {
		return Artifacts{}, err
	}
	if err := os.WriteFile(artifacts.Index, []byte(renderIndex(project)), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := os.WriteFile(artifacts.Visualization, []byte(renderVisualization(project, traces)), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := os.WriteFile(artifacts.Gaps, []byte(renderGaps(project)), 0o644); err != nil {
		return Artifacts{}, err
	}

	seenDocNames := map[string]int{}
	for _, entry := range project.EntryPoints {
		baseName := safeFileName(entry.Kind + "-" + entry.Name)
		if baseName == "" {
			baseName = safeFileName(entry.ID)
		}
		name := baseName
		if seenDocNames[baseName] > 0 {
			name = fmt.Sprintf("%s-%d", baseName, seenDocNames[baseName]+1)
		}
		seenDocNames[baseName]++
		path := filepath.Join(processesDir, name+".md")
		trace := traces[entry.ID]
		if err := os.WriteFile(path, []byte(renderProcess(project, trace)), 0o644); err != nil {
			return Artifacts{}, err
		}
		artifacts.Docs = append(artifacts.Docs, path)
	}
	sort.Strings(artifacts.Docs)
	return artifacts, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func renderIndex(project *model.Project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", project.Name)
	fmt.Fprintf(&b, "Pacote mecanico de evidencias gerado por analise estatica deterministica. Esta saida nao usa LLM e nao tenta inferir feature de negocio.\n\n")
	fmt.Fprintf(&b, "## Resumo\n\n")
	fmt.Fprintf(&b, "| Item | Total |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Modulos | %d |\n", project.Summary.Modules)
	fmt.Fprintf(&b, "| Arquivos Java | %d |\n", project.Summary.JavaFiles)
	fmt.Fprintf(&b, "| Tipos | %d |\n", project.Summary.Types)
	fmt.Fprintf(&b, "| Metodos | %d |\n", project.Summary.Methods)
	fmt.Fprintf(&b, "| Entry points | %d |\n", project.Summary.EntryPoints)
	fmt.Fprintf(&b, "| Properties | %d |\n", project.Summary.ConfigProperties)
	fmt.Fprintf(&b, "| Dependencias inventariadas | %d |\n", project.Summary.Dependencies)
	fmt.Fprintf(&b, "| Lacunas tecnicas | %d |\n\n", project.Summary.Gaps)

	fmt.Fprintf(&b, "## Processos detectados\n\n")
	if len(project.EntryPoints) == 0 {
		fmt.Fprintf(&b, "Nenhum entrypoint Spring foi identificado.\n\n")
	} else {
		fmt.Fprintf(&b, "| Tipo | Nome tecnico | Entrada | Evidencia |\n| --- | --- | --- | --- |\n")
		for _, entry := range project.EntryPoints {
			fmt.Fprintf(&b, "| %s | %s | %s | `%s:%d` |\n", entry.Kind, entry.Name, entryLabel(entry), entry.Evidence.Path, entry.Evidence.Line)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Artefatos\n\n")
	fmt.Fprintf(&b, "- `graph.json`: grafo tecnico global.\n")
	fmt.Fprintf(&b, "- `traces.json`: fluxo transitivo por entrypoint, com dependencias diretas e indiretas.\n")
	fmt.Fprintf(&b, "- `findings.json`: inventario completo extraido.\n")
	fmt.Fprintf(&b, "- `docs/processes/*.md`: pacote mecanico por processo.\n")
	fmt.Fprintf(&b, "- `docs/visualization.html`: visualizacao HTML local dos processos e dependencias.\n")
	fmt.Fprintf(&b, "- `docs/gaps.md`: lacunas tecnicas de parse/extracao.\n")
	return b.String()
}

func renderProcess(project *model.Project, trace flow.Trace) string {
	entry := trace.EntryPoint
	typ := findType(project, entry.ClassID)
	method := findMethod(typ, entry.MethodID)

	var b strings.Builder
	fmt.Fprintf(&b, "# Technical Evidence - %s\n\n", entry.Name)
	fmt.Fprintf(&b, "## Entrada detectada\n\n")
	fmt.Fprintf(&b, "- Projeto: `%s`\n", project.Name)
	fmt.Fprintf(&b, "- Origem da geracao: `static-analysis-script`\n")
	fmt.Fprintf(&b, "- Uso de LLM nesta etapa: `false`\n")
	fmt.Fprintf(&b, "- Tipo: `%s`\n", entry.Kind)
	fmt.Fprintf(&b, "- Framework/addon: `%s`\n", valueOrUnknown(entry.Framework))
	fmt.Fprintf(&b, "- Classe: `%s`\n", valueOrUnknown(typ.FQN))
	fmt.Fprintf(&b, "- Metodo: `%s`\n", valueOrUnknown(method.Name))
	fmt.Fprintf(&b, "- Entrada: `%s`\n", valueOrUnknown(entryLabel(entry)))
	fmt.Fprintf(&b, "- Evidencia: `%s:%d`\n\n", entry.Evidence.Path, entry.Evidence.Line)

	fmt.Fprintf(&b, "## Assinatura\n\n")
	fmt.Fprintf(&b, "| Nome | Tipo | Origem | Evidencia |\n| --- | --- | --- | --- |\n")
	for _, param := range method.Parameters {
		fmt.Fprintf(&b, "| %s | %s | parametro | `%s:%d` |\n", valueOrUnknown(param.Name), valueOrUnknown(param.Type), method.Evidence.Path, method.Evidence.Line)
	}
	if len(method.Parameters) == 0 {
		fmt.Fprintf(&b, "| n/a | n/a | sem parametros | `%s:%d` |\n", entry.Evidence.Path, entry.Evidence.Line)
	}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "Retorno detectado: `%s`\n\n", valueOrUnknown(method.ReturnType))

	fmt.Fprintf(&b, "## Fluxo tecnico transitivo\n\n")
	fmt.Fprintf(&b, "| Ordem | Profundidade | Chamador | Chamada | Resolvido para | Resolucao | Evidencia |\n| --- | ---: | --- | --- | --- | --- | --- |\n")
	if len(trace.Steps) == 0 {
		fmt.Fprintf(&b, "| 0 | 0 | %s.%s | n/a | n/a | sem chamadas detectadas | `%s:%d` |\n", valueOrUnknown(typ.FQN), valueOrUnknown(method.Name), method.Evidence.Path, method.Evidence.Line)
	}
	for _, step := range trace.Steps {
		resolved := "unresolved"
		if step.ResolvedClass != "" || step.ResolvedMethod != "" {
			resolved = strings.Trim(strings.TrimSpace(step.ResolvedClass+"."+step.ResolvedMethod), ".")
		}
		fmt.Fprintf(&b, "| %d | %d | %s.%s | %s | %s | %s/%s | `%s:%d` |\n",
			step.Order,
			step.Depth,
			valueOrUnknown(step.CallerClass),
			valueOrUnknown(step.CallerMethod),
			valueOrUnknown(step.Call),
			valueOrUnknown(resolved),
			valueOrUnknown(step.Resolution),
			step.Confidence,
			step.Evidence.Path,
			step.Evidence.Line,
		)
	}
	if trace.Truncated {
		fmt.Fprintf(&b, "\nFluxo truncado por limite de profundidade ou quantidade de passos.\n")
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Dependencias no fluxo\n\n")
	fmt.Fprintf(&b, "| Escopo | Recurso | Tipo | Detalhe | Via classe | Via metodo | Evidencia |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	if len(trace.Dependencies) == 0 {
		fmt.Fprintf(&b, "| n/a | n/a | n/a | n/a | n/a | n/a | nenhuma dependencia vinculada ao fluxo |\n")
	}
	for _, dep := range trace.Dependencies {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | `%s:%d` |\n",
			dep.Scope,
			dep.Dependency.Name,
			dep.Dependency.Kind,
			valueOrUnknown(dep.Dependency.Detail),
			valueOrUnknown(dep.ViaClass),
			valueOrUnknown(dep.ViaMethod),
			dep.Dependency.Evidence.Path,
			dep.Dependency.Evidence.Line,
		)
	}
	fmt.Fprintf(&b, "\n")

	if len(trace.ConfigProperties) > 0 {
		fmt.Fprintf(&b, "## Configuracoes vinculadas ao fluxo\n\n")
		fmt.Fprintf(&b, "| Chave | Valor | Origem | Observacao |\n| --- | --- | --- | --- |\n")
		for _, prop := range trace.ConfigProperties {
			note := ""
			if prop.DefinedExternally {
				note = "definido externamente; variavel de ambiente nao resolvida"
			}
			if prop.Redacted {
				note = strings.TrimSpace(note + " valor sensivel redigido")
			}
			fmt.Fprintf(&b, "| %s | %s | `%s:%d` | %s |\n", prop.Key, prop.Value, prop.Evidence.Path, prop.Evidence.Line, note)
		}
		fmt.Fprintf(&b, "\n")
	}

	return b.String()
}

func renderVisualization(project *model.Project, traces map[string]flow.Trace) string {
	entries := append([]model.EntryPoint{}, project.EntryPoints...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})

	totalSteps := 0
	totalDeps := 0
	unresolved := 0
	external := 0
	for _, trace := range traces {
		totalSteps += len(trace.Steps)
		totalDeps += len(trace.Dependencies)
		for _, step := range trace.Steps {
			if step.Resolution == "unresolved" {
				unresolved++
			}
			if step.Resolution == "external_import" {
				external++
			}
		}
	}
	sourceLines := newSourceLineReader()
	sourceContexts := newSourceContextStore()

	var b strings.Builder
	b.WriteString("<!doctype html>\n")
	b.WriteString("<html lang=\"pt-BR\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s - Java Process Mapper</title>\n", h(project.Name))
	b.WriteString(`<style>
:root {
  color-scheme: light;
  --bg: #f6f7f9;
  --panel: #ffffff;
  --ink: #202124;
  --muted: #5f6368;
  --line: #d8dde3;
  --accent: #0f766e;
  --accent-soft: #e6f4f1;
  --warn: #9a6700;
  --warn-soft: #fff4d6;
  --bad: #b42318;
  --bad-soft: #ffe8e5;
  --code: #f1f3f4;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--ink);
  font: 14px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
header {
  border-bottom: 1px solid var(--line);
  background: var(--panel);
  padding: 24px clamp(18px, 4vw, 44px);
}
h1, h2, h3 { margin: 0; letter-spacing: 0; }
h1 { font-size: clamp(24px, 4vw, 34px); }
h2 { font-size: 20px; }
h3 { font-size: 15px; margin-top: 22px; }
.subtle { color: var(--muted); }
.stats {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  margin-top: 18px;
}
.stat {
  border: 1px solid var(--line);
  background: var(--panel);
  border-radius: 8px;
  padding: 12px;
}
.stat strong { display: block; font-size: 22px; }
main {
  display: grid;
  grid-template-columns: minmax(240px, 320px) 1fr;
  gap: 18px;
  padding: 18px clamp(18px, 4vw, 44px) 44px;
}
aside {
  align-self: start;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  max-height: calc(100vh - 36px);
  overflow: auto;
  position: sticky;
  top: 18px;
}
aside h2 { padding: 14px 14px 8px; }
.process-search {
  border-top: 1px solid var(--line);
  padding: 10px 14px 12px;
}
.process-search input {
  border: 1px solid var(--line);
  border-radius: 6px;
  color: var(--ink);
  font: inherit;
  padding: 8px 10px;
  width: 100%;
}
.process-search input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}
.process-search-status {
  color: var(--muted);
  display: block;
  font-size: 12px;
  margin-top: 6px;
}
nav a {
  border-top: 1px solid var(--line);
  color: inherit;
  display: block;
  padding: 10px 14px;
  text-decoration: none;
}
nav a:hover { background: var(--accent-soft); }
nav span, .meta-row span {
  color: var(--muted);
  display: block;
  font-size: 12px;
}
nav strong {
  display: block;
  overflow-wrap: anywhere;
}
.process {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  margin-bottom: 18px;
  overflow: visible;
}
.process-header {
  border-bottom: 1px solid var(--line);
  padding: 16px;
}
.process-body { padding: 0 16px 16px; }
.meta-grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  margin-top: 12px;
}
.meta-row {
  background: var(--code);
  border-radius: 6px;
  padding: 8px 10px;
  overflow-wrap: anywhere;
}
.badges { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
.badge {
  border: 1px solid var(--line);
  border-radius: 999px;
  display: inline-flex;
  gap: 6px;
  padding: 4px 9px;
  white-space: nowrap;
}
.high { background: var(--accent-soft); color: var(--accent); }
.medium { background: var(--warn-soft); color: var(--warn); }
.low, .unresolved { background: var(--bad-soft); color: var(--bad); }
.external_import, .external-import { background: #e8f0fe; color: #174ea6; }
.view-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 16px 0;
}
.view-button {
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel);
  color: var(--ink);
  cursor: pointer;
  font: inherit;
  padding: 7px 12px;
}
.view-button.active {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}
.view[hidden] { display: none; }
.graph-layout {
  display: grid;
  gap: 14px;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 340px);
  align-items: start;
}
.graph-canvas {
  border: 1px solid var(--line);
  border-radius: 8px;
  background: #fbfcfd;
  height: min(76vh, 820px);
  min-height: 560px;
  overflow: hidden;
  position: relative;
}
.graph-toolbar {
  display: flex;
  gap: 6px;
  left: 10px;
  position: absolute;
  top: 10px;
  z-index: 2;
}
.graph-tool {
  border: 1px solid var(--line);
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.94);
  color: var(--ink);
  cursor: pointer;
  font: inherit;
  min-width: 34px;
  padding: 5px 8px;
}
.graph-tool:hover { background: var(--accent-soft); }
.graph-hint {
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid var(--line);
  border-radius: 6px;
  bottom: 10px;
  color: var(--muted);
  font-size: 12px;
  left: 10px;
  padding: 5px 8px;
  position: absolute;
  z-index: 2;
}
.call-graph {
  cursor: grab;
  display: block;
  height: 100%;
  touch-action: none;
  width: 100%;
}
.call-graph.panning { cursor: grabbing; }
.graph-node,
.graph-edge,
.graph-edge-label {
  cursor: pointer;
}
.graph-node rect {
  fill: #fff;
  stroke: #8a94a6;
  stroke-width: 1.2;
}
.graph-node.method rect { fill: #ffffff; }
.graph-node.class rect { fill: #f1f3f4; stroke: #5f6368; }
.graph-node.external rect { fill: #e8f0fe; stroke: #174ea6; }
.graph-node.dependency rect {
  fill: #e6f4f1;
  stroke: #0f766e;
  stroke-dasharray: 5 4;
}
.graph-node.condition rect { fill: #fff4d6; stroke: #9a6700; }
.graph-node.unresolved rect { fill: var(--bad-soft); stroke: var(--bad); }
.graph-node.entry rect { fill: #fef7e0; stroke: #b06000; }
.graph-node.selected rect {
  stroke: var(--accent);
  stroke-width: 3;
}
.graph-node text {
  fill: var(--ink);
  font-size: 12px;
  pointer-events: none;
}
.graph-node .node-kind {
  fill: var(--muted);
  font-size: 11px;
  text-transform: uppercase;
}
.graph-edge path {
  fill: none;
  pointer-events: stroke;
  stroke: #9aa0a6;
  stroke-width: 2;
}
.graph-edge.dependency path {
  stroke: #0f766e;
  stroke-dasharray: 5 4;
}
.graph-edge.declares path {
  stroke: #5f6368;
  stroke-dasharray: 3 4;
}
.graph-edge.selected path { stroke: var(--accent); stroke-width: 3; }
.graph-edge-label {
  fill: var(--muted);
  font-size: 11px;
}
.node-inspector {
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
  max-height: 72vh;
  overflow: auto;
  padding: 14px;
  position: sticky;
  top: 18px;
}
.node-inspector h3 { margin-top: 0; }
.inspector-content dl {
  display: grid;
  gap: 8px;
  margin: 0;
}
.inspector-content dt {
  color: var(--muted);
  font-size: 12px;
}
.inspector-content dd {
  border-bottom: 1px solid var(--line);
  margin: 0;
  overflow-wrap: anywhere;
  padding-bottom: 8px;
}
.inspector-content pre {
  background: var(--code);
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.45;
  margin: 0;
  max-height: 340px;
  overflow: auto;
  padding: 10px;
  white-space: pre;
}
.inspector-content pre code {
  background: transparent;
  padding: 0;
}
.table-wrap { border: 1px solid var(--line); border-radius: 8px; overflow: auto; }
table { border-collapse: collapse; min-width: 980px; width: 100%; }
th, td { border-bottom: 1px solid var(--line); padding: 8px 10px; text-align: left; vertical-align: top; }
th { background: #f1f3f4; font-size: 12px; position: sticky; top: 0; z-index: 1; }
td { overflow-wrap: anywhere; }
code {
  background: var(--code);
  border-radius: 4px;
  padding: 2px 4px;
}
.empty {
  border: 1px dashed var(--line);
  border-radius: 8px;
  color: var(--muted);
  padding: 14px;
}
.auxiliary-list {
  border: 1px solid var(--line);
  border-radius: 8px;
  margin-top: 14px;
  overflow: auto;
}
.auxiliary-list summary {
  cursor: pointer;
  padding: 10px 12px;
}
.auxiliary-list table { min-width: 780px; }
.auxiliary-pager {
  align-items: center;
  border-top: 1px solid var(--line);
  display: flex;
  gap: 8px;
  padding: 10px 12px;
}
.auxiliary-pager button {
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel);
  color: var(--ink);
  cursor: pointer;
  font: inherit;
  padding: 5px 9px;
}
.auxiliary-pager button:disabled {
  cursor: default;
  opacity: 0.45;
}
.auxiliary-pager span {
  color: var(--muted);
  font-size: 12px;
}
@media (max-width: 860px) {
  main { grid-template-columns: 1fr; }
  aside { max-height: none; position: static; }
  .graph-layout { grid-template-columns: 1fr; }
  .graph-canvas { height: 70vh; min-height: 440px; }
  .node-inspector { max-height: none; position: static; }
}
</style>
`)
	b.WriteString("</head>\n<body>\n")
	b.WriteString("<header>\n")
	fmt.Fprintf(&b, "<h1>%s</h1>\n", h(project.Name))
	b.WriteString("<p class=\"subtle\">Visualizacao mecanica gerada por static-analysis-script. Uso de LLM nesta etapa: false.</p>\n")
	b.WriteString("<div class=\"stats\">\n")
	renderStat(&b, "Modulos", project.Summary.Modules)
	renderStat(&b, "Arquivos Java", project.Summary.JavaFiles)
	renderStat(&b, "Entry points", project.Summary.EntryPoints)
	renderStat(&b, "Chamadas no fluxo", totalSteps)
	renderStat(&b, "Dependencias no fluxo", totalDeps)
	renderStat(&b, "Imports externos", external)
	renderStat(&b, "Nao resolvidas", unresolved)
	b.WriteString("</div>\n</header>\n")

	b.WriteString("<main>\n<aside>\n<h2>Processos</h2>\n")
	fmt.Fprintf(&b, "<div class=\"process-search\"><input type=\"search\" data-process-search-input placeholder=\"Pesquisar entrypoint\" aria-label=\"Pesquisar entrypoint\"><span class=\"process-search-status\" data-process-search-status>%d processos</span></div>\n", len(entries))
	b.WriteString("<nav data-process-nav>\n")
	for _, entry := range entries {
		trace := traces[entry.ID]
		searchText := strings.ToLower(strings.Join([]string{
			entry.Kind,
			entry.Name,
			entryLabel(entry),
			fmt.Sprintf("%d chamadas", len(trace.Steps)),
			fmt.Sprintf("%d deps", len(trace.Dependencies)),
		}, " "))
		fmt.Fprintf(&b, "<a href=\"#%s\" data-process-link data-process-search=\"%s\"><span>%s</span><strong>%s</strong><span>%d chamadas - %d deps</span></a>\n",
			h(htmlID(entry.ID)),
			h(searchText),
			h(entry.Kind),
			h(entry.Name),
			len(trace.Steps),
			len(trace.Dependencies),
		)
	}
	b.WriteString("</nav>\n</aside>\n<section>\n")
	if len(entries) == 0 {
		b.WriteString("<div class=\"empty\">Nenhum processo detectado.</div>\n")
	}
	for _, entry := range entries {
		trace := traces[entry.ID]
		renderProcessHTML(&b, project, trace, sourceLines, sourceContexts)
	}
	b.WriteString("</section>\n</main>\n")
	fmt.Fprintf(&b, "<script id=\"source-contexts\" type=\"application/json\">%s</script>\n", sourceContexts.json())
	b.WriteString(`<script>
(function () {
  var sourceContexts = {};
  var sourceContextEl = document.getElementById('source-contexts');
  if (sourceContextEl) {
    try {
      sourceContexts = JSON.parse(sourceContextEl.textContent || '{}');
    } catch (error) {
      sourceContexts = {};
    }
  }

  function escapeHTML(value) {
    return String(value || "").replace(/[&<>"']/g, function (char) {
      return {"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"}[char];
    });
  }

  function resolveInspectorValue(value) {
    value = String(value || "");
    if (value.indexOf('source-context:') === 0) {
      return sourceContexts[value.slice('source-context:'.length)] || '';
    }
    return value;
  }

  function renderInspector(panel, payload) {
    var keys = Object.keys(payload || {});
    if (!keys.length) {
      panel.innerHTML = '<p class="subtle">Nenhuma informacao para este item.</p>';
      return;
    }
    panel.innerHTML = '<dl>' + keys.map(function (key) {
      var value = escapeHTML(resolveInspectorValue(payload[key]));
      if (key.indexOf('Conteudo') === 0) {
        value = '<pre><code>' + value + '</code></pre>';
      }
      return '<div><dt>' + escapeHTML(key) + '</dt><dd>' + value + '</dd></div>';
    }).join('') + '</dl>';
  }

  function setupProcessSearch() {
    var input = document.querySelector('[data-process-search-input]');
    var status = document.querySelector('[data-process-search-status]');
    var links = Array.prototype.slice.call(document.querySelectorAll('[data-process-link]'));
    if (!input || !status || !links.length) {
      return;
    }
    function applyFilter() {
      var query = input.value.trim().toLowerCase();
      var visible = 0;
      links.forEach(function (link) {
        var haystack = link.getAttribute('data-process-search') || '';
        var match = query === '' || haystack.indexOf(query) >= 0;
        link.hidden = !match;
        if (match) {
          visible++;
        }
      });
      status.textContent = query ? (visible + ' de ' + links.length + ' processos') : (links.length + ' processos');
    }
    input.addEventListener('input', applyFilter);
    input.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') {
        input.value = '';
        applyFilter();
      }
    });
    applyFilter();
  }

  function setupAuxiliaryPagination() {
    document.querySelectorAll('[data-aux-list]').forEach(function (list) {
      var rows = Array.prototype.slice.call(list.querySelectorAll('[data-aux-row]'));
      var prev = list.querySelector('[data-aux-prev]');
      var next = list.querySelector('[data-aux-next]');
      var status = list.querySelector('[data-aux-status]');
      var pageSize = parseInt(list.getAttribute('data-aux-page-size') || '5', 10);
      var page = 0;
      if (!rows.length || !prev || !next || !status) {
        return;
      }
      function renderPage() {
        var totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
        if (page >= totalPages) {
          page = totalPages - 1;
        }
        var start = page * pageSize;
        var end = Math.min(start + pageSize, rows.length);
        rows.forEach(function (row, index) {
          row.hidden = index < start || index >= end;
        });
        prev.disabled = page === 0;
        next.disabled = page >= totalPages - 1;
        status.textContent = (start + 1) + '-' + end + ' de ' + rows.length;
      }
      prev.addEventListener('click', function (event) {
        event.preventDefault();
        page = Math.max(0, page - 1);
        renderPage();
      });
      next.addEventListener('click', function (event) {
        event.preventDefault();
        page++;
        renderPage();
      });
      renderPage();
    });
  }

  function closestGraphItem(target) {
    while (target && target !== document) {
      if (target.getAttribute && target.getAttribute('data-graph-info')) {
        return target;
      }
      target = target.parentNode;
    }
    return null;
  }

  function selectGraphItem(item) {
    if (!item) {
      return false;
    }
    var graph = item.closest('.graph-layout');
    var panel = graph && graph.querySelector('.inspector-content');
    if (!panel) {
      return false;
    }
    graph.querySelectorAll('.selected').forEach(function (selected) {
      selected.classList.remove('selected');
    });
    item.classList.add('selected');
    try {
      renderInspector(panel, JSON.parse(item.getAttribute('data-graph-info')));
    } catch (error) {
      renderInspector(panel, {"Erro": "Nao foi possivel ler os dados deste item."});
    }
    return true;
  }

  function pointInSvg(svg, event) {
    var point = svg.createSVGPoint();
    point.x = event.clientX;
    point.y = event.clientY;
    return point.matrixTransform(svg.getScreenCTM().inverse());
  }

  function graphState(canvas) {
    if (!canvas.__graphState) {
      canvas.__graphState = {x: 24, y: 40, scale: 1, initialized: false, dragging: false, moved: false};
    }
    return canvas.__graphState;
  }

  function applyGraphTransform(canvas) {
    var state = graphState(canvas);
    var viewport = canvas.querySelector('.graph-viewport');
    if (viewport) {
      viewport.setAttribute('transform', 'translate(' + state.x + ' ' + state.y + ') scale(' + state.scale + ')');
    }
  }

  function fitGraph(canvas) {
    var svg = canvas.querySelector('.call-graph');
    if (!svg) {
      return;
    }
    var rect = svg.getBoundingClientRect();
    if (!rect.width || !rect.height) {
      return;
    }
    var viewBox = svg.viewBox.baseVal;
    var graphWidth = parseFloat(canvas.getAttribute('data-graph-width') || '1000');
    var graphHeight = parseFloat(canvas.getAttribute('data-graph-height') || '700');
    var sx = (viewBox.width - 80) / graphWidth;
    var sy = (viewBox.height - 80) / graphHeight;
    var state = graphState(canvas);
    state.scale = Math.max(0.08, Math.min(1.1, Math.min(sx, sy)));
    state.x = 40;
    state.y = 40;
    applyGraphTransform(canvas);
  }

  function zoomGraph(canvas, factor, anchorEvent) {
    var svg = canvas.querySelector('.call-graph');
    var state = graphState(canvas);
    var point;
    if (anchorEvent) {
      point = pointInSvg(svg, anchorEvent);
    } else {
      var box = svg.viewBox.baseVal;
      point = {x: box.width / 2, y: box.height / 2};
    }
    var beforeX = (point.x - state.x) / state.scale;
    var beforeY = (point.y - state.y) / state.scale;
    state.scale = Math.max(0.05, Math.min(3.5, state.scale * factor));
    state.x = point.x - beforeX * state.scale;
    state.y = point.y - beforeY * state.scale;
    applyGraphTransform(canvas);
  }

  function initializeGraph(canvas) {
    if (!canvas || canvas.__panZoomReady) {
      if (canvas) {
        fitGraph(canvas);
      }
      return;
    }
    canvas.__panZoomReady = true;
    var svg = canvas.querySelector('.call-graph');
    var state = graphState(canvas);

    canvas.querySelectorAll('[data-graph-tool]').forEach(function (button) {
      button.addEventListener('click', function (event) {
        event.preventDefault();
        event.stopPropagation();
        var action = button.getAttribute('data-graph-tool');
        if (action === 'zoom-in') {
          zoomGraph(canvas, 1.18);
        } else if (action === 'zoom-out') {
          zoomGraph(canvas, 0.84);
        } else {
          fitGraph(canvas);
        }
      });
    });

    svg.addEventListener('wheel', function (event) {
      event.preventDefault();
      zoomGraph(canvas, event.deltaY < 0 ? 1.12 : 0.88, event);
    }, {passive: false});

    svg.addEventListener('pointerdown', function (event) {
      if (event.button !== 0) {
        return;
      }
      var point = pointInSvg(svg, event);
      state.dragging = true;
      state.moved = false;
      state.startX = point.x;
      state.startY = point.y;
      state.originX = state.x;
      state.originY = state.y;
      state.downItem = closestGraphItem(event.target);
      svg.classList.add('panning');
      svg.setPointerCapture(event.pointerId);
    });

    svg.addEventListener('pointermove', function (event) {
      if (!state.dragging) {
        return;
      }
      var point = pointInSvg(svg, event);
      var dx = point.x - state.startX;
      var dy = point.y - state.startY;
      if (Math.abs(dx) + Math.abs(dy) > 3) {
        state.moved = true;
      }
      state.x = state.originX + dx;
      state.y = state.originY + dy;
      applyGraphTransform(canvas);
    });

    function stopPan(event) {
      if (!state.dragging) {
        return;
      }
      state.dragging = false;
      svg.classList.remove('panning');
      try {
        svg.releasePointerCapture(event.pointerId);
      } catch (error) {}
      if (state.moved) {
        canvas.setAttribute('data-graph-dragged', 'true');
        setTimeout(function () {
          canvas.removeAttribute('data-graph-dragged');
        }, 0);
      } else if (state.downItem) {
        selectGraphItem(state.downItem);
      }
      state.downItem = null;
    }
    svg.addEventListener('pointerup', stopPan);
    svg.addEventListener('pointercancel', stopPan);

    fitGraph(canvas);
  }

  document.addEventListener('click', function (event) {
    var viewButton = event.target.closest('[data-view-button]');
    if (viewButton) {
      var process = viewButton.closest('.process');
      var view = viewButton.getAttribute('data-view-button');
      process.querySelectorAll('[data-view-button]').forEach(function (button) {
        button.classList.toggle('active', button === viewButton);
      });
      process.querySelectorAll('[data-view]').forEach(function (section) {
        section.hidden = section.getAttribute('data-view') !== view;
      });
      if (view === 'graph') {
        initializeGraph(process.querySelector('[data-pan-zoom]'));
      }
      return;
    }

    var canvas = event.target.closest('[data-pan-zoom]');
    if (canvas && canvas.getAttribute('data-graph-dragged') === 'true') {
      event.preventDefault();
      return;
    }

    var item = closestGraphItem(event.target);
    if (!item) {
      return;
    }
    selectGraphItem(item);
  });

  document.querySelectorAll('[data-view="graph"]:not([hidden]) [data-pan-zoom]').forEach(initializeGraph);
  setupProcessSearch();
  setupAuxiliaryPagination();
  window.addEventListener('resize', function () {
    document.querySelectorAll('[data-view="graph"]:not([hidden]) [data-pan-zoom]').forEach(fitGraph);
  });
})();
</script>
`)
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func renderStat(b *strings.Builder, label string, value int) {
	fmt.Fprintf(b, "<div class=\"stat\"><span class=\"subtle\">%s</span><strong>%d</strong></div>\n", h(label), value)
}

func renderProcessHTML(b *strings.Builder, project *model.Project, trace flow.Trace, sourceLines *sourceLineReader, sourceContexts *sourceContextStore) {
	entry := trace.EntryPoint
	typ := findType(project, entry.ClassID)
	method := findMethod(typ, entry.MethodID)
	unresolved := 0
	external := 0
	for _, step := range trace.Steps {
		if step.Resolution == "unresolved" {
			unresolved++
		}
		if step.Resolution == "external_import" {
			external++
		}
	}

	fmt.Fprintf(b, "<article class=\"process\" id=\"%s\">\n", h(htmlID(entry.ID)))
	b.WriteString("<div class=\"process-header\">\n")
	fmt.Fprintf(b, "<h2>%s</h2>\n", h(entry.Name))
	b.WriteString("<div class=\"meta-grid\">\n")
	renderMeta(b, "Tipo", entry.Kind)
	renderMeta(b, "Framework", valueOrUnknown(entry.Framework))
	renderMeta(b, "Classe", valueOrUnknown(typ.FQN))
	renderMeta(b, "Metodo", valueOrUnknown(method.Name))
	renderMeta(b, "Entrada", valueOrUnknown(entryLabel(entry)))
	renderMeta(b, "Evidencia", evidenceLabel(entry.Evidence))
	b.WriteString("</div>\n")
	b.WriteString("<div class=\"badges\">\n")
	fmt.Fprintf(b, "<span class=\"badge\">%d chamadas</span>", len(trace.Steps))
	fmt.Fprintf(b, "<span class=\"badge\">%d dependencias</span>", len(trace.Dependencies))
	fmt.Fprintf(b, "<span class=\"badge external_import\">%d imports externos</span>", external)
	fmt.Fprintf(b, "<span class=\"badge unresolved\">%d unresolved</span>", unresolved)
	if trace.Truncated {
		b.WriteString("<span class=\"badge medium\">truncado</span>")
	}
	b.WriteString("</div>\n</div>\n")
	b.WriteString("<div class=\"process-body\">\n")
	b.WriteString("<div class=\"view-controls\" aria-label=\"Alternar visualizacao do processo\">\n")
	b.WriteString("<button class=\"view-button active\" type=\"button\" data-view-button=\"text\">Texto</button>\n")
	b.WriteString("<button class=\"view-button\" type=\"button\" data-view-button=\"graph\">Grafo</button>\n")
	b.WriteString("</div>\n")
	b.WriteString("<div class=\"view\" data-view=\"text\">\n")

	b.WriteString("<h3>Fluxo tecnico transitivo</h3>\n")
	if len(trace.Steps) == 0 {
		b.WriteString("<div class=\"empty\">Sem chamadas detectadas.</div>\n")
	} else {
		b.WriteString("<div class=\"table-wrap\"><table><thead><tr>")
		b.WriteString("<th>Ordem</th><th>Prof.</th><th>Chamador</th><th>Chamada</th><th>Resolvido para</th><th>Resolucao</th><th>Evidencia</th>")
		b.WriteString("</tr></thead><tbody>\n")
		for _, step := range trace.Steps {
			fmt.Fprintf(b, "<tr><td>%d</td><td>%d</td><td>%s.%s</td><td><code>%s</code></td><td>%s</td><td><span class=\"badge %s\">%s/%s</span></td><td><code>%s</code></td></tr>\n",
				step.Order,
				step.Depth,
				h(valueOrUnknown(step.CallerClass)),
				h(valueOrUnknown(step.CallerMethod)),
				h(valueOrUnknown(step.Call)),
				h(valueOrUnknown(stepResolvedLabel(step))),
				h(cssClass(step.Resolution)),
				h(valueOrUnknown(step.Resolution)),
				h(string(step.Confidence)),
				h(evidenceLabel(step.Evidence)),
			)
		}
		b.WriteString("</tbody></table></div>\n")
	}

	b.WriteString("<h3>Dependencias no fluxo</h3>\n")
	if len(trace.Dependencies) == 0 {
		b.WriteString("<div class=\"empty\">Nenhuma dependencia vinculada ao fluxo.</div>\n")
	} else {
		b.WriteString("<div class=\"table-wrap\"><table><thead><tr>")
		b.WriteString("<th>Escopo</th><th>Recurso</th><th>Tipo</th><th>Detalhe</th><th>Via classe</th><th>Via metodo</th><th>Evidencia</th>")
		b.WriteString("</tr></thead><tbody>\n")
		for _, dep := range trace.Dependencies {
			fmt.Fprintf(b, "<tr><td>%s</td><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td></tr>\n",
				h(dep.Scope),
				h(dep.Dependency.Name),
				h(dep.Dependency.Kind),
				h(valueOrUnknown(dep.Dependency.Detail)),
				h(valueOrUnknown(dep.ViaClass)),
				h(valueOrUnknown(dep.ViaMethod)),
				h(evidenceLabel(dep.Dependency.Evidence)),
			)
		}
		b.WriteString("</tbody></table></div>\n")
	}

	if len(trace.ConfigProperties) > 0 {
		b.WriteString("<h3>Configuracoes vinculadas</h3>\n")
		b.WriteString("<div class=\"table-wrap\"><table><thead><tr>")
		b.WriteString("<th>Chave</th><th>Valor</th><th>Origem</th><th>Observacao</th>")
		b.WriteString("</tr></thead><tbody>\n")
		for _, prop := range trace.ConfigProperties {
			note := ""
			if prop.DefinedExternally {
				note = "definido externamente; variavel de ambiente nao resolvida"
			}
			if prop.Redacted {
				note = strings.TrimSpace(note + " valor sensivel redigido")
			}
			fmt.Fprintf(b, "<tr><td><code>%s</code></td><td>%s</td><td><code>%s</code></td><td>%s</td></tr>\n",
				h(prop.Key),
				h(prop.Value),
				h(evidenceLabel(prop.Evidence)),
				h(note),
			)
		}
		b.WriteString("</tbody></table></div>\n")
	}

	b.WriteString("</div>\n")
	b.WriteString("<div class=\"view\" data-view=\"graph\" hidden>\n")
	renderTraceGraphHTML(b, project, trace, sourceLines, sourceContexts)
	b.WriteString("</div>\n")
	b.WriteString("</div>\n</article>\n")
}

type traceGraphNode struct {
	ID             string
	Label          string
	Kind           string
	Depth          int
	Row            int
	AnchorID       string
	AnchorPosition string
	Evidence       model.Evidence
	Details        map[string]string
	X              int
	Y              int
	Width          int
	Height         int
}

type traceGraphEdge struct {
	ID       string
	From     string
	To       string
	Label    string
	Kind     string
	Details  map[string]string
	Evidence model.Evidence
}

type auxiliaryGraphStep struct {
	Order    int
	Call     string
	Category string
	Reason   string
	Evidence model.Evidence
}

type pendingGraphStep struct {
	Step        flow.Step
	NodeID      string
	ParentID    string
	CallerLabel string
	TargetLabel string
}

type conditionGraphNode struct {
	Use          flow.ConditionUse
	NodeID       string
	CallerLabel  string
	DisplayOrder int
	Visible      bool
}

type sourceContextStore struct {
	values map[string]string
	byKey  map[string]string
	next   int
}

type visualizationIndex struct {
	typeByID      map[string]model.Type
	typeByFQN     map[string]model.Type
	methodByID    map[string]model.Method
	methodTypeID  map[string]string
	methodsByFile map[string][]model.Method
}

type sourceLineReader struct {
	files map[string][]string
}

func newSourceContextStore() *sourceContextStore {
	return &sourceContextStore{
		values: map[string]string{},
		byKey:  map[string]string{},
	}
}

func (s *sourceContextStore) ref(kind, key, content string) string {
	if s == nil || strings.TrimSpace(content) == "" {
		return ""
	}
	if key == "" {
		key = fmt.Sprintf("%s:%d", kind, s.next+1)
	}
	cacheKey := kind + "\x00" + key
	if id := s.byKey[cacheKey]; id != "" {
		return sourceContextRef(id)
	}
	s.next++
	id := fmt.Sprintf("%s-%d", safeFileName(kind), s.next)
	s.byKey[cacheKey] = id
	s.values[id] = content
	return sourceContextRef(id)
}

func (s *sourceContextStore) json() string {
	if s == nil || len(s.values) == 0 {
		return "{}"
	}
	data, err := json.Marshal(s.values)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func sourceContextRef(id string) string {
	if id == "" {
		return ""
	}
	return "source-context:" + id
}

func sourceContextID(value string) string {
	return strings.TrimPrefix(value, "source-context:")
}

func isSourceContextRef(value string) bool {
	return strings.HasPrefix(value, "source-context:")
}

func newSourceLineReader() *sourceLineReader {
	return &sourceLineReader{files: map[string][]string{}}
}

func (r *sourceLineReader) fileLines(path string) []string {
	if r == nil || path == "" {
		return nil
	}
	path = filepath.Clean(path)
	lines, ok := r.files[path]
	if ok {
		return lines
	}
	data, err := os.ReadFile(path)
	if err != nil {
		r.files[path] = nil
		return nil
	}
	lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	r.files[path] = lines
	return lines
}

func (r *sourceLineReader) line(evidence model.Evidence) string {
	if r == nil || evidence.Path == "" || evidence.Line <= 0 {
		return ""
	}
	lines := r.fileLines(evidence.Path)
	if evidence.Line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[evidence.Line-1])
}

func (r *sourceLineReader) rangeText(path string, start, end int) string {
	lines := r.fileLines(path)
	if len(lines) == 0 {
		return ""
	}
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

func (r *sourceLineReader) methodContent(method model.Method, fallbackPath string) string {
	path := method.Evidence.Path
	if path == "" {
		path = fallbackPath
	}
	start := method.StartLine
	if start <= 0 {
		start = method.Evidence.Line
	}
	return r.rangeText(path, start, method.EndLine)
}

func (r *sourceLineReader) classContent(typ model.Type, methods []model.Method) string {
	lines := r.fileLines(typ.FilePath)
	if len(lines) == 0 {
		return ""
	}
	start := 1
	end := typ.EndLine
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}

	type omittedRange struct {
		start int
		end   int
	}
	var omitted []omittedRange
	sanitizedLines := map[int]string{}
	for _, method := range methods {
		if method.BodyLine <= 0 {
			continue
		}
		sanitizedLines[method.BodyLine] = sanitizeMethodBodyLine(lineAt(lines, method.BodyLine), method.BodyLine == method.EndLine)
		if method.EndLine <= method.BodyLine+1 {
			continue
		}
		omitted = append(omitted, omittedRange{
			start: method.BodyLine + 1,
			end:   method.EndLine - 1,
		})
	}
	sort.Slice(omitted, func(i, j int) bool {
		return omitted[i].start < omitted[j].start
	})

	var out []string
	rangeIdx := 0
	for line := start; line <= end; line++ {
		for rangeIdx < len(omitted) && line > omitted[rangeIdx].end {
			rangeIdx++
		}
		if rangeIdx < len(omitted) && line >= omitted[rangeIdx].start && line <= omitted[rangeIdx].end {
			continue
		}
		if sanitized, ok := sanitizedLines[line]; ok {
			out = append(out, sanitized)
			continue
		}
		out = append(out, lines[line-1])
	}
	return strings.Join(out, "\n")
}

func lineAt(lines []string, line int) string {
	if line <= 0 || line > len(lines) {
		return ""
	}
	return lines[line-1]
}

func leadingWhitespace(value string) string {
	for i, r := range value {
		if r != ' ' && r != '\t' {
			return value[:i]
		}
	}
	return value
}

func sanitizeMethodBodyLine(value string, oneLine bool) string {
	open := strings.Index(value, "{")
	if open < 0 {
		return value
	}
	prefix := strings.TrimRight(value[:open+1], " \t")
	omitted := " // ... conteudo interno do metodo omitido ..."
	if !oneLine {
		return prefix + omitted
	}
	close := strings.LastIndex(value, "}")
	if close <= open {
		return prefix + omitted
	}
	return prefix + omitted + " " + strings.TrimSpace(value[close:])
}

const (
	graphViewportW  = 1200
	graphViewportH  = 720
	graphNodeW      = 420
	graphNodeH      = 96
	graphClassNodeW = 340
	graphClassNodeH = 68
	graphDepNodeW   = 300
	graphDepNodeH   = 74
)

func renderTraceGraphHTML(b *strings.Builder, project *model.Project, trace flow.Trace, sourceLines *sourceLineReader, sourceContexts *sourceContextStore) {
	nodes, edges, auxiliary := buildTraceGraph(project, trace, sourceLines, sourceContexts)
	if len(nodes) == 0 {
		b.WriteString("<div class=\"empty\">Sem dados suficientes para gerar grafo.</div>\n")
		return
	}
	width, height := layoutTraceGraph(nodes)
	nodeByID := map[string]*traceGraphNode{}
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}

	b.WriteString("<div class=\"graph-layout\">\n")
	fmt.Fprintf(b, "<div class=\"graph-canvas\" data-pan-zoom data-graph-width=\"%d\" data-graph-height=\"%d\">\n", width, height)
	b.WriteString("<div class=\"graph-toolbar\"><button class=\"graph-tool\" type=\"button\" data-graph-tool=\"zoom-in\">+</button><button class=\"graph-tool\" type=\"button\" data-graph-tool=\"zoom-out\">-</button><button class=\"graph-tool\" type=\"button\" data-graph-tool=\"reset\">Reset</button></div>\n")
	b.WriteString("<div class=\"graph-hint\">Arraste para navegar. Use scroll para zoom.</div>\n")
	fmt.Fprintf(b, "<svg class=\"call-graph\" viewBox=\"0 0 %d %d\" role=\"img\" aria-label=\"Grafo de chamadas do processo\">\n", graphViewportW, graphViewportH)
	arrowID := htmlID(trace.EntryPoint.ID) + "-arrow"
	fmt.Fprintf(b, "<defs><marker id=\"%s\" markerWidth=\"10\" markerHeight=\"10\" refX=\"9\" refY=\"3\" orient=\"auto\" markerUnits=\"strokeWidth\"><path d=\"M0,0 L0,6 L9,3 z\" fill=\"#9aa0a6\"></path></marker></defs>\n", h(arrowID))
	b.WriteString("<g class=\"graph-viewport\">\n")
	b.WriteString("<g class=\"graph-edges\">\n")
	for _, edge := range edges {
		from := nodeByID[edge.From]
		to := nodeByID[edge.To]
		if from == nil || to == nil {
			continue
		}
		renderTraceGraphEdge(b, edge, from, to, arrowID)
	}
	b.WriteString("</g>\n<g class=\"graph-nodes\">\n")
	for i, node := range nodes {
		renderTraceGraphNode(b, node, i == 0)
	}
	b.WriteString("</g>\n</g>\n</svg>\n</div>\n")
	b.WriteString("<div class=\"node-inspector\"><h3>Item selecionado</h3><div class=\"inspector-content\">")
	renderInspectorHTML(b, nodes[0].Details, sourceContexts)
	b.WriteString("</div></div>\n</div>\n")
	renderAuxiliaryGraphSteps(b, auxiliary, sourceLines)
}

func buildTraceGraph(project *model.Project, trace flow.Trace, sourceLines *sourceLineReader, sourceContexts *sourceContextStore) ([]*traceGraphNode, []traceGraphEdge, []auxiliaryGraphStep) {
	idx := newVisualizationIndex(project)
	nodeByID := map[string]*traceGraphNode{}
	var nodes []*traceGraphNode
	var edges []traceGraphEdge
	var auxiliary []auxiliaryGraphStep
	var pendingSteps []pendingGraphStep
	edgeSeen := map[string]bool{}
	occupiedRows := map[int]bool{0: true}
	maxRowUsed := 0

	reserveRow := func(row int) int {
		if row < 0 {
			row = 0
		}
		for occupiedRows[row] {
			row++
		}
		occupiedRows[row] = true
		if row > maxRowUsed {
			maxRowUsed = row
		}
		return row
	}

	addNode := func(id, label, kind string, depth, row int, evidence model.Evidence, details map[string]string) *traceGraphNode {
		if existing := nodeByID[id]; existing != nil {
			if depth < existing.Depth {
				existing.Depth = depth
			}
			if row < existing.Row {
				existing.Row = row
			}
			return existing
		}
		node := &traceGraphNode{
			ID:       id,
			Label:    label,
			Kind:     kind,
			Depth:    depth,
			Row:      row,
			Evidence: evidence,
			Details:  details,
		}
		node.Width, node.Height = graphNodeSize(kind)
		nodeByID[id] = node
		nodes = append(nodes, node)
		return node
	}
	addEdge := func(from, to, label, kind string, evidence model.Evidence, details map[string]string) {
		id := strings.Join([]string{from, to, label, evidence.Path, fmt.Sprint(evidence.Line)}, "\x00")
		if from == "" || to == "" || edgeSeen[id] {
			return
		}
		edgeSeen[id] = true
		edges = append(edges, traceGraphEdge{
			ID:       id,
			From:     from,
			To:       to,
			Label:    label,
			Kind:     kind,
			Evidence: evidence,
			Details:  details,
		})
	}

	entryID := "entry:" + trace.EntryPoint.ID
	entryDetails := map[string]string{
		"Tipo":            "entrypoint",
		"Nome":            entryMethodDisplayName(trace.EntryPoint),
		"Kind":            trace.EntryPoint.Kind,
		"Entrada":         entryLabel(trace.EntryPoint),
		"Evidencia":       evidenceLabel(trace.EntryPoint.Evidence),
		"Linha de codigo": sourceLines.line(trace.EntryPoint.Evidence),
	}
	entryType := idx.typeByID[trace.EntryPoint.ClassID]
	entryMethod := idx.methodByID[trace.EntryPoint.MethodID]
	addMethodContent(entryDetails, sourceLines, sourceContexts, entryType, entryMethod)
	addNode(entryID, entryMethodDisplayName(trace.EntryPoint), "entry", 0, 0, trace.EntryPoint.Evidence, entryDetails)
	addClassContextNode(addNode, addEdge, sourceLines, sourceContexts, idx, entryID, entryType, trace.EntryPoint.Evidence)

	if len(trace.Steps) == 0 && len(trace.Conditions) == 0 && len(trace.Dependencies) == 0 {
		return nodes, edges, auxiliary
	}

	stack := map[int]string{0: entryID}
	stepNodeByMethod := map[string]string{}
	if entryType.FQN != "" && entryMethod.Name != "" {
		stepNodeByMethod[methodLabel(entryType.FQN, entryMethod.Name)] = entryID
	}
	maxDepth := 0
	for _, step := range trace.Steps {
		if category, reason, ok := classifyAuxiliaryStep(step); ok {
			auxiliary = append(auxiliary, auxiliaryGraphStep{
				Order:    step.Order,
				Call:     step.Call,
				Category: category,
				Reason:   reason,
				Evidence: step.Evidence,
			})
			continue
		}
		callerLabel := methodLabel(step.CallerClass, step.CallerMethod)
		targetKey := stepResolvedLabel(step)
		targetLabel := stepMethodDisplayLabel(step)
		if targetLabel == "" {
			targetLabel = step.Call
		}
		targetKind := "method"
		targetID := fmt.Sprintf("step:%d", step.Order)
		switch step.Resolution {
		case "external_import":
			targetKind = "external"
		case "unresolved":
			targetKind = "unresolved"
		}
		if step.Depth > maxDepth {
			maxDepth = step.Depth
		}
		row := reserveRow(step.Order)
		displayLabel := fmt.Sprintf("#%d %s", step.Order, targetLabel)
		targetType, targetMethod := resolvedStepContext(idx, step)
		nodeDetails := map[string]string{
			"Ordem":           fmt.Sprint(step.Order),
			"Profundidade":    fmt.Sprint(step.Depth),
			"Tipo":            graphKindLabel(targetKind),
			"Nome":            targetLabel,
			"Classe":          step.ResolvedClass,
			"Chamada":         step.Call,
			"Chamador":        callerLabel,
			"Resolucao":       valueOrUnknown(step.Resolution),
			"Confianca":       string(step.Confidence),
			"ResolvedClass":   step.ResolvedClass,
			"ResolvedMethod":  step.ResolvedMethod,
			"Evidencia":       evidenceLabel(step.Evidence),
			"Linha de codigo": sourceLines.line(step.Evidence),
		}
		if targetKind == "method" {
			addMethodContent(nodeDetails, sourceLines, sourceContexts, targetType, targetMethod)
		} else if targetType.ID != "" {
			addClassContent(nodeDetails, sourceLines, sourceContexts, idx, targetType)
		}
		addNode(targetID, displayLabel, targetKind, step.Depth, row, step.Evidence, nodeDetails)
		addClassContextNode(addNode, addEdge, sourceLines, sourceContexts, idx, targetID, targetTypeForClassNode(targetType, step), step.Evidence)
		parentID := stack[step.Depth-1]
		if parentID == "" {
			parentID = entryID
		}
		pendingSteps = append(pendingSteps, pendingGraphStep{
			Step:        step,
			NodeID:      targetID,
			ParentID:    parentID,
			CallerLabel: callerLabel,
			TargetLabel: targetLabel,
		})
		stack[step.Depth] = targetID
		for depth := range stack {
			if depth > step.Depth {
				delete(stack, depth)
			}
		}
		if targetKey != "unresolved" {
			stepNodeByMethod[targetKey] = targetID
		}
		stepNodeByMethod[callerLabel] = parentID
	}

	conditions := make([]conditionGraphNode, 0, len(trace.Conditions))
	for i, conditionUse := range trace.Conditions {
		condition := conditionUse.Condition
		conditions = append(conditions, conditionGraphNode{
			Use:          conditionUse,
			NodeID:       fmt.Sprintf("condition:%s:%d", condition.ID, i+1),
			CallerLabel:  methodLabel(conditionUse.CallerClass, conditionUse.CallerMethod),
			DisplayOrder: i + 1,
		})
	}
	markVisibleConditions(conditions)

	visibleOrder := 0
	for i := range conditions {
		if !conditions[i].Visible {
			continue
		}
		visibleOrder++
		conditions[i].DisplayOrder = visibleOrder
		conditionUse := conditions[i].Use
		condition := conditionUse.Condition
		conditionDepth := conditionUse.Depth
		if conditionDepth <= 0 {
			conditionDepth = 1
		}
		if conditionDepth > maxDepth {
			maxDepth = conditionDepth
		}
		conditionIndex := conditions[i].DisplayOrder
		conditionRow := conditionPreferredRow(conditionUse, pendingSteps)
		conditionLabel := condition.Kind
		if condition.Expression != "" {
			conditionLabel = condition.Kind + " " + condition.Expression
		}
		nodeID := conditions[i].NodeID
		displayLabel := fmt.Sprintf("C%d %s", conditionIndex, conditionLabel)
		callerLabel := conditions[i].CallerLabel
		parentID, _ := nearestVisibleParentConditionNode(conditions, conditions[i])
		if parentID == "" {
			parentID = stepNodeByMethod[callerLabel]
		}
		if parentID == "" {
			parentID = entryID
		}
		conditionDetails := map[string]string{
			"Ordem":           fmt.Sprintf("C%d", conditionIndex),
			"Profundidade":    fmt.Sprint(conditionDepth),
			"Tipo":            graphKindLabel("condition"),
			"Estrutura":       condition.Kind,
			"Expressao":       condition.Expression,
			"Metodo":          callerLabel,
			"Confianca":       string(condition.Confidence),
			"Evidencia":       evidenceLabel(condition.Evidence),
			"Linha de codigo": sourceLines.line(condition.Evidence),
		}
		addNode(nodeID, displayLabel, "condition", conditionDepth, conditionRow, condition.Evidence, conditionDetails)
		addEdge(parentID, nodeID, fmt.Sprintf("C%d %s", conditionIndex, condition.Kind), "condition", condition.Evidence, map[string]string{
			"Ordem":           fmt.Sprintf("C%d", conditionIndex),
			"Tipo":            "condicao",
			"Estrutura":       condition.Kind,
			"Expressao":       condition.Expression,
			"Metodo":          callerLabel,
			"Confianca":       string(condition.Confidence),
			"Evidencia":       evidenceLabel(condition.Evidence),
			"Linha de codigo": sourceLines.line(condition.Evidence),
		})
	}

	for _, pending := range pendingSteps {
		step := pending.Step
		parentID := pending.ParentID
		if conditionID, conditionDepth := nearestVisibleConditionNode(conditions, pending.CallerLabel, step.Evidence, ""); conditionID != "" {
			parentID = conditionID
			if node := nodeByID[pending.NodeID]; node != nil && node.Depth <= conditionDepth {
				node.Depth = conditionDepth + 1
			}
		}
		addEdge(parentID, pending.NodeID, fmt.Sprintf("#%d", step.Order), "call", step.Evidence, map[string]string{
			"Ordem":           fmt.Sprint(step.Order),
			"Profundidade":    fmt.Sprint(step.Depth),
			"Tipo":            "chamada",
			"Chamada":         step.Call,
			"Chamador":        pending.CallerLabel,
			"Resolvido para":  pending.TargetLabel,
			"Resolucao":       valueOrUnknown(step.Resolution),
			"Confianca":       string(step.Confidence),
			"Evidencia":       evidenceLabel(step.Evidence),
			"Linha de codigo": sourceLines.line(step.Evidence),
		})
	}

	for _, dep := range trace.Dependencies {
		viaLabel := methodLabel(dep.ViaClass, dep.ViaMethod)
		viaID, _ := nearestVisibleConditionNode(conditions, viaLabel, dep.Dependency.Evidence, "")
		if viaID == "" {
			viaID = stepNodeByMethod[viaLabel]
		}
		if viaID == "" {
			viaID = entryID
		}
		depID := "dependency:" + dep.Dependency.ID
		depDetails := map[string]string{
			"Tipo":            dep.Dependency.Kind,
			"Nome":            dep.Dependency.Name,
			"Escopo":          dep.Scope,
			"Detalhe":         dep.Dependency.Detail,
			"Via classe":      dep.ViaClass,
			"Via metodo":      dep.ViaMethod,
			"Layout":          "dependencia-satelite",
			"Confianca":       string(dep.Dependency.Confidence),
			"Evidencia":       evidenceLabel(dep.Dependency.Evidence),
			"Linha de codigo": sourceLines.line(dep.Dependency.Evidence),
		}
		depNode := addNode(depID, dep.Dependency.Name, "dependency", 0, 0, dep.Dependency.Evidence, depDetails)
		depNode.AnchorID = viaID
		depNode.AnchorPosition = "dependency"
		addEdge(viaID, depID, dep.Dependency.Kind, "dependency", dep.Dependency.Evidence, map[string]string{
			"Tipo":            "dependencia",
			"Recurso":         dep.Dependency.Name,
			"Kind":            dep.Dependency.Kind,
			"Escopo":          dep.Scope,
			"Detalhe":         dep.Dependency.Detail,
			"Via classe":      dep.ViaClass,
			"Via metodo":      dep.ViaMethod,
			"Layout":          "dependencia-satelite",
			"Evidencia":       evidenceLabel(dep.Dependency.Evidence),
			"Linha de codigo": sourceLines.line(dep.Dependency.Evidence),
		})
	}

	return nodes, edges, auxiliary
}

func newVisualizationIndex(project *model.Project) visualizationIndex {
	idx := visualizationIndex{
		typeByID:      map[string]model.Type{},
		typeByFQN:     map[string]model.Type{},
		methodByID:    map[string]model.Method{},
		methodTypeID:  map[string]string{},
		methodsByFile: map[string][]model.Method{},
	}
	if project == nil {
		return idx
	}
	for _, typ := range project.Types {
		idx.typeByID[typ.ID] = typ
		if typ.FQN != "" {
			idx.typeByFQN[typ.FQN] = typ
		}
		for _, method := range typ.Methods {
			idx.methodByID[method.ID] = method
			idx.methodTypeID[method.ID] = typ.ID
			path := filepath.Clean(typ.FilePath)
			idx.methodsByFile[path] = append(idx.methodsByFile[path], method)
		}
	}
	return idx
}

func (idx visualizationIndex) methodContext(methodID string) (model.Type, model.Method) {
	if methodID == "" {
		return model.Type{}, model.Method{}
	}
	method := idx.methodByID[methodID]
	if method.ID == "" {
		return model.Type{}, model.Method{}
	}
	return idx.typeByID[idx.methodTypeID[methodID]], method
}

func resolvedStepContext(idx visualizationIndex, step flow.Step) (model.Type, model.Method) {
	if step.ResolvedMethodID != "" {
		return idx.methodContext(step.ResolvedMethodID)
	}
	typ := idx.typeByFQN[step.ResolvedClass]
	if typ.ID == "" || step.ResolvedMethod == "" {
		return typ, model.Method{}
	}
	for _, method := range typ.Methods {
		if method.Name == step.ResolvedMethod {
			return typ, method
		}
	}
	return typ, model.Method{}
}

func addClassContextNode(
	addNode func(string, string, string, int, int, model.Evidence, map[string]string) *traceGraphNode,
	addEdge func(string, string, string, string, model.Evidence, map[string]string),
	sourceLines *sourceLineReader,
	sourceContexts *sourceContextStore,
	idx visualizationIndex,
	anchorID string,
	typ model.Type,
	evidence model.Evidence,
) {
	className := typ.FQN
	if className == "" {
		className = typ.Name
	}
	if anchorID == "" || className == "" {
		return
	}
	details := map[string]string{
		"Tipo":      "classe",
		"Classe":    className,
		"Evidencia": evidenceLabel(evidence),
	}
	addClassContent(details, sourceLines, sourceContexts, idx, typ)
	classID := "class-context:" + safeFileName(className)
	node := addNode(classID, shortTypeName(className), "class", 0, 0, evidence, details)
	if node.AnchorID == "" {
		node.AnchorID = anchorID
		node.AnchorPosition = "class"
	}
	addEdge(classID, anchorID, "classe", "declares", evidence, map[string]string{
		"Tipo":      "classe-metodo",
		"Classe":    className,
		"Evidencia": evidenceLabel(evidence),
	})
}

func targetTypeForClassNode(typ model.Type, step flow.Step) model.Type {
	if typ.ID != "" || typ.FQN != "" || typ.Name != "" {
		return typ
	}
	if step.ResolvedClass == "" {
		return model.Type{}
	}
	return model.Type{
		Name:       shortTypeName(step.ResolvedClass),
		FQN:        step.ResolvedClass,
		Evidence:   step.Evidence,
		Source:     step.Source,
		Confidence: step.Confidence,
	}
}

func (idx visualizationIndex) methodsInsideType(typ model.Type) []model.Method {
	if typ.FilePath == "" {
		return typ.Methods
	}
	path := filepath.Clean(typ.FilePath)
	candidates := idx.methodsByFile[path]
	if len(candidates) == 0 {
		return typ.Methods
	}
	start := typ.StartLine
	if start <= 0 {
		start = typ.Evidence.Line
	}
	end := typ.EndLine
	var methods []model.Method
	for _, method := range candidates {
		methodStart := method.StartLine
		if methodStart <= 0 {
			methodStart = method.Evidence.Line
		}
		if start > 0 && methodStart < start {
			continue
		}
		if end > 0 && method.EndLine > end {
			continue
		}
		methods = append(methods, method)
	}
	return methods
}

func addMethodContent(details map[string]string, sourceLines *sourceLineReader, sourceContexts *sourceContextStore, typ model.Type, method model.Method) {
	if details == nil || sourceLines == nil {
		return
	}
	if method.ID == "" {
		return
	}
	if content := sourceLines.methodContent(method, typ.FilePath); content != "" {
		details["Conteudo"] = sourceContexts.ref("method", method.ID, content)
	}
}

func addClassContent(details map[string]string, sourceLines *sourceLineReader, sourceContexts *sourceContextStore, idx visualizationIndex, typ model.Type) {
	if details == nil || sourceLines == nil || typ.ID == "" {
		return
	}
	if content := sourceLines.classContent(typ, idx.methodsInsideType(typ)); content != "" {
		details["Conteudo"] = sourceContexts.ref("class", typ.ID, content)
	}
}

func markVisibleConditions(conditions []conditionGraphNode) {
	for i := range conditions {
		conditions[i].Visible = true
	}
}

func nearestVisibleConditionNode(conditions []conditionGraphNode, callerLabel string, evidence model.Evidence, excludeID string) (string, int) {
	bestIndex := -1
	bestSpan := 0
	for i, conditionNode := range conditions {
		if !conditionNode.Visible || conditionNode.NodeID == excludeID || conditionNode.CallerLabel != callerLabel {
			continue
		}
		if !conditionContainsEvidence(conditionNode.Use.Condition, evidence) {
			continue
		}
		span := conditionSpan(conditionNode.Use.Condition)
		if bestIndex < 0 || span < bestSpan || (span == bestSpan && conditionStartLine(conditionNode.Use.Condition) > conditionStartLine(conditions[bestIndex].Use.Condition)) {
			bestIndex = i
			bestSpan = span
		}
	}
	if bestIndex < 0 {
		return "", 0
	}
	depth := conditions[bestIndex].Use.Depth
	if depth <= 0 {
		depth = 1
	}
	return conditions[bestIndex].NodeID, depth
}

func nearestVisibleParentConditionNode(conditions []conditionGraphNode, child conditionGraphNode) (string, int) {
	bestIndex := -1
	bestSpan := 0
	for i, conditionNode := range conditions {
		if !conditionNode.Visible || conditionNode.NodeID == child.NodeID || conditionNode.CallerLabel != child.CallerLabel {
			continue
		}
		if !conditionContainsCondition(conditionNode.Use.Condition, child.Use.Condition) {
			continue
		}
		span := conditionSpan(conditionNode.Use.Condition)
		if bestIndex < 0 || span < bestSpan {
			bestIndex = i
			bestSpan = span
		}
	}
	if bestIndex < 0 {
		return "", 0
	}
	depth := conditions[bestIndex].Use.Depth
	if depth <= 0 {
		depth = 1
	}
	return conditions[bestIndex].NodeID, depth
}

func conditionContainsEvidence(condition model.Condition, evidence model.Evidence) bool {
	if evidence.Line <= 0 || !conditionPathMatches(condition, evidence) {
		return false
	}
	start := conditionStartLine(condition)
	end := conditionEndLine(condition)
	return start > 0 && end >= start && evidence.Line >= start && evidence.Line <= end
}

func conditionContainsCondition(parent model.Condition, child model.Condition) bool {
	if parent.ID == "" || parent.ID == child.ID || !conditionPathMatches(parent, child.Evidence) {
		return false
	}
	parentStart := conditionStartLine(parent)
	parentEnd := conditionEndLine(parent)
	childStart := conditionStartLine(child)
	childEnd := conditionEndLine(child)
	if parentStart <= 0 || parentEnd < parentStart || childStart <= 0 || childEnd < childStart {
		return false
	}
	if childStart < parentStart || childEnd > parentEnd {
		return false
	}
	return childStart > parentStart || childEnd < parentEnd
}

func conditionPathMatches(condition model.Condition, evidence model.Evidence) bool {
	if condition.Evidence.Path == "" || evidence.Path == "" {
		return true
	}
	return filepath.Clean(condition.Evidence.Path) == filepath.Clean(evidence.Path)
}

func conditionStartLine(condition model.Condition) int {
	if condition.StartLine > 0 {
		return condition.StartLine
	}
	if condition.Evidence.Line > 0 {
		return condition.Evidence.Line
	}
	return condition.BodyLine
}

func conditionEndLine(condition model.Condition) int {
	if condition.EndLine > 0 {
		return condition.EndLine
	}
	if condition.BodyLine > 0 {
		return condition.BodyLine
	}
	if condition.Evidence.Line > 0 {
		return condition.Evidence.Line
	}
	return condition.StartLine
}

func conditionSpan(condition model.Condition) int {
	start := conditionStartLine(condition)
	end := conditionEndLine(condition)
	if start <= 0 || end < start {
		return 0
	}
	return end - start
}

func conditionPreferredRow(conditionUse flow.ConditionUse, steps []pendingGraphStep) int {
	condition := conditionUse.Condition
	callerLabel := methodLabel(conditionUse.CallerClass, conditionUse.CallerMethod)
	preferred := 1
	for _, pending := range steps {
		step := pending.Step
		if pending.CallerLabel != callerLabel {
			continue
		}
		if !conditionPathMatches(condition, step.Evidence) {
			continue
		}
		if conditionContainsEvidence(condition, step.Evidence) {
			return step.Order
		}
		if step.Evidence.Line > 0 && conditionStartLine(condition) > 0 && step.Evidence.Line < conditionStartLine(condition) {
			preferred = step.Order + 1
		}
	}
	return preferred
}

func layoutTraceGraph(nodes []*traceGraphNode) (int, int) {
	const (
		left          = 42
		top           = 84
		columnGap     = 96
		rowGap        = 56
		satelliteGap  = 10
		dependencyGap = 10
	)
	maxDepth := 0
	maxRow := 0
	maxX := 0
	maxY := 0
	nodeByID := map[string]*traceGraphNode{}
	satellites := map[string][]*traceGraphNode{}
	columnWidths := map[int]int{}
	rowHeights := map[int]int{}
	rowTopExtras := map[int]int{}
	rowBottomExtras := map[int]int{}
	for _, node := range nodes {
		if node.Width <= 0 || node.Height <= 0 {
			node.Width, node.Height = graphNodeSize(node.Kind)
		}
		nodeByID[node.ID] = node
		if node.AnchorID != "" {
			satellites[node.AnchorID] = append(satellites[node.AnchorID], node)
			continue
		}
		if node.Depth < 0 {
			node.Depth = 0
		}
		if node.Row < 0 {
			node.Row = 0
		}
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
		if node.Row > maxRow {
			maxRow = node.Row
		}
		if node.Width > columnWidths[node.Depth] {
			columnWidths[node.Depth] = node.Width
		}
		if node.Height > rowHeights[node.Row] {
			rowHeights[node.Row] = node.Height
		}
	}

	for anchorID, group := range satellites {
		anchor := nodeByID[anchorID]
		if anchor == nil {
			continue
		}
		classCount := 0
		depCount := 0
		maxClassH := 0
		maxDepH := 0
		for _, node := range group {
			switch node.AnchorPosition {
			case "class":
				classCount++
				if node.Height > maxClassH {
					maxClassH = node.Height
				}
			default:
				depCount++
				if node.Height > maxDepH {
					maxDepH = node.Height
				}
			}
		}
		if classCount > 0 {
			extra := classCount*maxClassH + classCount*satelliteGap
			if extra > rowTopExtras[anchor.Row] {
				rowTopExtras[anchor.Row] = extra
			}
		}
		if depCount > 0 {
			depRows := (depCount + 1) / 2
			extra := satelliteGap + depRows*maxDepH + (depRows-1)*dependencyGap
			if extra > rowBottomExtras[anchor.Row] {
				rowBottomExtras[anchor.Row] = extra
			}
		}
	}

	columnX := map[int]int{}
	nextX := left
	for depth := 0; depth <= maxDepth; depth++ {
		width := columnWidths[depth]
		if width <= 0 {
			width = graphNodeW
		}
		columnX[depth] = nextX
		nextX += width + columnGap
	}

	rowY := map[int]int{}
	nextY := top
	for row := 0; row <= maxRow; row++ {
		height := rowHeights[row]
		if height <= 0 {
			height = graphNodeH
		}
		rowY[row] = nextY
		nextY += rowTopExtras[row] + height + rowBottomExtras[row] + rowGap
	}

	for _, node := range nodes {
		if node.AnchorID != "" {
			continue
		}
		node.X = columnX[node.Depth]
		node.Y = rowY[node.Row] + rowTopExtras[node.Row]
		if node.X+node.Width > maxX {
			maxX = node.X + node.Width
		}
		if node.Y+node.Height > maxY {
			maxY = node.Y + node.Height
		}
	}

	for anchorID, group := range satellites {
		anchor := nodeByID[anchorID]
		if anchor == nil {
			continue
		}
		classIndex := 0
		depIndex := 0
		for _, node := range group {
			switch node.AnchorPosition {
			case "class":
				node.X = anchor.X + (anchor.Width-node.Width)/2
				node.Y = anchor.Y - (classIndex+1)*(node.Height+satelliteGap)
				classIndex++
			default:
				col := depIndex % 2
				row := depIndex / 2
				node.X = anchor.X + 20 + col*(node.Width+dependencyGap)
				node.Y = anchor.Y + anchor.Height + satelliteGap + row*(node.Height+dependencyGap)
				depIndex++
			}
			if node.X+node.Width > maxX {
				maxX = node.X + node.Width
			}
			if node.Y+node.Height > maxY {
				maxY = node.Y + node.Height
			}
		}
	}
	if maxX == 0 {
		maxX = left + graphNodeW
	}
	if maxY == 0 {
		maxY = top + graphNodeH
	}
	return maxX + 120, maxY + 80
}

func renderTraceGraphNode(b *strings.Builder, node *traceGraphNode, selected bool) {
	classes := "graph-node " + cssClass(node.Kind)
	if selected {
		classes += " selected"
	}
	fmt.Fprintf(b, "<g class=\"%s\" data-graph-info=\"%s\" tabindex=\"0\">\n", h(classes), graphPayloadAttr(node.Details))
	fmt.Fprintf(b, "<title>%s</title>\n", h(node.Label))
	fmt.Fprintf(b, "<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" rx=\"8\"></rect>\n", node.X, node.Y, node.Width, node.Height)
	renderNodeLabel(b, node.X+14, node.Y+22, node.Label, nodeLabelLimit(node.Width))
	fmt.Fprintf(b, "<text class=\"node-kind\" x=\"%d\" y=\"%d\">%s</text>\n", node.X+14, node.Y+node.Height-18, h(graphKindLabel(node.Kind)))
	b.WriteString("</g>\n")
}

func renderTraceGraphEdge(b *strings.Builder, edge traceGraphEdge, from, to *traceGraphNode, arrowID string) {
	x1 := from.X + from.Width
	y1 := from.Y + from.Height/2
	x2 := to.X
	y2 := to.Y + to.Height/2
	labelX := (x1 + x2) / 2
	labelY := (y1 + y2) / 2
	path := ""
	if edge.Kind == "declares" {
		x1 = from.X + from.Width/2
		y1 = from.Y + from.Height
		x2 = to.X + to.Width/2
		y2 = to.Y
		labelX = (x1 + x2) / 2
		labelY = (y1 + y2) / 2
		path = fmt.Sprintf("M%d,%d C%d,%d %d,%d %d,%d", x1, y1, x1, y1+18, x2, y2-18, x2, y2)
	} else if edge.Kind == "dependency" {
		x1 = from.X + from.Width/2
		y1 = from.Y + from.Height
		x2 = to.X + to.Width/2
		y2 = to.Y
		labelX = (x1 + x2) / 2
		labelY = (y1 + y2) / 2
		c1 := y1 + 28
		c2 := y2 - 28
		if c2 < c1 {
			c1 = y1 + 18
			c2 = y2 - 18
		}
		path = fmt.Sprintf("M%d,%d C%d,%d %d,%d %d,%d", x1, y1, x1, c1, x2, c2, x2, y2)
	} else if from.ID == to.ID {
		x := from.X + from.Width
		y := from.Y + 8
		path = fmt.Sprintf("M%d,%d C%d,%d %d,%d %d,%d", x, y, x+86, y-64, x+86, y+86, x, y+50)
		labelX = x + 72
		labelY = y + 4
	} else {
		c1 := x1 + 86
		c2 := x2 - 86
		if c2 < c1 {
			c1 = x1 + 44
			c2 = x2 - 44
		}
		path = fmt.Sprintf("M%d,%d C%d,%d %d,%d %d,%d", x1, y1, c1, y1, c2, y2, x2, y2)
	}
	fmt.Fprintf(b, "<g class=\"graph-edge %s\" data-graph-info=\"%s\">\n", h(cssClass(edge.Kind)), graphPayloadAttr(edge.Details))
	fmt.Fprintf(b, "<path d=\"%s\" marker-end=\"url(#%s)\"></path>\n", h(path), h(arrowID))
	fmt.Fprintf(b, "<text class=\"graph-edge-label\" x=\"%d\" y=\"%d\">%s</text>\n", labelX, labelY-6, h(shorten(edge.Label, 46)))
	b.WriteString("</g>\n")
}

func renderAuxiliaryGraphSteps(b *strings.Builder, steps []auxiliaryGraphStep, sourceLines *sourceLineReader) {
	if len(steps) == 0 {
		return
	}
	fmt.Fprintf(b, "<details class=\"auxiliary-list\" data-aux-list data-aux-page-size=\"5\"><summary>%d chamadas auxiliares omitidas do grafo</summary>", len(steps))
	b.WriteString("<div class=\"auxiliary-pager\"><button type=\"button\" data-aux-prev>Anterior</button><button type=\"button\" data-aux-next>Proxima</button><span data-aux-status></span></div>\n")
	b.WriteString("<table><thead><tr><th>Ordem</th><th>Categoria</th><th>Chamada</th><th>Motivo</th><th>Evidencia</th><th>Linha de codigo</th></tr></thead><tbody>\n")
	for i, step := range steps {
		fmt.Fprintf(b, "<tr data-aux-row data-aux-index=\"%d\"><td>#%d</td><td>%s</td><td><code>%s</code></td><td>%s</td><td><code>%s</code></td><td><code>%s</code></td></tr>\n",
			i,
			step.Order,
			h(step.Category),
			h(step.Call),
			h(step.Reason),
			h(evidenceLabel(step.Evidence)),
			h(sourceLines.line(step.Evidence)),
		)
	}
	b.WriteString("</tbody></table></details>\n")
}

func classifyAuxiliaryStep(step flow.Step) (string, string, bool) {
	call := strings.TrimSpace(step.Call)
	lowerCall := strings.ToLower(call)
	method := lowerCall
	if idx := strings.LastIndex(method, "."); idx >= 0 {
		method = method[idx+1:]
	}
	resolvedClass := strings.ToLower(step.ResolvedClass)

	switch {
	case strings.HasPrefix(lowerCall, "log.") || strings.HasPrefix(lowerCall, "logger."):
		return "observabilidade", "chamada de logging; nao altera o fluxo de negocio", true
	case strings.HasSuffix(lowerCall, ".printstacktrace") || lowerCall == "printstacktrace":
		return "tratamento de erro", "stack trace dentro de bloco de erro/catch", true
	case method == "equals" || method == "hashcode" || method == "tostring":
		return "condicao/linguagem", "comparacao ou metodo basico de linguagem", true
	case isCollectionUtilityCall(resolvedClass, method):
		return "estrutura de dados", "operacao em colecao/mapa usada como suporte ao fluxo", true
	case isRequestExtractionCall(resolvedClass, method):
		return "extracao de request", "leitura de parametros da requisicao", true
	default:
		return "", "", false
	}
}

func isCollectionUtilityCall(resolvedClass, method string) bool {
	if !strings.HasPrefix(resolvedClass, "java.util.") &&
		!strings.Contains(resolvedClass, "collectionutils") &&
		!strings.Contains(resolvedClass, ".collutil") {
		return false
	}
	switch method {
	case "get", "put", "keyset", "entryset", "values", "size", "add", "remove", "isempty", "isnotempty", "iterator":
		return true
	default:
		return false
	}
}

func isRequestExtractionCall(resolvedClass, method string) bool {
	if !strings.Contains(resolvedClass, "httpservletrequest") {
		return false
	}
	return strings.HasPrefix(method, "getparameter")
}

func renderInspectorHTML(b *strings.Builder, details map[string]string, sourceContexts *sourceContextStore) {
	if len(details) == 0 {
		b.WriteString("<p class=\"subtle\">Clique em um item do grafo para ver os detalhes.</p>")
		return
	}
	keys := make([]string, 0, len(details))
	for key, value := range details {
		if strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	b.WriteString("<dl>")
	for _, key := range keys {
		fmt.Fprintf(b, "<div><dt>%s</dt><dd>%s</dd></div>", h(key), inspectorValueHTML(key, details[key], sourceContexts))
	}
	b.WriteString("</dl>")
}

func inspectorValueHTML(key, value string, sourceContexts *sourceContextStore) string {
	if isSourceContextRef(value) && sourceContexts != nil {
		value = sourceContexts.values[sourceContextID(value)]
	}
	if strings.HasPrefix(key, "Conteudo") {
		return fmt.Sprintf("<pre><code>%s</code></pre>", h(value))
	}
	return h(value)
}

func graphPayloadAttr(details map[string]string) string {
	data, err := json.Marshal(details)
	if err != nil {
		return "{}"
	}
	return h(string(data))
}

func methodLabel(className, methodName string) string {
	className = valueOrUnknown(className)
	methodName = valueOrUnknown(methodName)
	if className == "desconhecido" {
		return methodName
	}
	if methodName == "desconhecido" {
		return className
	}
	return className + "." + methodName
}

func graphKindLabel(kind string) string {
	switch kind {
	case "entry":
		return "entrypoint"
	case "method":
		return "metodo"
	case "class":
		return "classe"
	case "external":
		return "import externo"
	case "dependency":
		return "dependencia"
	case "condition":
		return "condicao"
	case "unresolved":
		return "unresolved"
	default:
		return kind
	}
}

func graphNodeSize(kind string) (int, int) {
	if kind == "class" {
		return graphClassNodeW, graphClassNodeH
	}
	if kind == "dependency" {
		return graphDepNodeW, graphDepNodeH
	}
	return graphNodeW, graphNodeH
}

func nodeLabelLimit(width int) int {
	limit := (width - 28) / 8
	if limit < 24 {
		return 24
	}
	if limit > 44 {
		return 44
	}
	return limit
}

func renderNodeLabel(b *strings.Builder, x, y int, value string, limit int) {
	lines := graphLabelLines(value, limit)
	b.WriteString("<text>")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(b, "<tspan x=\"%d\" y=\"%d\">%s</tspan>", x, y, h(line))
			continue
		}
		fmt.Fprintf(b, "<tspan x=\"%d\" dy=\"15\">%s</tspan>", x, h(line))
	}
	b.WriteString("</text>\n")
}

func graphLabelLines(value string, limit int) []string {
	label := graphDisplayLabel(value)
	if limit <= 0 {
		limit = 44
	}
	if len([]rune(label)) <= limit {
		return []string{label}
	}
	parts := strings.Split(label, ".")
	if len(parts) >= 2 {
		className := strings.Join(parts[:len(parts)-1], ".")
		methodName := parts[len(parts)-1]
		if len([]rune(className)) <= limit && len([]rune(methodName)) <= limit {
			return []string{className, methodName}
		}
	}
	return wrapRunes(label, limit, 3)
}

func graphDisplayLabel(value string) string {
	parts := strings.Split(value, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return value
}

func wrapRunes(value string, limit, maxLines int) []string {
	runes := []rune(value)
	var lines []string
	for len(runes) > 0 && len(lines) < maxLines {
		if len(runes) <= limit {
			lines = append(lines, string(runes))
			break
		}
		cut := limit
		for i := limit; i > 0; i-- {
			if runes[i-1] == '.' || runes[i-1] == '_' || runes[i-1] == '-' {
				cut = i
				break
			}
		}
		lines = append(lines, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 && len(lines) > 0 {
		lines[len(lines)-1] = strings.TrimRight(lines[len(lines)-1], ".-_") + "..."
	}
	return lines
}

func shorten(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "..."
}

func renderMeta(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "<div class=\"meta-row\"><span>%s</span>%s</div>\n", h(label), h(value))
}

func stepResolvedLabel(step flow.Step) string {
	if step.ResolvedClass == "" && step.ResolvedMethod == "" {
		return "unresolved"
	}
	return strings.Trim(strings.TrimSpace(step.ResolvedClass+"."+step.ResolvedMethod), ".")
}

func stepMethodDisplayLabel(step flow.Step) string {
	if step.ResolvedMethod != "" {
		return step.ResolvedMethod
	}
	if idx := strings.LastIndex(step.Call, "."); idx >= 0 && idx+1 < len(step.Call) {
		return step.Call[idx+1:]
	}
	return step.Call
}

func entryMethodDisplayName(entry model.EntryPoint) string {
	if idx := strings.LastIndex(entry.Name, "."); idx >= 0 && idx+1 < len(entry.Name) {
		return entry.Name[idx+1:]
	}
	return entry.Name
}

func shortTypeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.LastIndex(value, "."); idx >= 0 && idx+1 < len(value) {
		return value[idx+1:]
	}
	return value
}

func evidenceLabel(evidence model.Evidence) string {
	if evidence.Path == "" {
		return "desconhecido"
	}
	if evidence.Line > 0 {
		return fmt.Sprintf("%s:%d", evidence.Path, evidence.Line)
	}
	return evidence.Path
}

func htmlID(value string) string {
	id := safeFileName(value)
	if id == "" {
		return "item"
	}
	return "p-" + id
}

func cssClass(value string) string {
	class := safeFileName(value)
	if class == "" {
		return "unknown"
	}
	return class
}

func h(value string) string {
	return html.EscapeString(value)
}

func renderGaps(project *model.Project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Lacunas tecnicas\n\n")
	if len(project.Gaps) == 0 {
		fmt.Fprintf(&b, "Nenhuma lacuna critica foi registrada.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "| Severidade | Mensagem | Evidencia |\n| --- | --- | --- |\n")
	for _, gap := range project.Gaps {
		fmt.Fprintf(&b, "| %s | %s | `%s:%d` |\n", gap.Severity, gap.Message, gap.Evidence.Path, gap.Evidence.Line)
	}
	return b.String()
}

func findType(project *model.Project, id string) model.Type {
	for _, typ := range project.Types {
		if typ.ID == id {
			return typ
		}
	}
	return model.Type{}
}

func findMethod(typ model.Type, id string) model.Method {
	for _, method := range typ.Methods {
		if method.ID == id {
			return method
		}
	}
	return model.Method{}
}

func entryLabel(entry model.EntryPoint) string {
	if entry.HTTPMethod != "" || entry.Path != "" {
		return strings.TrimSpace(entry.HTTPMethod + " " + entry.Path)
	}
	if entry.Resource != "" {
		return entry.Resource
	}
	return entry.Name
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "desconhecido"
	}
	return value
}

func safeFileName(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		isAlphaNum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value = strings.Trim(b.String(), "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}

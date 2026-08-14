package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bee/java-process-mapper/internal/model"
)

func Build(project *model.Project) {
	builder := newBuilder()
	productID := "product:" + stableID(project.Name)
	builder.node(model.GraphNode{
		ID:         productID,
		Kind:       "Product",
		Name:       project.Name,
		Properties: map[string]any{"root": project.Root, "javaVersion": project.JavaVersion},
		Source:     model.SourceInferred,
		Confidence: model.ConfidenceMedium,
	})

	for _, module := range project.Modules {
		moduleID := "module:" + module.ID
		builder.node(model.GraphNode{
			ID:         moduleID,
			Kind:       "Service",
			Name:       module.Name,
			Properties: map[string]any{"path": module.Path, "buildTool": module.BuildTool, "packaging": module.Packaging, "javaVersion": module.JavaVersion},
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
		})
		builder.edge(productID, moduleID, "contains", "", nil, model.SourceFound, model.ConfidenceHigh)
	}

	methodToType := map[string]model.Type{}
	for _, typ := range project.Types {
		classNodeID := "class:" + typ.ID
		builder.node(model.GraphNode{
			ID:         classNodeID,
			Kind:       "Class",
			Name:       typ.FQN,
			Properties: map[string]any{"kind": typ.Kind, "package": typ.Package},
			Evidence:   []model.Evidence{typ.Evidence},
			Source:     typ.Source,
			Confidence: typ.Confidence,
		})
		if typ.ModuleID != "" {
			builder.edge("module:"+typ.ModuleID, classNodeID, "contains", "", []model.Evidence{typ.Evidence}, model.SourceFound, model.ConfidenceHigh)
		}
		for _, method := range typ.Methods {
			methodToType[method.ID] = typ
			methodNodeID := "method:" + method.ID
			builder.node(model.GraphNode{
				ID:         methodNodeID,
				Kind:       "Method",
				Name:       typ.FQN + "." + method.Name,
				Properties: map[string]any{"returnType": method.ReturnType},
				Evidence:   []model.Evidence{method.Evidence},
				Source:     method.Source,
				Confidence: method.Confidence,
			})
			builder.edge(classNodeID, methodNodeID, "contains", "", []model.Evidence{method.Evidence}, model.SourceFound, model.ConfidenceHigh)
			for _, call := range method.Calls {
				if call.ResolvedMethodID != "" {
					builder.edge(methodNodeID, "method:"+call.ResolvedMethodID, "calls", call.Target, []model.Evidence{call.Evidence}, call.Source, call.Confidence)
					continue
				}
				callNodeID := "call:" + stableID(method.ID, call.Target)
				builder.node(model.GraphNode{
					ID:         callNodeID,
					Kind:       "MethodCall",
					Name:       call.Target,
					Evidence:   []model.Evidence{call.Evidence},
					Source:     call.Source,
					Confidence: call.Confidence,
				})
				builder.edge(methodNodeID, callNodeID, "calls", call.Target, []model.Evidence{call.Evidence}, call.Source, call.Confidence)
			}
		}
	}

	for _, entry := range project.EntryPoints {
		processID := "process:" + entry.ID
		builder.node(model.GraphNode{
			ID:         processID,
			Kind:       "Process",
			Name:       entry.Name,
			Properties: map[string]any{"kind": entry.Kind, "path": entry.Path, "resource": entry.Resource},
			Evidence:   []model.Evidence{entry.Evidence},
			Source:     entry.Source,
			Confidence: entry.Confidence,
		})
		entryNodeID := "entrypoint:" + entry.ID
		builder.node(model.GraphNode{
			ID:         entryNodeID,
			Kind:       "EntryPoint",
			Name:       entry.Name,
			Properties: map[string]any{"framework": entry.Framework, "httpMethod": entry.HTTPMethod, "path": entry.Path, "resource": entry.Resource},
			Evidence:   []model.Evidence{entry.Evidence},
			Source:     entry.Source,
			Confidence: entry.Confidence,
		})
		builder.edge(productID, processID, "contains", "", []model.Evidence{entry.Evidence}, model.SourceFound, model.ConfidenceHigh)
		builder.edge(processID, entryNodeID, "starts_at", entry.Name, []model.Evidence{entry.Evidence}, entry.Source, entry.Confidence)
		if entry.MethodID != "" {
			builder.edge(entryNodeID, "method:"+entry.MethodID, "starts_at", entry.Name, []model.Evidence{entry.Evidence}, entry.Source, entry.Confidence)
		} else if entry.ClassID != "" {
			builder.edge(entryNodeID, "class:"+entry.ClassID, "starts_at", entry.Name, []model.Evidence{entry.Evidence}, entry.Source, entry.Confidence)
		}
	}

	for _, dep := range project.Dependencies {
		nodeID := "dependency:" + dep.ID
		builder.node(model.GraphNode{
			ID:         nodeID,
			Kind:       dependencyNodeKind(dep.Kind),
			Name:       dep.Name,
			Properties: map[string]any{"kind": dep.Kind, "detail": dep.Detail},
			Evidence:   []model.Evidence{dep.Evidence},
			Source:     dep.Source,
			Confidence: dep.Confidence,
		})
		target := ""
		if dep.MethodID != "" {
			target = "method:" + dep.MethodID
		} else if dep.ClassID != "" {
			target = "class:" + dep.ClassID
		}
		if target != "" {
			builder.edge(target, nodeID, dependencyEdgeKind(dep.Kind), dep.Detail, []model.Evidence{dep.Evidence}, dep.Source, dep.Confidence)
			if dep.MethodID != "" {
				if typ, ok := methodToType[dep.MethodID]; ok {
					builder.edge("class:"+typ.ID, nodeID, "depends_on", dep.Detail, []model.Evidence{dep.Evidence}, dep.Source, dep.Confidence)
				}
			}
		}
	}

	for _, prop := range project.ConfigProperties {
		propID := "config:" + stableID(prop.SourceFile, prop.Key)
		builder.node(model.GraphNode{
			ID:         propID,
			Kind:       "ConfigProperty",
			Name:       prop.Key,
			Properties: map[string]any{"definedExternally": prop.DefinedExternally, "value": prop.Value},
			Evidence:   []model.Evidence{prop.Evidence},
			Source:     prop.Source,
			Confidence: prop.Confidence,
		})
	}

	project.Graph = model.Graph{Nodes: builder.nodesList(), Edges: builder.edges}
}

type builder struct {
	nodes map[string]model.GraphNode
	edges []model.GraphEdge
}

func newBuilder() *builder {
	return &builder{nodes: map[string]model.GraphNode{}}
}

func (b *builder) node(node model.GraphNode) {
	if existing, ok := b.nodes[node.ID]; ok {
		if len(existing.Evidence) == 0 && len(node.Evidence) > 0 {
			existing.Evidence = node.Evidence
		}
		b.nodes[node.ID] = existing
		return
	}
	b.nodes[node.ID] = node
}

func (b *builder) edge(from, to, kind, how string, evidence []model.Evidence, source model.SourceKind, confidence model.Confidence) {
	if from == "" || to == "" {
		return
	}
	id := stableID("edge", from, kind, to, how)
	for _, edge := range b.edges {
		if edge.ID == id {
			return
		}
	}
	b.edges = append(b.edges, model.GraphEdge{
		ID:         id,
		From:       from,
		To:         to,
		Kind:       kind,
		How:        how,
		Evidence:   evidence,
		Source:     source,
		Confidence: confidence,
	})
}

func (b *builder) nodesList() []model.GraphNode {
	nodes := make([]model.GraphNode, 0, len(b.nodes))
	for _, node := range b.nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(b.edges, func(i, j int) bool { return b.edges[i].ID < b.edges[j].ID })
	return nodes
}

func (b *builder) relatedFeatureEdges() {
	depToFeatures := map[string]map[string]bool{}
	methodToFeature := map[string]string{}
	for _, edge := range b.edges {
		if edge.Kind == "starts_at" && strings.HasPrefix(edge.From, "entrypoint:") && strings.HasPrefix(edge.To, "method:") {
			methodToFeature[edge.To] = featureForEntryPoint(b.edges, edge.From)
		}
	}
	for _, edge := range b.edges {
		if !strings.HasPrefix(edge.To, "dependency:") {
			continue
		}
		if feature := methodToFeature[edge.From]; feature != "" {
			if depToFeatures[edge.To] == nil {
				depToFeatures[edge.To] = map[string]bool{}
			}
			depToFeatures[edge.To][feature] = true
		}
	}
	for dep, features := range depToFeatures {
		var ids []string
		for feature := range features {
			ids = append(ids, feature)
		}
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				b.edge(ids[i], ids[j], "related_by", dep, nil, model.SourceInferred, model.ConfidenceLow)
			}
		}
	}
}

func featureForEntryPoint(edges []model.GraphEdge, entrypoint string) string {
	process := ""
	for _, edge := range edges {
		if edge.Kind == "starts_at" && edge.To == entrypoint && strings.HasPrefix(edge.From, "process:") {
			process = edge.From
			break
		}
	}
	if process == "" {
		return ""
	}
	for _, edge := range edges {
		if edge.Kind == "contains" && edge.To == process && strings.HasPrefix(edge.From, "feature:") {
			return edge.From
		}
	}
	return ""
}

func dependencyNodeKind(kind string) string {
	switch kind {
	case "datastore", "database_repository", "database_access", "database_client", "database_migration", "persistence_unit":
		return "DataStore"
	case "table":
		return "Table"
	case "bucket":
		return "Bucket"
	case "queue":
		return "Queue"
	case "topic":
		return "Topic"
	case "external_api":
		return "ExternalApi"
	case "external_dependency":
		return "ExternalDependency"
	case "mail_server":
		return "ExternalApi"
	case "ftp_endpoint":
		return "ExternalApi"
	case "ui_api_call":
		return "ExternalApi"
	case "ui_websocket":
		return "ExternalApi"
	case "http_filter":
		return "Dependency"
	case "cache":
		return "DataStore"
	case "auth_provider":
		return "Dependency"
	case "config_property":
		return "ConfigProperty"
	default:
		return "Dependency"
	}
}

func dependencyEdgeKind(kind string) string {
	switch kind {
	case "datastore", "database_repository", "database_access", "database_client", "database_migration", "persistence_unit", "repository_call", "table", "cache", "auth_provider", "ui_api_call", "ui_websocket":
		return "depends_on"
	case "bucket":
		return "depends_on"
	case "message_publish":
		return "publishes_to"
	case "queue", "topic":
		return "depends_on"
	default:
		return "depends_on"
	}
}

func stableID(parts ...string) string {
	joined := strings.Join(parts, ":")
	replacer := strings.NewReplacer("\\", "/", " ", "-", ":", "-", ".", "-", "_", "-", "{", "", "}", "", "(", "", ")", "")
	return strings.Trim(replacer.Replace(strings.ToLower(fmt.Sprint(joined))), "-")
}

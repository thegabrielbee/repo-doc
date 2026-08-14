package flow

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bee/java-process-mapper/internal/model"
)

const (
	DefaultMaxDepth = 8
	DefaultMaxSteps = 200
)

type Trace struct {
	EntryPoint       model.EntryPoint       `json:"entryPoint"`
	Steps            []Step                 `json:"steps"`
	Conditions       []ConditionUse         `json:"conditions,omitempty"`
	Dependencies     []DependencyUse        `json:"dependencies"`
	ConfigProperties []model.ConfigProperty `json:"configProperties,omitempty"`
	Truncated        bool                   `json:"truncated"`
}

type Step struct {
	Order            int              `json:"order"`
	Depth            int              `json:"depth"`
	CallerClass      string           `json:"callerClass"`
	CallerMethod     string           `json:"callerMethod"`
	Call             string           `json:"call,omitempty"`
	ResolvedClass    string           `json:"resolvedClass,omitempty"`
	ResolvedMethod   string           `json:"resolvedMethod,omitempty"`
	ResolvedMethodID string           `json:"resolvedMethodId,omitempty"`
	Resolution       string           `json:"resolution,omitempty"`
	Evidence         model.Evidence   `json:"evidence"`
	Source           model.SourceKind `json:"source"`
	Confidence       model.Confidence `json:"confidence"`
}

type DependencyUse struct {
	Dependency model.Dependency `json:"dependency"`
	Scope      string           `json:"scope"`
	ViaClass   string           `json:"viaClass,omitempty"`
	ViaMethod  string           `json:"viaMethod,omitempty"`
	Depth      int              `json:"depth"`
}

type ConditionUse struct {
	Condition    model.Condition `json:"condition"`
	CallerClass  string          `json:"callerClass"`
	CallerMethod string          `json:"callerMethod"`
	Depth        int             `json:"depth"`
}

type indexes struct {
	typeByID     map[string]model.Type
	methodByID   map[string]model.Method
	methodTypeID map[string]string
}

func Build(project *model.Project, entry model.EntryPoint) Trace {
	idx := buildIndexes(project)
	trace := Trace{EntryPoint: entry}
	entryMethod := idx.methodByID[entry.MethodID]
	entryType := idx.typeByID[entry.ClassID]
	if entry.MethodID == "" || entryMethod.ID == "" {
		trace.Dependencies = dependenciesFor(project, idx, map[string]int{}, map[string]int{entry.ClassID: 0})
		trace.Dependencies = appendHTTPFilters(project, idx, entry, trace.Dependencies)
		trace.Dependencies = appendUIClientCalls(project, entry, trace.Dependencies)
		trace.ConfigProperties = configForDependencies(project, trace.Dependencies)
		return trace
	}
	methodDepth := map[string]int{entry.MethodID: 0}
	typeDepth := map[string]int{entry.ClassID: 0}
	visited := map[string]bool{}
	var order int
	var walk func(methodID string, depth int)
	walk = func(methodID string, depth int) {
		if trace.Truncated || depth > DefaultMaxDepth {
			trace.Truncated = true
			return
		}
		if visited[methodID] {
			return
		}
		visited[methodID] = true
		method := idx.methodByID[methodID]
		typeID := idx.methodTypeID[methodID]
		typ := idx.typeByID[typeID]
		if method.ID == "" || typ.ID == "" {
			return
		}
		typeDepth[typeID] = minDepth(typeDepth, typeID, depth)
		methodDepth[methodID] = minDepth(methodDepth, methodID, depth)
		for _, condition := range method.Conditions {
			trace.Conditions = append(trace.Conditions, ConditionUse{
				Condition:    condition,
				CallerClass:  typ.FQN,
				CallerMethod: method.Name,
				Depth:        depth + 1,
			})
		}
		for _, call := range method.Calls {
			if len(trace.Steps) >= DefaultMaxSteps {
				trace.Truncated = true
				return
			}
			order++
			step := Step{
				Order:        order,
				Depth:        depth + 1,
				CallerClass:  typ.FQN,
				CallerMethod: method.Name,
				Call:         call.Target,
				Resolution:   call.Resolution,
				Evidence:     call.Evidence,
				Source:       call.Source,
				Confidence:   call.Confidence,
			}
			if call.ResolvedTypeID != "" {
				resolvedType := idx.typeByID[call.ResolvedTypeID]
				step.ResolvedClass = resolvedType.FQN
				typeDepth[call.ResolvedTypeID] = minDepth(typeDepth, call.ResolvedTypeID, depth+1)
			}
			if call.ResolvedExternalType != "" {
				step.ResolvedClass = call.ResolvedExternalType
				step.ResolvedMethod = call.Method
			}
			if call.ResolvedMethodID != "" {
				resolvedMethod := idx.methodByID[call.ResolvedMethodID]
				step.ResolvedMethod = resolvedMethod.Name
				step.ResolvedMethodID = call.ResolvedMethodID
				methodDepth[call.ResolvedMethodID] = minDepth(methodDepth, call.ResolvedMethodID, depth+1)
			}
			trace.Steps = append(trace.Steps, step)
			if call.ResolvedMethodID != "" {
				walk(call.ResolvedMethodID, depth+1)
			}
		}
	}
	_ = entryType
	walk(entry.MethodID, 0)
	trace.Dependencies = dependenciesFor(project, idx, methodDepth, typeDepth)
	trace.Dependencies = appendHTTPFilters(project, idx, entry, trace.Dependencies)
	trace.Dependencies = appendUIClientCalls(project, entry, trace.Dependencies)
	trace.ConfigProperties = configForDependencies(project, trace.Dependencies)
	return trace
}

func buildIndexes(project *model.Project) indexes {
	idx := indexes{
		typeByID:     map[string]model.Type{},
		methodByID:   map[string]model.Method{},
		methodTypeID: map[string]string{},
	}
	for _, typ := range project.Types {
		idx.typeByID[typ.ID] = typ
		for _, method := range typ.Methods {
			idx.methodByID[method.ID] = method
			idx.methodTypeID[method.ID] = typ.ID
		}
	}
	return idx
}

func dependenciesFor(project *model.Project, idx indexes, methodDepth map[string]int, typeDepth map[string]int) []DependencyUse {
	var uses []DependencyUse
	seen := map[string]bool{}
	for _, dep := range project.Dependencies {
		scope := ""
		depth := 0
		if dep.MethodID != "" {
			if d, ok := methodDepth[dep.MethodID]; ok {
				depth = d
				if d == 0 {
					scope = "direct"
				} else {
					scope = "indirect"
				}
			}
		} else if dep.ClassID != "" {
			if d, ok := typeDepth[dep.ClassID]; ok {
				depth = d
				if d == 0 {
					scope = "direct"
				} else {
					scope = "indirect"
				}
			}
		}
		if scope == "" || seen[dep.ID] {
			continue
		}
		seen[dep.ID] = true
		typ := idx.typeByID[dep.ClassID]
		method := idx.methodByID[dep.MethodID]
		uses = append(uses, DependencyUse{
			Dependency: dep,
			Scope:      scope,
			ViaClass:   typ.FQN,
			ViaMethod:  method.Name,
			Depth:      depth,
		})
	}
	sort.Slice(uses, func(i, j int) bool {
		if uses[i].Depth != uses[j].Depth {
			return uses[i].Depth < uses[j].Depth
		}
		return uses[i].Dependency.ID < uses[j].Dependency.ID
	})
	return uses
}

func configForDependencies(project *model.Project, deps []DependencyUse) []model.ConfigProperty {
	keys := map[string]bool{}
	for _, dep := range deps {
		if dep.Dependency.Kind == "config_property" && dep.Dependency.Detail != "" {
			keys[dep.Dependency.Detail] = true
		}
	}
	if len(keys) == 0 {
		return nil
	}
	var props []model.ConfigProperty
	for _, prop := range project.ConfigProperties {
		if keys[prop.Key] {
			props = append(props, prop)
		}
	}
	return props
}

func appendHTTPFilters(project *model.Project, idx indexes, entry model.EntryPoint, deps []DependencyUse) []DependencyUse {
	if project == nil || !entrySupportsHTTPFilters(entry) {
		return deps
	}
	entryPath := strings.TrimSpace(entry.Path)
	if entryPath == "" {
		return deps
	}
	entryModuleID := moduleIDForEntry(project, idx, entry)
	seen := map[string]bool{}
	for _, dep := range deps {
		seen[dep.Dependency.ID] = true
	}
	for _, dep := range project.Dependencies {
		if dep.Kind != "http_filter" || seen[dep.ID] {
			continue
		}
		if !httpPatternMatches(dep.Detail, entryPath) {
			continue
		}
		filterModuleID := moduleIDForDependency(project, idx, dep)
		if entryModuleID != "" && filterModuleID != "" && entryModuleID != filterModuleID {
			continue
		}
		typ := idx.typeByID[dep.ClassID]
		deps = append(deps, DependencyUse{
			Dependency: dep,
			Scope:      "direct",
			ViaClass:   typ.FQN,
			Depth:      0,
		})
		seen[dep.ID] = true
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Depth != deps[j].Depth {
			return deps[i].Depth < deps[j].Depth
		}
		return deps[i].Dependency.ID < deps[j].Dependency.ID
	})
	return deps
}

func appendUIClientCalls(project *model.Project, entry model.EntryPoint, deps []DependencyUse) []DependencyUse {
	if project == nil || entry.Kind != "ui_page" || strings.TrimSpace(entry.Path) == "" {
		return deps
	}
	seen := map[string]bool{}
	for _, dep := range deps {
		seen[dep.Dependency.ID] = true
	}
	for _, dep := range project.Dependencies {
		if dep.Kind != "ui_api_call" && dep.Kind != "ui_websocket" {
			continue
		}
		if seen[dep.ID] || dep.Detail != entry.Path || !samePath(dep.Evidence.Path, entry.Evidence.Path) {
			continue
		}

		if dep.ClassID == "" || dep.MethodID == "" {
			classID, methodID := resolveBackendEndpoint(project, dep.Name)
			if classID != "" {
				dep.ClassID = classID
				dep.MethodID = methodID
			}
		}

		deps = append(deps, DependencyUse{
			Dependency: dep,
			Scope:      "direct",
			Depth:      0,
		})
		seen[dep.ID] = true
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Depth != deps[j].Depth {
			return deps[i].Depth < deps[j].Depth
		}
		return deps[i].Dependency.ID < deps[j].Dependency.ID
	})
	return deps
}

func resolveBackendEndpoint(project *model.Project, target string) (string, string) {
	targetLower := strings.ToLower(target)
	var bestMatch *model.EntryPoint
	longestMatch := -1

	for i := range project.EntryPoints {
		ep := &project.EntryPoints[i]
		if ep.Kind != "http" && ep.Kind != "websocket" {
			continue
		}
		
		if ep.Kind == "http" && ep.HTTPMethod != "" {
			if !strings.HasPrefix(targetLower, strings.ToLower(ep.HTTPMethod)+" ") {
				continue
			}
		}

		epPath := strings.ToLower(ep.Path)
		epPath = strings.TrimPrefix(epPath, "/")
		
		if epPath == "" {
			continue
		}

		if strings.Contains(targetLower, "/"+epPath) || strings.HasSuffix(targetLower, epPath) {
			if len(epPath) > longestMatch {
				longestMatch = len(epPath)
				bestMatch = ep
			}
		}
	}
	if bestMatch != nil {
		return bestMatch.ClassID, bestMatch.MethodID
	}
	return "", ""
}

func moduleIDForEntry(project *model.Project, idx indexes, entry model.EntryPoint) string {
	if typ := idx.typeByID[entry.ClassID]; typ.ModuleID != "" {
		return typ.ModuleID
	}
	return moduleIDForPath(project, entry.Evidence.Path)
}

func moduleIDForDependency(project *model.Project, idx indexes, dep model.Dependency) string {
	if typ := idx.typeByID[dep.ClassID]; typ.ModuleID != "" {
		return typ.ModuleID
	}
	return moduleIDForPath(project, dep.Evidence.Path)
}

func moduleIDForPath(project *model.Project, path string) string {
	if project == nil || path == "" {
		return ""
	}
	for _, module := range project.Modules {
		for _, candidate := range module.JavaFiles {
			if samePath(candidate, path) {
				return module.ID
			}
		}
		for _, candidate := range module.DescriptorFiles {
			if samePath(candidate, path) {
				return module.ID
			}
		}
		for _, candidate := range module.UIFiles {
			if samePath(candidate, path) {
				return module.ID
			}
		}
	}
	return ""
}

func samePath(left, right string) bool {
	left = filepath.ToSlash(filepath.Clean(left))
	right = filepath.ToSlash(filepath.Clean(right))
	return strings.EqualFold(left, right)
}

func entrySupportsHTTPFilters(entry model.EntryPoint) bool {
	switch entry.Kind {
	case "http", "servlet":
		return true
	default:
		return false
	}
}

func httpPatternMatches(pattern, path string) bool {
	pattern = normalizeHTTPPath(pattern)
	path = normalizeHTTPPath(path)
	if pattern == "" || path == "" {
		return false
	}
	if pattern == "/" || pattern == "/*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(path, strings.TrimPrefix(pattern, "*"))
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return pattern == path
}

func normalizeHTTPPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "*.") {
		value = "/" + value
	}
	return value
}

func minDepth(depths map[string]int, key string, candidate int) int {
	current, ok := depths[key]
	if !ok || candidate < current {
		depths[key] = candidate
		return candidate
	}
	return current
}

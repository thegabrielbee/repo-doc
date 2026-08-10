package flow

import (
	"sort"

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

func minDepth(depths map[string]int, key string, candidate int) int {
	current, ok := depths[key]
	if !ok || candidate < current {
		depths[key] = candidate
		return candidate
	}
	return current
}

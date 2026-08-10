package addon

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bee/java-process-mapper/internal/model"
)

type Spring struct{}

func (Spring) Name() string { return "spring" }

func (Spring) Analyze(project *model.Project) error {
	sourceByPath := map[string]model.SourceFile{}
	for _, source := range project.SourceFiles {
		sourceByPath[source.Path] = source
	}

	seenEntryPoints := map[string]bool{}
	seenDeps := map[string]bool{}
	for i := range project.Types {
		typ := &project.Types[i]
		classPath := mappingPath(annotationValue(findAnnotation(typ.Annotations, "RequestMapping")))
		isController := hasAnnotation(typ.Annotations, "RestController", "Controller")

		for _, method := range typ.Methods {
			if mapping, httpMethod := findHTTPMapping(method.Annotations); mapping != nil {
				path := combinePaths(classPath, mappingPath(annotationValue(mapping)))
				entry := entryPoint(project, typ, method, "http", path, httpMethod, path, mapping.Evidence)
				addEntryPoint(project, seenEntryPoints, entry)
				continue
			}
			if isController && classPath != "" && hasAnnotation(method.Annotations, "RequestMapping") {
				mapping := findAnnotation(method.Annotations, "RequestMapping")
				path := combinePaths(classPath, mappingPath(annotationValue(mapping)))
				entry := entryPoint(project, typ, method, "http", path, "ANY", path, mapping.Evidence)
				addEntryPoint(project, seenEntryPoints, entry)
			}
			for _, annName := range []string{"Scheduled", "KafkaListener", "RabbitListener", "JmsListener", "EventListener"} {
				ann := findAnnotation(method.Annotations, annName)
				if ann == nil {
					continue
				}
				kind := listenerKind(annName)
				resource := listenerResource(ann)
				entry := entryPoint(project, typ, method, kind, "", "", resource, ann.Evidence)
				addEntryPoint(project, seenEntryPoints, entry)
			}
			if isRunnerMethod(method) {
				entry := entryPoint(project, typ, method, "runner", "", "", method.ReturnType, method.Evidence)
				addEntryPoint(project, seenEntryPoints, entry)
			}
			if isBatchMethod(method) {
				entry := entryPoint(project, typ, method, "batch", "", "", method.ReturnType, method.Evidence)
				addEntryPoint(project, seenEntryPoints, entry)
			}
		}

		if isRunnerType(*typ) {
			entry := model.EntryPoint{
				ID:         stableID("entrypoint", typ.ID, "runner"),
				Kind:       "runner",
				Framework:  "spring",
				Name:       typ.Name,
				Product:    project.Name,
				ClassID:    typ.ID,
				Evidence:   typ.Evidence,
				Source:     model.SourceFound,
				Confidence: model.ConfidenceHigh,
			}
			addEntryPoint(project, seenEntryPoints, entry)
		}

		addTypeDependencies(project, seenDeps, *typ, sourceByPath[typ.FilePath])
	}

	for _, module := range project.Modules {
		for _, migration := range module.MigrationFiles {
			addDependency(project, seenDeps, model.Dependency{
				ID:         stableID("dependency", "migration", migration),
				Kind:       "database_migration",
				Name:       filepath.Base(migration),
				Detail:     "migration file",
				Evidence:   model.Evidence{Path: migration, Kind: "migration"},
				Source:     model.SourceFound,
				Confidence: model.ConfidenceHigh,
			})
		}
	}
	sort.Slice(project.EntryPoints, func(i, j int) bool { return project.EntryPoints[i].ID < project.EntryPoints[j].ID })
	sort.Slice(project.Dependencies, func(i, j int) bool { return project.Dependencies[i].ID < project.Dependencies[j].ID })
	return nil
}

func addTypeDependencies(project *model.Project, seen map[string]bool, typ model.Type, source model.SourceFile) {
	if hasAnnotation(typ.Annotations, "Repository") || implementsAny(typ, "JpaRepository", "CrudRepository", "PagingAndSortingRepository") {
		addDependency(project, seen, model.Dependency{
			ID:         stableID("dependency", "repository", typ.ID),
			Kind:       "database_repository",
			Name:       typ.Name,
			ClassID:    typ.ID,
			Evidence:   typ.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
		})
	}
	if hasAnnotation(typ.Annotations, "Mapper") || strings.HasSuffix(typ.Name, "Mapper") || strings.HasSuffix(typ.Name, "Dao") {
		addDependency(project, seen, model.Dependency{
			ID:         stableID("dependency", "database-access", typ.ID),
			Kind:       "database_access",
			Name:       typ.Name,
			ClassID:    typ.ID,
			Evidence:   typ.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceMedium,
		})
	}
	if hasAnnotation(typ.Annotations, "Entity") {
		tableName := typ.Name
		if table := findAnnotation(typ.Annotations, "Table"); table != nil {
			if value := annotationNamedValue(table, "name"); value != "" {
				tableName = value
			}
		}
		addDependency(project, seen, model.Dependency{
			ID:         stableID("dependency", "table", typ.ID, tableName),
			Kind:       "table",
			Name:       tableName,
			ClassID:    typ.ID,
			Evidence:   typ.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
		})
	}
	if feign := findAnnotation(typ.Annotations, "FeignClient"); feign != nil {
		name := annotationValue(feign)
		if name == "" {
			name = typ.Name
		}
		addDependency(project, seen, model.Dependency{
			ID:         stableID("dependency", "external_api", typ.ID, name),
			Kind:       "external_api",
			Name:       name,
			Detail:     "Feign client",
			ClassID:    typ.ID,
			Evidence:   feign.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
		})
	}
	for _, imp := range source.Imports {
		lower := strings.ToLower(imp)
		switch {
		case strings.Contains(lower, "s3"):
			addDependency(project, seen, dependencyFromImport("s3", "s3_client", imp, typ))
		case strings.Contains(lower, "resttemplate") || strings.Contains(lower, "webclient"):
			addDependency(project, seen, dependencyFromImport("external_api", "http_client", imp, typ))
		case strings.Contains(lower, "kafka"):
			addDependency(project, seen, dependencyFromImport("topic", "kafka", imp, typ))
		case strings.Contains(lower, "rabbit"):
			addDependency(project, seen, dependencyFromImport("queue", "rabbit", imp, typ))
		case strings.Contains(lower, "jms"):
			addDependency(project, seen, dependencyFromImport("queue", "jms", imp, typ))
		case strings.Contains(lower, "jdbctemplate"):
			addDependency(project, seen, dependencyFromImport("database_client", "jdbc", imp, typ))
		}
	}
	for _, method := range typ.Methods {
		if hasAnnotation(method.Annotations, "Value", "ConfigurationProperties") {
			addDependency(project, seen, model.Dependency{
				ID:         stableID("dependency", "config", method.ID),
				Kind:       "config_property",
				Name:       typ.Name + "." + method.Name,
				ClassID:    typ.ID,
				MethodID:   method.ID,
				Evidence:   method.Evidence,
				Source:     model.SourceFound,
				Confidence: model.ConfidenceMedium,
			})
		}
		for _, call := range method.Calls {
			if call.ResolvedExternalType != "" {
				addDependency(project, seen, externalImportDependency(call, typ, method))
			}
			target := strings.ToLower(call.Target)
			switch {
			case strings.Contains(target, "bucket") || strings.Contains(target, "s3"):
				addDependency(project, seen, methodDependency("bucket", call.Target, typ, method, call.Evidence))
			case strings.Contains(target, "send") || strings.Contains(target, "publish"):
				addDependency(project, seen, methodDependency("message_publish", call.Target, typ, method, call.Evidence))
			case strings.Contains(target, "save") || strings.Contains(target, "delete") || strings.Contains(target, "find"):
				addDependency(project, seen, methodDependency("repository_call", call.Target, typ, method, call.Evidence))
			}
		}
	}
}

func externalImportDependency(call model.Call, typ model.Type, method model.Method) model.Dependency {
	return model.Dependency{
		ID:         stableID("dependency", "external-import", method.ID, call.ResolvedExternalType, call.Method),
		Kind:       "external_dependency",
		Name:       call.ResolvedExternalType,
		Detail:     "import: " + call.ResolvedExternalType + "; call: " + call.Target,
		ClassID:    typ.ID,
		MethodID:   method.ID,
		Evidence:   call.Evidence,
		Source:     model.SourceFound,
		Confidence: call.Confidence,
	}
}

func dependencyFromImport(kind, detail, imp string, typ model.Type) model.Dependency {
	return model.Dependency{
		ID:         stableID("dependency", kind, typ.ID, imp),
		Kind:       kind,
		Name:       shortName(imp),
		Detail:     detail,
		ClassID:    typ.ID,
		Evidence:   model.Evidence{Path: typ.FilePath, Symbol: imp, Kind: "import"},
		Source:     model.SourceFound,
		Confidence: model.ConfidenceMedium,
	}
}

func methodDependency(kind, name string, typ model.Type, method model.Method, evidence model.Evidence) model.Dependency {
	return model.Dependency{
		ID:         stableID("dependency", kind, method.ID, name),
		Kind:       kind,
		Name:       name,
		ClassID:    typ.ID,
		MethodID:   method.ID,
		Evidence:   evidence,
		Source:     model.SourceInferred,
		Confidence: model.ConfidenceLow,
	}
}

func addDependency(project *model.Project, seen map[string]bool, dep model.Dependency) {
	if dep.ID == "" || seen[dep.ID] {
		return
	}
	seen[dep.ID] = true
	project.Dependencies = append(project.Dependencies, dep)
}

func addEntryPoint(project *model.Project, seen map[string]bool, entry model.EntryPoint) {
	if entry.ID == "" || seen[entry.ID] {
		return
	}
	seen[entry.ID] = true
	project.EntryPoints = append(project.EntryPoints, entry)
}

func entryPoint(project *model.Project, typ *model.Type, method model.Method, kind, path, httpMethod, resource string, evidence model.Evidence) model.EntryPoint {
	name := typ.Name + "." + method.Name
	return model.EntryPoint{
		ID:         stableID("entrypoint", kind, typ.ID, method.ID, path, resource),
		Kind:       kind,
		Framework:  "spring",
		Name:       name,
		Product:    project.Name,
		Path:       path,
		HTTPMethod: httpMethod,
		Resource:   resource,
		ClassID:    typ.ID,
		MethodID:   method.ID,
		Evidence:   evidence,
		Source:     model.SourceFound,
		Confidence: model.ConfidenceHigh,
	}
}

func findHTTPMapping(annotations []model.Annotation) (*model.Annotation, string) {
	for i := range annotations {
		switch annotations[i].Name {
		case "GetMapping":
			return &annotations[i], "GET"
		case "PostMapping":
			return &annotations[i], "POST"
		case "PutMapping":
			return &annotations[i], "PUT"
		case "PatchMapping":
			return &annotations[i], "PATCH"
		case "DeleteMapping":
			return &annotations[i], "DELETE"
		case "RequestMapping":
			method := strings.ToUpper(annotationNamedValue(&annotations[i], "method"))
			if strings.Contains(method, "POST") {
				return &annotations[i], "POST"
			}
			if strings.Contains(method, "PUT") {
				return &annotations[i], "PUT"
			}
			if strings.Contains(method, "PATCH") {
				return &annotations[i], "PATCH"
			}
			if strings.Contains(method, "DELETE") {
				return &annotations[i], "DELETE"
			}
			if strings.Contains(method, "GET") {
				return &annotations[i], "GET"
			}
			return &annotations[i], "ANY"
		}
	}
	return nil, ""
}

func findAnnotation(annotations []model.Annotation, names ...string) *model.Annotation {
	for i := range annotations {
		for _, name := range names {
			if annotations[i].Name == name {
				return &annotations[i]
			}
		}
	}
	return nil
}

func hasAnnotation(annotations []model.Annotation, names ...string) bool {
	return findAnnotation(annotations, names...) != nil
}

func annotationValue(annotation *model.Annotation) string {
	if annotation == nil {
		return ""
	}
	if value := annotation.Values["value"]; value != "" {
		return value
	}
	if value := annotation.Values["path"]; value != "" {
		return value
	}
	if value := annotation.Values["name"]; value != "" {
		return value
	}
	return ""
}

func annotationNamedValue(annotation *model.Annotation, name string) string {
	if annotation == nil {
		return ""
	}
	return annotation.Values[name]
}

func mappingPath(path string) string {
	if path == "" {
		return ""
	}
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "\"")
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func combinePaths(base, child string) string {
	if base == "" {
		return child
	}
	if child == "" {
		return base
	}
	return "/" + strings.Trim(strings.TrimRight(base, "/")+"/"+strings.TrimLeft(child, "/"), "/")
}

func listenerKind(annotation string) string {
	switch annotation {
	case "Scheduled":
		return "scheduler"
	case "EventListener":
		return "event_listener"
	default:
		return "message_listener"
	}
}

func listenerResource(annotation *model.Annotation) string {
	for _, key := range []string{"topics", "queues", "destination", "value", "fixedDelay", "cron"} {
		if value := annotation.Values[key]; value != "" {
			return value
		}
	}
	return annotation.Name
}

func isRunnerType(typ model.Type) bool {
	return implementsAny(typ, "CommandLineRunner", "ApplicationRunner")
}

func isRunnerMethod(method model.Method) bool {
	return strings.Contains(method.ReturnType, "CommandLineRunner") || strings.Contains(method.ReturnType, "ApplicationRunner")
}

func isBatchMethod(method model.Method) bool {
	if strings.Contains(method.ReturnType, "Job") || strings.Contains(method.ReturnType, "Step") {
		return hasAnnotation(method.Annotations, "Bean")
	}
	return false
}

func implementsAny(typ model.Type, names ...string) bool {
	all := append([]string{}, typ.Implements...)
	all = append(all, typ.Extends...)
	for _, candidate := range all {
		for _, name := range names {
			if candidate == name || strings.HasSuffix(candidate, "."+name) || strings.Contains(candidate, name+"<") {
				return true
			}
		}
	}
	return false
}

func shortName(value string) string {
	if idx := strings.LastIndex(value, "."); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func stableID(parts ...string) string {
	joined := strings.Join(parts, ":")
	replacer := strings.NewReplacer("\\", "/", " ", "-", ":", "-", ".", "-", "_", "-", "{", "", "}", "", "(", "", ")", "")
	return strings.Trim(replacer.Replace(strings.ToLower(joined)), "-")
}

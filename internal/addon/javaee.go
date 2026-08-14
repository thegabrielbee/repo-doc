package addon

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/bee/java-process-mapper/internal/model"
)

type JavaEE struct{}

func (JavaEE) Name() string { return "javaee" }

func (JavaEE) Analyze(project *model.Project) error {
	sourceByPath := map[string]model.SourceFile{}
	for _, source := range project.SourceFiles {
		sourceByPath[source.Path] = source
	}
	namedBeans := map[string]*model.Type{}
	for i := range project.Types {
		typ := &project.Types[i]
		for _, beanName := range beanNames(typ) {
			namedBeans[beanName] = typ
		}
	}

	seenEntryPoints := existingEntryPointIDs(project)
	seenDeps := existingDependencyIDs(project)
	for i := range project.Types {
		typ := &project.Types[i]
		addJavaEETypeEntryPoints(project, seenEntryPoints, typ)
		addJavaEEMethodEntryPoints(project, seenEntryPoints, typ)
		addJavaEETypeDependencies(project, seenDeps, *typ, sourceByPath[typ.FilePath])
	}
	for _, module := range project.Modules {
		for _, descriptor := range module.DescriptorFiles {
			addDescriptorEntryPoints(project, seenEntryPoints, descriptor)
			addDescriptorDependencies(project, seenDeps, descriptor)
		}
		for _, uiFile := range module.UIFiles {
			addUIEntryPoints(project, seenEntryPoints, seenDeps, uiFile, namedBeans)
		}
	}
	sort.Slice(project.EntryPoints, func(i, j int) bool { return project.EntryPoints[i].ID < project.EntryPoints[j].ID })
	sort.Slice(project.Dependencies, func(i, j int) bool { return project.Dependencies[i].ID < project.Dependencies[j].ID })
	return nil
}

func addJavaEETypeEntryPoints(project *model.Project, seen map[string]bool, typ *model.Type) {
	if ann := findAnnotation(typ.Annotations, "WebServlet"); ann != nil {
		resource := firstAnnotationValue(ann, "urlPatterns", "value", "name")
		addEntryPoint(project, seen, javaEEEntryPoint(project, typ, model.Method{}, "servlet", mappingPath(resource), "", resource, ann.Evidence))
	}
	if ann := findAnnotation(typ.Annotations, "WebListener"); ann != nil {
		resource := firstAnnotationValue(ann, "value", "name")
		if resource == "" {
			resource = typ.Name
		}
		addEntryPoint(project, seen, javaEEEntryPoint(project, typ, model.Method{}, "listener", "", "", resource, ann.Evidence))
	}
	if ann := findAnnotation(typ.Annotations, "Startup"); ann != nil && !hasMethodAnnotation(typ, "PostConstruct") {
		addEntryPoint(project, seen, javaEEEntryPoint(project, typ, model.Method{}, "startup", "", "", typ.Name, ann.Evidence))
	}
	if ann := findAnnotation(typ.Annotations, "MessageDriven"); ann != nil && !hasMethodNamed(typ, "onMessage") {
		resource := messageDrivenResource(ann)
		addEntryPoint(project, seen, javaEEEntryPoint(project, typ, model.Method{}, "message_listener", "", "", resource, ann.Evidence))
	}
}

func addJavaEEMethodEntryPoints(project *model.Project, seen map[string]bool, typ *model.Type) {
	classPath := mappingPath(annotationValue(findAnnotation(typ.Annotations, "Path")))
	isWebService := hasAnnotation(typ.Annotations, "WebService")
	isMDB := hasAnnotation(typ.Annotations, "MessageDriven")
	websocketPath := mappingPath(annotationValue(findAnnotation(typ.Annotations, "ServerEndpoint")))
	isServerEndpoint := websocketPath != ""
	isManagedType := isContainerManagedType(typ)

	for _, method := range typ.Methods {
		if ann, httpMethod := findJaxRSHTTPMethod(method.Annotations); ann != nil {
			methodPath := mappingPath(annotationValue(findAnnotation(method.Annotations, "Path")))
			path := combinePaths(classPath, methodPath)
			if path != "" {
				addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "http", path, httpMethod, path, ann.Evidence))
			}
		}

		if isWebService && !webMethodExcluded(method) && isWebServiceOperation(typ, method) {
			webMethod := findAnnotation(method.Annotations, "WebMethod")
			evidence := typ.Evidence
			if webMethod != nil {
				evidence = webMethod.Evidence
			}
			resource := soapOperationName(typ, method, webMethod)
			addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "soap", "", "", resource, evidence))
		}

		for _, annName := range []string{"Schedule", "Schedules", "Timeout"} {
			ann := findAnnotation(method.Annotations, annName)
			if ann == nil {
				continue
			}
			addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "scheduler", "", "", javaEEScheduleResource(ann), ann.Evidence))
		}
		if ann := findAnnotation(method.Annotations, "PostConstruct"); ann != nil && isManagedType {
			addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "startup", "", "", method.Name, ann.Evidence))
		}
		if isMDB && method.Name == "onMessage" {
			ann := findAnnotation(typ.Annotations, "MessageDriven")
			resource := messageDrivenResource(ann)
			addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "message_listener", "", "", resource, ann.Evidence))
		}
		if isServerEndpoint {
			if ann := findAnnotation(method.Annotations, "OnOpen", "OnMessage", "OnClose", "OnError"); ann != nil {
				resource := websocketResource(websocketPath, ann.Name)
				addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "websocket", websocketPath, "", resource, ann.Evidence))
			}
		}
		for _, param := range method.Parameters {
			if ann := findAnnotation(param.Annotations, "Observes", "ObservesAsync"); ann != nil {
				resource := param.Type
				if resource == "" {
					resource = ann.Name
				}
				if isCDIPortableExtensionEvent(resource) {
					continue
				}
				addEntryPoint(project, seen, javaEEEntryPoint(project, typ, method, "event_listener", "", "", resource, ann.Evidence))
			}
		}
	}
}

func addJavaEETypeDependencies(project *model.Project, seen map[string]bool, typ model.Type, source model.SourceFile) {
	if ann := findAnnotation(typ.Annotations, "WebFilter"); ann != nil {
		pattern := firstAnnotationValue(ann, "urlPatterns", "value")
		name := firstAnnotationValue(ann, "filterName", "name")
		if name == "" {
			name = typ.Name
		}
		addDependency(project, seen, model.Dependency{
			ID:         stableID("dependency", "http-filter", typ.ID, pattern),
			Kind:       "http_filter",
			Name:       name,
			Detail:     mappingPath(pattern),
			ClassID:    typ.ID,
			Evidence:   ann.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
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
	if implementsAny(typ, "LoginModule") || hasImportContaining(source.Imports, "javax.security.auth.spi.loginmodule") {
		addDependency(project, seen, model.Dependency{
			ID:         stableID("dependency", "auth-provider", typ.ID),
			Kind:       "auth_provider",
			Name:       typ.Name,
			Detail:     "JAAS LoginModule",
			ClassID:    typ.ID,
			Evidence:   typ.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
		})
	}
	for _, field := range typ.Fields {
		if ann := findAnnotation(field.Annotations, "PersistenceContext", "PersistenceUnit"); ann != nil {
			name := firstAnnotationValue(ann, "unitName", "name", "value")
			if name == "" {
				name = field.FieldType
			}
			if name == "" {
				name = "EntityManager"
			}
			addDependency(project, seen, model.Dependency{
				ID:         stableID("dependency", "persistence-context", field.ID, name),
				Kind:       "database_client",
				Name:       name,
				Detail:     ann.Name,
				ClassID:    typ.ID,
				Evidence:   ann.Evidence,
				Source:     model.SourceFound,
				Confidence: model.ConfidenceHigh,
			})
		}
		if ann := findAnnotation(field.Annotations, "Resource"); ann != nil {
			name := firstAnnotationValue(ann, "lookup", "mappedName", "name", "value")
			if name != "" && looksLikeJMSResource(name) {
				addDependency(project, seen, model.Dependency{
					ID:         stableID("dependency", "jndi-resource", field.ID, name),
					Kind:       "queue",
					Name:       name,
					Detail:     "JNDI resource",
					ClassID:    typ.ID,
					Evidence:   ann.Evidence,
					Source:     model.SourceFound,
					Confidence: model.ConfidenceHigh,
				})
			}
		}
	}
	for _, imp := range source.Imports {
		lower := strings.ToLower(imp)
		switch {
		case strings.Contains(lower, "entitymanager") || strings.Contains(lower, ".persistence."):
			addDependency(project, seen, dependencyFromImport("database_client", "jpa", imp, typ))
		case strings.Contains(lower, ".jms.") || strings.Contains(lower, "jms."):
			addDependency(project, seen, dependencyFromImport("queue", "jms", imp, typ))
		case strings.Contains(lower, ".xml.ws") || strings.Contains(lower, "javax.jws") || strings.Contains(lower, "jakarta.jws") || strings.Contains(lower, "org.apache.axis"):
			addDependency(project, seen, dependencyFromImport("external_api", "soap_client", imp, typ))
		case strings.Contains(lower, "httpclient") || strings.Contains(lower, "httpurlconnection") || strings.Contains(lower, "resteasy") || strings.Contains(lower, "jaxrs.client") || strings.Contains(lower, ".client.client"):
			addDependency(project, seen, dependencyFromImport("external_api", "http_client", imp, typ))
		case strings.Contains(lower, "javax.mail") || strings.Contains(lower, "jakarta.mail"):
			addDependency(project, seen, dependencyFromImport("mail_server", "mail", imp, typ))
		case strings.Contains(lower, "ftp") || strings.Contains(lower, "sftp"):
			addDependency(project, seen, dependencyFromImport("ftp_endpoint", "ftp", imp, typ))
		case strings.Contains(lower, "redis") || strings.Contains(lower, "jedis"):
			addDependency(project, seen, dependencyFromImport("cache", "redis", imp, typ))
		case strings.Contains(lower, "s3"):
			addDependency(project, seen, dependencyFromImport("s3", "s3_client", imp, typ))
		case strings.Contains(lower, "sqs"):
			addDependency(project, seen, dependencyFromImport("queue", "sqs", imp, typ))
		}
	}
	for _, method := range typ.Methods {
		for _, call := range method.Calls {
			if call.ResolvedExternalType != "" {
				addDependency(project, seen, externalImportDependency(call, typ, method))
			}
			target := strings.ToLower(call.Target)
			switch {
			case strings.Contains(target, "createquery") || strings.Contains(target, "createbuilder") || strings.Contains(target, "find") || strings.Contains(target, "persist") || strings.Contains(target, "merge") || strings.Contains(target, "remove"):
				addDependency(project, seen, methodDependency("repository_call", call.Target, typ, method, call.Evidence))
			case strings.Contains(target, "send") || strings.Contains(target, "publish") || strings.Contains(target, "fire"):
				addDependency(project, seen, methodDependency("message_publish", call.Target, typ, method, call.Evidence))
			case strings.Contains(target, "bucket") || strings.Contains(target, "s3"):
				addDependency(project, seen, methodDependency("bucket", call.Target, typ, method, call.Evidence))
			}
		}
	}
}

func addDescriptorEntryPoints(project *model.Project, seen map[string]bool, path string) {
	name := strings.ToLower(filepath.Base(path))
	data := readDescriptor(path)
	switch name {
	case "web.xml":
		for _, item := range parseWebXMLMappings(data) {
			if item.Kind != "servlet" {
				continue
			}
			entry := model.EntryPoint{
				ID:         stableID("entrypoint", "javaee", "webxml", path, item.Kind, item.Name, item.Pattern),
				Kind:       item.Kind,
				Framework:  "javaee",
				Name:       item.Name,
				Product:    project.Name,
				Path:       mappingPath(item.Pattern),
				Resource:   item.Pattern,
				Evidence:   model.Evidence{Path: path, Symbol: item.Name, Kind: "descriptor"},
				Source:     model.SourceFound,
				Confidence: model.ConfidenceMedium,
			}
			addEntryPoint(project, seen, entry)
		}
	}
}

func addDescriptorDependencies(project *model.Project, seen map[string]bool, path string) {
	name := strings.ToLower(filepath.Base(path))
	data := readDescriptor(path)
	switch name {
	case "persistence.xml":
		for _, unit := range regexp.MustCompile(`(?is)<persistence-unit[^>]*\bname\s*=\s*["']([^"']+)["']`).FindAllStringSubmatch(data, -1) {
			addDependency(project, seen, model.Dependency{
				ID:         stableID("dependency", "persistence-unit", path, unit[1]),
				Kind:       "persistence_unit",
				Name:       strings.TrimSpace(unit[1]),
				Detail:     "persistence.xml",
				Evidence:   model.Evidence{Path: path, Symbol: strings.TrimSpace(unit[1]), Kind: "descriptor"},
				Source:     model.SourceFound,
				Confidence: model.ConfidenceHigh,
			})
		}
	case "web.xml":
		for _, item := range parseWebXMLMappings(data) {
			if item.Kind != "filter" {
				continue
			}
			addDependency(project, seen, model.Dependency{
				ID:         stableID("dependency", "http-filter", path, item.Name, item.Pattern),
				Kind:       "http_filter",
				Name:       item.Name,
				Detail:     mappingPath(item.Pattern),
				Evidence:   model.Evidence{Path: path, Symbol: item.Name, Kind: "descriptor"},
				Source:     model.SourceFound,
				Confidence: model.ConfidenceMedium,
			})
		}
		for _, jndi := range descriptorJNDIResources(data) {
			kind := "queue"
			if strings.Contains(strings.ToLower(jndi), "topic") {
				kind = "topic"
			}
			addDependency(project, seen, model.Dependency{
				ID:         stableID("dependency", "jndi", path, jndi),
				Kind:       kind,
				Name:       jndi,
				Detail:     "descriptor JNDI resource",
				Evidence:   model.Evidence{Path: path, Symbol: jndi, Kind: "descriptor"},
				Source:     model.SourceFound,
				Confidence: model.ConfidenceMedium,
			})
		}
	case "ejb-jar.xml", "jboss-ejb3.xml":
		for _, jndi := range descriptorJNDIResources(data) {
			kind := "queue"
			if strings.Contains(strings.ToLower(jndi), "topic") {
				kind = "topic"
			}
			addDependency(project, seen, model.Dependency{
				ID:         stableID("dependency", "jndi", path, jndi),
				Kind:       kind,
				Name:       jndi,
				Detail:     "descriptor JNDI resource",
				Evidence:   model.Evidence{Path: path, Symbol: jndi, Kind: "descriptor"},
				Source:     model.SourceFound,
				Confidence: model.ConfidenceMedium,
			})
		}
	}
}

func addUIEntryPoints(project *model.Project, seen map[string]bool, seenDeps map[string]bool, path string, namedBeans map[string]*model.Type) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	apiCalls := uiClientAPICalls(path, string(data))
	for _, call := range apiCalls {
		addEntryPoint(project, seen, model.EntryPoint{
			ID:         stableID("entrypoint", "javaee", "ui-api", path, call.Evidence.Path, call.Method, call.Target),
			Kind:       "ui_page",
			Framework:  "javaee",
			Name:       filepath.Base(path) + " -> " + call.Name,
			Product:    project.Name,
			Path:       call.Target,
			HTTPMethod: call.Method,
			Resource:   filepath.ToSlash(path),
			Evidence:   call.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceMedium,
		})
		addDependency(project, seenDeps, model.Dependency{
			ID:         stableID("dependency", call.Kind, path, call.Evidence.Path, call.Method, call.Target),
			Kind:       call.Kind,
			Name:       call.Name,
			Detail:     call.Target,
			Evidence:   call.Evidence,
			Source:     model.SourceFound,
			Confidence: model.ConfidenceMedium,
		})
	}
	matches := regexp.MustCompile(`#\{\s*([A-Za-z_$][A-Za-z0-9_$]*)\.([A-Za-z_$][A-Za-z0-9_$]*)`).FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		if len(apiCalls) > 0 {
			return
		}
		addEntryPoint(project, seen, model.EntryPoint{
			ID:         stableID("entrypoint", "javaee", "ui", path),
			Kind:       "ui_page",
			Framework:  "javaee",
			Name:       filepath.Base(path),
			Product:    project.Name,
			Resource:   filepath.ToSlash(path),
			Evidence:   model.Evidence{Path: path, Kind: "ui_page"},
			Source:     model.SourceFound,
			Confidence: model.ConfidenceMedium,
		})
		return
	}
	localSeen := map[string]bool{}
	for _, match := range matches {
		beanName, methodName := match[1], match[2]
		key := beanName + "." + methodName
		if localSeen[key] {
			continue
		}
		localSeen[key] = true
		entry := model.EntryPoint{
			ID:         stableID("entrypoint", "javaee", "ui", path, key),
			Kind:       "ui_page",
			Framework:  "javaee",
			Name:       filepath.Base(path) + " -> " + key,
			Product:    project.Name,
			Resource:   filepath.ToSlash(path),
			Evidence:   model.Evidence{Path: path, Symbol: key, Kind: "ui_page"},
			Source:     model.SourceFound,
			Confidence: model.ConfidenceMedium,
		}
		if typ := namedBeans[beanName]; typ != nil {
			entry.ClassID = typ.ID
			if method := findMethodByName(*typ, methodName); method.ID != "" {
				entry.MethodID = method.ID
				entry.Confidence = model.ConfidenceHigh
			}
		}
		addEntryPoint(project, seen, entry)
	}
}

type uiClientAPICall struct {
	Kind     string
	Method   string
	Target   string
	Name     string
	Evidence model.Evidence
}

type uiScriptSource struct {
	Path string
	Data string
}

func uiClientAPICalls(pagePath, data string) []uiClientAPICall {
	sources := []uiScriptSource{{Path: pagePath, Data: data}}
	for _, scriptPath := range uiScriptRefs(pagePath, data) {
		scriptData, err := os.ReadFile(scriptPath)
		if err != nil {
			continue
		}
		sources = append(sources, uiScriptSource{Path: scriptPath, Data: string(scriptData)})
	}
	var calls []uiClientAPICall
	seen := map[string]bool{}
	for _, source := range sources {
		for _, call := range parseUIClientAPICalls(source.Path, source.Data) {
			key := call.Kind + "\x00" + call.Method + "\x00" + call.Target + "\x00" + call.Evidence.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			calls = append(calls, call)
		}
	}
	return calls
}

func uiScriptRefs(pagePath, data string) []string {
	refs := map[string]bool{}
	re := regexp.MustCompile(`(?is)<script[^>]+\bsrc\s*=\s*["']([^"']+)["']`)
	for _, match := range re.FindAllStringSubmatch(data, -1) {
		src := strings.TrimSpace(match[1])
		if src == "" || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "//") {
			continue
		}
		if idx := strings.IndexAny(src, "?#"); idx >= 0 {
			src = src[:idx]
		}
		scriptPath := filepath.Clean(filepath.Join(filepath.Dir(pagePath), filepath.FromSlash(src)))
		if info, err := os.Stat(scriptPath); err == nil && !info.IsDir() {
			refs[scriptPath] = true
		}
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func parseUIClientAPICalls(path, data string) []uiClientAPICall {
	vars := jsURLVariables(data)
	var calls []uiClientAPICall
	openRe := regexp.MustCompile(`(?i)\.open\s*\(\s*["']([A-Z]+)["']\s*,\s*([^,\)]+)`)
	for _, match := range openRe.FindAllStringSubmatchIndex(data, -1) {
		method := strings.ToUpper(data[match[2]:match[3]])
		target := resolveJSURL(data[match[4]:match[5]], vars)
		if target == "" {
			continue
		}
		calls = append(calls, uiClientAPICall{
			Kind:   "ui_api_call",
			Method: method,
			Target: target,
			Name:   method + " " + target,
			Evidence: model.Evidence{
				Path:   path,
				Line:   lineNumberAt(data, match[0]),
				Symbol: method + " " + target,
				Kind:   "ui_api_call",
			},
		})
	}
	fetchRe := regexp.MustCompile(`(?is)\bfetch\s*\(\s*([^,\)]+)(.*?)\)`)
	for _, match := range fetchRe.FindAllStringSubmatchIndex(data, -1) {
		target := resolveJSURL(data[match[2]:match[3]], vars)
		if target == "" {
			continue
		}
		method := "GET"
		if match[4] >= 0 {
			if methodMatch := regexp.MustCompile(`(?i)\bmethod\s*:\s*["']([A-Z]+)["']`).FindStringSubmatch(data[match[4]:match[5]]); len(methodMatch) == 2 {
				method = strings.ToUpper(methodMatch[1])
			}
		}
		calls = append(calls, uiClientAPICall{
			Kind:   "ui_api_call",
			Method: method,
			Target: target,
			Name:   method + " " + target,
			Evidence: model.Evidence{
				Path:   path,
				Line:   lineNumberAt(data, match[0]),
				Symbol: method + " " + target,
				Kind:   "ui_api_call",
			},
		})
	}
	wsRe := regexp.MustCompile(`(?i)\bnew\s+WebSocket\s*\(\s*([^)]+)\)`)
	for _, match := range wsRe.FindAllStringSubmatchIndex(data, -1) {
		target := resolveJSURL(data[match[2]:match[3]], vars)
		if target == "" {
			continue
		}
		calls = append(calls, uiClientAPICall{
			Kind:   "ui_websocket",
			Method: "WEBSOCKET",
			Target: target,
			Name:   "WebSocket " + target,
			Evidence: model.Evidence{
				Path:   path,
				Line:   lineNumberAt(data, match[0]),
				Symbol: "WebSocket " + target,
				Kind:   "ui_websocket",
			},
		})
	}
	return calls
}

func jsURLVariables(data string) map[string]string {
	vars := map[string]string{}
	re := regexp.MustCompile(`(?s)\b(?:var|let|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*([^;]+);`)
	for _, match := range re.FindAllStringSubmatch(data, -1) {
		if target := summarizeJSURLExpression(match[2]); target != "" {
			vars[match[1]] = target
		}
	}
	return vars
}

func resolveJSURL(expr string, vars map[string]string) string {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, ";")
	if value := vars[expr]; value != "" {
		return value
	}
	return summarizeJSURLExpression(expr)
}

func summarizeJSURLExpression(expr string) string {
	expr = strings.TrimSpace(expr)
	expr = strings.Trim(expr, "\"'")
	replacer := strings.NewReplacer(
		"window.location.hostname", "{hostname}",
		"document.location.hostname", "{hostname}",
		"location.hostname", "{hostname}",
		"window.location.host", "{host}",
		"document.location.host", "{host}",
		"location.host", "{host}",
		"window.location.port", "{port}",
		"document.location.port", "{port}",
		"location.port", "{port}",
		"window.location.pathname", "{path}",
		"document.location.pathname", "{path}",
		"location.pathname", "{path}",
		"\"", "",
		"'", "",
		"+", "",
	)
	target := strings.Join(strings.Fields(replacer.Replace(expr)), "")
	if !looksLikeClientTarget(target) {
		return ""
	}
	return target
}

func looksLikeClientTarget(target string) bool {
	lower := strings.ToLower(target)
	switch lower {
	case "http://", "https://", "ws://", "wss://":
		return false
	}
	return strings.Contains(lower, "http://") ||
		strings.Contains(lower, "https://") ||
		strings.Contains(lower, "ws://") ||
		strings.Contains(lower, "wss://") ||
		strings.Contains(lower, "webresources") ||
		strings.Contains(lower, "/api") ||
		strings.Contains(lower, "/rest")
}

func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func lineNumberAt(data string, index int) int {
	if index <= 0 {
		return 1
	}
	if index > len(data) {
		index = len(data)
	}
	return strings.Count(data[:index], "\n") + 1
}

func javaEEEntryPoint(project *model.Project, typ *model.Type, method model.Method, kind, path, httpMethod, resource string, evidence model.Evidence) model.EntryPoint {
	name := typ.Name
	methodID := ""
	if method.Name != "" {
		name += "." + method.Name
		methodID = method.ID
	}
	return model.EntryPoint{
		ID:         stableID("entrypoint", "javaee", kind, typ.ID, methodID, path, resource),
		Kind:       kind,
		Framework:  "javaee",
		Name:       name,
		Product:    project.Name,
		Path:       path,
		HTTPMethod: httpMethod,
		Resource:   resource,
		ClassID:    typ.ID,
		MethodID:   methodID,
		Evidence:   evidence,
		Source:     model.SourceFound,
		Confidence: model.ConfidenceHigh,
	}
}

func findJaxRSHTTPMethod(annotations []model.Annotation) (*model.Annotation, string) {
	for i := range annotations {
		switch annotations[i].Name {
		case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
			return &annotations[i], annotations[i].Name
		}
	}
	return nil, ""
}

func webMethodExcluded(method model.Method) bool {
	ann := findAnnotation(method.Annotations, "WebMethod")
	if ann == nil {
		return false
	}
	return strings.EqualFold(annotationNamedValue(ann, "exclude"), "true")
}

func isWebServiceOperation(typ *model.Type, method model.Method) bool {
	if !hasBusinessSignature(method) || hasModifier(method.Modifiers, "static") {
		return false
	}
	if hasAnnotation(method.Annotations, "WebMethod") {
		return true
	}
	if typ.Kind == "interface" {
		return true
	}
	return hasModifier(method.Modifiers, "public")
}

func hasBusinessSignature(method model.Method) bool {
	return method.Name != "" && method.ReturnType != "<constructor>" && method.ReturnType != ""
}

func hasModifier(modifiers []string, name string) bool {
	for _, modifier := range modifiers {
		if modifier == name {
			return true
		}
	}
	return false
}

func soapOperationName(typ *model.Type, method model.Method, ann *model.Annotation) string {
	if ann != nil {
		if value := firstAnnotationValue(ann, "operationName", "action", "name", "value"); value != "" {
			return value
		}
	}
	if service := findAnnotation(typ.Annotations, "WebService"); service != nil {
		if value := firstAnnotationValue(service, "serviceName", "name", "value"); value != "" {
			return value + "." + method.Name
		}
	}
	return typ.Name + "." + method.Name
}

func websocketResource(path, event string) string {
	if path == "" {
		return event
	}
	if event == "" {
		return path
	}
	return event + " " + path
}

func isCDIPortableExtensionEvent(resource string) bool {
	normalized := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(resource)
	if idx := strings.Index(normalized, "<"); idx >= 0 {
		normalized = normalized[:idx]
	}
	if idx := strings.LastIndex(normalized, "."); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	switch normalized {
	case "BeforeBeanDiscovery",
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
		"ProcessSyntheticObserverMethod":
		return true
	default:
		return false
	}
}

func javaEEScheduleResource(annotation *model.Annotation) string {
	var parts []string
	for _, key := range []string{"second", "minute", "hour", "dayOfWeek", "dayOfMonth", "month", "year", "info"} {
		if value := annotationNamedValue(annotation, key); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if annotation.Raw != "" {
		return annotation.Raw
	}
	return annotation.Name
}

func messageDrivenResource(annotation *model.Annotation) string {
	if annotation == nil {
		return "MessageDriven"
	}
	for _, key := range []string{"destinationLookup", "destination", "mappedName", "name", "value"} {
		if value := annotationNamedValue(annotation, key); value != "" {
			return value
		}
	}
	raw := annotation.Raw
	for _, key := range []string{"destinationLookup", "destination", "queue", "topic"} {
		if value := activationConfigValue(raw, key); value != "" {
			return value
		}
	}
	if annotation.Raw != "" {
		return annotation.Raw
	}
	return "MessageDriven"
}

func activationConfigValue(raw, key string) string {
	propertyPattern := regexp.MustCompile(`propertyName\s*=\s*"` + regexp.QuoteMeta(key) + `"\s*,\s*propertyValue\s*=\s*"([^"]+)"`)
	if match := propertyPattern.FindStringSubmatch(raw); len(match) == 2 {
		return match[1]
	}
	directPattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*=\s*"([^"]+)"`)
	if match := directPattern.FindStringSubmatch(raw); len(match) == 2 {
		return match[1]
	}
	return ""
}

func firstAnnotationValue(annotation *model.Annotation, keys ...string) string {
	for _, key := range keys {
		if value := annotationNamedValue(annotation, key); value != "" {
			return firstAnnotationScalar(value)
		}
	}
	return ""
}

func firstAnnotationScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "{}")
	parts := splitAnnotationList(value)
	if len(parts) > 0 {
		value = parts[0]
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
}

func splitAnnotationList(value string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range value {
		switch r {
		case '{', '(':
			depth++
		case '}', ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:i]))
				start = i + 1
			}
		}
	}
	tail := strings.TrimSpace(value[start:])
	if tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func beanNames(typ *model.Type) []string {
	var names []string
	for _, annName := range []string{"Named", "ManagedBean"} {
		if ann := findAnnotation(typ.Annotations, annName); ann != nil {
			if value := annotationValue(ann); value != "" {
				names = append(names, value)
			}
			names = append(names, lowerFirst(typ.Name))
		}
	}
	return names
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func findMethodByName(typ model.Type, name string) model.Method {
	for _, method := range typ.Methods {
		if method.Name == name {
			return method
		}
	}
	return model.Method{}
}

func hasMethodNamed(typ *model.Type, name string) bool {
	for _, method := range typ.Methods {
		if method.Name == name {
			return true
		}
	}
	return false
}

func hasMethodAnnotation(typ *model.Type, names ...string) bool {
	for _, method := range typ.Methods {
		if hasAnnotation(method.Annotations, names...) {
			return true
		}
	}
	return false
}

func isContainerManagedType(typ *model.Type) bool {
	return hasAnnotation(typ.Annotations,
		"Stateless",
		"Stateful",
		"Singleton",
		"MessageDriven",
		"Startup",
		"WebServlet",
		"WebFilter",
		"WebListener",
		"WebService",
		"Path",
		"ServerEndpoint",
		"Named",
		"ManagedBean",
		"ApplicationScoped",
		"RequestScoped",
		"SessionScoped",
		"ConversationScoped",
		"Dependent",
		"FacesComponent",
		"FacesConverter",
		"FacesValidator",
	)
}

func hasImportContaining(imports []string, fragment string) bool {
	fragment = strings.ToLower(fragment)
	for _, imp := range imports {
		if strings.Contains(strings.ToLower(imp), fragment) {
			return true
		}
	}
	return false
}

func looksLikeJMSResource(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "jms/") || strings.Contains(lower, "queue") || strings.Contains(lower, "topic")
}

type webXMLMapping struct {
	Kind    string
	Name    string
	Pattern string
}

func parseWebXMLMappings(data string) []webXMLMapping {
	namesByKind := map[string]map[string]string{
		"servlet": {},
		"filter":  {},
	}
	for _, kind := range []string{"servlet", "filter"} {
		blockPattern := regexp.MustCompile(`(?is)<` + kind + `>\s*(.*?)\s*</` + kind + `>`)
		for _, block := range blockPattern.FindAllStringSubmatch(data, -1) {
			if len(block) != 2 {
				continue
			}
			name := firstXMLValue(block[1], kind+"-name")
			className := firstXMLValue(block[1], kind+"-class")
			if name == "" {
				name = className
			}
			if name != "" {
				namesByKind[kind][name] = className
			}
		}
	}
	var mappings []webXMLMapping
	for _, kind := range []string{"servlet", "filter"} {
		blockPattern := regexp.MustCompile(`(?is)<` + kind + `-mapping>\s*(.*?)\s*</` + kind + `-mapping>`)
		for _, block := range blockPattern.FindAllStringSubmatch(data, -1) {
			if len(block) != 2 {
				continue
			}
			name := firstXMLValue(block[1], kind+"-name")
			for _, pattern := range xmlTagValues(block[1], "url-pattern") {
				label := name
				if className := namesByKind[kind][name]; className != "" {
					label = className
				}
				mappings = append(mappings, webXMLMapping{Kind: kind, Name: label, Pattern: pattern})
			}
		}
	}
	return mappings
}

func descriptorJNDIResources(data string) []string {
	var resources []string
	for _, tag := range []string{"mapped-name", "lookup-name", "jndi-name", "destination", "destination-jndi-name", "res-ref-name", "message-destination-link"} {
		resources = append(resources, xmlTagValues(data, tag)...)
	}
	return uniqueStrings(resources)
}

func readDescriptor(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func xmlTagValues(data, tag string) []string {
	pattern := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `>\s*([^<]+?)\s*</` + regexp.QuoteMeta(tag) + `>`)
	var values []string
	for _, match := range pattern.FindAllStringSubmatch(data, -1) {
		if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			values = append(values, strings.TrimSpace(match[1]))
		}
	}
	return values
}

func firstXMLValue(data, tag string) string {
	values := xmlTagValues(data, tag)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func existingEntryPointIDs(project *model.Project) map[string]bool {
	seen := map[string]bool{}
	for _, entry := range project.EntryPoints {
		seen[entry.ID] = true
	}
	return seen
}

func existingDependencyIDs(project *model.Project) map[string]bool {
	seen := map[string]bool{}
	for _, dep := range project.Dependencies {
		seen[dep.ID] = true
	}
	return seen
}

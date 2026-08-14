package discovery

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bee/java-process-mapper/internal/model"
	"gopkg.in/yaml.v3"
)

type Options struct {
	RootPath     string
	JavaVersion  string
	IncludeTests bool
}

func Discover(opts Options) (*model.Project, error) {
	root, err := filepath.Abs(opts.RootPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path is not a directory: %s", root)
	}

	project := &model.Project{
		Name: filepath.Base(root),
		Root: filepath.Clean(root),
	}

	modules, err := discoverModules(root)
	if err != nil {
		return nil, err
	}
	if len(modules) == 0 {
		modules = []model.Module{{
			ID:   stableID("module", root),
			Name: filepath.Base(root),
			Path: filepath.Clean(root),
		}}
	}

	modulePaths := map[string]bool{}
	for _, module := range modules {
		modulePaths[filepath.Clean(module.Path)] = true
	}
	for i := range modules {
		if err := fillModule(&modules[i], opts.IncludeTests, modulePaths); err != nil {
			return nil, err
		}
		fillModuleBuildMetadata(root, &modules[i], opts.JavaVersion)
		for _, cfg := range modules[i].ConfigFiles {
			props, err := parseConfigFile(cfg)
			if err != nil {
				project.Gaps = append(project.Gaps, model.Gap{
					ID:         stableID("gap", cfg, "config-parse"),
					Message:    "Could not parse config file: " + err.Error(),
					Severity:   "medium",
					Evidence:   evidence(cfg, 1, "config_file", ""),
					Source:     model.SourceFound,
					Confidence: model.ConfidenceMedium,
				})
				continue
			}
			project.ConfigProperties = append(project.ConfigProperties, props...)
		}
	}

	sort.Slice(modules, func(i, j int) bool { return modules[i].Path < modules[j].Path })
	project.Modules = modules
	project.JavaVersion = aggregateJavaVersion(modules)
	project.RefreshSummary()
	return project, nil
}

func discoverModules(root string) ([]model.Module, error) {
	var modules []model.Module
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name != "pom.xml" && name != "build.gradle" && name != "build.gradle.kts" {
			return nil
		}
		dir := filepath.Dir(path)
		modules = append(modules, model.Module{
			ID:        stableID("module", dir),
			Name:      moduleName(root, dir),
			Path:      filepath.Clean(dir),
			BuildTool: buildTool(name),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dedupeModules(modules), nil
}

func fillModule(module *model.Module, includeTests bool, modulePaths map[string]bool) error {
	sourceRootSet := map[string]bool{}
	err := filepath.WalkDir(module.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			clean := filepath.Clean(path)
			if clean != filepath.Clean(module.Path) && modulePaths[clean] {
				return filepath.SkipDir
			}
			if shouldSkipDir(d.Name()) && path != module.Path {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		clean := filepath.Clean(path)
		lower := strings.ToLower(filepath.ToSlash(clean))

		switch {
		case strings.HasSuffix(name, ".java"):
			if !includeTests && isTestPath(lower) {
				return nil
			}
			module.JavaFiles = append(module.JavaFiles, clean)
			if root := sourceRoot(clean); root != "" {
				sourceRootSet[root] = true
			}
		case isConfigFile(name):
			module.ConfigFiles = append(module.ConfigFiles, clean)
		case isMigrationFile(lower):
			module.MigrationFiles = append(module.MigrationFiles, clean)
		case isDescriptorFile(name, lower):
			module.DescriptorFiles = append(module.DescriptorFiles, clean)
		case isUIFile(name):
			module.UIFiles = append(module.UIFiles, clean)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for root := range sourceRootSet {
		module.SourceRoots = append(module.SourceRoots, root)
	}
	sort.Strings(module.SourceRoots)
	sort.Strings(module.JavaFiles)
	sort.Strings(module.ConfigFiles)
	sort.Strings(module.MigrationFiles)
	sort.Strings(module.DescriptorFiles)
	sort.Strings(module.UIFiles)
	return nil
}

func fillModuleBuildMetadata(root string, module *model.Module, overrideJavaVersion string) {
	module.Packaging = inferPackaging(module.Path, module.BuildTool)
	if version := normalizeJavaVersion(overrideJavaVersion); version != "" {
		module.JavaVersion = version
		return
	}
	if version := inferJavaVersionFromAncestors(root, module.Path, module.BuildTool); version != "" {
		module.JavaVersion = version
		return
	}
	module.JavaVersion = "unknown"
}

func inferPackaging(modulePath, buildTool string) string {
	switch buildTool {
	case "maven":
		if packaging := mavenPackaging(filepath.Join(modulePath, "pom.xml")); packaging != "" {
			return packaging
		}
	case "gradle":
		if packaging := gradlePackaging(firstExisting(modulePath, "build.gradle", "build.gradle.kts")); packaging != "" {
			return packaging
		}
	}
	return ""
}

func inferJavaVersionFromAncestors(root, modulePath, buildTool string) string {
	root = filepath.Clean(root)
	for dir := filepath.Clean(modulePath); ; dir = filepath.Dir(dir) {
		if buildTool == "maven" || buildTool == "" {
			if version := mavenJavaVersion(filepath.Join(dir, "pom.xml")); version != "" {
				return version
			}
		}
		if buildTool == "gradle" || buildTool == "" {
			if version := gradleJavaVersion(firstExisting(dir, "build.gradle", "build.gradle.kts")); version != "" {
				return version
			}
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return ""
}

func aggregateJavaVersion(modules []model.Module) string {
	if len(modules) == 0 {
		return "unknown"
	}
	seen := map[string]bool{}
	for _, module := range modules {
		version := module.JavaVersion
		if version == "" {
			version = "unknown"
		}
		seen[version] = true
	}
	if len(seen) == 1 {
		for version := range seen {
			return version
		}
	}
	return "mixed"
}

func parseConfigFile(path string) ([]model.ConfigProperty, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".properties":
		return parseProperties(path)
	case ".yml", ".yaml":
		return parseYAML(path)
	default:
		return nil, nil
	}
}

func parseProperties(path string) ([]model.ConfigProperty, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var props []model.ConfigProperty
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		idx := strings.IndexAny(line, "=:")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		props = append(props, configProperty(path, lineNo, key, value))
	}
	return props, scanner.Err()
}

func parseYAML(path string) ([]model.ConfigProperty, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var props []model.ConfigProperty
	if len(root.Content) == 0 {
		return props, nil
	}
	flattenYAML(path, root.Content[0], nil, &props)
	return props, nil
}

func flattenYAML(path string, node *yaml.Node, prefix []string, props *[]model.ConfigProperty) {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			flattenYAML(path, value, append(prefix, key.Value), props)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			flattenYAML(path, child, append(prefix, fmt.Sprintf("%d", i)), props)
		}
	case yaml.ScalarNode:
		key := strings.Join(prefix, ".")
		if key == "" {
			return
		}
		*props = append(*props, configProperty(path, node.Line, key, node.Value))
	}
}

func configProperty(path string, line int, key, value string) model.ConfigProperty {
	definedExternally := strings.Contains(value, "${")
	redacted := isSensitiveKey(key)
	if definedExternally {
		value = "defined_externally"
	} else if redacted {
		value = "redacted"
	}
	return model.ConfigProperty{
		Key:               key,
		Value:             value,
		DefinedExternally: definedExternally,
		Redacted:          redacted,
		SourceFile:        filepath.Clean(path),
		Evidence:          evidence(path, line, "config_property", key),
		Source:            model.SourceFound,
		Confidence:        model.ConfidenceHigh,
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	sensitive := []string{
		"password",
		"passwd",
		"secret",
		"token",
		"access-key",
		"accesskey",
		"private-key",
		"privatekey",
		"credential",
		"credentials",
		"client-secret",
	}
	for _, marker := range sensitive {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func buildTool(name string) string {
	switch name {
	case "pom.xml":
		return "maven"
	case "build.gradle", "build.gradle.kts":
		return "gradle"
	default:
		return ""
	}
}

func moduleName(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return filepath.Base(root)
	}
	return filepath.ToSlash(rel)
}

func dedupeModules(modules []model.Module) []model.Module {
	seen := map[string]model.Module{}
	for _, module := range modules {
		if existing, ok := seen[module.Path]; ok {
			if existing.BuildTool == "" || module.BuildTool == "maven" {
				existing.BuildTool = module.BuildTool
			}
			seen[module.Path] = existing
			continue
		}
		seen[module.Path] = module
	}
	result := make([]model.Module, 0, len(seen))
	for _, module := range seen {
		result = append(result, module)
	}
	return result
}

func sourceRoot(path string) string {
	slash := filepath.ToSlash(path)
	for _, marker := range []string{"/src/main/java/", "/src/test/java/"} {
		if idx := strings.Index(slash, marker); idx >= 0 {
			return filepath.Clean(slash[:idx+len(marker)-1])
		}
	}
	return ""
}

func shouldSkipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".gradle", ".idea", ".vscode", "target", "build", "out", "node_modules", "bin", "dist":
		return true
	default:
		return false
	}
}

func isTestPath(path string) bool {
	return strings.Contains(path, "/src/test/") || strings.Contains(path, "/test/")
}

func isConfigFile(name string) bool {
	lower := strings.ToLower(name)
	if lower == "application.properties" || lower == "application.yml" || lower == "application.yaml" {
		return true
	}
	if lower == "bootstrap.properties" || lower == "bootstrap.yml" || lower == "bootstrap.yaml" {
		return true
	}
	return strings.HasPrefix(lower, "application-") && (strings.HasSuffix(lower, ".properties") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml"))
}

func isDescriptorFile(name, path string) bool {
	lowerName := strings.ToLower(name)
	switch lowerName {
	case "web.xml",
		"ejb-jar.xml",
		"application.xml",
		"persistence.xml",
		"beans.xml",
		"webservices.xml",
		"jboss-web.xml",
		"jboss-ejb3.xml",
		"jboss-deployment-structure.xml",
		"jboss-app.xml",
		"ra.xml":
		return true
	}
	return strings.HasSuffix(path, ".wsdl")
}

func isUIFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".xhtml") ||
		strings.HasSuffix(lower, ".jsp") ||
		strings.HasSuffix(lower, ".html") ||
		strings.HasSuffix(lower, ".htm")
}

func isMigrationFile(path string) bool {
	return strings.Contains(path, "/db/migration/") ||
		strings.Contains(path, "/db/changelog/") ||
		strings.Contains(path, "/liquibase/")
}

func firstExisting(dir string, names ...string) string {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func mavenPackaging(path string) string {
	data, ok := readOptionalFile(path)
	if !ok {
		return ""
	}
	return firstXMLTagValue(data, "packaging")
}

func mavenJavaVersion(path string) string {
	data, ok := readOptionalFile(path)
	if !ok {
		return ""
	}
	props := xmlProperties(data)
	for _, tag := range []string{
		"maven.compiler.release",
		"maven.compiler.source",
		"maven.compiler.target",
		"java.version",
		"jdk.version",
		"source",
		"target",
		"release",
	} {
		if version := resolveVersionValue(firstXMLTagValue(data, tag), props); version != "" {
			return version
		}
	}
	return ""
}

func gradlePackaging(path string) string {
	data, ok := readOptionalFile(path)
	if !ok {
		return ""
	}
	lower := strings.ToLower(data)
	switch {
	case strings.Contains(lower, "id 'ear'") || strings.Contains(lower, "id(\"ear\")") || strings.Contains(lower, "apply plugin: 'ear'") || strings.Contains(lower, "apply(plugin = \"ear\")"):
		return "ear"
	case strings.Contains(lower, "id 'war'") || strings.Contains(lower, "id(\"war\")") || strings.Contains(lower, "apply plugin: 'war'") || strings.Contains(lower, "apply(plugin = \"war\")"):
		return "war"
	default:
		return ""
	}
}

func gradleJavaVersion(path string) string {
	data, ok := readOptionalFile(path)
	if !ok {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)\boptions\.release\s*=\s*([A-Za-z0-9_.'"()]+)`),
		regexp.MustCompile(`(?m)\bsourceCompatibility\s*=\s*([A-Za-z0-9_.'"()]+)`),
		regexp.MustCompile(`(?m)\btargetCompatibility\s*=\s*([A-Za-z0-9_.'"()]+)`),
		regexp.MustCompile(`(?m)\bsourceCompatibility\s+([A-Za-z0-9_.'"()]+)`),
		regexp.MustCompile(`(?m)\btargetCompatibility\s+([A-Za-z0-9_.'"()]+)`),
		regexp.MustCompile(`(?m)\blanguageVersion\s*=\s*JavaLanguageVersion\.of\(([^)]+)\)`),
	}
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(data); len(match) == 2 {
			if version := normalizeJavaVersion(match[1]); version != "" {
				return version
			}
		}
	}
	return ""
}

func readOptionalFile(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func firstXMLTagValue(data, tag string) string {
	pattern := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `>\s*([^<]+?)\s*</` + regexp.QuoteMeta(tag) + `>`)
	match := pattern.FindStringSubmatch(data)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func xmlProperties(data string) map[string]string {
	props := map[string]string{}
	pattern := regexp.MustCompile(`(?is)<([A-Za-z0-9_.-]+)>\s*([^<]+?)\s*</[A-Za-z0-9_.-]+>`)
	for _, match := range pattern.FindAllStringSubmatch(data, -1) {
		if len(match) != 3 {
			continue
		}
		props[match[1]] = strings.TrimSpace(match[2])
	}
	return props
}

func resolveVersionValue(value string, props map[string]string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		key := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
		value = props[key]
	}
	return normalizeJavaVersion(value)
}

func normalizeJavaVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimPrefix(value, "JavaVersion.")
	value = strings.TrimPrefix(value, "VERSION_")
	value = strings.TrimPrefix(value, "VERSION.")
	value = strings.ReplaceAll(value, "_", ".")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "1.") {
		value = strings.TrimPrefix(value, "1.")
	}
	digits := regexp.MustCompile(`[0-9]+`).FindString(value)
	if digits == "" {
		return ""
	}
	return digits
}

func stableID(parts ...string) string {
	joined := strings.Join(parts, ":")
	replacer := strings.NewReplacer("\\", "/", " ", "-", ":", "-", ".", "-", "_", "-")
	return strings.Trim(replacer.Replace(strings.ToLower(joined)), "-")
}

func evidence(path string, line int, kind, symbol string) model.Evidence {
	return model.Evidence{
		Path:   filepath.Clean(path),
		Line:   line,
		Symbol: symbol,
		Kind:   kind,
	}
}

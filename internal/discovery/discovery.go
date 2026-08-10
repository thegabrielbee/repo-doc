package discovery

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bee/java-process-mapper/internal/model"
	"gopkg.in/yaml.v3"
)

type Options struct {
	RootPath     string
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
	return nil
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

func isMigrationFile(path string) bool {
	return strings.Contains(path, "/db/migration/") ||
		strings.Contains(path, "/db/changelog/") ||
		strings.Contains(path, "/liquibase/")
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

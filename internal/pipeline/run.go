package pipeline

import (
	"fmt"
	"path/filepath"

	"github.com/bee/java-process-mapper/internal/addon"
	"github.com/bee/java-process-mapper/internal/analysis"
	"github.com/bee/java-process-mapper/internal/discovery"
	"github.com/bee/java-process-mapper/internal/graph"
	"github.com/bee/java-process-mapper/internal/model"
	"github.com/bee/java-process-mapper/internal/output"
	javaparser "github.com/bee/java-process-mapper/internal/parser/java"
	"github.com/bee/java-process-mapper/internal/resolve"
)

type Result struct {
	Project   *model.Project   `json:"project"`
	Artifacts output.Artifacts `json:"artifacts"`
	Summary   model.Summary    `json:"summary"`
}

func Run(opts analysis.Options, progress analysis.ProgressFunc) (Result, error) {
	if opts.RootPath == "" {
		return Result{}, fmt.Errorf("root path is required")
	}
	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Join(opts.RootPath, "out", "java-process-mapper")
	}
	report(progress, "discovering", nil)
	project, err := discovery.Discover(discovery.Options{RootPath: opts.RootPath, JavaVersion: opts.JavaVersion, IncludeTests: opts.IncludeTests})
	if err != nil {
		return Result{}, err
	}

	report(progress, "parsing_java", map[string]int{"javaFiles": countJava(project)})
	if err := parseJava(project); err != nil {
		return Result{}, err
	}
	report(progress, "resolving_calls", map[string]int{"types": len(project.Types)})
	resolve.Calls(project)

	report(progress, "running_addons", map[string]int{"types": len(project.Types)})
	for _, add := range addon.Resolve(opts.Addons) {
		if err := add.Analyze(project); err != nil {
			return Result{}, fmt.Errorf("addon %s failed: %w", add.Name(), err)
		}
	}

	report(progress, "building_graph", map[string]int{"entryPoints": len(project.EntryPoints), "dependencies": len(project.Dependencies)})
	graph.Build(project)
	project.RefreshSummary()

	report(progress, "writing_artifacts", nil)
	artifacts, err := output.Write(project, opts.OutputDir)
	if err != nil {
		return Result{}, err
	}
	report(progress, "completed", map[string]int{"entryPoints": project.Summary.EntryPoints, "gaps": project.Summary.Gaps})
	return Result{Project: project, Artifacts: artifacts, Summary: project.Summary}, nil
}

func parseJava(project *model.Project) error {
	for moduleIndex := range project.Modules {
		module := &project.Modules[moduleIndex]
		for _, path := range module.JavaFiles {
			source, types, gaps, err := javaparser.ParseFile(path, module.ID)
			if err != nil {
				project.Gaps = append(project.Gaps, model.Gap{
					ID:         stableID("gap", path, "java-parse"),
					Message:    "Could not parse Java file: " + err.Error(),
					Severity:   "high",
					Evidence:   model.Evidence{Path: path, Kind: "java_file"},
					Source:     model.SourceFound,
					Confidence: model.ConfidenceHigh,
				})
				continue
			}
			project.SourceFiles = append(project.SourceFiles, source)
			project.Types = append(project.Types, types...)
			project.Gaps = append(project.Gaps, gaps...)
		}
	}
	project.RefreshSummary()
	return nil
}

func countJava(project *model.Project) int {
	total := 0
	for _, module := range project.Modules {
		total += len(module.JavaFiles)
	}
	return total
}

func report(progress analysis.ProgressFunc, phase string, counts map[string]int) {
	if progress != nil {
		progress(phase, counts)
	}
}

func stableID(parts ...string) string {
	return filepath.ToSlash(filepath.Clean(filepath.Join(parts...)))
}

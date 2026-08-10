package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bee/java-process-mapper/internal/flow"
	"github.com/bee/java-process-mapper/internal/jobs"
	"github.com/bee/java-process-mapper/internal/mappingstate"
	"github.com/bee/java-process-mapper/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetNextMappingItemInput struct {
	JobID                     string `json:"jobId" jsonschema:"job id returned by start_mapping"`
	IncludeMechanicalMarkdown bool   `json:"includeMechanicalMarkdown,omitempty" jsonschema:"include the mechanical process Markdown in the response"`
}

type GetNextMappingItemOutput struct {
	JobID       string           `json:"jobId"`
	Status      jobs.Status      `json:"status"`
	Ready       bool             `json:"ready"`
	Done        bool             `json:"done"`
	Total       int              `json:"total"`
	Mapped      int              `json:"mapped"`
	Remaining   int              `json:"remaining"`
	StatePath   string           `json:"statePath,omitempty"`
	FeaturesDir string           `json:"featuresDir,omitempty"`
	Item        *MappingWorkItem `json:"item,omitempty"`
}

type MappingWorkItem struct {
	EntryPointID       string              `json:"entryPointId"`
	Name               string              `json:"name"`
	Kind               string              `json:"kind"`
	Product            string              `json:"product,omitempty"`
	Path               string              `json:"path,omitempty"`
	HTTPMethod         string              `json:"httpMethod,omitempty"`
	Resource           string              `json:"resource,omitempty"`
	Class              string              `json:"class,omitempty"`
	Method             string              `json:"method,omitempty"`
	MechanicalDocPath  string              `json:"mechanicalDocPath"`
	FinalDocPath       string              `json:"finalDocPath"`
	Evidence           model.Evidence      `json:"evidence"`
	TraceSummary       MappingTraceSummary `json:"traceSummary"`
	MechanicalMarkdown string              `json:"mechanicalMarkdown,omitempty"`
}

type MappingTraceSummary struct {
	Steps            int  `json:"steps"`
	Conditions       int  `json:"conditions"`
	Dependencies     int  `json:"dependencies"`
	ConfigProperties int  `json:"configProperties"`
	Truncated        bool `json:"truncated"`
}

type MarkMappingItemMappedInput struct {
	JobID        string `json:"jobId" jsonschema:"job id returned by start_mapping"`
	EntryPointID string `json:"entryPointId" jsonschema:"entrypoint id returned by get_next_mapping_item"`
	Markdown     string `json:"markdown" jsonschema:"final Markdown documentation in the user-facing template"`
	Title        string `json:"title,omitempty" jsonschema:"optional final document title"`
	Notes        string `json:"notes,omitempty" jsonschema:"optional review notes"`
	FinalDocPath string `json:"finalDocPath,omitempty" jsonschema:"optional output path; must stay inside the mapping output directory"`
}

type MarkMappingItemMappedOutput struct {
	JobID            string      `json:"jobId"`
	Status           jobs.Status `json:"status"`
	EntryPointID     string      `json:"entryPointId"`
	FinalDocPath     string      `json:"finalDocPath"`
	StatePath        string      `json:"statePath"`
	Total            int         `json:"total"`
	Mapped           int         `json:"mapped"`
	Remaining        int         `json:"remaining"`
	Done             bool        `json:"done"`
	NextEntryPointID string      `json:"nextEntryPointId,omitempty"`
}

func (s *Server) getNextMappingItem(ctx context.Context, _ *mcp.CallToolRequest, input GetNextMappingItemInput) (*mcp.CallToolResult, GetNextMappingItemOutput, error) {
	_ = ctx
	job := s.store.Get(input.JobID)
	if job == nil {
		return nil, GetNextMappingItemOutput{}, fmt.Errorf("unknown jobId: %s", input.JobID)
	}
	out := GetNextMappingItemOutput{JobID: job.ID, Status: job.Status, Ready: job.Status == jobs.StatusCompleted}
	if job.Status != jobs.StatusCompleted {
		return nil, out, nil
	}

	ctxData, err := loadMappingContext(job)
	if err != nil {
		return nil, GetNextMappingItemOutput{}, err
	}
	total, mapped, remaining := mappingstate.Counts(ctxData.State)
	out.Total = total
	out.Mapped = mapped
	out.Remaining = remaining
	out.StatePath = ctxData.StatePath
	out.FeaturesDir = mappingstate.FinalDocsDir(ctxData.OutputDir)

	item, ok := mappingstate.Next(ctxData.State)
	if !ok {
		out.Done = true
		return nil, out, nil
	}
	workItem := buildMappingWorkItem(ctxData.Project, ctxData.Traces[item.EntryPointID], item)
	if input.IncludeMechanicalMarkdown {
		workItem.MechanicalMarkdown = readOptionalText(item.MechanicalDocPath)
	}
	out.Item = &workItem
	return nil, out, nil
}

func (s *Server) markMappingItemMapped(ctx context.Context, _ *mcp.CallToolRequest, input MarkMappingItemMappedInput) (*mcp.CallToolResult, MarkMappingItemMappedOutput, error) {
	_ = ctx
	if strings.TrimSpace(input.EntryPointID) == "" {
		return nil, MarkMappingItemMappedOutput{}, fmt.Errorf("entryPointId is required")
	}
	if strings.TrimSpace(input.Markdown) == "" {
		return nil, MarkMappingItemMappedOutput{}, fmt.Errorf("markdown is required")
	}
	job := s.store.Get(input.JobID)
	if job == nil {
		return nil, MarkMappingItemMappedOutput{}, fmt.Errorf("unknown jobId: %s", input.JobID)
	}
	if job.Status != jobs.StatusCompleted {
		return nil, MarkMappingItemMappedOutput{}, fmt.Errorf("job %s is not completed: %s", job.ID, job.Status)
	}
	ctxData, err := loadMappingContext(job)
	if err != nil {
		return nil, MarkMappingItemMappedOutput{}, err
	}

	itemIndex := -1
	for i, item := range ctxData.State.Items {
		if item.EntryPointID == input.EntryPointID {
			itemIndex = i
			break
		}
	}
	if itemIndex < 0 {
		return nil, MarkMappingItemMappedOutput{}, fmt.Errorf("unknown entryPointId for job %s: %s", job.ID, input.EntryPointID)
	}

	finalDocPath, err := resolveFinalDocPath(ctxData.OutputDir, input.FinalDocPath, ctxData.State.Items[itemIndex].FinalDocPath)
	if err != nil {
		return nil, MarkMappingItemMappedOutput{}, err
	}
	if err := os.MkdirAll(filepath.Dir(finalDocPath), 0o755); err != nil {
		return nil, MarkMappingItemMappedOutput{}, err
	}
	markdown := input.Markdown
	if !strings.HasSuffix(markdown, "\n") {
		markdown += "\n"
	}
	if err := os.WriteFile(finalDocPath, []byte(markdown), 0o644); err != nil {
		return nil, MarkMappingItemMappedOutput{}, err
	}

	ctxData.State.Items[itemIndex].Status = mappingstate.StatusMapped
	ctxData.State.Items[itemIndex].FinalDocPath = finalDocPath
	ctxData.State.Items[itemIndex].Title = strings.TrimSpace(input.Title)
	ctxData.State.Items[itemIndex].Notes = strings.TrimSpace(input.Notes)
	ctxData.State.Items[itemIndex].MappedAt = time.Now().UTC()
	if err := mappingstate.Save(ctxData.StatePath, ctxData.State); err != nil {
		return nil, MarkMappingItemMappedOutput{}, err
	}

	total, mapped, remaining := mappingstate.Counts(ctxData.State)
	next, hasNext := mappingstate.Next(ctxData.State)
	out := MarkMappingItemMappedOutput{
		JobID:        job.ID,
		Status:       job.Status,
		EntryPointID: input.EntryPointID,
		FinalDocPath: finalDocPath,
		StatePath:    ctxData.StatePath,
		Total:        total,
		Mapped:       mapped,
		Remaining:    remaining,
		Done:         !hasNext,
	}
	if hasNext {
		out.NextEntryPointID = next.EntryPointID
	}
	return nil, out, nil
}

type mappingContext struct {
	OutputDir string
	StatePath string
	State     mappingstate.State
	Project   model.Project
	Traces    map[string]flow.Trace
}

func loadMappingContext(job *jobs.Job) (mappingContext, error) {
	outputDir := job.OutputDir
	if job.Artifacts.OutputDir != "" {
		outputDir = job.Artifacts.OutputDir
	}
	artifacts := job.Artifacts
	if artifacts.OutputDir == "" {
		artifacts.OutputDir = outputDir
	}
	if artifacts.Findings == "" {
		artifacts.Findings = filepath.Join(outputDir, "findings.json")
	}
	if artifacts.Traces == "" {
		artifacts.Traces = filepath.Join(outputDir, "traces.json")
	}
	if artifacts.MappingState == "" {
		artifacts.MappingState = mappingstate.StatePath(outputDir)
	}

	project, err := readJSONFile[model.Project](artifacts.Findings)
	if err != nil {
		return mappingContext{}, err
	}
	traces, err := readJSONFile[map[string]flow.Trace](artifacts.Traces)
	if err != nil {
		return mappingContext{}, err
	}
	state, err := mappingstate.Load(artifacts.MappingState)
	if err != nil {
		if !os.IsNotExist(err) {
			return mappingContext{}, err
		}
		state, err = mappingstate.Initialize(outputDir, &project)
		if err != nil {
			return mappingContext{}, err
		}
	}
	return mappingContext{
		OutputDir: outputDir,
		StatePath: artifacts.MappingState,
		State:     state,
		Project:   project,
		Traces:    traces,
	}, nil
}

func buildMappingWorkItem(project model.Project, trace flow.Trace, item mappingstate.Item) MappingWorkItem {
	entry := trace.EntryPoint
	typ := findTypeByID(project, entry.ClassID)
	method := findMethodByID(typ, entry.MethodID)
	return MappingWorkItem{
		EntryPointID:      item.EntryPointID,
		Name:              item.Name,
		Kind:              item.Kind,
		Product:           entry.Product,
		Path:              item.Path,
		HTTPMethod:        item.HTTPMethod,
		Resource:          item.Resource,
		Class:             typ.FQN,
		Method:            method.Name,
		MechanicalDocPath: item.MechanicalDocPath,
		FinalDocPath:      item.FinalDocPath,
		Evidence:          entry.Evidence,
		TraceSummary: MappingTraceSummary{
			Steps:            len(trace.Steps),
			Conditions:       len(trace.Conditions),
			Dependencies:     len(trace.Dependencies),
			ConfigProperties: len(trace.ConfigProperties),
			Truncated:        trace.Truncated,
		},
	}
}

func readJSONFile[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func readOptionalText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func resolveFinalDocPath(outputDir, requested, fallback string) (string, error) {
	path := strings.TrimSpace(requested)
	if path == "" {
		path = fallback
	}
	if path == "" {
		return "", fmt.Errorf("finalDocPath could not be determined")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(outputDir, path)
	}
	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	cleanOutput, err := filepath.Abs(filepath.Clean(outputDir))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(cleanOutput, cleanPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("finalDocPath must stay inside outputDir")
	}
	return cleanPath, nil
}

func findTypeByID(project model.Project, id string) model.Type {
	for _, typ := range project.Types {
		if typ.ID == id {
			return typ
		}
	}
	return model.Type{}
}

func findMethodByID(typ model.Type, id string) model.Method {
	for _, method := range typ.Methods {
		if method.ID == id {
			return method
		}
	}
	return model.Method{}
}

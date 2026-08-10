package mcpserver

import (
	"context"
	"fmt"

	"github.com/bee/java-process-mapper/internal/analysis"
	"github.com/bee/java-process-mapper/internal/jobs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	store *jobs.Store
}

func New() *Server {
	return &Server{store: jobs.NewStore()}
}

func (s *Server) Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "java-process-mapper", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_mapping",
		Description: "Start static Java/Spring process mapping and generate Markdown plus JSON artifacts.",
	}, s.startMapping)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_mapping_status",
		Description: "Return status, phase, counters and errors for a mapping job.",
	}, s.getMappingStatus)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_mapping_result",
		Description: "Return generated artifact paths and summary for a completed mapping job.",
	}, s.getMappingResult)
	return server.Run(ctx, &mcp.StdioTransport{})
}

type StartMappingInput struct {
	RootPath     string   `json:"rootPath" jsonschema:"absolute or relative path to the Java repository root"`
	OutputDir    string   `json:"outputDir,omitempty" jsonschema:"directory where graph.json, findings.json and docs will be written"`
	Addons       []string `json:"addons,omitempty" jsonschema:"framework addons to run; use spring for Spring/Spring Boot"`
	IncludeTests bool     `json:"includeTests,omitempty" jsonschema:"include src/test and test folders in analysis"`
}

type StartMappingOutput struct {
	JobID     string      `json:"jobId"`
	Status    jobs.Status `json:"status"`
	Phase     string      `json:"phase"`
	OutputDir string      `json:"outputDir,omitempty"`
}

func (s *Server) startMapping(ctx context.Context, _ *mcp.CallToolRequest, input StartMappingInput) (*mcp.CallToolResult, StartMappingOutput, error) {
	if input.RootPath == "" {
		return nil, StartMappingOutput{}, fmt.Errorf("rootPath is required")
	}
	job := s.store.Start(ctx, analysis.Options{
		RootPath:     input.RootPath,
		OutputDir:    input.OutputDir,
		Addons:       input.Addons,
		IncludeTests: input.IncludeTests,
	})
	return nil, StartMappingOutput{JobID: job.ID, Status: job.Status, Phase: job.Phase, OutputDir: job.OutputDir}, nil
}

type JobInput struct {
	JobID string `json:"jobId" jsonschema:"job id returned by start_mapping"`
}

type StatusOutput struct {
	JobID     string         `json:"jobId"`
	Status    jobs.Status    `json:"status"`
	Phase     string         `json:"phase"`
	Counts    map[string]int `json:"counts,omitempty"`
	Error     string         `json:"error,omitempty"`
	OutputDir string         `json:"outputDir,omitempty"`
}

func (s *Server) getMappingStatus(ctx context.Context, _ *mcp.CallToolRequest, input JobInput) (*mcp.CallToolResult, StatusOutput, error) {
	_ = ctx
	job := s.store.Get(input.JobID)
	if job == nil {
		return nil, StatusOutput{}, fmt.Errorf("unknown jobId: %s", input.JobID)
	}
	return nil, StatusOutput{
		JobID:     job.ID,
		Status:    job.Status,
		Phase:     job.Phase,
		Counts:    job.Counts,
		Error:     job.Error,
		OutputDir: job.OutputDir,
	}, nil
}

type ResultOutput struct {
	JobID     string      `json:"jobId"`
	Status    jobs.Status `json:"status"`
	Ready     bool        `json:"ready"`
	Error     string      `json:"error,omitempty"`
	Artifacts any         `json:"artifacts,omitempty"`
	Summary   any         `json:"summary,omitempty"`
}

func (s *Server) getMappingResult(ctx context.Context, _ *mcp.CallToolRequest, input JobInput) (*mcp.CallToolResult, ResultOutput, error) {
	_ = ctx
	job := s.store.Get(input.JobID)
	if job == nil {
		return nil, ResultOutput{}, fmt.Errorf("unknown jobId: %s", input.JobID)
	}
	return nil, ResultOutput{
		JobID:     job.ID,
		Status:    job.Status,
		Ready:     job.Status == jobs.StatusCompleted,
		Error:     job.Error,
		Artifacts: job.Artifacts,
		Summary:   job.Summary,
	}, nil
}

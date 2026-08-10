package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStdioTools(t *testing.T) {
	repoRoot := repoRoot(t)
	sourceRoot := mcpFixture(t)
	out := filepath.Join(t.TempDir(), "out")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "java-process-mapper-test", Version: "0.1.0"}, nil)
	cmd := exec.Command("go", "run", "./cmd/java-process-mapper", "serve")
	cmd.Dir = repoRoot
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	start := callTool[map[string]any](t, ctx, session, "start_mapping", map[string]any{
		"rootPath":     sourceRoot,
		"outputDir":    out,
		"addons":       []string{"spring"},
		"includeTests": false,
	})
	jobID, _ := start["jobId"].(string)
	if jobID == "" {
		t.Fatalf("missing jobId in start response: %+v", start)
	}

	var status map[string]any
	for i := 0; i < 50; i++ {
		status = callTool[map[string]any](t, ctx, session, "get_mapping_status", map[string]any{"jobId": jobID})
		if status["status"] == "completed" {
			break
		}
		if status["status"] == "failed" {
			t.Fatalf("job failed: %+v", status)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status["status"] != "completed" {
		t.Fatalf("job did not complete: %+v", status)
	}

	result := callTool[map[string]any](t, ctx, session, "get_mapping_result", map[string]any{"jobId": jobID})
	if result["ready"] != true {
		t.Fatalf("result not ready: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(out, "graph.json")); err != nil {
		t.Fatalf("graph artifact missing: %v", err)
	}

	next := callTool[map[string]any](t, ctx, session, "get_next_mapping_item", map[string]any{
		"jobId":                     jobID,
		"includeMechanicalMarkdown": true,
	})
	if next["done"] == true || next["item"] == nil {
		t.Fatalf("expected next mapping item: %+v", next)
	}
	item := next["item"].(map[string]any)
	entryPointID, _ := item["entryPointId"].(string)
	if entryPointID == "" {
		t.Fatalf("missing entrypoint id in next item: %+v", item)
	}
	if markdown, _ := item["mechanicalMarkdown"].(string); !strings.Contains(markdown, "Technical Evidence") {
		t.Fatalf("expected mechanical markdown evidence in next item")
	}
	final := callTool[map[string]any](t, ctx, session, "mark_mapping_item_mapped", map[string]any{
		"jobId":        jobID,
		"entryPointId": entryPointID,
		"title":        "Demo feature",
		"markdown":     "# Demo feature\n\nFinal user-facing mapping.\n",
	})
	if final["done"] != true {
		t.Fatalf("expected single-item queue to be done after mark: %+v", final)
	}
	finalPath, _ := final["finalDocPath"].(string)
	if finalPath == "" {
		t.Fatalf("missing final doc path: %+v", final)
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("final mapped markdown missing: %v", err)
	}
}

func callTool[T any](t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool %s returned error: %+v", name, res)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func mcpFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeMCPFile(t, filepath.Join(root, "pom.xml"), "<project/>")
	writeMCPFile(t, filepath.Join(root, "src", "main", "java", "com", "acme", "DemoController.java"), `
package com.acme;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class DemoController {
  @GetMapping("/demo")
  public String demo() { return "ok"; }
}
`)
	return root
}

func writeMCPFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

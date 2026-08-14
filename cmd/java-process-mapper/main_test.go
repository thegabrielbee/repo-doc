package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bee/java-process-mapper/internal/model"
)

func TestScanJavaEEJavaVersionFlag(t *testing.T) {
	sourceRoot := cliJavaEEFixture(t)
	out := filepath.Join(t.TempDir(), "out")

	cmd := exec.Command("go", "run", "./cmd/java-process-mapper", "scan", "--root", sourceRoot, "--out", out, "--addons", "javaee", "--java-version", "8")
	cmd.Dir = cliRepoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scan failed: %v\n%s", err, output)
	}
	var scan struct {
		Status    string `json:"status"`
		Artifacts struct {
			Findings string `json:"findings"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(output, &scan); err != nil {
		t.Fatalf("invalid scan JSON: %v\n%s", err, output)
	}
	if scan.Status != "completed" {
		t.Fatalf("status = %s, want completed", scan.Status)
	}
	data, err := os.ReadFile(scan.Artifacts.Findings)
	if err != nil {
		t.Fatal(err)
	}
	var project model.Project
	if err := json.Unmarshal(data, &project); err != nil {
		t.Fatal(err)
	}
	if project.JavaVersion != "8" {
		t.Fatalf("project java version = %s, want 8", project.JavaVersion)
	}
	if len(project.EntryPoints) != 1 || project.EntryPoints[0].Framework != "javaee" {
		t.Fatalf("expected one Java EE entrypoint, got %+v", project.EntryPoints)
	}
}

func cliJavaEEFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cliWriteFile(t, filepath.Join(root, "pom.xml"), "<project><properties><maven.compiler.source>11</maven.compiler.source></properties></project>")
	cliWriteFile(t, filepath.Join(root, "src", "main", "java", "com", "acme", "RestApi.java"), `
package com.acme;

import javax.ws.rs.GET;
import javax.ws.rs.Path;

@Path("/api")
public class RestApi {
  @GET
  public String ping() { return "pong"; }
}
`)
	return root
}

func cliWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cliRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bee/java-process-mapper/internal/analysis"
	"github.com/bee/java-process-mapper/internal/mcpserver"
	"github.com/bee/java-process-mapper/internal/pipeline"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		if err := mcpserver.New().Run(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "scan":
		if err := runScan(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "path to Java repository root")
	out := fs.String("out", "", "output directory")
	addons := fs.String("addons", "spring", "comma-separated addons")
	includeTests := fs.Bool("include-tests", false, "include test source folders")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	result, err := pipeline.Run(analysis.Options{
		RootPath:     *root,
		OutputDir:    *out,
		Addons:       splitCSV(*addons),
		IncludeTests: *includeTests,
	}, nil)
	if err != nil {
		return err
	}
	summary := map[string]any{
		"status":    "completed",
		"outputDir": result.Artifacts.OutputDir,
		"artifacts": result.Artifacts,
		"summary":   result.Summary,
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func usage() {
	fmt.Fprintf(os.Stderr, `java-process-mapper %s

Usage:
  java-process-mapper serve
  java-process-mapper scan --root <path> --out <path> --addons spring

Commands:
  serve   Start MCP server over stdio.
  scan    Run the same mapping pipeline locally and print a JSON summary.
`, version)
}

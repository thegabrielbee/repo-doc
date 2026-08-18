package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmd, err := pythonCommand(root, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pythonCommand(root string, args []string) (*exec.Cmd, error) {
	for _, name := range []string{"python", "python3", "py"} {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		cmdArgs := []string{"-m", "java_process_mapper"}
		if name == "py" {
			cmdArgs = []string{"-3", "-m", "java_process_mapper"}
		}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.Command(name, cmdArgs...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "JAVA_PROCESS_MAPPER_REPO_ROOT="+root)
		return cmd, nil
	}
	return nil, fmt.Errorf("python executable not found")
}

func repoRoot() (string, error) {
	if root := os.Getenv("JAVA_PROCESS_MAPPER_REPO_ROOT"); root != "" {
		return filepath.Clean(root), nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if exists(filepath.Join(dir, "java_process_mapper", "cli.py")) && exists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root from %s", dir)
		}
		dir = parent
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

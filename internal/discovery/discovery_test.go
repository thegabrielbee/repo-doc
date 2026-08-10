package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverDoesNotResolveEnvironmentPlaceholders(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "src", "main", "resources")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "application.yml"), []byte("spring:\n  datasource:\n    url: ${DB_URL}\naws:\n  s3:\n    bucket: ${ORDERS_BUCKET}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "application.properties"), []byte("spring.datasource.password=super-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	project, err := Discover(Options{RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(project.ConfigProperties) != 3 {
		t.Fatalf("properties = %d, want 3", len(project.ConfigProperties))
	}
	redacted := false
	for _, prop := range project.ConfigProperties {
		if prop.Key == "spring.datasource.password" {
			redacted = prop.Redacted && prop.Value == "redacted"
			continue
		}
		if strings.Contains(prop.Value, "DB_URL") || strings.Contains(prop.Value, "ORDERS_BUCKET") {
			t.Fatalf("environment placeholder leaked in value: %+v", prop)
		}
	}
	if !redacted {
		t.Fatalf("expected static password to be redacted: %+v", project.ConfigProperties)
	}
}

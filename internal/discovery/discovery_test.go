package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bee/java-process-mapper/internal/model"
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

func TestDiscoverJavaVersionPackagingAndJavaEEArtifacts(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(root, "pom.xml"), `
<project>
  <packaging>pom</packaging>
  <properties>
    <java.version>1.8</java.version>
    <maven.compiler.source>${java.version}</maven.compiler.source>
  </properties>
</project>
`)
	writeDiscoveryFile(t, filepath.Join(root, "legacy-web", "pom.xml"), `
<project>
  <packaging>war</packaging>
</project>
`)
	writeDiscoveryFile(t, filepath.Join(root, "legacy-web", "src", "main", "webapp", "WEB-INF", "web.xml"), "<web-app/>")
	writeDiscoveryFile(t, filepath.Join(root, "legacy-web", "src", "main", "webapp", "index.xhtml"), "<html/>")
	writeDiscoveryFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "Portal.java"), "package com.acme; class Portal {}")
	writeDiscoveryFile(t, filepath.Join(root, "legacy-ear", "build.gradle"), `
plugins {
  id 'ear'
}
java {
  sourceCompatibility = JavaVersion.VERSION_11
}
`)

	project, err := Discover(Options{RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if project.JavaVersion != "mixed" {
		t.Fatalf("project java version = %s, want mixed", project.JavaVersion)
	}
	web := findDiscoveryModule(project.Modules, "legacy-web")
	if web.Packaging != "war" {
		t.Fatalf("web packaging = %s, want war", web.Packaging)
	}
	if web.JavaVersion != "8" {
		t.Fatalf("web java version = %s, want inherited 8", web.JavaVersion)
	}
	if len(web.DescriptorFiles) != 1 || len(web.UIFiles) != 1 {
		t.Fatalf("expected descriptor and UI files, got descriptors=%v ui=%v", web.DescriptorFiles, web.UIFiles)
	}
	ear := findDiscoveryModule(project.Modules, "legacy-ear")
	if ear.Packaging != "ear" || ear.JavaVersion != "11" {
		t.Fatalf("ear metadata = packaging %s java %s, want ear/11", ear.Packaging, ear.JavaVersion)
	}

	override, err := Discover(Options{RootPath: root, JavaVersion: "1.8"})
	if err != nil {
		t.Fatal(err)
	}
	if override.JavaVersion != "8" {
		t.Fatalf("override java version = %s, want 8", override.JavaVersion)
	}
	for _, module := range override.Modules {
		if module.JavaVersion != "8" {
			t.Fatalf("module %s java version = %s, want override 8", module.Name, module.JavaVersion)
		}
	}
}

func findDiscoveryModule(modules []model.Module, name string) model.Module {
	for _, module := range modules {
		if module.Name == name {
			return module
		}
	}
	return model.Module{}
}

func writeDiscoveryFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

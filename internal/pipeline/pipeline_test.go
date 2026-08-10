package pipeline

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/bee/java-process-mapper/internal/analysis"
	"github.com/bee/java-process-mapper/internal/flow"
	"github.com/bee/java-process-mapper/internal/model"
)

func TestRunSpringMultiModuleGeneratesArtifacts(t *testing.T) {
	root := springFixture(t)
	out := filepath.Join(t.TempDir(), "mapper-out")

	result, err := Run(analysis.Options{
		RootPath:  root,
		OutputDir: out,
		Addons:    []string{"spring"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.EntryPoints < 2 {
		t.Fatalf("entrypoints = %d, want at least 2", result.Summary.EntryPoints)
	}
	for _, path := range []string{result.Artifacts.Graph, result.Artifacts.Findings, result.Artifacts.Traces, result.Artifacts.Index, result.Artifacts.Visualization, result.Artifacts.Gaps} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	if len(result.Artifacts.Docs) == 0 {
		t.Fatalf("expected process docs")
	}

	data, err := os.ReadFile(result.Artifacts.Graph)
	if err != nil {
		t.Fatal(err)
	}
	var graph model.Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Fatalf("expected non-empty graph: %+v", graph)
	}
	if strings.Contains(string(data), "DB_URL") || strings.Contains(string(data), "ORDERS_BUCKET") {
		t.Fatalf("environment placeholder leaked into graph.json")
	}
	visualizationData, err := os.ReadFile(result.Artifacts.Visualization)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(visualizationData), "CollectionUtils.isEmpty") || !strings.Contains(string(visualizationData), "external_import") {
		t.Fatalf("expected visualization to include external import evidence")
	}
	if !strings.Contains(string(visualizationData), `data-view-button="graph"`) || !strings.Contains(string(visualizationData), "data-graph-info") {
		t.Fatalf("expected visualization to include graph controls and clickable graph data")
	}
	if !strings.Contains(string(visualizationData), "data-pan-zoom") || !strings.Contains(string(visualizationData), "data-graph-tool=\"reset\"") {
		t.Fatalf("expected visualization to include pannable graph canvas controls")
	}
	if !strings.Contains(string(visualizationData), "function selectGraphItem") || !strings.Contains(string(visualizationData), "state.downItem") {
		t.Fatalf("expected visualization to update inspector from graph pointer selection")
	}
	if !strings.Contains(string(visualizationData), "#1 ") || !strings.Contains(string(visualizationData), ">#1<") {
		t.Fatalf("expected visualization graph to include indexed trace nodes and edge labels")
	}
	if !strings.Contains(string(visualizationData), "data-process-search-input") || !strings.Contains(string(visualizationData), "setupProcessSearch") {
		t.Fatalf("expected visualization to include process search")
	}
	if !strings.Contains(string(visualizationData), "chamadas auxiliares omitidas do grafo") {
		t.Fatalf("expected visualization to separate auxiliary graph calls")
	}
	if !strings.Contains(string(visualizationData), "Linha de codigo") || !strings.Contains(string(visualizationData), "data-aux-next") {
		t.Fatalf("expected visualization to include source lines and paginated auxiliary calls")
	}
	if !strings.Contains(string(visualizationData), "condicao") || !strings.Contains(string(visualizationData), "CollectionUtils.isEmpty(null)") {
		t.Fatalf("expected visualization graph to include conditional structures")
	}
	if !strings.Contains(string(visualizationData), "id == null") {
		t.Fatalf("expected visualization graph to include conditions even when they do not contain internal calls")
	}
	conditionPayloads := graphInfoPayloads(t, string(visualizationData), "condicao")
	foundElseCondition := false
	foundConditionLine := false
	for _, payload := range conditionPayloads {
		if payload["Estrutura"] == "else" {
			foundElseCondition = true
		}
		if strings.TrimSpace(payload["Linha de codigo"]) != "" {
			foundConditionLine = true
		}
		if strings.TrimSpace(payload["Conteudo"]) != "" {
			t.Fatalf("expected condition graph payload to expose only the condition line, not method content")
		}
	}
	if !foundElseCondition {
		t.Fatalf("expected visualization graph to include else branches")
	}
	if !foundConditionLine {
		t.Fatalf("expected condition graph payloads to include source line evidence")
	}
	if !strings.Contains(string(visualizationData), "Conteudo") ||
		!strings.Contains(string(visualizationData), "conteudo interno do metodo omitido") {
		t.Fatalf("expected visualization to include contextual source content")
	}
	if strings.Contains(string(visualizationData), "Conteudo do metodo") ||
		strings.Contains(string(visualizationData), "Conteudo da classe") {
		t.Fatalf("expected visualization to use a single Conteudo field")
	}
	if !strings.Contains(string(visualizationData), "dependencia-satelite") {
		t.Fatalf("expected dependencies to be rendered as satellite graph nodes")
	}
	if strings.Contains(string(visualizationData), "hidden-class-body") {
		t.Fatalf("expected class context to omit nested method bodies")
	}

	traceData, err := os.ReadFile(result.Artifacts.Traces)
	if err != nil {
		t.Fatal(err)
	}
	var traces map[string]flow.Trace
	if err := json.Unmarshal(traceData, &traces); err != nil {
		t.Fatal(err)
	}
	foundResolvedService := false
	foundResolvedLocalVariable := false
	foundResolvedExternalImport := false
	foundIndirectDependency := false
	foundExternalDependency := false
	foundCondition := false
	for _, trace := range traces {
		for _, step := range trace.Steps {
			if step.Call == "service.findById" && strings.Contains(step.ResolvedClass, "OrderService") {
				foundResolvedService = true
			}
			if step.Call == "dto.touch" && strings.Contains(step.ResolvedClass, "OrderDto") && step.Resolution == "local_variable" {
				foundResolvedLocalVariable = true
			}
			if step.Call == "CollectionUtils.isEmpty" && step.ResolvedClass == "org.springframework.util.CollectionUtils" && step.Resolution == "external_import" {
				foundResolvedExternalImport = true
			}
		}
		for _, dep := range trace.Dependencies {
			if dep.Scope == "indirect" && dep.Dependency.Kind == "s3" {
				foundIndirectDependency = true
			}
			if dep.Dependency.Kind == "external_dependency" && dep.Dependency.Name == "org.springframework.util.CollectionUtils" {
				foundExternalDependency = true
			}
		}
		for _, condition := range trace.Conditions {
			if condition.Condition.Kind == "if" && strings.Contains(condition.Condition.Expression, "CollectionUtils.isEmpty") {
				foundCondition = true
			}
		}
	}
	if !foundResolvedService {
		t.Fatalf("expected trace to resolve service.findById")
	}
	if !foundResolvedLocalVariable {
		t.Fatalf("expected trace to resolve dto.touch from a local variable")
	}
	if !foundResolvedExternalImport {
		t.Fatalf("expected trace to resolve CollectionUtils.isEmpty from an external import")
	}
	if !foundIndirectDependency {
		t.Fatalf("expected indirect S3 dependency in trace")
	}
	if !foundExternalDependency {
		t.Fatalf("expected external import dependency in trace")
	}
	if !foundCondition {
		t.Fatalf("expected conditional structure in trace")
	}
}

func springFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "pom.xml"), "<project><modules><module>orders-service</module></modules></project>")
	writeFile(t, filepath.Join(root, "orders-service", "pom.xml"), "<project/>")
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "resources", "application.yml"), `
spring:
  datasource:
    url: ${DB_URL}
aws:
  s3:
    bucket: ${ORDERS_BUCKET}
orders:
  topic: orders.events
`)
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "resources", "db", "migration", "V1__orders.sql"), "create table orders(id varchar(32));")
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "java", "com", "acme", "orders", "OrderController.java"), `
package com.acme.orders;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/orders")
public class OrderController {
  private final OrderService service;
  public OrderController(OrderService service) { this.service = service; }

  @GetMapping("/{id}")
  public OrderDto get(@PathVariable String id) {
    return service.findById(id);
  }
}
`)
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "java", "com", "acme", "orders", "OrderEvents.java"), `
package com.acme.orders;

import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

@Component
public class OrderEvents {
  @KafkaListener(topics = "orders.events")
  public void consume(OrderEvent event) {
    event.toString();
  }
}
`)
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "java", "com", "acme", "orders", "OrderService.java"), `
package com.acme.orders;

import org.springframework.stereotype.Service;
import org.springframework.util.CollectionUtils;
import software.amazon.awssdk.services.s3.S3Client;

@Service
public class OrderService {
  private final OrderRepository repository;
  private final S3Client s3;
  public OrderService(OrderRepository repository, S3Client s3) {
    this.repository = repository;
    this.s3 = s3;
  }
  public OrderDto findById(String id) {
    repository.findById(id);
    s3.getObject(null);
    OrderDto dto = new OrderDto();
    dto.touch();
    if (CollectionUtils.isEmpty(null)) {
      dto.touch();
    } else {
      repository.findById(id);
    }
    if (id == null) {
      return dto;
    }
    return dto;
  }

  private static class Helper {
    String hiddenBody() {
      return "hidden-class-body";
    }
  }
}
`)
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "java", "com", "acme", "orders", "OrderDto.java"), `
package com.acme.orders;

public class OrderDto {
  public void touch() {}
}
`)
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "java", "com", "acme", "orders", "OrderRepository.java"), `
package com.acme.orders;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

@Repository
public interface OrderRepository extends JpaRepository<OrderEntity, String> {}
`)
	writeFile(t, filepath.Join(root, "orders-service", "src", "main", "java", "com", "acme", "orders", "OrderEntity.java"), `
package com.acme.orders;

import jakarta.persistence.Entity;
import jakarta.persistence.Table;

@Entity
@Table(name = "orders")
public class OrderEntity {}
`)
	return root
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func graphInfoPayloads(t *testing.T, data string, tipo string) []map[string]string {
	t.Helper()
	matches := regexp.MustCompile(`data-graph-info="([^"]*)"`).FindAllStringSubmatch(data, -1)
	var payloads []map[string]string
	for _, match := range matches {
		var payload map[string]string
		if err := json.Unmarshal([]byte(html.UnescapeString(match[1])), &payload); err != nil {
			t.Fatalf("invalid graph payload: %v", err)
		}
		if payload["Tipo"] == tipo {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

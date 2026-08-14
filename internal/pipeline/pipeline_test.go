package pipeline

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	visualizationHTML := string(visualizationData)
	processCardsHTML := strings.Join(mapValues(t, visualizationProcessCards(t, visualizationHTML)), "\n")
	if strings.Contains(visualizationHTML, `<article class="process"`) {
		t.Fatalf("visualization should not render all process cards before selection")
	}
	if !strings.Contains(visualizationHTML, `id="process-cards"`) ||
		!strings.Contains(visualizationHTML, `data-process-detail`) ||
		!strings.Contains(visualizationHTML, "function renderSelectedProcess") {
		t.Fatalf("expected visualization to lazy-render process details from the process list")
	}
	if !strings.Contains(processCardsHTML, "CollectionUtils.isEmpty") || !strings.Contains(processCardsHTML, "external_import") {
		t.Fatalf("expected visualization to include external import evidence")
	}
	if !strings.Contains(processCardsHTML, `data-view-button="graph"`) || !strings.Contains(processCardsHTML, "data-graph-info") {
		t.Fatalf("expected visualization to include graph controls and clickable graph data")
	}
	if !strings.Contains(processCardsHTML, "data-pan-zoom") || !strings.Contains(processCardsHTML, "data-graph-tool=\"reset\"") {
		t.Fatalf("expected visualization to include pannable graph canvas controls")
	}
	if !strings.Contains(visualizationHTML, "function selectGraphItem") || !strings.Contains(visualizationHTML, "state.downItem") {
		t.Fatalf("expected visualization to update inspector from graph pointer selection")
	}
	if !strings.Contains(processCardsHTML, "#1 ") || !strings.Contains(processCardsHTML, ">#1<") {
		t.Fatalf("expected visualization graph to include indexed trace nodes and edge labels")
	}
	if !strings.Contains(visualizationHTML, "data-process-search-input") || !strings.Contains(visualizationHTML, "setupProcessSearch") {
		t.Fatalf("expected visualization to include process search")
	}
	if !strings.Contains(visualizationHTML, "normalizeSearchText") ||
		!strings.Contains(visualizationHTML, "processCardText") ||
		!strings.Contains(visualizationHTML, "terms.every") {
		t.Fatalf("expected visualization search to normalize typed text and filter using rendered process card content")
	}
	if !strings.Contains(processCardsHTML, "chamadas auxiliares omitidas do grafo") {
		t.Fatalf("expected visualization to separate auxiliary graph calls")
	}
	if !strings.Contains(processCardsHTML, "Linha de codigo") || !strings.Contains(processCardsHTML, "data-aux-next") {
		t.Fatalf("expected visualization to include source lines and paginated auxiliary calls")
	}
	if !strings.Contains(processCardsHTML, "condicao") || !strings.Contains(processCardsHTML, "CollectionUtils.isEmpty(null)") {
		t.Fatalf("expected visualization graph to include conditional structures")
	}
	if !strings.Contains(processCardsHTML, "id == null") {
		t.Fatalf("expected visualization graph to include conditions even when they do not contain internal calls")
	}
	conditionPayloads := graphInfoPayloads(t, processCardsHTML, "condicao")
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
	if !strings.Contains(processCardsHTML, "Conteudo") ||
		!strings.Contains(visualizationHTML, "conteudo interno do metodo omitido") {
		t.Fatalf("expected visualization to include contextual source content")
	}
	if strings.Contains(processCardsHTML, "Conteudo do metodo") ||
		strings.Contains(processCardsHTML, "Conteudo da classe") {
		t.Fatalf("expected visualization to use a single Conteudo field")
	}
	if !strings.Contains(processCardsHTML, "dependencia-satelite") {
		t.Fatalf("expected dependencies to be rendered as satellite graph nodes")
	}
	if strings.Contains(visualizationHTML, "hidden-class-body") {
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

func TestRunJavaEELegacyMultiModuleGeneratesArtifacts(t *testing.T) {
	root := javaEEFixture(t)
	out := filepath.Join(t.TempDir(), "mapper-out")

	result, err := Run(analysis.Options{
		RootPath:    root,
		OutputDir:   out,
		Addons:      []string{"javaee"},
		JavaVersion: "8",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.JavaVersion != "8" {
		t.Fatalf("project java version = %s, want 8", result.Project.JavaVersion)
	}
	requireModuleMetadata(t, result.Project.Modules, "legacy-ear", "ear", "8")
	requireModuleMetadata(t, result.Project.Modules, "legacy-web", "war", "8")

	wantKinds := []string{"http", "soap", "scheduler", "startup", "message_listener", "servlet", "listener", "websocket", "ui_page", "event_listener"}
	for _, kind := range wantKinds {
		if !hasEntryPointKind(result.Project.EntryPoints, kind) {
			t.Fatalf("missing Java EE entrypoint kind %s in %+v", kind, result.Project.EntryPoints)
		}
	}
	if hasEntryPointKind(result.Project.EntryPoints, "filter") {
		t.Fatalf("filters must not be treated as primary entrypoints: %+v", result.Project.EntryPoints)
	}
	if hasEntryPointKind(result.Project.EntryPoints, "async") {
		t.Fatalf("@Asynchronous must not be treated as a primary entrypoint: %+v", result.Project.EntryPoints)
	}
	if !hasEntryPointPath(result.Project.EntryPoints, "GET", "/v1/orders/{id}") {
		t.Fatalf("missing JAX-RS GET /v1/orders/{id}: %+v", result.Project.EntryPoints)
	}
	if hasEntryPointPath(result.Project.EntryPoints, "ANY", "/v1/orders") {
		t.Fatalf("@Path without an HTTP method must not become a JAX-RS entrypoint: %+v", result.Project.EntryPoints)
	}
	if !hasEntryPointResource(result.Project.EntryPoints, "message_listener", "jms/queue/orders") {
		t.Fatalf("missing MDB queue resource: %+v", result.Project.EntryPoints)
	}
	if hasEntryPointResource(result.Project.EntryPoints, "soap", "FormalizacaoIntegrationWS.internal") ||
		hasEntryPointResource(result.Project.EntryPoints, "soap", "FormalizacaoIntegrationWS.ignored") {
		t.Fatalf("private or excluded SOAP methods must not become entrypoints: %+v", result.Project.EntryPoints)
	}
	if hasEntryPointResource(result.Project.EntryPoints, "event_listener", "BeforeBeanDiscovery") {
		t.Fatalf("CDI portable extension lifecycle events must not become business entrypoints: %+v", result.Project.EntryPoints)
	}
	if !hasResolvedUIEntry(result.Project.EntryPoints) {
		t.Fatalf("expected JSF action to resolve to @Named bean method: %+v", result.Project.EntryPoints)
	}

	wantDeps := []string{"table", "database_client", "persistence_unit", "queue", "auth_provider", "http_filter", "ui_api_call"}
	for _, kind := range wantDeps {
		if !hasDependencyKind(result.Project.Dependencies, kind) {
			t.Fatalf("missing Java EE dependency kind %s in %+v", kind, result.Project.Dependencies)
		}
	}
	traceData, err := os.ReadFile(result.Artifacts.Traces)
	if err != nil {
		t.Fatal(err)
	}
	var traces map[string]flow.Trace
	if err := json.Unmarshal(traceData, &traces); err != nil {
		t.Fatal(err)
	}
	if !traceHasDependencyKind(traces, "GET", "/v1/orders/{id}", "http_filter") {
		t.Fatalf("expected matching HTTP filters to be attached to the HTTP flow")
	}
	if !traceHasDependencyKind(traces, "POST", "http://{host}{path}webresources/orders", "ui_api_call") {
		t.Fatalf("expected client-side UI API calls to be attached to the UI flow")
	}
	indexData, err := os.ReadFile(result.Artifacts.Index)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(indexData), "Nenhum entrypoint Spring") {
		t.Fatalf("index should use framework-neutral empty entrypoint wording")
	}
}

func javaEEFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "pom.xml"), `
<project>
  <packaging>pom</packaging>
  <properties>
    <maven.compiler.source>1.8</maven.compiler.source>
  </properties>
</project>
`)
	writeFile(t, filepath.Join(root, "legacy-ear", "pom.xml"), "<project><packaging>ear</packaging></project>")
	writeFile(t, filepath.Join(root, "legacy-ear", "src", "main", "application", "META-INF", "application.xml"), `
<application>
  <module><web><web-uri>legacy-web.war</web-uri><context-root>/legacy</context-root></web></module>
</application>
`)
	writeFile(t, filepath.Join(root, "legacy-web", "pom.xml"), "<project><packaging>war</packaging></project>")
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "webapp", "WEB-INF", "web.xml"), `
<web-app>
  <servlet>
    <servlet-name>XmlServlet</servlet-name>
    <servlet-class>com.acme.legacy.XmlServlet</servlet-class>
  </servlet>
  <servlet-mapping>
    <servlet-name>XmlServlet</servlet-name>
    <url-pattern>/xml</url-pattern>
  </servlet-mapping>
  <filter>
    <filter-name>AuditXmlFilter</filter-name>
    <filter-class>com.acme.legacy.AuditXmlFilter</filter-class>
  </filter>
  <filter-mapping>
    <filter-name>AuditXmlFilter</filter-name>
    <url-pattern>/*</url-pattern>
  </filter-mapping>
  <resource-ref>
    <res-ref-name>jms/queue/xml</res-ref-name>
  </resource-ref>
</web-app>
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "webapp", "index.xhtml"), `
<html xmlns:h="http://java.sun.com/jsf/html">
  <h:commandButton action="#{portalBean.save}" value="Save"/>
</html>
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "webapp", "index.jsp"), `
<!DOCTYPE html>
<html>
  <body>
    <button onclick="saveOrder()">Save</button>
    <script src="rest.js"></script>
  </body>
</html>
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "webapp", "rest.js"), `
var restUri = "http://" + document.location.host + document.location.pathname + "webresources/orders";

function saveOrder() {
  var xhr = new XMLHttpRequest();
  xhr.open("POST", restUri, false);
  xhr.send("ok");
}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "resources", "META-INF", "persistence.xml"), `
<persistence>
  <persistence-unit name="legacyPU"/>
</persistence>
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "RestApi.java"), `
package com.acme.legacy;

import javax.ws.rs.GET;
import javax.ws.rs.Path;

@Path("/v1")
public class RestApi {
  @GET
  @Path("/orders/{id}")
  public String getOrder(String id) { return "ok"; }

  @Path("/orders")
  public OrderResource orders() { return new OrderResource(); }
}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "LegacyServlet.java"), `
package com.acme.legacy;

import javax.servlet.annotation.WebServlet;

@WebServlet(urlPatterns = {"/legacy"})
public class LegacyServlet {}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "AuditFilter.java"), `
package com.acme.legacy;

import javax.servlet.annotation.WebFilter;

@WebFilter("/*")
public class AuditFilter {}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "LegacyListener.java"), `
package com.acme.legacy;

import javax.servlet.annotation.WebListener;

@WebListener
public class LegacyListener {}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "OrderSocket.java"), `
package com.acme.legacy;

import javax.websocket.OnMessage;
import javax.websocket.server.ServerEndpoint;

@ServerEndpoint("/ws/orders")
public class OrderSocket {
  @OnMessage
  public String receive(String message) { return message; }
}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "PortalBean.java"), `
package com.acme.legacy;

import javax.inject.Named;

@Named("portalBean")
public class PortalBean {
  public String save() { return "saved"; }
}
`)
	writeFile(t, filepath.Join(root, "legacy-web", "src", "main", "java", "com", "acme", "legacy", "OrderEntity.java"), `
package com.acme.legacy;

import javax.persistence.Entity;
import javax.persistence.Table;

@Entity
@Table(name = "orders")
public class OrderEntity {}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "pom.xml"), "<project/>")
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "resources", "META-INF", "ejb-jar.xml"), `
<ejb-jar>
  <enterprise-beans>
    <session><ejb-name>DescriptorBean</ejb-name></session>
  </enterprise-beans>
</ejb-jar>
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "FormalizacaoIntegrationWS.java"), `
package com.acme.legacy;

import javax.jws.WebMethod;
import javax.jws.WebService;

@WebService(serviceName = "FormalizacaoIntegrationWS")
public class FormalizacaoIntegrationWS {
  @WebMethod(operationName = "formalizar")
  public String formalizar(String documento) { return documento; }

  public String consultar(String documento) { return documento; }

  @WebMethod(exclude = true)
  public String ignored(String documento) { return documento; }

  private String internal(String documento) { return documento; }
}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "OrderScheduler.java"), `
package com.acme.legacy;

import javax.annotation.PostConstruct;
import javax.ejb.Asynchronous;
import javax.ejb.Schedule;
import javax.ejb.Singleton;
import javax.ejb.Startup;

@Singleton
@Startup
public class OrderScheduler {
  @PostConstruct
  public void init() {}

  @Schedule(minute = "*/2", hour = "*")
  public void tick() {}

  @Asynchronous
  public void reprocess() {}
}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "OrderConsumer.java"), `
package com.acme.legacy;

import javax.ejb.ActivationConfigProperty;
import javax.ejb.MessageDriven;
import javax.jms.Message;

@MessageDriven(activationConfig = {
  @ActivationConfigProperty(propertyName = "destinationLookup", propertyValue = "jms/queue/orders")
})
public class OrderConsumer {
  public void onMessage(Message message) {}
}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "EventObserver.java"), `
package com.acme.legacy;

import javax.enterprise.event.Observes;

public class EventObserver {
  public void observe(@Observes OrderEvent event) {}
}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "PortableExtension.java"), `
package com.acme.legacy;

import javax.enterprise.event.Observes;
import javax.enterprise.inject.spi.BeforeBeanDiscovery;
import javax.enterprise.inject.spi.Extension;

public class PortableExtension implements Extension {
  public void boot(@Observes BeforeBeanDiscovery event) {}
}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "PersistenceService.java"), `
package com.acme.legacy;

import javax.persistence.EntityManager;
import javax.persistence.PersistenceContext;

public class PersistenceService {
  @PersistenceContext(unitName = "legacyPU")
  private EntityManager em;

  public void load(String id) {
    em.find(OrderEntity.class, id);
  }
}
`)
	writeFile(t, filepath.Join(root, "legacy-ejb", "src", "main", "java", "com", "acme", "legacy", "SecurityLoginModule.java"), `
package com.acme.legacy;

import javax.security.auth.spi.LoginModule;

public class SecurityLoginModule implements LoginModule {}
`)
	return root
}

func requireModuleMetadata(t *testing.T, modules []model.Module, name, packaging, javaVersion string) {
	t.Helper()
	for _, module := range modules {
		if module.Name == name {
			if module.Packaging != packaging || module.JavaVersion != javaVersion {
				t.Fatalf("module %s metadata = packaging %s java %s, want %s/%s", name, module.Packaging, module.JavaVersion, packaging, javaVersion)
			}
			return
		}
	}
	t.Fatalf("module %s not found in %+v", name, modules)
}

func hasEntryPointKind(entries []model.EntryPoint, kind string) bool {
	for _, entry := range entries {
		if entry.Framework == "javaee" && entry.Kind == kind {
			return true
		}
	}
	return false
}

func hasEntryPointPath(entries []model.EntryPoint, method, path string) bool {
	for _, entry := range entries {
		if entry.Framework == "javaee" && entry.HTTPMethod == method && entry.Path == path {
			return true
		}
	}
	return false
}

func hasEntryPointResource(entries []model.EntryPoint, kind, resource string) bool {
	for _, entry := range entries {
		if entry.Framework == "javaee" && entry.Kind == kind && entry.Resource == resource {
			return true
		}
	}
	return false
}

func hasResolvedUIEntry(entries []model.EntryPoint) bool {
	for _, entry := range entries {
		if entry.Framework == "javaee" && entry.Kind == "ui_page" && entry.ClassID != "" && entry.MethodID != "" {
			return true
		}
	}
	return false
}

func hasDependencyKind(deps []model.Dependency, kind string) bool {
	for _, dep := range deps {
		if dep.Kind == kind {
			return true
		}
	}
	return false
}

func traceHasDependencyKind(traces map[string]flow.Trace, method, path, kind string) bool {
	for _, trace := range traces {
		if trace.EntryPoint.HTTPMethod != method || trace.EntryPoint.Path != path {
			continue
		}
		for _, dep := range trace.Dependencies {
			if dep.Dependency.Kind == kind {
				return true
			}
		}
	}
	return false
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

func visualizationProcessCards(t *testing.T, data string) map[string]string {
	t.Helper()
	matches := regexp.MustCompile(`(?s)<script id="process-cards" type="application/json">(.*?)</script>`).FindStringSubmatch(data)
	if len(matches) != 2 {
		t.Fatalf("process-cards script not found")
	}
	var cards map[string]string
	if err := json.Unmarshal([]byte(html.UnescapeString(matches[1])), &cards); err != nil {
		t.Fatalf("invalid process-cards json: %v", err)
	}
	if len(cards) == 0 {
		t.Fatalf("expected process cards")
	}
	return cards
}

func mapValues(t *testing.T, values map[string]string) []string {
	t.Helper()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

package java

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileJavaVersions(t *testing.T) {
	tests := map[string]string{
		"java8": `
			package com.acme.legacy;
			import java.util.Optional;
			import java.util.Arrays;
			public class LegacyService {
				public Optional<String> find(String id) { return Optional.of(id.trim()); }
				public void callbacks() {
					Arrays.asList("a").forEach(this::accept);
					Runnable run = () -> accept("ok");
				}
				void accept(String value) {}
			}
		`,
		"java11": `
			package com.acme.modern;
			public interface ModernService {
				default String normalize(String input) { return input.strip(); }
			}
		`,
		"java17": `
			package com.acme.domain;
			public sealed class Payment permits CardPayment {
				public String id() { return "p"; }
			}
			final class CardPayment extends Payment {}
		`,
		"java21": `
			package com.acme.records;
			public record UserView(String id, String name) {
				public String label() { return name.formatted(); }
			}
		`,
	}

	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name+".java")
			if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			file, types, gaps, err := ParseFile(path, "module-main")
			if err != nil {
				t.Fatal(err)
			}
			if len(gaps) != 0 {
				t.Fatalf("unexpected gaps: %+v", gaps)
			}
			if file.Package == "" {
				t.Fatalf("expected package, got empty")
			}
			if len(types) == 0 {
				t.Fatalf("expected at least one type")
			}
			if len(types[0].Methods) == 0 && name != "java17" {
				t.Fatalf("expected methods in first type: %+v", types[0])
			}
			if name == "java8" && !containsString(types[0].Methods[0].Modifiers, "public") {
				t.Fatalf("expected public modifier on first Java 8 method, got %+v", types[0].Methods[0].Modifiers)
			}
		})
	}
}

func TestParseFileAnnotatedParametersAndJava8RecordIdentifier(t *testing.T) {
	source := `
		package com.acme.legacy;
		import javax.enterprise.event.Observes;

		class record {
			void observe(@Observes OrderEvent event) {
				handle(event);
			}
			void handle(OrderEvent event) {}
		}
	`
	path := filepath.Join(t.TempDir(), "Record.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, types, _, err := ParseFile(path, "module-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 || types[0].Name != "record" {
		t.Fatalf("expected Java 8 class named record, got %+v", types)
	}
	method := types[0].Methods[0]
	if len(method.Parameters) != 1 {
		t.Fatalf("parameters = %+v, want one", method.Parameters)
	}
	param := method.Parameters[0]
	if param.Name != "event" || param.Type != "OrderEvent" {
		t.Fatalf("parameter = %+v, want event OrderEvent", param)
	}
	if len(param.Annotations) != 1 || param.Annotations[0].Name != "Observes" {
		t.Fatalf("expected @Observes parameter annotation, got %+v", param.Annotations)
	}
}

func TestParseFileSpringAnnotationsAndCalls(t *testing.T) {
	source := `
		package com.acme.orders;
		import org.springframework.web.bind.annotation.GetMapping;
		import org.springframework.web.bind.annotation.RestController;

		@RestController
		class OrderController {
			@GetMapping(path = "/orders/{id}")
			OrderDto get(String id) {
				return service.findById(id);
			}
		}
	`
	path := filepath.Join(t.TempDir(), "OrderController.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, types, _, err := ParseFile(path, "module-orders")
	if err != nil {
		t.Fatal(err)
	}
	if got := types[0].Annotations[0].Name; got != "RestController" {
		t.Fatalf("annotation = %s, want RestController", got)
	}
	method := types[0].Methods[0]
	if got := method.Annotations[0].Values["path"]; got != "/orders/{id}" {
		t.Fatalf("path = %s, want /orders/{id}", got)
	}
	if len(method.Calls) == 0 || method.Calls[0].Target != "service.findById" {
		t.Fatalf("expected service.findById call, got %+v", method.Calls)
	}
}

func TestParseFileLocalVariables(t *testing.T) {
	source := `
		package com.acme.orders;
		import com.acme.orders.model.OrderSetting;
		import java.util.List;

		class OrderService {
			void cancel() {
				final OrderSetting orderSetting = mapper.selectByPrimaryKey(1L);
				List<Order> orders = dao.find(orderSetting.getNormalOrderOvertime());
			}
		}
	`
	path := filepath.Join(t.TempDir(), "OrderService.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, types, _, err := ParseFile(path, "module-orders")
	if err != nil {
		t.Fatal(err)
	}
	method := types[0].Methods[0]
	foundOrderSetting := false
	foundOrders := false
	for _, local := range method.LocalVariables {
		if local.Name == "orderSetting" && local.VariableType == "OrderSetting" {
			foundOrderSetting = true
		}
		if local.Name == "orders" && strings.Contains(local.VariableType, "List") {
			foundOrders = true
		}
	}
	if !foundOrderSetting {
		t.Fatalf("expected local variable orderSetting, got %+v", method.LocalVariables)
	}
	if !foundOrders {
		t.Fatalf("expected local variable orders, got %+v", method.LocalVariables)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


class PythonCLITest(unittest.TestCase):
    def test_version(self) -> None:
        result = subprocess.run(
            [sys.executable, "-m", "java_process_mapper", "--version"],
            cwd=repo_root(),
            capture_output=True,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), "0.1.0")

    def test_scan_javaee_java8_without_go_core(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            temp_path = Path(temp)
            source_root = javaee_fixture(temp_path / "fixture")
            output_dir = temp_path / "out"

            result = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "java_process_mapper",
                    "scan",
                    "--root",
                    str(source_root),
                    "--out",
                    str(output_dir),
                    "--addons",
                    "javaee",
                    "--java-version",
                    "8",
                ],
                cwd=repo_root(),
                capture_output=True,
                text=True,
            )

            self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
            scan = json.loads(result.stdout)
            self.assertEqual(scan["status"], "completed")

            project = json.loads(Path(scan["artifacts"]["findings"]).read_text(encoding="utf-8"))
            entries = project["entryPoints"]
            entry_names = {entry["name"] for entry in entries}
            entry_kinds = {(entry["kind"], entry["name"]) for entry in entries}
            dependency_kinds = {dep["kind"] for dep in project["dependencies"]}

            self.assertEqual(project["javaVersion"], "8")
            self.assertEqual(module_by_name(project, "fixture")["packaging"], "ear")
            self.assertEqual(module_by_name(project, "web")["packaging"], "war")
            self.assertIn(("http", "OrderResource.get"), entry_kinds)
            self.assertIn(("soap", "OrderSoap.submit"), entry_kinds)
            self.assertIn(("scheduler", "NightlyJob.run"), entry_kinds)
            self.assertIn(("message_listener", "OrderMdb.onMessage"), entry_kinds)
            self.assertIn(("servlet", "LegacyServlet"), entry_kinds)
            self.assertIn(("ui_page", "index.xhtml -> orderBean.save"), entry_kinds)
            self.assertNotIn("AuditFilter", entry_names)
            self.assertNotIn("AsyncBean.addNumbers", entry_names)
            self.assertNotIn("CdiExtension.afterBean", entry_names)
            self.assertIn("http_filter", dependency_kinds)
            self.assertIn("table", dependency_kinds)
            self.assertIn("persistence_unit", dependency_kinds)

            traces = json.loads(Path(scan["artifacts"]["traces"]).read_text(encoding="utf-8"))
            http_entry = next(entry for entry in entries if entry["name"] == "OrderResource.get")
            http_trace = traces[http_entry["id"]]
            self.assertTrue(any(dep["kind"] == "http_filter" for dep in http_trace["dependencies"]))

            visual = Path(scan["artifacts"]["visualization"]).read_text(encoding="utf-8")
            self.assertIn("data-process-search-input", visual)
            self.assertIn("renderSelectedProcess", visual)
            self.assertNotIn('<article class="process"', visual.split('<script type="application/json"', 1)[0])


def javaee_fixture(root: Path) -> Path:
    write_file(
        root / "pom.xml",
        """
        <project>
          <packaging>ear</packaging>
          <properties><maven.compiler.source>11</maven.compiler.source></properties>
          <modules><module>web</module></modules>
        </project>
        """,
    )
    write_file(
        root / "web" / "pom.xml",
        """
        <project>
          <packaging>war</packaging>
          <properties><maven.compiler.source>1.8</maven.compiler.source></properties>
        </project>
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "OrderResource.java",
        """
        package com.acme;

        import javax.ws.rs.GET;
        import javax.ws.rs.Path;

        @Path("/api/orders")
        public class OrderResource {
          @GET
          @Path("/{id}")
          public String get() {
            return "ok";
          }
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "OrderSoap.java",
        """
        package com.acme;

        import javax.jws.WebMethod;
        import javax.jws.WebService;

        @WebService
        public class OrderSoap {
          @WebMethod
          public String submit(String id) { return id; }

          @WebMethod(exclude = true)
          public String ignored() { return ""; }
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "NightlyJob.java",
        """
        package com.acme;

        import javax.ejb.Schedule;
        import javax.ejb.Stateless;

        @Stateless
        public class NightlyJob {
          @Schedule(hour = "2")
          public void run() {}
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "OrderMdb.java",
        """
        package com.acme;

        import javax.ejb.ActivationConfigProperty;
        import javax.ejb.MessageDriven;
        import javax.jms.Message;
        import javax.jms.MessageListener;

        @MessageDriven(activationConfig = {
          @ActivationConfigProperty(propertyName = "destinationLookup", propertyValue = "java:/jms/queue/orders")
        })
        public class OrderMdb implements MessageListener {
          public void onMessage(Message message) {}
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "LegacyServlet.java",
        """
        package com.acme;

        import javax.servlet.annotation.WebServlet;
        import javax.servlet.http.HttpServlet;

        @WebServlet(urlPatterns = "/legacy/*")
        public class LegacyServlet extends HttpServlet {}
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "AuditFilter.java",
        """
        package com.acme;

        import javax.servlet.annotation.WebFilter;

        @WebFilter(urlPatterns = "/api/*")
        public class AuditFilter {}
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "AsyncBean.java",
        """
        package com.acme;

        import javax.ejb.Asynchronous;
        import javax.ejb.Stateless;

        @Stateless
        public class AsyncBean {
          @Asynchronous
          public void addNumbers() {}
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "CdiExtension.java",
        """
        package com.acme;

        import javax.enterprise.event.Observes;
        import javax.enterprise.inject.spi.AfterBeanDiscovery;

        public class CdiExtension {
          public void afterBean(@Observes AfterBeanDiscovery event) {}
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "OrderBean.java",
        """
        package com.acme;

        import javax.inject.Named;

        @Named("orderBean")
        public class OrderBean {
          public String save() { return "ok"; }
        }
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "java" / "com" / "acme" / "Order.java",
        """
        package com.acme;

        import javax.persistence.Entity;
        import javax.persistence.Table;

        @Entity
        @Table(name = "orders")
        public class Order {}
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "webapp" / "index.xhtml",
        """
        <html xmlns:h="http://xmlns.jcp.org/jsf/html">
          <h:commandButton action="#{orderBean.save}" />
          <script src="app.js"></script>
        </html>
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "webapp" / "app.js",
        """
        const xhr = new XMLHttpRequest();
        xhr.open("POST", "/api/orders");
        """,
    )
    write_file(
        root / "web" / "src" / "main" / "resources" / "META-INF" / "persistence.xml",
        """
        <persistence>
          <persistence-unit name="legacyPU">
            <jta-data-source>java:/jdbc/LegacyDS</jta-data-source>
          </persistence-unit>
        </persistence>
        """,
    )
    return root


def write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(textwrap.dedent(content).strip() + "\n", encoding="utf-8")


def module_by_name(project: dict, name: str) -> dict:
    return next(module for module in project["modules"] if module["name"] == name)


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


if __name__ == "__main__":
    unittest.main()

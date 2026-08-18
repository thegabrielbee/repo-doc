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

    def test_scan_javaee_java_version_flag(self) -> None:
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

            findings = Path(scan["artifacts"]["findings"])
            project = json.loads(findings.read_text(encoding="utf-8"))
            self.assertEqual(project["javaVersion"], "8")
            self.assertEqual(len(project["entryPoints"]), 1)
            self.assertEqual(project["entryPoints"][0]["framework"], "javaee")


def javaee_fixture(root: Path) -> Path:
    write_file(
        root / "pom.xml",
        "<project><properties><maven.compiler.source>11</maven.compiler.source></properties></project>",
    )
    write_file(
        root / "src" / "main" / "java" / "com" / "acme" / "RestApi.java",
        """
        package com.acme;

        import javax.ws.rs.GET;
        import javax.ws.rs.Path;

        @Path("/api")
        public class RestApi {
          @GET
          public String ping() { return "pong"; }
        }
        """,
    )
    return root


def write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(textwrap.dedent(content).strip() + "\n", encoding="utf-8")


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


if __name__ == "__main__":
    unittest.main()

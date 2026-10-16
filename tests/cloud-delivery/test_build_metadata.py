"""Execute the real CI and development-Docker Go build recipes, without Docker/network.

The runtime-only image consumes the CI artifact; it must not rebuild it. These
checks exercise the two compilation entries and their CLI metadata, not merely
whether a linker variable name appears in a file.
"""

import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
COMMIT = "0123456789abcdef0123456789abcdef01234567"
BUILD_DATE = "2026-10-16T00:00:00Z"


class TestBuildMetadata(unittest.TestCase):
    def test_static_binary_version_from_supported_build_entries(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        ci_recipe = re.search(
            r"      - name: Build pure-static Go binary\n        run: \|\n(.*?)(?=\n      - name:)",
            workflow, re.S,
        )
        self.assertIsNotNone(ci_recipe, "CI compilation recipe missing")
        docker_recipe = re.search(
            r"^RUN BUILD_TIME=.*?(?=\n\n)", (ROOT / "Dockerfile").read_text(), re.S | re.M,
        )
        self.assertIsNotNone(docker_recipe, "Docker compilation recipe missing")
        for entry, recipe in (("CI", ci_recipe.group(1)), ("Dockerfile", docker_recipe.group(0))):
            with self.subTest(entry=entry), tempfile.TemporaryDirectory(prefix="csp-build-version-") as tmp:
                binary = Path(tmp) / "csp"
                if entry == "CI":
                    # Preserve the build command verbatim, substituting only the
                    # runner-provided SHA/time/output boundary (no runner writes).
                    command = next(line.strip() for line in recipe.splitlines() if " go build " in line)
                    command = command.replace("${{ github.sha }}", COMMIT).replace("bin/csp-linux-amd64", str(binary))
                else:
                    # Execute the actual RUN command, including its BUILD_DATE
                    # fallback and linker arguments, with container output isolated.
                    command = recipe.removeprefix("RUN ").replace("/src/bin/csp", str(binary))
                env = os.environ.copy()
                env.update(
                    CGO_ENABLED="0", GOOS="linux", GOARCH="amd64",
                    COMMIT_SHA=COMMIT, BUILD_DATE=BUILD_DATE, BUILD_TIME=BUILD_DATE,
                    GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local",
                )
                build = subprocess.run(["bash", "-euc", command], cwd=ROOT, env=env, capture_output=True, text=True, timeout=300)
                self.assertEqual(build.returncode, 0, f"{entry} build failed: {build.stdout}\n{build.stderr}")
                version = subprocess.run([str(binary), "--version"], cwd=tmp, env=env, capture_output=True, text=True, timeout=10)
                self.assertEqual(version.returncode, 0, version.stderr)
                self.assertIn(f"commit: {COMMIT}, built: {BUILD_DATE}", version.stdout)
                elf = subprocess.run(["readelf", "-l", str(binary)], capture_output=True, text=True, timeout=10)
                self.assertEqual(elf.returncode, 0, elf.stderr)
                self.assertNotIn("INTERP", elf.stdout, "binary must have no dynamic ELF interpreter")
                print(f"{entry}: static binary --version asserted: {version.stdout.strip()}", flush=True)


if __name__ == "__main__":
    unittest.main(verbosity=2)

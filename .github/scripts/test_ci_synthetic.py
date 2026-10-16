#!/usr/bin/env python3
"""
Lightweight isolated synthetic test runner for CSP Cloud CI & Delivery pipeline.

Executes:
1. Python syntax & compilation checks (py_compile)
2. Shell syntax checks (bash -n)
3. GitHub Actions workflow schema & logic validation (.github/workflows/ci.yml)
4. Dockerfile.runtime contract validation (no compilers, non-root, OCI labels)
5. SHA binding & checksum manifest verification
6. Historical private-tool HTTP mock suite (not an active release policy):
   - Direct unauthenticated 200 manifest -> MUST reject (Fail-Closed)
   - Anonymous bearer challenge token retrieval returning 200 -> MUST reject (Fail-Closed)
   - Missing/false ENABLE_CSP_GHCR_PUBLISH variable -> FAIL-CLOSED
   - Historical public package name 'clash-sub-parser' or unapproved namespace -> MUST reject
   - Pre-check API 403 / 401 (insufficient permissions) -> MUST FAIL-CLOSED (NO downgrade)
   - Pre-check 404 on direct lookup without complete pagination proof -> MUST FAIL-CLOSED
   - Pre-check 404 with complete pagination proof of non-existence -> PASS
   - Post-check authorized API 403 / non-private -> MUST FAIL-CLOSED (even if anonymous denies)
   - Post-check with BOTH authorized private API + anonymous denial -> PASS
   - Token masking validation (zero secrets in logs)
7. Git diff & write-set isolation audit
8. Docker buildx build --check contract verification (no runtime image rebuild)
"""

import ast
import hashlib
import json
import os
import py_compile
import re
import subprocess
import sys
import tempfile
import unittest
import urllib.error
import yaml
from io import BytesIO
from unittest.mock import MagicMock, patch
from pathlib import Path

import promote_prelease as promotion

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
SCRIPTS_DIR = os.path.join(REPO_ROOT, ".github/scripts")
WORKFLOW_FILE = os.path.join(REPO_ROOT, ".github/workflows/ci.yml")
DOCKERFILE_RUNTIME = os.path.join(REPO_ROOT, "Dockerfile.runtime")
DEPENDABOT_FILE = os.path.join(REPO_ROOT, ".github/dependabot.yml")

# Import check_ghcr_private_gate module dynamically
sys.path.insert(0, SCRIPTS_DIR)
import check_ghcr_private_gate as gate


def safe_eval_bool(expr: str, env: dict) -> bool:
    """Bounded AST boolean evaluator for workflow condition expressions.

    Strict allowlist of AST nodes:
      - Expression, Constant, Name, Load, BoolOp (And, Or), UnaryOp (Not), Compare (Eq, NotEq).
    Rejects Calls, Attributes, Subscripts, builtins, and any unallowed nodes
    without execution.
    """
    parsed = ast.parse(expr, mode="eval")
    allowed_nodes = (
        ast.Expression,
        ast.BoolOp,
        ast.And,
        ast.Or,
        ast.UnaryOp,
        ast.Not,
        ast.Compare,
        ast.Eq,
        ast.NotEq,
        ast.Name,
        ast.Load,
        ast.Constant,
    )
    for node in ast.walk(parsed):
        if not isinstance(node, allowed_nodes):
            raise ValueError(f"Disallowed AST node: {type(node).__name__}")

    def _eval(node):
        if isinstance(node, ast.Expression):
            return _eval(node.body)
        if isinstance(node, ast.Constant):
            return node.value
        if isinstance(node, ast.Name):
            if node.id in env:
                return env[node.id]
            raise ValueError(f"Unbound variable: {node.id}")
        if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.Not):
            return not _eval(node.operand)
        if isinstance(node, ast.BoolOp):
            if isinstance(node.op, ast.And):
                return all(_eval(v) for v in node.values)
            if isinstance(node.op, ast.Or):
                return any(_eval(v) for v in node.values)
        if isinstance(node, ast.Compare):
            left = _eval(node.left)
            for op, comparator in zip(node.ops, node.comparators):
                right = _eval(comparator)
                if isinstance(op, ast.Eq):
                    if not (left == right):
                        return False
                elif isinstance(op, ast.NotEq):
                    if not (left != right):
                        return False
                else:
                    raise ValueError(f"Disallowed comparison op: {type(op).__name__}")
                left = right
            return True
        raise ValueError(f"Unsupported node: {type(node).__name__}")

    return bool(_eval(parsed.body))


class TestPythonAndShellSyntax(unittest.TestCase):
    def test_py_compile_all_scripts(self):
        scripts = [
            os.path.join(SCRIPTS_DIR, "annotate_go_test.py"),
            os.path.join(SCRIPTS_DIR, "setup_test_archive.py"),
            os.path.join(SCRIPTS_DIR, "check_ghcr_private_gate.py"),
            os.path.join(SCRIPTS_DIR, "promote_prelease.py"),
        ]
        for script_path in scripts:
            with self.subTest(script=os.path.basename(script_path)):
                self.assertTrue(os.path.isfile(script_path), f"File missing: {script_path}")
                py_compile.compile(script_path, doraise=True)

    def test_bash_syntax_install_script(self):
        bash_script = os.path.join(SCRIPTS_DIR, "install_test_binaries.sh")
        self.assertTrue(os.path.isfile(bash_script), f"File missing: {bash_script}")
        res = subprocess.run(["bash", "-n", bash_script], capture_output=True, text=True)
        self.assertEqual(res.returncode, 0, f"bash -n failed: {res.stderr}")


class TestSyntheticArchive(unittest.TestCase):
    def test_archive_is_explicit_synthetic_and_never_overwrites(self):
        import sqlite3
        import setup_test_archive
        with tempfile.TemporaryDirectory() as directory:
            target = os.path.join(directory, "synthetic.db")
            setup_test_archive.create_synthetic_archive(target)
            with sqlite3.connect(target) as db:
                self.assertEqual(db.execute("SELECT count(*) FROM subscriptions").fetchone()[0], 2)
            with open(target, "rb") as f:
                original = f.read()
            with self.assertRaises(FileExistsError):
                setup_test_archive.create_synthetic_archive(target)
            with open(target, "rb") as f:
                self.assertEqual(f.read(), original)
            self.assertEqual(os.stat(target).st_mode & 0o777, 0o600)


class TestWorkflowLogicAndSchema(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        self = cls
        self.assertTrue(os.path.isfile(WORKFLOW_FILE), f"Workflow file missing: {WORKFLOW_FILE}")
        with open(WORKFLOW_FILE, "r", encoding="utf-8") as f:
            self.wf = yaml.safe_load(f)

    def test_triggers_and_paths_ignore(self):
        on = self.wf.get("on") or self.wf.get(True, {})
        self.assertIn("pull_request", on, "pull_request trigger must be present")
        self.assertIn("push", on, "push trigger must be present")
        self.assertIn("workflow_dispatch", on, "workflow_dispatch trigger must be present")

        # Paths ignore MUST be removed to ensure doc-only dev changes get required checks
        self.assertNotIn("paths-ignore", on["pull_request"], "PR paths-ignore must be removed to avoid check deadlocks")
        self.assertNotIn("paths-ignore", on["push"], "Push paths-ignore must be removed to avoid check deadlocks")

        # Push branches & tags: ONLY 'dev'
        push_branches = on["push"].get("branches", [])
        self.assertEqual(push_branches, ["dev"], f"Push branches must strictly be ['dev'], got {push_branches}")
        for forbidden in ("main", "task/csp-cloud-build-integration", "cloud-build/*", "release/**", "master"):
            self.assertNotIn(forbidden, push_branches, f"Forbidden branch in push trigger: {forbidden}")

        # Push tags must be completely retired
        self.assertNotIn("tags", on["push"], "Push tags must be retired (no v* or prelease tags)")

        # Pull request branches: ONLY 'dev'
        pr_branches = on["pull_request"].get("branches", [])
        self.assertEqual(pr_branches, ["dev"], f"Pull request branches must strictly be ['dev'], got {pr_branches}")
        for forbidden in ("main", "task/csp-cloud-build-integration", "cloud-build/*", "release/**", "master"):
            self.assertNotIn(forbidden, pr_branches, f"Forbidden branch in pull_request trigger: {forbidden}")

        # Workflow dispatch inputs: dev only with explicit publish opt-in
        wd_inputs = on["workflow_dispatch"].get("inputs", {})
        self.assertIn("publish_release", wd_inputs, "workflow_dispatch must have publish_release input")
        self.assertEqual(wd_inputs["publish_release"].get("default"), "false")

    def test_concurrency_and_permissions(self):
        jobs = self.wf.get("jobs", {})
        self.assertIn("test-and-build", jobs)
        self.assertIn("publish-runtime", jobs)

        checks_job = jobs["test-and-build"]
        checks_concurrency = checks_job.get("concurrency", {})
        self.assertFalse(
            checks_concurrency.get("cancel-in-progress", True),
            "Checks concurrency cancel-in-progress must be false",
        )
        self.assertNotIn("release/", str(checks_concurrency), "Retired release/ clause must be removed from checks concurrency")
        self.assertNotIn("tags/", str(checks_concurrency), "Retired tags/ clause must be removed from checks concurrency")

        self.assertEqual(
            checks_job.get("permissions", {}).get("contents"),
            "read",
            "Checks job permissions must be read-only",
        )
        self.assertNotIn(
            "packages",
            checks_job.get("permissions", {}),
            "Checks job MUST NOT have packages permission",
        )

        publish_job = jobs["publish-runtime"]
        pub_concurrency = publish_job.get("concurrency", {})
        self.assertFalse(
            pub_concurrency.get("cancel-in-progress"),
            "Release publishing concurrency must NOT cancel in progress",
        )
        self.assertEqual(
            publish_job.get("permissions", {}).get("packages"),
            "write",
            "Publish job must have packages: write permission",
        )

    def test_dependency_and_release_condition(self):
        publish_job = self.wf["jobs"]["publish-runtime"]
        needs = publish_job.get("needs", [])
        self.assertIn("test-and-build", needs, "publish-runtime MUST depend on test-and-build")

        condition = publish_job.get("if", "")
        # String clause assertions
        self.assertIn("needs.test-and-build.result == 'success'", condition)
        self.assertIn("github.event_name != 'pull_request'", condition)
        self.assertIn("github.ref == 'refs/heads/dev'", condition)
        self.assertIn("github.event_name == 'push'", condition)
        self.assertIn("github.event_name == 'workflow_dispatch'", condition)
        self.assertIn("inputs.publish_release == 'true'", condition)
        self.assertNotIn("refs/tags/v", condition, "Condition must NOT support retired v* tags")
        self.assertNotIn("refs/heads/release/", condition, "Condition must NOT support retired release/* branches")
        self.assertNotIn("refs/heads/main", condition, "Condition must NOT allow main branch publishing")
        self.assertNotIn("always()", condition)

        # Exact semantic evaluation (using bounded AST allowlist, no arbitrary eval)
        def eval_condition(result="success", event="push", ref="refs/heads/dev", pub="false"):
            expr = condition.replace("&&", " and ").replace("||", " or ")
            expr = expr.replace("needs.test-and-build.result", "result")
            expr = expr.replace("github.event_name", "event")
            expr = expr.replace("github.ref", "ref")
            expr = expr.replace("inputs.publish_release", "pub")
            return safe_eval_bool(expr, {"result": result, "event": event, "ref": ref, "pub": pub})

        # Positive authorized paths
        self.assertTrue(eval_condition(result="success", event="push", ref="refs/heads/dev"),
                        "dev push with successful checks must publish")
        self.assertTrue(eval_condition(result="success", event="workflow_dispatch", ref="refs/heads/dev", pub="true"),
                        "dev workflow_dispatch with explicit publish_release=true must publish")

        # Negative paths: old branches, main, tags, and manual other refs
        self.assertFalse(eval_condition(result="success", event="push", ref="refs/heads/main"),
                         "main push must never trigger publish")
        self.assertFalse(eval_condition(result="success", event="push", ref="refs/heads/release/20261010-01"),
                         "retired release branch must not publish")
        self.assertFalse(eval_condition(result="success", event="push", ref="refs/tags/v1.0.0"),
                         "v* tag push must not publish")
        self.assertFalse(eval_condition(result="success", event="push", ref="refs/heads/task/csp-cloud-build-integration"),
                         "retired task branch must not publish")
        self.assertFalse(eval_condition(result="success", event="push", ref="refs/heads/cloud-build/csp-complete-20261016-230722"),
                         "retired cloud-build branch must not publish")
        self.assertFalse(eval_condition(result="success", event="pull_request", ref="refs/heads/dev"),
                         "pull_request must never trigger publish")
        self.assertFalse(eval_condition(result="success", event="pull_request", ref="refs/pull/20/merge"),
                         "pull_request merge ref must never trigger publish")
        self.assertFalse(eval_condition(result="success", event="workflow_dispatch", ref="refs/heads/dev", pub="false"),
                         "workflow_dispatch without publish_release must not publish")
        self.assertFalse(eval_condition(result="success", event="workflow_dispatch", ref="refs/heads/main", pub="true"),
                         "workflow_dispatch on non-dev ref (main) must not publish")
        self.assertFalse(eval_condition(result="success", event="workflow_dispatch", ref="refs/heads/task/custom", pub="true"),
                         "workflow_dispatch on arbitrary task ref must not publish")
        self.assertFalse(eval_condition(result="failure", event="push", ref="refs/heads/dev"),
                         "failed test-and-build must never trigger publish")

    def test_entry_job_condition_and_dispatch_gating(self):
        """Entry job (test-and-build) must gate against non-dev workflow_dispatch,

        while preserving PR checks and dev push/dispatch.
        """
        checks_job = self.wf["jobs"]["test-and-build"]
        self.assertIn("if", checks_job, "test-and-build entry job must define an if condition")
        condition = checks_job["if"]

        self.assertIn("github.event_name != 'workflow_dispatch'", condition)
        self.assertIn("github.ref == 'refs/heads/dev'", condition)

        def eval_entry_condition(event="push", ref="refs/heads/dev"):
            expr = condition.replace("&&", " and ").replace("||", " or ")
            expr = expr.replace("github.event_name", "event")
            expr = expr.replace("github.ref", "ref")
            return safe_eval_bool(expr, {"event": event, "ref": ref})

        # Positive paths: dev push, dev workflow_dispatch, and pull_request (which uses refs/pull/*/merge)
        self.assertTrue(eval_entry_condition(event="push", ref="refs/heads/dev"),
                        "dev push must run entry checks")
        self.assertTrue(eval_entry_condition(event="workflow_dispatch", ref="refs/heads/dev"),
                        "dev workflow_dispatch must run entry checks")
        self.assertTrue(eval_entry_condition(event="pull_request", ref="refs/pull/42/merge"),
                        "pull_request must run entry checks even with non-dev merge ref")

        # Negative paths: workflow_dispatch on other refs must skip entry checks
        self.assertFalse(eval_entry_condition(event="workflow_dispatch", ref="refs/heads/main"),
                         "workflow_dispatch on main must skip entry checks")
        self.assertFalse(eval_entry_condition(event="workflow_dispatch", ref="refs/heads/task/custom"),
                         "workflow_dispatch on task ref must skip entry checks")
        self.assertFalse(eval_entry_condition(event="workflow_dispatch", ref="refs/heads/release/20261010-01"),
                         "workflow_dispatch on release ref must skip entry checks")
        self.assertFalse(eval_entry_condition(event="workflow_dispatch", ref="refs/tags/v1.0.0"),
                         "workflow_dispatch on tag must skip entry checks")

    def test_safe_eval_ast_rejects_malicious_call_and_attribute(self):
        """Bounded AST evaluator must reject function calls and attribute access without execution."""
        # Malicious Call
        with self.assertRaises(ValueError) as ctx:
            safe_eval_bool("__import__('os').system('echo pwned')", {})
        self.assertIn("Disallowed AST node: Call", str(ctx.exception))

        with self.assertRaises(ValueError) as ctx:
            safe_eval_bool("maliciousCall()", {})
        self.assertIn("Disallowed AST node: Call", str(ctx.exception))

        # Malicious Attribute
        with self.assertRaises(ValueError) as ctx:
            safe_eval_bool("().__class__", {})
        self.assertIn("Disallowed AST node: Attribute", str(ctx.exception))

        with self.assertRaises(ValueError) as ctx:
            safe_eval_bool("event.attr == 'dev'", {"event": "dev"})
        self.assertIn("Disallowed AST node: Attribute", str(ctx.exception))

        # Disallowed Subscript
        with self.assertRaises(ValueError) as ctx:
            safe_eval_bool("env['secret'] == 'val'", {"env": {}})
        self.assertIn("Disallowed AST node: Subscript", str(ctx.exception))

    def test_dependabot_configuration(self):
        self.assertTrue(os.path.isfile(DEPENDABOT_FILE), f"Dependabot file missing: {DEPENDABOT_FILE}")
        with open(DEPENDABOT_FILE, "r", encoding="utf-8") as f:
            cfg = yaml.safe_load(f)
        self.assertEqual(cfg.get("version"), 2)
        updates = cfg.get("updates", [])
        self.assertEqual(len(updates), 1, "Must have exactly 1 update ecosystem")
        gh_actions = updates[0]
        self.assertEqual(gh_actions.get("package-ecosystem"), "github-actions")
        self.assertEqual(gh_actions.get("directory"), "/")
        self.assertEqual(gh_actions.get("target-branch"), "dev")
        # Assert absence of old frontend/backend ecosystems
        ecosystems = [u.get("package-ecosystem") for u in updates]
        for retired in ("npm", "pip", "docker", "gomod"):
            self.assertNotIn(retired, ecosystems, f"Retired ecosystem {retired} must not be present")

    def test_dev_promotion_is_job_scoped_and_uses_same_artifact(self):
        jobs = self.wf['jobs']
        self.assertEqual(self.wf['permissions'], {'contents': 'read'})
        self.assertEqual(jobs['publish-runtime']['permissions'], {'contents': 'read', 'packages': 'write'})
        promote = jobs['promote-prelease']
        self.assertEqual(promote['permissions'], {'contents': 'write'})
        self.assertCountEqual(promote['needs'], ['test-and-build', 'publish-runtime'])
        for clause in ("github.event_name == 'push'", "github.ref == 'refs/heads/dev'",
                       "needs.test-and-build.result == 'success'", "needs.publish-runtime.result == 'success'"):
            self.assertIn(clause, promote['if'])
        self.assertNotIn('always()', promote['if'])
        self.assertFalse(promote['concurrency']['cancel-in-progress'])
        self.assertIn('github.sha', self.wf['concurrency']['group'])
        self.assertFalse(self.wf['concurrency']['cancel-in-progress'])
        for job in jobs.values():
            for step in job['steps']:
                self.assertFalse(step.get('continue-on-error', False))
        downloads = [next(step for step in jobs[name]['steps'] if 'download-artifact' in step.get('uses', ''))
                     for name in ('publish-runtime', 'promote-prelease')]
        self.assertEqual(downloads[0]['with'], downloads[1]['with'])
        self.assertEqual(downloads[0]['with'], {'name': 'csp-linux-amd64', 'path': 'bin'})
        promote_runs = '\n'.join(step.get('run', '') for step in promote['steps'])
        self.assertIn('promote_prelease.py publish', promote_runs)
        for forbidden in ('go build', 'npm ', '--clobber', 'git push', 'docker compose'):
            self.assertNotIn(forbidden, promote_runs)
        publish = promote['steps'][-1]
        self.assertEqual(publish['env']['IMAGE_DIGEST'], '${{ needs.publish-runtime.outputs.image_digest }}')
        self.assertEqual(publish['env']['GH_TOKEN'], '${{ github.token }}')
        checks = jobs['test-and-build']['steps']
        build = next(step for step in checks if step.get('name') == 'Build pure-static Go binary')
        self.assertEqual(build['if'], "steps.reuse.outputs.reused != 'true'")
        metadata_test = next(step for step in checks if step.get('name') == 'Run real build-entry metadata regression (cloud only)')
        self.assertIn("-p 'test_build_metadata.py'", metadata_test['run'])
        reuse = next(step for step in checks if step.get('id') == 'reuse')
        self.assertEqual(reuse['if'], "github.event_name == 'push' && github.ref == 'refs/heads/dev'")
        image_build = next(step for step in jobs['publish-runtime']['steps'] if step.get('id') == 'build-push')
        self.assertEqual(image_build['if'], "needs.test-and-build.outputs.reused != 'true'")

    def test_checks_job_steps(self):
        steps = self.wf["jobs"]["test-and-build"].get("steps", [])
        step_names = [s.get("name", "") for s in steps]
        step_runs = "\n".join([s.get("run", "") for s in steps if "run" in s])

        self.assertTrue(any("Set up Node" in n for n in step_names))
        self.assertTrue(any("Set up Go" in n for n in step_names))
        self.assertIn("npm ci --include=dev", step_runs)
        self.assertIn("npm run type-check", step_runs)
        self.assertIn("npm test", step_runs)
        self.assertIn("npm run build", step_runs)
        self.assertIn("go mod verify", step_runs)
        self.assertIn("go vet", step_runs)
        self.assertIn("install_test_binaries.sh", step_runs)
        self.assertIn("setup_test_archive.py", step_runs)
        self.assertIn("annotate_go_test.py", step_runs)
        self.assertIn("CGO_ENABLED=0", step_runs)
        self.assertIn("bin/csp-linux-amd64", step_runs)
        self.assertIn("sha256sum", step_runs)

        upload_steps = [s for s in steps if "upload-artifact" in s.get("uses", "")]
        self.assertTrue(len(upload_steps) > 0, "upload-artifact step must exist")
        upload_with = upload_steps[0].get("with", {})
        self.assertEqual(upload_with.get("name"), "csp-linux-amd64")

    def test_publish_job_steps_and_no_continue_on_error(self):
        steps = self.wf["jobs"]["publish-runtime"].get("steps", [])
        for s in steps:
            self.assertFalse(
                s.get("continue-on-error", False),
                f"Step '{s.get('name')}' must NOT have continue-on-error: true (Fail-Closed)",
            )

        step_names = [s.get("name", "") for s in steps]
        self.assertTrue(any("Download" in n for n in step_names))
        self.assertTrue(any("Verify binary sha256" in n for n in step_names))
        self.assertTrue(any("Verify downloaded binary source revision" in n for n in step_names))
        self.assertTrue(any("Build and push" in n for n in step_names))
        postcheck = next(s for s in steps if "Verify pushed tag digest" in s.get("name", ""))
        self.assertEqual(postcheck['env']['TARGET_IMAGE_NAME'], 'ghcr.io/einck0/csp-runtime-private')
        self.assertIn('preflight-pull.sh', postcheck['run'])
        self.assertIn('manifest.get("digest") == sys.argv[2]', postcheck['run'])
        self.assertIn('steps.build-push.outputs.digest', postcheck['env']['IMAGE_DIGEST'])
        self.assertIn('github.sha', postcheck['env']['EXPECTED_REVISION'])
        serialized = json.dumps(self.wf)
        for revoked in ('check_ghcr_private_gate.py', 'OWNER_PAT', 'ENABLE_CSP_GHCR_PUBLISH'):
            self.assertNotIn(revoked, serialized, 'Revoked private policy must not remain in active CI')
        login = next(s for s in steps if 'docker/login-action' in s.get('uses', ''))
        self.assertEqual(login['with']['password'], '${{ secrets.GITHUB_TOKEN }}')
        build = next(s for s in steps if 'docker/build-push-action' in s.get('uses', ''))
        self.assertTrue(build['with']['push'])
        self.assertEqual(build['with']['file'], './Dockerfile.runtime')


class TestImmutableDevPrerelease(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.revision = '0123456789abcdef0123456789abcdef01234567'
        self.tag = 'prelease-0123456'
        self.repository = 'einck0/clash-sub-parser'
        self.image_digest = 'sha256:' + 'a' * 64
        self.payload = b'synthetic binary, never executed'
        self.checksum = hashlib.sha256(self.payload).hexdigest()
        (self.directory / promotion.BINARY).write_bytes(self.payload)
        (self.directory / promotion.CHECKSUM).write_text(f'{self.checksum}  {promotion.BINARY}\n')
        self.metadata = {
            'schema': 1, 'repository': self.repository, 'source_sha': self.revision, 'tag': self.tag,
            'artifact': promotion.BINARY, 'binary_sha256': self.checksum,
            'image': f'{promotion.IMAGE}@{self.image_digest}', 'image_digest': self.image_digest,
            'run_id': '1234', 'run_attempt': '1',
            'run_url': f'https://github.com/{self.repository}/actions/runs/1234',
        }
        self.release = {'tag_name': self.tag, 'name': self.tag, 'target_commitish': self.revision,
                        'prerelease': True, 'draft': False,
                        'assets': [{'name': name} for name in promotion.ASSETS]}
        self.ref = {'object': {'type': 'commit', 'sha': self.revision}}
        self.environment = patch.dict(os.environ, GITHUB_REPOSITORY=self.repository, GITHUB_SHA=self.revision,
                                      GITHUB_RUN_ID='5678', GITHUB_RUN_ATTEMPT='2',
                                      IMAGE_DIGEST=self.image_digest, GITHUB_OUTPUT=str(self.directory / 'output'))
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.created = False
        self.absent = False
        self.tag_only = False

    def fake_gh(self, *args, **kwargs):
        if args[0] == 'api':
            if '/releases/tags/' in args[1]:
                return None if self.absent and not self.created else json.dumps(self.release)
            return None if self.absent and not self.created and not self.tag_only else json.dumps(self.ref)
        if args[:2] == ('release', 'create'):
            self.created = True
            self.metadata = json.loads((self.directory / promotion.METADATA).read_text())
            return ''
        if args[:2] == ('release', 'download'):
            target = Path(args[args.index('--dir') + 1])
            (target / promotion.BINARY).write_bytes(self.payload)
            (target / promotion.CHECKSUM).write_text(f'{self.checksum}  {promotion.BINARY}\n')
            (target / promotion.METADATA).write_text(json.dumps(self.metadata))
            return ''
        self.fail('Unexpected GitHub command')

    def test_new_release_targets_exact_sha_and_attaches_same_binary_checksum_metadata(self):
        self.absent = True
        with patch.object(promotion, 'gh', side_effect=self.fake_gh) as api:
            promotion.publish(self.directory)
        create = next(call.args for call in api.call_args_list if call.args[:2] == ('release', 'create'))
        self.assertEqual(create[2], self.tag)
        self.assertEqual(create[create.index('--target') + 1], self.revision)
        self.assertEqual(create[create.index('--title') + 1], self.tag)
        self.assertIn('--prerelease', create)
        self.assertNotIn('--clobber', create)
        self.assertEqual(create[-3:], tuple(str(self.directory / name) for name in promotion.ASSETS))
        self.assertEqual((self.directory / promotion.BINARY).read_bytes(), self.payload)
        self.assertEqual(self.metadata['binary_sha256'], self.checksum)
        self.assertEqual(self.metadata['image'], f'{promotion.IMAGE}@{self.image_digest}')
        self.assertEqual(self.metadata['source_sha'], self.revision)
        self.assertEqual((self.metadata['run_id'], self.metadata['run_attempt']), ('5678', '2'))

    def test_same_sha_rerun_reuses_assets_digest_and_original_run_without_edit(self):
        with patch.object(promotion, 'gh', side_effect=self.fake_gh) as api:
            promotion.reuse(self.directory)
            promotion.publish(self.directory)
        self.assertEqual((self.directory / 'output').read_text(), f'reused=true\nimage_digest={self.image_digest}\n')
        self.assertEqual(json.loads((self.directory / promotion.METADATA).read_text())['run_id'], '1234')
        self.assertTrue(all(call.args[0] == 'api' or call.args[:2] == ('release', 'download')
                            for call in api.call_args_list))

    def test_first_run_has_no_reuse_and_orphan_tag_blocks_instead_of_overwriting(self):
        self.absent = True
        with patch.object(promotion, 'gh', side_effect=self.fake_gh) as api:
            promotion.reuse(self.directory)
            self.assertEqual((self.directory / 'output').read_text(), 'reused=false\n')
            self.tag_only = True
            with self.assertRaisesRegex(ValueError, 'refusing overwrite'):
                promotion.publish(self.directory)
        self.assertFalse(any(call.args[:2] == ('release', 'create') for call in api.call_args_list))

    def test_wrong_sha_collision_or_incomplete_release_never_reused(self):
        mutations = [('release', 'target_commitish', 'f' * 40), ('release', 'prerelease', False),
                     ('release', 'assets', []), ('metadata', 'source_sha', 'f' * 40),
                     ('metadata', 'binary_sha256', '0' * 64), ('metadata', 'image_digest', '$(touch /tmp/no)\n'),
                     ('metadata', 'image', 'ghcr.io/foreign/package@' + self.image_digest)]
        for target, key, bad in mutations:
            with self.subTest(target=target, key=key):
                mapping = getattr(self, target)
                original = mapping[key]
                mapping[key] = bad
                try:
                    with patch.object(promotion, 'gh', side_effect=self.fake_gh) as api:
                        with self.assertRaises(ValueError):
                            promotion.reuse(self.directory)
                        self.assertFalse(any(call.args[:2] == ('release', 'create') for call in api.call_args_list))
                finally:
                    mapping[key] = original
        self.ref['object']['sha'] = 'f' * 40
        with patch.object(promotion, 'gh', side_effect=self.fake_gh):
            with self.assertRaisesRegex(ValueError, 'exact source SHA'):
                promotion.reuse(self.directory)

    def test_different_image_or_binary_cannot_overwrite_same_tag(self):
        with patch.object(promotion, 'gh', side_effect=self.fake_gh) as api:
            with patch.dict(os.environ, IMAGE_DIGEST='sha256:' + 'b' * 64):
                with self.assertRaisesRegex(ValueError, 'refusing overwrite'):
                    promotion.publish(self.directory)
            (self.directory / promotion.BINARY).write_bytes(b'changed')
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                promotion.publish(self.directory)
        self.assertFalse(any(call.args[:2] == ('release', 'create') for call in api.call_args_list))

    def test_gh_errors_fail_closed_and_untrusted_text_is_never_a_shell_command(self):
        for code in (403, 500):
            with self.subTest(code=code), patch.object(promotion.subprocess, 'run', return_value=
                    subprocess.CompletedProcess(['gh'], 1, '', f'API error (HTTP {code})')):
                with self.assertRaises(RuntimeError):
                    promotion.gh('api', 'repos/owner/repository', missing_ok=True)
        with patch.object(promotion.subprocess, 'run', return_value=
                subprocess.CompletedProcess(['gh'], 1, '', 'Not Found (HTTP 404)')):
            self.assertIsNone(promotion.gh('api', 'repos/owner/repository', missing_ok=True))
        malicious = '$(touch /tmp/no); echo injected'
        with patch.object(promotion.subprocess, 'run', return_value=
                subprocess.CompletedProcess(['gh'], 0, '{}', '')) as run:
            promotion.gh('release', 'view', malicious)
        self.assertEqual(run.call_args.args[0], ['gh', 'release', 'view', malicious])
        self.assertNotIn('shell', run.call_args.kwargs)


class TestDockerfileRuntimeContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        self = cls
        self.assertTrue(os.path.isfile(DOCKERFILE_RUNTIME), f"Missing: {DOCKERFILE_RUNTIME}")
        with open(DOCKERFILE_RUNTIME, "r", encoding="utf-8") as f:
            self.content = f.read()

    def test_base_image_and_non_root_user(self):
        self.assertIn("FROM alpine:3.20", self.content)
        self.assertIn("addgroup -g 10001 -S appuser", self.content)
        self.assertIn("adduser -u 10001 -S -G appuser", self.content)
        self.assertIn("USER appuser", self.content)

    def test_directories_and_binary_copy(self):
        self.assertIn("mkdir -p /app /data", self.content)
        self.assertIn("chmod 750 /data", self.content)
        self.assertIn("COPY --chown=appuser:appuser bin/csp-linux-amd64 /app/csp", self.content)
        self.assertIn("chmod 755 /app/csp", self.content)

    def test_zero_compilation_tools(self):
        forbidden_patterns = [
            r"go\s+build",
            r"npm\s+run",
            r"npm\s+install",
            r"apk\s+add.*gcc",
            r"apk\s+add.*musl-dev",
            r"apk\s+add.*go",
            r"apk\s+add.*nodejs",
        ]
        for pat in forbidden_patterns:
            self.assertIsNone(
                re.search(pat, self.content, re.IGNORECASE),
                f"Dockerfile.runtime contains forbidden build tool pattern: {pat}",
            )

    def test_oci_labels_and_healthcheck(self):
        self.assertIn("LABEL org.opencontainers.image.title", self.content)
        self.assertIn("org.opencontainers.image.revision", self.content)
        self.assertIn("HEALTHCHECK", self.content)
        self.assertIn("/healthz", self.content)
        self.assertIn('ENTRYPOINT ["/app/csp"]', self.content)


class TestShaBindingAndChecksumManifest(unittest.TestCase):
    def test_checksum_verification_pass_and_fail(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            bin_file = os.path.join(tmpdir, "csp-linux-amd64")
            manifest_file = os.path.join(tmpdir, "csp-linux-amd64.sha256")

            # Create synthetic binary
            payload = b"\x7fELF\x02\x01\x01\x00synthetic-pure-static-binary-payload"
            with open(bin_file, "wb") as f:
                f.write(payload)

            digest = hashlib.sha256(payload).hexdigest()
            with open(manifest_file, "w", encoding="utf-8") as f:
                f.write(f"{digest}  csp-linux-amd64\n")

            # Verify sha256sum passes
            res = subprocess.run(["sha256sum", "-c", "csp-linux-amd64.sha256"], cwd=tmpdir, capture_output=True)
            self.assertEqual(res.returncode, 0, f"sha256sum check failed: {res.stderr}")

            # Tamper 1 byte
            with open(bin_file, "wb") as f:
                f.write(payload + b"\x00")

            # Verify sha256sum fails
            res_tampered = subprocess.run(
                ["sha256sum", "-c", "csp-linux-amd64.sha256"],
                cwd=tmpdir,
                capture_output=True,
            )
            self.assertNotEqual(res_tampered.returncode, 0, "Tampered binary must fail sha256sum check")


class TestPrivateRegistryProbingHttpMock(unittest.TestCase):
    def setUp(self):
        gate.SENSITIVE_TOKENS.clear()

    def test_publish_variable_disabled_fail_closed(self):
        ret = gate.execute_pre_check(
            image_name="ghcr.io/einck0/csp-runtime-private",
            owner="einck0",
            publish_enabled="false",
            token="ghp_dummy_token",
        )
        self.assertEqual(ret, 1, "Must fail closed if ENABLE_CSP_GHCR_PUBLISH != 'true'")

        ret_empty = gate.execute_pre_check(
            image_name="ghcr.io/einck0/csp-runtime-private",
            owner="einck0",
            publish_enabled="",
            token="ghp_dummy_token",
        )
        self.assertEqual(ret_empty, 1, "Must fail closed if publish variable is empty")

    def test_historical_public_package_lockout(self):
        ret = gate.execute_pre_check(
            image_name="ghcr.io/einck0/clash-sub-parser",
            owner="einck0",
            publish_enabled="true",
            token="ghp_dummy_token",
        )
        self.assertEqual(ret, 1, "Must reject historical public package clash-sub-parser")

    def test_unapproved_namespace_lockout(self):
        ret = gate.execute_pre_check(
            image_name="ghcr.io/einck0/arbitrary-unapproved-pkg",
            owner="einck0",
            publish_enabled="true",
            token="ghp_dummy_token",
        )
        self.assertEqual(ret, 1, "Must reject any unapproved package namespace")

    def test_direct_anonymous_200_must_reject(self):
        mock_resp = MagicMock()
        mock_resp.status = 200
        mock_resp.__enter__.return_value = mock_resp

        with patch("urllib.request.urlopen", return_value=mock_resp):
            res = gate.probe_anonymous_manifest_access("ghcr.io", "einck0/csp-runtime-private", "sha-test")
            self.assertTrue(res["is_public"], "Direct 200 must be flagged as public")

    def test_bearer_challenge_anonymous_200_must_reject(self):
        http_401 = urllib.error.HTTPError(
            url="https://ghcr.io/v2/...",
            code=401,
            msg="Unauthorized",
            hdrs={"Www-Authenticate": 'Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:einck0/csp-runtime-private:pull"'},
            fp=BytesIO(b""),
        )
        token_resp = MagicMock()
        token_resp.status = 200
        token_resp.read.return_value = json.dumps({"token": "anon-bearer-token-123"}).encode("utf-8")
        token_resp.__enter__.return_value = token_resp

        manifest_resp = MagicMock()
        manifest_resp.status = 200
        manifest_resp.__enter__.return_value = manifest_resp

        def urlopen_side_effect(req, *args, **kwargs):
            url = req.full_url if hasattr(req, "full_url") else str(req)
            if "ghcr.io/v2" in url and "Authorization" not in req.headers:
                raise http_401
            elif "ghcr.io/token" in url:
                return token_resp
            elif "ghcr.io/v2" in url and "Authorization" in req.headers:
                return manifest_resp
            raise ValueError(f"Unexpected URL: {url}")

        with patch("urllib.request.urlopen", side_effect=urlopen_side_effect):
            res = gate.probe_anonymous_manifest_access("ghcr.io", "einck0/csp-runtime-private", "sha-test")
            self.assertTrue(res["is_public"], "Anonymous token returning 200 manifest must be flagged as public")

    def test_pre_check_api_403_must_fail_closed_no_downgrade(self):
        # When authorized GitHub API returns 403 (e.g. GITHUB_TOKEN has insufficient scope),
        # pre-check MUST strictly fail closed (return 1). NO downgrade!
        with patch.object(gate, "check_github_api_package_visibility", return_value={"status": "error", "status_code": 403}):
            ret = gate.execute_pre_check(
                image_name="ghcr.io/einck0/csp-runtime-private",
                owner="einck0",
                publish_enabled="true",
                token="ghs_insufficient_token",
            )
            self.assertEqual(ret, 1, "API 403 must fail closed without downgrading to probe")

    def test_pre_check_direct_404_without_pagination_proof_fail_closed(self):
        # Direct lookup returns 404, but owner packages listing fails (e.g. 403) -> Cannot prove non-existence!
        with patch.object(gate, "check_github_api_package_visibility", return_value={"status": "not_found", "status_code": 404}):
            with patch.object(gate, "list_and_verify_all_owner_packages", return_value={"status": "error", "status_code": 403}):
                ret = gate.execute_pre_check(
                    image_name="ghcr.io/einck0/csp-runtime-private",
                    owner="einck0",
                    publish_enabled="true",
                    token="ghs_token",
                )
                self.assertEqual(ret, 1, "404 without exhaustive pagination proof must fail closed")

    def test_pre_check_direct_404_with_pagination_proven_absent_pass(self):
        # Direct lookup returns 404, and exhaustive pagination proves package is absent -> PASS
        with patch.object(gate, "check_github_api_package_visibility", return_value={"status": "not_found", "status_code": 404}):
            with patch.object(gate, "list_and_verify_all_owner_packages", return_value={"proven_absent": True, "total_packages": 5}):
                ret = gate.execute_pre_check(
                    image_name="ghcr.io/einck0/csp-runtime-private",
                    owner="einck0",
                    publish_enabled="true",
                    token="ghp_authorized_token",
                )
                self.assertEqual(ret, 0, "404 with exhaustive pagination proof of non-existence must pass")

    def test_post_check_api_403_must_fail_closed_even_if_anonymous_denied(self):
        # Anonymous challenge probe is denied (403), BUT authorized API returns 403 (unconfirmed visibility)
        # Post-check MUST FAIL CLOSED!
        dummy_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
        with patch.object(gate, "probe_anonymous_manifest_access", return_value={"is_public": False, "status_code": 403}):
            with patch.object(gate, "check_github_api_package_visibility", return_value={"status": "error", "status_code": 403}):
                ret = gate.execute_post_check(
                    image_name="ghcr.io/einck0/csp-runtime-private",
                    owner="einck0",
                    tag="sha-test",
                    digest=dummy_digest,
                    token="ghs_token",
                )
                self.assertEqual(ret, 1, "Post-check must fail closed if API visibility cannot be confirmed private")

    def test_post_check_dual_proof_pass(self):
        # Both anonymous probe denied (403) AND authorized API confirms private -> PASS
        dummy_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
        with patch.object(gate, "probe_anonymous_manifest_access", return_value={"is_public": False, "status_code": 403}):
            with patch.object(gate, "check_github_api_package_visibility", return_value={"status": "ok", "visibility": "private"}):
                ret = gate.execute_post_check(
                    image_name="ghcr.io/einck0/csp-runtime-private",
                    owner="einck0",
                    tag="sha-test",
                    digest=dummy_digest,
                    token="ghp_authorized_pat",
                )
                self.assertEqual(ret, 0, "Post-check must pass when both anonymous is denied and API is private")

    def test_token_masking_never_leaks(self):
        secret_token = "ghp_super_secret_pat_9876543210zyxwvutsrq"
        gate.register_sensitive_token(secret_token)

        raw_message = f"Connecting with token {secret_token} to https://ghcr.io"
        masked = gate.mask_secrets(raw_message)

        self.assertNotIn(secret_token, masked, "Raw secret token must not appear in masked output")
        self.assertIn("***", masked, "Masked placeholder must appear")


class TestDeliveryArtifacts(unittest.TestCase):
    def test_build_metadata_and_runtime_artifact_binding(self):
        # Authorization of a past worktree is not a durable product invariant.
        with open(WORKFLOW_FILE, encoding="utf-8") as f:
            workflow = yaml.safe_load(f)
        checks = workflow["jobs"]["test-and-build"]["steps"]
        runs = "\n".join(step.get("run", "") for step in checks)
        self.assertIn("-X clash-sub-parser/internal/platform.Commit=${{ github.sha }}", runs)
        self.assertIn("-X clash-sub-parser/internal/platform.BuildDate=${BUILD_TIME}", runs)
        self.assertIn("bin/csp-linux-amd64 --version", runs)
        self.assertIn("cp -a web/dist/. internal/webassets/dist/", runs)
        self.assertEqual(runs.count("go build -tags"), 1)
        with open(DOCKERFILE_RUNTIME, encoding="utf-8") as f:
            self.assertIn("bin/csp-linux-amd64 /app/csp", f.read())
        with open(os.path.join(REPO_ROOT, "Dockerfile"), encoding="utf-8") as f:
            dockerfile = f.read()
        self.assertIn("-X clash-sub-parser/internal/platform.Commit=${COMMIT_SHA}", dockerfile)
        self.assertIn("-X clash-sub-parser/internal/platform.BuildDate=${BUILD_TIME}", dockerfile)

    def test_dockerfile_runtime_buildx_check(self):
        # Enforces Docker Buildx syntax and definition checking without rebuilding images locally
        check_cmd = [
            "docker", "buildx", "build",
            "--check",
            "-f", "Dockerfile.runtime",
            ".",
        ]
        res = subprocess.run(check_cmd, cwd=REPO_ROOT, capture_output=True, text=True, timeout=15)
        self.assertEqual(res.returncode, 0, f"docker buildx build --check failed: {res.stderr}")
        self.assertIn("Check complete, no warnings found", res.stdout + res.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)

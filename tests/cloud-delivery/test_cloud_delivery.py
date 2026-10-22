#!/usr/bin/env python3
"""
tests/cloud-delivery/test_cloud_delivery.py
CSP 日常云交付合成验证测试套件 (Zero External Dependencies, 标准 unittest 库)
验证 Compose 模板规范、运行前置脚本、隔离环境约束与运维文档。
"""

import ast
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
SYNTHETIC_IMAGE = (
    "ghcr.io/einck0/clash-sub-parser@"
    "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)
SYNTHETIC_REVISION = "0123456789abcdef0123456789abcdef01234567"


def safe_eval_bool(expr: str, env: dict) -> bool:
    """Bounded AST boolean evaluator for workflow conditions."""
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


class TestComposeConfigurations(unittest.TestCase):
    """验证 Compose 配置解耦与无构建保障"""

    def test_production_compose_requires_image(self):
        """生产 compose (docker-compose.prod.yml) 必须强制要求 CSP_IMAGE 环境变量，未提供时立即失败"""
        env = {k: v for k, v in os.environ.items() if not k.startswith("CSP_")}
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", "docker-compose.prod.yml", "config"],
            cwd=str(REPO_ROOT),
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("CSP_IMAGE", result.stderr)
        self.assertIn("required", result.stderr)

    def test_production_compose_rendering_and_invariants(self):
        """生产 compose (docker-compose.prod.yml) 提供合法镜像后成功渲染，且无 build 块，保留既有端口/数据卷与 pull_policy: always"""
        env = {k: v for k, v in os.environ.items() if not k.startswith("CSP_")}
        env["CSP_IMAGE"] = SYNTHETIC_IMAGE
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", "docker-compose.prod.yml", "config", "--format", "json"],
            cwd=str(REPO_ROOT),
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, f"Config render failed: {result.stderr}")

        cfg = json.loads(result.stdout)
        services = cfg.get("services", {})
        self.assertIn("app", services, "生产 compose 必须包含 app 服务")
        app = services["app"]

        # 核心防现场编译铁律：严禁包含 build 块
        self.assertNotIn("build", app, "生产 compose 绝不能包含 build 语法块！")
        self.assertEqual(app.get("pull_policy"), "always", "生产 compose 必须显式声明 pull_policy: always")
        self.assertEqual(app.get("image"), SYNTHETIC_IMAGE)

        # 容器名保持 clash-sub-parser
        self.assertEqual(app.get("container_name"), "clash-sub-parser")

        # 校验端口映射：必须包含 17000:18080 与 18080:18080 两个 loopback 绑定
        ports = app.get("ports", [])
        published_ports = {str(p.get("published")): p.get("host_ip") for p in ports}
        self.assertIn("17000", published_ports)
        self.assertIn("18080", published_ports)
        self.assertEqual(published_ports["17000"], "127.0.0.1")
        self.assertEqual(published_ports["18080"], "127.0.0.1")

        # 校验数据卷绑定：必须挂载 csp-v1-data -> /data
        volumes = app.get("volumes", [])
        volume_sources = [v.get("source") for v in volumes]
        self.assertIn("csp-v1-data", volume_sources)

        # 校验健康检查
        hc = app.get("healthcheck", {})
        self.assertIn("test", hc)
        self.assertTrue(any("/healthz" in str(arg) for arg in hc["test"]))

    def test_example_compose_rendering_and_invariants(self):
        """示例 compose (docker-compose.example.yml) 渲染正常，无 build 块，显式 pull_policy: always 且默认使用 prelease 镜像"""
        env = {k: v for k, v in os.environ.items() if not k.startswith("CSP_")}
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", "docker-compose.example.yml", "config", "--format", "json"],
            cwd=str(REPO_ROOT),
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, f"Example config render failed: {result.stderr}")

        cfg = json.loads(result.stdout)
        services = cfg.get("services", {})
        self.assertIn("app", services, "示例 compose 必须包含 app 服务")
        app = services["app"]

        self.assertNotIn("build", app, "示例 compose 绝不能包含 build 语法块！")
        self.assertEqual(app.get("pull_policy"), "always", "示例 compose 必须显式声明 pull_policy: always")
        self.assertEqual(app.get("image"), "ghcr.io/einck0/clash-sub-parser:prelease")
        self.assertEqual(app.get("container_name"), "clash-sub-parser")

    def test_dev_compose_has_explicit_build(self):
        """本地开发 compose 必须显式定义 build 块与现场编译上下文"""
        env = {k: v for k, v in os.environ.items() if not k.startswith("CSP_")}
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", "docker-compose.dev.yml", "config", "--format", "json"],
            cwd=str(REPO_ROOT),
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, f"Dev config render failed: {result.stderr}")

        cfg = json.loads(result.stdout)
        app = cfg["services"]["app"]
        self.assertIn("build", app, "开发 compose 必须包含显式 build 语法块")
        self.assertEqual(app["build"].get("dockerfile"), "Dockerfile")
        self.assertEqual(app.get("container_name"), "clash-sub-parser-dev")

        # 开发卷必须与生产卷隔离
        dev_volumes = [v.get("source") for v in app.get("volumes", [])]
        self.assertIn("csp-v1-dev-data", dev_volumes)
        self.assertNotIn("csp-v1-data", dev_volumes)

    def test_isolated_compose_invariants_and_labels(self):
        """隔离运行 compose 必须属于独立 project，使用 18081/17001 端口与独立测试卷，并具有所有权标签"""
        env = {k: v for k, v in os.environ.items() if not k.startswith("CSP_")}
        env["CSP_IMAGE"] = SYNTHETIC_IMAGE
        env["CSP_ISOLATED_PROJECT"] = "csp-isolated-test"
        env["CSP_ISOLATED_RUN_MARKER"] = "run-marker-1234"
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", "docker-compose.isolated.yml", "config", "--format", "json"],
            cwd=str(REPO_ROOT),
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, f"Isolated config render failed: {result.stderr}")

        cfg = json.loads(result.stdout)
        self.assertEqual(cfg.get("name"), "csp-isolated-test", "项目名称必须支持独立 project")

        app = cfg["services"]["app"]
        self.assertNotIn("build", app, "隔离预览 compose 同样严禁现场 build")
        self.assertEqual(app.get("container_name"), "csp-isolated-preview")
        self.assertEqual(app.get("restart"), "no")
        self.assertEqual(app.get("command"), [
            "serve", "--disable-subscription-scheduler", "--disable-probe-coordinator",
        ])
        self.assertTrue(cfg["networks"]["isolated-network"].get("internal"))

        # 校验所有权 labels
        labels = app.get("labels", {})
        self.assertEqual(labels.get("com.csp.isolated.owner"), "cloud-delivery")
        self.assertEqual(labels.get("com.csp.isolated.run-marker"), "run-marker-1234")

        # 端口检查：必须默认监听 18081 与 17001
        ports = app.get("ports", [])
        published_ports = {str(p.get("published")): p.get("host_ip") for p in ports}
        self.assertIn("18081", published_ports)
        self.assertIn("17001", published_ports)

        # 数据卷独立：必须使用全新数据卷，绝不引用生产路径
        volumes = app.get("volumes", [])
        volume_sources = [v.get("source") for v in volumes]
        self.assertIn("csp-isolated-data", volume_sources)
        self.assertNotIn("csp-v1-data", volume_sources)
        for v in volumes:
            source = str(v.get("source"))
            self.assertNotIn("/home/service/clash-sub-parser", source)
            self.assertNotIn("csp-v1-data", source)


class TestShellScriptsSyntaxAndLint(unittest.TestCase):
    """验证 Shell 脚本语法与静态检查"""

    def test_bash_syntax_all_scripts(self):
        scripts = [
            "scripts/cloud-delivery/preflight-pull.sh",
            "scripts/cloud-delivery/run-isolated.sh",
            "scripts/cloud-delivery/cleanup-isolated.sh",
        ]
        for script_relpath in scripts:
            with self.subTest(script=script_relpath):
                script_path = REPO_ROOT / script_relpath
                self.assertTrue(script_path.exists(), f"脚本文件不存在: {script_path}")
                self.assertTrue(os.access(script_path, os.X_OK), f"脚本缺少可执行权限: {script_path}")

                result = subprocess.run(["bash", "-n", str(script_path)], capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, f"Bash 语法检查报错 ({script_relpath}): {result.stderr}")


class TestPreflightPullLogic(unittest.TestCase):
    """验证镜像拉取预检与安全边界门禁逻辑"""

    def test_missing_image_arg(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        env = os.environ.copy()
        env.pop("CSP_IMAGE", None)
        result = subprocess.run([script], env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("必须提供镜像引用", result.stderr)

    def test_missing_revision_arg(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        env = os.environ.copy()
        env.pop("EXPECTED_REVISION", None)
        result = subprocess.run(
            [script, "--image", SYNTHETIC_IMAGE],
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("Revision SHA 必选不得省略", result.stderr)

    def test_reject_invalid_revision_hex(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        result = subprocess.run(
            [script, "--image", SYNTHETIC_IMAGE, "--expected-revision", "not-a-valid-hex-sha"],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("revision SHA 格式不合规", result.stderr)

    def test_reject_tag_without_digest(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        result = subprocess.run(
            [
                script,
                "--image",
                "ghcr.io/einck0/clash-sub-parser:latest",
                "--expected-revision",
                SYNTHETIC_REVISION,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("不合规的镜像引用格式", result.stderr)

    def test_reject_short_or_non_hex_digest(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        # 短哈希
        res1 = subprocess.run(
            [
                script,
                "--image",
                "ghcr.io/einck0/clash-sub-parser@sha256:1234abcd",
                "--expected-revision",
                SYNTHETIC_REVISION,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res1.returncode, 1)

        # 非 16 进制字符 (含 'z')
        res2 = subprocess.run(
            [
                script,
                "--image",
                "ghcr.io/einck0/clash-sub-parser@sha256:" + "z" * 64,
                "--expected-revision",
                SYNTHETIC_REVISION,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res2.returncode, 1)

    def test_reject_legacy_package(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        result = subprocess.run(
            [
                script,
                "--image",
                "ghcr.io/einck0/csp-runtime-private@sha256:" + "a" * 64,
                "--expected-revision",
                SYNTHETIC_REVISION,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("镜像包名不匹配", result.stderr)

    def test_reject_package_name_mismatch(self):
        script = str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh")
        result = subprocess.run(
            [
                script,
                "--image",
                "ghcr.io/einck0/unauthorized-repo@sha256:" + "a" * 64,
                "--expected-revision",
                SYNTHETIC_REVISION,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("镜像包名不匹配", result.stderr)

    def run_docker_fixture(self, image=None, *, check_only=False, pull_exit=0, inspect_exit=0, extra_args=()):
        """Only Docker is replaced: execute the real shell and JSON validation."""
        with tempfile.TemporaryDirectory(prefix="csp-preflight-") as tmp:
            directory = Path(tmp)
            fixture = directory / "inspect.json"
            fixture.write_text(json.dumps([image or self.valid_image()]))
            calls = directory / "calls.jsonl"
            docker = directory / "docker"
            docker.write_text(
                "#!/usr/bin/env python3\n"
                "import json, os, pathlib, sys\n"
                "with open(os.environ['CALLS'], 'a') as log: log.write(json.dumps(sys.argv[1:]) + '\\n')\n"
                "if sys.argv[1] == 'pull': sys.exit(int(os.environ['PULL_EXIT']))\n"
                "if sys.argv[1:3] == ['image', 'inspect']:\n"
                "    print(pathlib.Path(os.environ['FIXTURE']).read_text())\n"
                "    sys.exit(int(os.environ['INSPECT_EXIT']))\n"
                "sys.exit('unexpected Docker operation')\n"
            )
            docker.chmod(0o700)
            env = {k: v for k, v in os.environ.items() if not k.startswith(('CSP_', 'EXPECTED_')) and k not in ('OWNER_PAT', 'GHCR_TOKEN')}
            env.update(PATH=tmp + os.pathsep + env['PATH'], FIXTURE=str(fixture), CALLS=str(calls), PULL_EXIT=str(pull_exit), INSPECT_EXIT=str(inspect_exit))
            result = subprocess.run(
                [str(REPO_ROOT / "scripts/cloud-delivery/preflight-pull.sh"), "--image", SYNTHETIC_IMAGE,
                 "--expected-revision", SYNTHETIC_REVISION, *(["--check-only"] if check_only else []), *extra_args],
                env=env, capture_output=True, text=True, timeout=10,
            )
            return result, [json.loads(line) for line in calls.read_text().splitlines()] if calls.exists() else []

    @staticmethod
    def valid_image():
        return {"Os": "linux", "Architecture": "amd64", "RepoDigests": [SYNTHETIC_IMAGE],
                "Config": {"Labels": {"org.opencontainers.image.revision": SYNTHETIC_REVISION}}}

    def test_public_pull_without_owner_pat_verifies_identity(self):
        result, calls = self.run_docker_fixture()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["pull", "--platform", "linux/amd64", SYNTHETIC_IMAGE], ["image", "inspect", SYNTHETIC_IMAGE]])
        self.assertIn("exact RepoDigests and OCI revision verified", result.stdout)

    def test_check_only_validates_cached_image_without_pull(self):
        result, calls = self.run_docker_fixture(check_only=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["image", "inspect", SYNTHETIC_IMAGE]])

    def test_reject_wrong_identity_even_if_digest_appears_elsewhere(self):
        for mutation in ('digest', 'owner', 'revision', 'missing_revision', 'platform'):
            with self.subTest(mutation=mutation):
                image = self.valid_image()
                if mutation == 'digest':
                    image['RepoDigests'] = [SYNTHETIC_IMAGE[:-64] + 'b' * 64]
                    image['Config']['Labels']['untrusted-digest'] = SYNTHETIC_IMAGE
                elif mutation == 'owner':
                    image['RepoDigests'] = [SYNTHETIC_IMAGE.replace('/einck0/', '/attacker/')]
                elif mutation == 'revision':
                    image['Config']['Labels']['org.opencontainers.image.revision'] = 'f' * 40
                elif mutation == 'missing_revision':
                    image['Config']['Labels'] = {'revision': SYNTHETIC_REVISION}
                else:
                    image['Architecture'] = 'arm64'
                result, _ = self.run_docker_fixture(image)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn('image identity verification failed', result.stderr)

    def test_pull_or_inspect_failure_is_not_success(self):
        for exits in ({'pull_exit': 1}, {'inspect_exit': 1}):
            with self.subTest(exits=exits):
                result, _ = self.run_docker_fixture(**exits)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('镜像确权验证通过', result.stdout)

    def test_foreign_owner_cannot_override_expected_package(self):
        result, calls = self.run_docker_fixture(extra_args=("--expected-package", "ghcr.io/attacker/clash-sub-parser"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('owner/包名不合规', result.stderr)
        self.assertEqual(calls, [])

    def test_dry_run_never_claims_image_verification(self):
        result, calls = self.run_docker_fixture(extra_args=("--dry-run",))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('未验证镜像', result.stdout)
        self.assertEqual(calls, [])


class TestRunIsolatedScriptLogic(unittest.TestCase):
    """验证隔离运行脚本边界与前置检查"""

    def test_check_only_uncached_image(self):
        """待测镜像不在本地时必须干净阻断，不自动 pull 或要求 OWNER_PAT。"""
        script = str(REPO_ROOT / "scripts/cloud-delivery/run-isolated.sh")
        result = subprocess.run(
            [script, "--image", SYNTHETIC_IMAGE, "--check-only"],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("STATUS: BLOCKED", result.stdout)
        self.assertIn("不在本地 Docker 缓存中", result.stdout)


class TestCleanupIsolatedScriptGuards(unittest.TestCase):
    """验证安全清理脚本的所有权与防误删门禁"""

    def test_cleanup_refuses_when_project_or_marker_missing(self):
        """缺失明确 project 或 run-marker 时，清理脚本必须拒绝执行 down -v 并退出 code 1"""
        script = str(REPO_ROOT / "scripts/cloud-delivery/cleanup-isolated.sh")
        env = os.environ.copy()
        env.pop("CSP_ISOLATED_PROJECT", None)
        env.pop("CSP_ISOLATED_RUN_MARKER", None)
        with tempfile.TemporaryDirectory() as preview_dir:
            env["CSP_PREVIEW_DIR"] = preview_dir
            result = subprocess.run(
                [script], env=env, capture_output=True, text=True,
            )
        self.assertEqual(result.returncode, 1)
        self.assertIn("缺少明确的 project", result.stderr)
        self.assertIn("run-marker", result.stderr)

    def test_cleanup_script_dry_run_with_marker(self):
        """提供合法 project 与 run-marker 时，dry-run 成功执行并验证生产 ID 守护"""
        script = str(REPO_ROOT / "scripts/cloud-delivery/cleanup-isolated.sh")
        result = subprocess.run(
            [script, "--project", "csp-isolated-test", "--run-marker", "marker-abc", "--dry-run"],
            cwd=str(REPO_ROOT),
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("生产环境现状断言", result.stdout)
        self.assertIn("模拟清理验证完毕", result.stdout)


class TestDocumentationCompleteness(unittest.TestCase):
    """验证日常运维文档的内容完备性与真实性契约"""

    def test_doc_file_exists_and_content_complete(self):
        doc_path = REPO_ROOT / "docs/operations/csp-cloud-delivery.md"
        self.assertTrue(doc_path.exists(), "文档 docs/operations/csp-cloud-delivery.md 必须存在")
        content = doc_path.read_text(encoding="utf-8")

        # 1. 显式可追踪生产模板与开发独立模式
        self.assertIn("docker-compose.prod.yml", content)
        self.assertIn("docker-compose.dev.yml", content)
        self.assertIn("--no-build", content)

        # 2. 开发云测与发布解耦，prelease 唯一预发布与远程拉取安全
        self.assertIn("绝不创建 GitHub Release", content)
        self.assertIn("唯一专用预发布分支", content)
        self.assertIn("严禁强推代码分支", content)
        self.assertIn("preflight-pull.sh", content)

        # 3. 制品复用与零重复编译
        self.assertIn("csp-linux-amd64", content)
        self.assertIn("Dockerfile.runtime", content)
        self.assertIn("BUILD_DATE", content)

        # 4. 历史命名不代表私有；旧包不擅删
        self.assertIn("ghcr.io/einck0/csp-runtime-private", content)
        self.assertIn("ghcr.io/einck0/clash-sub-parser", content)
        self.assertTrue("严禁擅自删除历史公开包" in content or "停止向该公开包推送" in content)

        # 5. 不可变摘要链条
        self.assertTrue("Same-SHA" in content or "同一 Commit SHA" in content)
        self.assertIn("@sha256:", content)

        # 6. 生产目录确权与只读 stat 事实 (750 / 10001:10001)
        self.assertIn("750", content)
        self.assertIn("10001", content)

        # 7. 资源所有权 run-marker 与防误删
        self.assertIn("run-marker", content)
        self.assertIn("防误删", content)

        # 8. SQLite 在线热备与回滚
        self.assertIn("sqlite3", content)
        self.assertIn(".backup", content)
        self.assertIn("PRAGMA integrity_check;", content)

        # 9. 公开镜像允许，凭据由 Docker 管理，不提取或索取。
        self.assertIn("允许匿名", content)
        self.assertIn("不依赖 OWNER_PAT", content)
        self.assertIn("read:packages", content)
        self.assertIn("0600", content)
        self.assertIn("base64", content)
        self.assertTrue("严禁向用户索取明文 Token" in content or "聊天不索" in content)


class TestBranchGovernanceInvariants(unittest.TestCase):
    """验证长期分支收敛、触发门禁、Dependabot 目标与零编译晋级规则"""

    def setUp(self):
        import yaml
        self.yaml = yaml
        self.ci_path = REPO_ROOT / ".github/workflows/ci.yml"
        self.dependabot_path = REPO_ROOT / ".github/dependabot.yml"
        self.doc_path = REPO_ROOT / "docs/operations/csp-cloud-delivery.md"

    def test_ci_workflow_dev_only_and_required_check_name(self):
        self.assertTrue(self.ci_path.exists(), "CI workflow 必须存在")
        with open(self.ci_path, "r", encoding="utf-8") as f:
            wf = self.yaml.safe_load(f)

        on = wf.get("on") or wf.get(True, {})
        # Push 覆盖 dev, prelease, main，无 tags，无 paths-ignore
        push = on.get("push", {})
        self.assertEqual(set(push.get("branches", [])), {"dev", "prelease", "main"}, "Push 必须包含 dev, prelease, main 分支")
        self.assertNotIn("tags", push, "Push tags 必须彻底退出 CI 触发入口")
        self.assertNotIn("paths-ignore", push, "Push 绝不能包含 paths-ignore，避免文档变更死锁")

        # Pull Request 覆盖 dev, prelease, main，无 paths-ignore
        pr = on.get("pull_request", {})
        self.assertEqual(set(pr.get("branches", [])), {"dev", "prelease", "main"}, "PR 必须包含 dev, prelease, main 分支")
        self.assertNotIn("paths-ignore", pr, "PR 绝不能包含 paths-ignore")

        # Required check job 名称必须为精确匹配的 "Backend & Frontend Checks"
        jobs = wf.get("jobs", {})
        self.assertIn("test-and-build", jobs)
        self.assertEqual(jobs["test-and-build"].get("name"), "Backend & Frontend Checks")

        # Entry job 门禁：限制非授权分支的 workflow_dispatch，同时保留 PR checks 与 dev/prelease/main push/dispatch
        entry_if = jobs["test-and-build"].get("if", "")
        self.assertIn("github.event_name != 'workflow_dispatch'", entry_if)

        # publish-runtime 必须排除 pull_request，仅在 dev push 或 prelease 上构建镜像
        pub_if = jobs["publish-runtime"].get("if", "")
        self.assertIn("github.event_name != 'pull_request'", pub_if)
        self.assertNotIn("refs/tags/v", pub_if)
        self.assertNotIn("refs/heads/release/", pub_if)

        # promote-prelease 必须限定 prelease 分支
        promote_if = jobs["promote-prelease"].get("if", "")
        self.assertIn("github.ref == 'refs/heads/prelease'", promote_if)
        self.assertIn("github.event_name != 'pull_request'", promote_if)

    def test_dependabot_dev_target_and_no_legacy_ecosystems(self):
        self.assertTrue(self.dependabot_path.exists(), "Dependabot 配置文件必须存在")
        with open(self.dependabot_path, "r", encoding="utf-8") as f:
            cfg = self.yaml.safe_load(f)

        self.assertEqual(cfg.get("version"), 2)
        updates = cfg.get("updates", [])
        self.assertEqual(len(updates), 1, "仅保留单一 github-actions 更新配置")
        entry = updates[0]
        self.assertEqual(entry.get("package-ecosystem"), "github-actions")
        self.assertEqual(entry.get("target-branch"), "dev", "Dependabot 目标分支必须为 dev")
        self.assertNotIn("npm", [u.get("package-ecosystem") for u in updates])
        self.assertNotIn("pip", [u.get("package-ecosystem") for u in updates])

    def test_operations_doc_governance_amendment(self):
        content = self.doc_path.read_text(encoding="utf-8")
        self.assertIn("Fast-Forward", content)
        self.assertIn("Backend & Frontend Checks", content)
        self.assertIn("绝不创建 GitHub Release", content)
        self.assertIn("唯一专用预发布分支", content)
        self.assertIn("严禁强推代码分支", content)

    def test_workflow_dispatch_other_ref_skips_entry_and_publish(self):
        """workflow_dispatch 触发规则与发布分支隔离验证"""
        with open(self.ci_path, "r", encoding="utf-8") as f:
            wf = self.yaml.safe_load(f)

        jobs = wf.get("jobs", {})
        entry_if = jobs["test-and-build"].get("if", "")
        pub_if = jobs["publish-runtime"].get("if", "")
        promote_if = jobs["promote-prelease"].get("if", "")

        def eval_entry(event: str, ref: str) -> bool:
            expr = entry_if.replace("&&", " and ").replace("||", " or ")
            expr = expr.replace("github.event_name", "event").replace("github.ref", "ref")
            return safe_eval_bool(expr, {"event": event, "ref": ref})

        def eval_pub(event: str, ref: str, result: str = "success") -> bool:
            expr = pub_if.replace("&&", " and ").replace("||", " or ")
            expr = (
                expr.replace("needs.test-and-build.result", "result")
                .replace("github.event_name", "event")
                .replace("github.ref", "ref")
            )
            return safe_eval_bool(expr, {"result": result, "event": event, "ref": ref})

        def eval_promote(event: str, ref: str, tb_res: str = "success", pub_res: str = "success") -> bool:
            expr = promote_if.replace("&&", " and ").replace("||", " or ")
            expr = (
                expr.replace("needs.test-and-build.result", "tb_res")
                .replace("needs.publish-runtime.result", "pub_res")
                .replace("github.event_name", "event")
                .replace("github.ref", "ref")
            )
            return safe_eval_bool(expr, {"tb_res": tb_res, "pub_res": pub_res, "event": event, "ref": ref})

        # prelease 分支：entry 执行，publish 执行，promote 执行
        self.assertTrue(eval_entry("push", "refs/heads/prelease"))
        self.assertTrue(eval_pub("push", "refs/heads/prelease"))
        self.assertTrue(eval_promote("push", "refs/heads/prelease"))

        self.assertTrue(eval_entry("workflow_dispatch", "refs/heads/prelease"))
        self.assertTrue(eval_pub("workflow_dispatch", "refs/heads/prelease"))
        self.assertTrue(eval_promote("workflow_dispatch", "refs/heads/prelease"))

        # dev 分支：entry 执行，publish 执行（构建测试镜像），但 promote 绝不执行！
        self.assertTrue(eval_entry("push", "refs/heads/dev"))
        self.assertTrue(eval_pub("push", "refs/heads/dev"))
        self.assertFalse(eval_promote("push", "refs/heads/dev"), "dev push 绝不能触发发布 promote")

        # main 分支：entry 执行，publish 与 promote 绝不执行！
        self.assertTrue(eval_entry("push", "refs/heads/main"))
        self.assertFalse(eval_pub("push", "refs/heads/main"), "main push 绝不构建/推送运行时镜像")
        self.assertFalse(eval_promote("push", "refs/heads/main"), "main push 绝不触发发布 promote")

        # 非授权分支 (task/*, release/*, tags) 的 workflow_dispatch：entry、publish、promote 均跳过
        for other_ref in (
            "refs/heads/task/csp-cloud-build-integration",
            "refs/heads/release/20261010-01",
            "refs/tags/v1.0.0",
        ):
            self.assertFalse(eval_entry("workflow_dispatch", other_ref),
                             f"非授权分支 {other_ref} 的 workflow_dispatch 必须跳过 entry 检查")
            self.assertFalse(eval_pub("workflow_dispatch", other_ref),
                             f"非授权分支 {other_ref} 的 workflow_dispatch 必须跳过 publish")
            self.assertFalse(eval_promote("workflow_dispatch", other_ref),
                             f"非授权分支 {other_ref} 的 workflow_dispatch 必须跳过 promote")

        # PR 流程保护：PR (merge ref) 必须执行 entry 检查以提供绿标，且绝不执行 publish 与 promote
        self.assertTrue(eval_entry("pull_request", "refs/pull/99/merge"),
                        "PR 必须执行 entry 检查以提供绿标")
        self.assertFalse(eval_pub("pull_request", "refs/pull/99/merge"),
                         "PR 绝不能执行 publish")
        self.assertFalse(eval_promote("pull_request", "refs/pull/99/merge"),
                         "PR 绝不能执行 promote")


if __name__ == "__main__":
    unittest.main()

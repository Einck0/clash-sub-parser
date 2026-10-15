# CSP 云交付与隔离验证操作手册

## 1. 范围、当前权威与授权边界

当前需求入口是 [002-policy 当前交付修订](../../specs/002-policy/spec.md)、[实施计划](../../specs/002-policy/plan.md) 和 [任务](../../specs/002-policy/tasks.md)。`openspec/` 仅可作为历史材料，不是当前活动规格；不要导入远端旧规格覆盖本地 Spec Kit。

本地源码施工已获授权；这不等于 Git 提交、推送、发布、包 visibility 变更或生产部署授权。当前只收敛发布源与本地证据，不修改 ignored 的 owner 生产配置 `docker-compose.yml`，不触碰生产数据库、不 refresh 真实订阅、不启停生产容器。

已确认远端 `origin/dev` 有云交付与工具链历史，本地有尚未提交的业务改动。导入正规交付文件不表示已合并分支。后续必须先完成独立审查、按明确授权提交源码，再在独立 integration tree 正常合并并保留 `origin/dev` 祖先；不在 dirty 工作树 merge/rebase，不 force push。具体提交、双亲与归档冲突处理记录在本次交付的 `integration-plan.json`，不把旧 SHA 写成未来发布版本。

### 当前外部阻断

- 新项目交付授权已取得，不再等待旧“包名审批”；生产替换与重启仍未授权。
- 先前检查中 `gh auth` 失败，且名为 `ghcr.io/einck0/csp-runtime-private` 的包匿名可读。**名字含 private 不证明私有**；未满足门禁前不发布、不拉取冒充私有制品。此安全问题已由主脑通知用户，本手册不要求重新登录、索取凭据或更改可见性。
- `auto_test` 尚未完整实现，不能把兼容导入或本地绿色测试称作完整产品验收。
- 本地合成预览可以由当前源码构建，不依赖不合格云镜像；它不证明已上线或云私有门禁通过。独立 reviewer、主脑风险核对和适用 Critic 是后续独立步骤。

## 2. Same-SHA 构建链与官方入口

唯一官方 CI 为 [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)：

1. 固定 checkout `${{ github.sha }}`；Node 22 下 `web/npm ci --include=dev`、type-check、unit tests 和 build。
2. 清空 **CI runner 内** `internal/webassets/dist`，完整复制该次 `web/dist`，避免混入旧 bundle。不要在正式工作树提前刷新 embedded assets。
3. 按 `go.mod` 设置 Go，执行 mod verify、带 `with_quic with_wireguard` 的 vet/tests，安装 CI 测试所需协议二进制、准备合成 legacy archive。
4. `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags "with_quic with_wireguard"` 仅编译一次到 `bin/csp-linux-amd64`；通过 `-X main.commit=<sha> -X main.buildTime=<RFC3339>` 接入实际 CLI/platform 元数据，并检查 `--version`。
5. 校验 static ELF（readelf 失败或存在 INTERP 都失败），生成 `csp-linux-amd64.sha256`、上传 artifact。
6. publish job 下载并验算同一个 binary，交给 [Dockerfile.runtime](../../Dockerfile.runtime)；此镜像不包含 Go/Node 编译过程，不重复构建前端或 Go。
7. OCI `COMMIT_SHA`/`BUILD_DATE` 表示源码版本和镜像打包时间，二进制 buildTime 为上一步实际编译时间；两者不必相等。镜像绑定 `sha-<完整SHA>` 和 run 标签，交付使用 `@sha256:<digest>`，不使用可变 latest。

普通 `main`/`dev` push 与 PR 只 checks；发布必须有 release/tag/明确 workflow_dispatch 触发，且仓库变量 `ENABLE_CSP_GHCR_PUBLISH=true` 和私有门禁全部满足。发布 concurrency 不取消在途 release。Tag push 不应用 `paths-ignore`；参考 [GitHub workflow trigger 语义](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#onpushpull_requestpull_request_targetpathspaths-ignore)。

本地开发 [Dockerfile](../../Dockerfile) 是独立多阶段构建入口，使用官方 npm HTTPS lockfile、构建 tags 和同样的 commit/buildTime 参数；它不是生产现场编译入口。

### 本地验证

```bash
python3 -B -m unittest discover -s tests/cloud-delivery -p 'test_*.py' -v
python3 -B .github/scripts/test_ci_synthetic.py
# 必须显式使用 scratch 路径；现存 archive 会被拒绝覆盖。
python3 -B .github/scripts/setup_test_archive.py "${TMPDIR:-/tmp}/csp-synthetic-archive.db"
go build ./...
go vet -tags 'with_quic with_wireguard' ./...
```

Synthetic CI suite 使用 Python unittest/PyYAML、Compose config 和 buildx definition check；registry 响应全部 mock，不调用真实发布 API。Compose config 不启动服务。真实 archive 不是测试输出路径；旧 Go archive tests 仍有历史固定路径约束，只在已具备隔离测试宿主时运行，不用本机真实备份填造夹具。

## 3. 私有门禁：双证明且 fail-closed

[check_ghcr_private_gate.py](../../.github/scripts/check_ghcr_private_gate.py) 必须同时验证：

- 官方 GitHub Packages API 返回 `visibility=private`；API 不可用、鉴权不足、不明状态一律阻断。初次创建只有完整分页证明目标包不存在后才可按既有门禁继续。
- 匿名 manifest 访问（包括 401 challenge 后匿名 token 重试）仍被拒绝；200 即失败，不因包名或登录成功放行。
- push 后 digest 非空、格式正确，并重新验证官方 API 私有性及匿名拒绝。

`ghcr.io/einck0/clash-sub-parser` 是历史公开包：停止向该公开包推送，严禁擅自删除历史公开包。不得用 public 发布或跳过 API 来“修复”门禁。

[preflight-pull.sh](../../scripts/cloud-delivery/preflight-pull.sh) 验证包名、完整 revision 和不可变 digest 后才允许授权拉取：

```bash
scripts/cloud-delivery/preflight-pull.sh --image "$APPROVED_IMAGE_DIGEST" \
  --expected-revision "$REVIEWED_SOURCE_SHA" --check-only
```

`--check-only` 的 2/BLOCKED 不是成功。后续凭据若确需配置，由用户在安全终端执行：Classic PAT 最小 `read:packages`，官方 API 访问还须实际权限；`OWNER_PAT` 是配置名，不要求聊天发送明文。严禁向用户索取明文 Token。Docker config 的 base64 不是加密，文件应为 0600；不读取或打印现存凭据、不将其写进报告。脚本的显式 `--configure-pat` 会写登录配置，仅可另获授权后执行。

## 4. Compose 职责与不变量

| 文件 | 职责 | 约束 |
| --- | --- | --- |
| [docker-compose.dev.yml](../../docker-compose.dev.yml) | 明确 local build | 独立 dev 容器/卷；不用于生产 rollout |
| [docker-compose.prod.yml](../../docker-compose.prod.yml) | image-only 发布模板 | `CSP_IMAGE` 必填，无 build；保留 loopback 17000/18080 → 18080、`csp-v1-data`、host proxy extra_hosts |
| [docker-compose.isolated.yml](../../docker-compose.isolated.yml) | 合成隔离预览 | 独立 project/端口/全新卷、run-marker 标签、无 restart，无 build |
| `docker-compose.yml` | ignored owner 生产配置 | 本轮不修改；模板不自动替换此文件 |

生产 owner 路径为 `/home/service/clash-sub-parser`；历史记录的非 root UID/GID 是 10001:10001、卷目录权限 750。它们不是永远有效的动态证据，正式运维前只读核对，不能据此自动 chown/chmod。生产端口 17000/18080 不供自动测试或预览使用。

隔离 Compose 默认命令包含 `--disable-subscription-scheduler --disable-probe-coordinator`，两个后台协调器均被真正禁用；internal Docker network 阻止外部连通。代理为空不等于禁网，不能只靠清空代理宣称隔离。不要导入真实 source、生产 DB 或默认启用 copy 后的订阅定时任务。

## 5. 隔离启动、交接与受控清理

仅在镜像门禁通过且获准运行容器时：

```bash
export CSP_PREVIEW_DIR="${TMPDIR:-/tmp}/csp-preview-owned-run"
scripts/cloud-delivery/run-isolated.sh --image "$APPROVED_IMAGE_DIGEST" \
  --project csp-isolated-owned-run --run-marker owned-run --check-only
# 获准启动后去掉 --check-only；脚本强制 up -d --no-build。
```

端口默认 18081/17001，必须确认未占用且不使用生产 17000/18080。脚本仅使用本地已就绪镜像，不盲 pull。`CSP_PREVIEW_DIR` 支持每次独立 scratch，避免覆盖他人预览。

浏览器交接 `instance.json` 必须包含 `instanceId/url/health/allowedOrigins/sourceManifest/testDataBoundary/scenarios/shutdownOwner/cleanupToken`。同时保留 PID 或 container identity、snapshot/binary hash、healthz/readyz 和合成数据边界证据；只交付当前 UI，不把旧 embedded assets 称作当前实现。源码私有 snapshot 可嵌入已验证的当前 web-dist，正式 embedded assets 保持不动。

准备者负责健康实例与清理移交，不扮演 Critic。Critic 需工程门禁满足后由主脑独立派发，读取多视口 PNG 并给出真实视觉结论。退出码 0 或 healthy 不等于 Critic PASSED。

容器清理由 [cleanup-isolated.sh](../../scripts/cloud-delivery/cleanup-isolated.sh) 按 project/run-marker 校验防误删；先 `--dry-run`，禁止不明目标 `down -v`。源码本地预览不使用此 Docker 清理脚本，应按交接 PID、命令和 cleanupToken 定向停止，主脑接手后负责清理。不得清理生产服务、卷或其他会话资源。

## 6. 生产部署与回滚（另获授权才执行）

本轮不部署、不备份生产。后续操作参照 [现有运维入口](../operations.md) 与 [受控部署手册](../controlled_deployment_runbook.md)，冻结 reviewed source SHA、binary SHA256、image digest、旧 digest、owner Compose 路径和兼容 schema。

SQLite WAL 不能用裸文件 copy 当在线备份：另获授权后用容器内 sqlite3 `.backup` 生成一致性快照，在**相同容器路径命名空间**检查非空、`PRAGMA schema_version;`、`PRAGMA integrity_check;` 为 ok、`PRAGMA foreign_key_check;` 为空。不要用宿主 `test -s` 检查容器内路径，也不要自动写入不存在/未挂载的备份目录。

默认回滚是将 owner 配置恢复到前一已知良好 digest 并 `up -d --no-build`，仅当 schema 向后兼容时适用。确需数据库恢复时必须先停目标服务、保存故障 DB/WAL/SHM、验证备份、按已核实挂载点恢复与权限，再启动旧 digest；不要在在线服务上直接移除 WAL/SHM。恢复后核验 healthz、readyz 与关键数据行为。未获明确生产写入与重启授权时只交计划，不执行这些命令。

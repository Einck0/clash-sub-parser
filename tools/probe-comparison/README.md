# 无测速 stage-first 对照（实现，自测局限待终态审查）

同一 Change，禁止重复库存 reset/import。此工具不读生产 DB，不抓商业订阅，不发布。
默认 plan/freeze/report 不出网；execute 必须同时提供 `--allow-network` 和绑定 manifest SHA 的独立 Reviewer PASS 报告。本轮未运行真实批次。

## 私有准备与构建

调用者指定不存在的私有目录，固定真实 upstream commit；不修改缓存源码。

```sh
python3 tools/probe-comparison/prepare.py --upstream "$UPSTREAM_CACHE" --workspace "$PRIVATE_REFERENCE"
GOPROXY=off GOTOOLCHAIN=local go build -o "$CSP_SIDE" ./tools/probe-comparison/csp
(cd "$PRIVATE_REFERENCE" && GOPROXY=off GOTOOLCHAIN=local go build -o "$NATIVE_SIDE" ./cmd/reference)
(cd "$PRIVATE_REFERENCE" && GOPROXY=off GOTOOLCHAIN=local go build -mod=mod -modfile=samecore.mod -o "$SAMECORE_SIDE" ./cmd/reference)
```

native v1.19.31 与 comparative v1.19.32 分开；实际 engine 来自 Go build metadata。plan 保存完整 mod/sum 差异及传递依赖，不冒称相同方法。

## 接口

```sh
python3 tools/probe-comparison/run.py plan --inventory "$INVENTORY" --output "$PLAN" \
  --csp "$CSP_SIDE" --reference "$SAMECORE_SIDE" --reference-source "$PRIVATE_REFERENCE" \
  --response-body-budget-bytes 134217728 --deadline-seconds 1800 \
  --baseline-body-limit-bytes 65536 --alive-concurrency 8 --media-concurrency 2 --speed-concurrency 0
python3 tools/probe-comparison/run.py freeze --plan "$PLAN" --manifest "$MANIFEST" --ledger "$LEDGER"
# 仅独立审查和授权批次准备完成之后：
python3 tools/probe-comparison/run.py execute --manifest "$MANIFEST" --ledger "$LEDGER" \
  --output "$RESULTS" --allow-network --review-report "$FINAL_REVIEW_JSON"
python3 tools/probe-comparison/run.py report --manifest "$MANIFEST" --ledger "$LEDGER" \
  --results "$RESULTS" --output "$REDACTED_REPORT"
```

输入必须0600、ALL90唯一ID。review JSON须 `gate=reviewer, verdict=PASS/PASSED, final=true, change=csp-fresh-inventory-full-validation, manifest_sha256=<实际摘要>`。此字段由独立会话提供，施工者不得造凭据。

可选 native baseline两侧仅首次plan `--native-baseline-sides 2 --native "$NATIVE_SIDE"`；同一总盘子扣除11.25MiB，不新增速度或media控制。默认native_network_sides=[]。

## 计量与公平

四侧先完成ALL90 baseline，64KiB/node/side，共22.5MiB；随后四侧baseline intersection。
平台余量105.5MiB，四侧等额，按预定能力组和eligible节点数均分，不能首节点吞全盘。
每个响应读前经loopback IPC调用SQLite BEGIN IMMEDIATE原子预留、settle退款。
ledger持久保存start/deadline、used/reserved/attempt；崩溃未结算额度保守占用，禁止自动重启/重试。
loopback桥仅计量，不实现第二套探针。真实reader仍为CSP runner或真实upstream方法。
应用body不含header/TLS/NIC，transport计数单列。

## 保真与尚待完善

baseline comparative采用HTTP1、拒绝redirect、target TLS验证、keepalive禁用和相同UA；proxy TLS保持冻结配置。
平台算法/子请求与UA未全部归一，manifest保留源码target列表和method mismatch；输出始终PARTIAL，不宣称full parity。
真实native Check保留原生去重和defaults，无速度URL；被native去重的输入可能not_scheduled，不能伪装为全90已执行。
CSP/native errors、非collector平台结果保留；执行器超时/子进程崩溃仍需补足未launch终态与原始queued状态，不能视作完整14B.3通过。
需要独立review后才能恢复批准实网；全量race仍须真实退出码0或由主脑裁定有据环境局限。

## 离线测试

```sh
python3 -m unittest discover -s tools/probe-comparison -p 'test_*.py'
GOPROXY=off GOTOOLCHAIN=local go test ./...
(cd "$PRIVATE_REFERENCE" && GOPROXY=off GOTOOLCHAIN=local go test ./check -run '^TestReference' -count=1)
```

使用unittest/mock、SQLite事务、Go httptest loopback代理；不新建测试框架，不调用外部DNS/目标。
私有cache及源快照由准备者负责有界留存清理，不删除共享cache或无关进程。

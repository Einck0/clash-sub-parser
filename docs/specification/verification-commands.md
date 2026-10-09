# CSP 规格符合度验证命令目录 (Verification Commands Directory)

- **基线版本**：HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **编制日期**：2026-10-09 (Epoch 3 Remediation)
- **执行收据**：`/tmp/csp-conformance-real-tests-receipt.json`
- **收据 SHA-256**：`{'total_commands': 41, 'total_executions': 178, 'top_level_executions': 125, 'subtest_executions': 53, 'unique_top_level_tests': 91, 'unique_subtests': 46, 'total_unique_tests': 137, 'zero_run_commands': [], 'failed_commands': []}`

本目录收录 41 条标准实机自测验证命令（编号 `CV-001` ~ `CV-041`）。所有命令采用独立 Markdown fenced code block 格式存储，避免 Markdown 表格渲染转义导致正则管道符 `|` 被逃逸为字面量 `\|` 的问题。所有命令均已在实机实际执行并生成确定性测试收据。

---

### CV-001

- **关联需求**：`FR-INV-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `1.109s`)
- **日志存根**：`/tmp/csp-test-logs/cv-001.log`
- **验证命令**：

```bash
go test -json ./internal/application/inventory -run ^TestReconcile_DisabledSubscriptionRejected$
```

### CV-002

- **关联需求**：`FR-INV-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.74s`)
- **日志存根**：`/tmp/csp-test-logs/cv-002.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestSubscriptionEndpointsEnforceSecurityAndReturnPlaintextURL$
```

### CV-003

- **关联需求**：`FR-INV-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `4`, passed: `4` (top: `1`, sub: `3`, time: `0.823s`)
- **日志存根**：`/tmp/csp-test-logs/cv-003.log`
- **验证命令**：

```bash
go test -json ./internal/application/inventory -run ^TestReconcile_FetchOrParseFailure_PreservesNodes$
```

### CV-004

- **关联需求**：`FR-INV-03`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `2`, passed: `2` (top: `2`, sub: `0`, time: `0.371s`)
- **日志存根**：`/tmp/csp-test-logs/cv-004.log`
- **验证命令**：

```bash
go test -json ./internal/parser -run ^(TestParseLogicalIDDistinguishesCredentialsAndIgnoresNames|TestParseYAMLNormalizesSupportedProtocols)$
```

### CV-005

- **关联需求**：`FR-INV-04`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.315s`)
- **日志存根**：`/tmp/csp-test-logs/cv-005.log`
- **验证命令**：

```bash
go test -json ./internal/repository/sqlite -run ^TestNodeSourceHistoryRepository_BatchInsertAndQuery$
```

### CV-006

- **关联需求**：`FR-INV-04`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.501s`)
- **日志存根**：`/tmp/csp-test-logs/cv-006.log`
- **验证命令**：

```bash
go test -json ./internal/application/inventory -run ^TestReconcile_DualSourceMerge$
```

### CV-007

- **关联需求**：`FR-INV-05`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.301s`)
- **日志存根**：`/tmp/csp-test-logs/cv-007.log`
- **验证命令**：

```bash
go test -json ./internal/repository/sqlite -run ^TestNodeRepository_ListAll_AndHealthPredicatesConservation$
```

### CV-008

- **关联需求**：`FR-INV-06`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.503s`)
- **日志存根**：`/tmp/csp-test-logs/cv-008.log`
- **验证命令**：

```bash
go test -json ./internal/application/inventory -run ^TestUpdateNodeConnectionPlaintextDirectEdit$
```

### CV-009

- **关联需求**：`FR-POL-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.095s`)
- **日志存根**：`/tmp/csp-test-logs/cv-009.log`
- **验证命令**：

```bash
go test -json ./internal/resolver -run ^TestResolverExplicitPassNoInheritanceAndDigest$
```

### CV-010

- **关联需求**：`FR-POL-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.088s`)
- **日志存根**：`/tmp/csp-test-logs/cv-010.log`
- **验证命令**：

```bash
go test -json ./internal/resolver -run ^TestResolver_NestedGroupProjection_RestrictsChildNodes$
```

### CV-011

- **关联需求**：`FR-POL-03`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `13`, passed: `13` (top: `13`, sub: `0`, time: `4.604s`)
- **日志存根**：`/tmp/csp-test-logs/cv-011.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/features/policy/PolicyView.test.ts
```

### CV-012

- **关联需求**：`FR-POL-04`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.342s`)
- **日志存根**：`/tmp/csp-test-logs/cv-012.log`
- **验证命令**：

```bash
go test -json ./internal/resolver -run ^TestResolver_DynamicGroup_MatchesFromGlobalPool$
```

### CV-013

- **关联需求**：`FR-POL-05`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `2`, passed: `2` (top: `2`, sub: `0`, time: `0.933s`)
- **日志存根**：`/tmp/csp-test-logs/cv-013.log`
- **验证命令**：

```bash
go test -json ./internal/compiler -run ^(TestExplicitEmptyPassCompilerAndCapabilities|TestMihomoNativePassContinuesRules)$
```

### CV-014

- **关联需求**：`FR-POL-06`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.083s`)
- **日志存根**：`/tmp/csp-test-logs/cv-014.log`
- **验证命令**：

```bash
go test -json ./internal/resolver -run ^TestResolver_DynamicGroup_MatchesFromGlobalPool$
```

### CV-015

- **关联需求**：`FR-POL-06`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.369s`)
- **日志存根**：`/tmp/csp-test-logs/cv-015.log`
- **验证命令**：

```bash
go test -json ./internal/repository/sqlite -run ^TestNodeRepository_ListAll_AndHealthPredicatesConservation$
```

### CV-016

- **关联需求**：`FR-PROB-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `6`, passed: `6` (top: `1`, sub: `5`, time: `0.359s`)
- **日志存根**：`/tmp/csp-test-logs/cv-016.log`
- **验证命令**：

```bash
go test -json ./internal/probe/mihomo -run ^TestNodeToMapping_SupportsSkipCertVerify$
```

### CV-017

- **关联需求**：`FR-PROB-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.302s`)
- **日志存根**：`/tmp/csp-test-logs/cv-017.log`
- **验证命令**：

```bash
go test -json ./internal/repository/sqlite -run ^TestProbeScheduleRepository$
```

### CV-018

- **关联需求**：`FR-PROB-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.525s`)
- **日志存根**：`/tmp/csp-test-logs/cv-018.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestProbeScheduleEndpoints_GetAndUpdate$
```

### CV-019

- **关联需求**：`FR-PROB-03`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.106s`)
- **日志存根**：`/tmp/csp-test-logs/cv-019.log`
- **验证命令**：

```bash
go test -json ./internal/probe/queue -run ^TestSchedulerManualPreemptFrontAndPromotion$
```

### CV-020

- **关联需求**：`FR-PROB-04`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `2`, passed: `2` (top: `2`, sub: `0`, time: `0.133s`)
- **日志存根**：`/tmp/csp-test-logs/cv-020.log`
- **验证命令**：

```bash
go test -json ./internal/domain -run ^(TestEvaluateBaselineHealthRequiresCurrentVersionAndEvidence|TestClassifyNodeHealthAndConservation)$
```

### CV-021

- **关联需求**：`FR-PROB-05`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `9`, passed: `9` (top: `1`, sub: `8`, time: `0.501s`)
- **日志存根**：`/tmp/csp-test-logs/cv-021.log`
- **验证命令**：

```bash
go test -json ./internal/application/probe -run ^TestRunnerReasonCategoriesHaveNoSecretLeakageAndAssociateRevision$
```

### CV-022

- **关联需求**：`FR-PROB-06`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `6`, passed: `6` (top: `1`, sub: `5`, time: `0.094s`)
- **日志存根**：`/tmp/csp-test-logs/cv-022.log`
- **验证命令**：

```bash
go test -json ./internal/domain -run ^TestFilterConditionMatchingSemantics$
```

### CV-023

- **关联需求**：`FR-PROB-07`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `7`, passed: `7` (top: `1`, sub: `6`, time: `0.315s`)
- **日志存根**：`/tmp/csp-test-logs/cv-023.log`
- **验证命令**：

```bash
go test -json ./internal/probe/singbox -run ^TestBuildOptionsRejectsUnsafeProtocolOptions$
```

### CV-024

- **关联需求**：`FR-PUB-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.532s`)
- **日志存根**：`/tmp/csp-test-logs/cv-024.log`
- **验证命令**：

```bash
go test -json ./internal/compiler -run ^TestCompileMihomo_AllTenProtocolsFourGroupsFourteenRules_AndOfficialValidation$
```

### CV-025

- **关联需求**：`FR-PUB-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `3`, passed: `3` (top: `3`, sub: `0`, time: `0.494s`)
- **日志存根**：`/tmp/csp-test-logs/cv-025.log`
- **验证命令**：

```bash
go test -json ./internal/compiler -run ^(TestSingBoxGoldenFixture|TestSurgeGoldenFixture|TestQuantumultXGoldenFixture)$
```

### CV-026

- **关联需求**：`FR-PUB-03`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `5`, passed: `5` (top: `1`, sub: `4`, time: `0.516s`)
- **日志存根**：`/tmp/csp-test-logs/cv-026.log`
- **验证命令**：

```bash
go test -json ./internal/compiler -run ^TestValidateSnapshot_GroupsRulesAndEmptyBoundary$
```

### CV-027

- **关联需求**：`FR-PUB-04`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `8`, passed: `8` (top: `1`, sub: `7`, time: `0.525s`)
- **日志存根**：`/tmp/csp-test-logs/cv-027.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestPublicationClientExportAuthModes$
```

### CV-028

- **关联需求**：`FR-PUB-05`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `8`, passed: `8` (top: `1`, sub: `7`, time: `0.545s`)
- **日志存根**：`/tmp/csp-test-logs/cv-028.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestPublicationClientExportAuthModes$
```

### CV-029

- **关联需求**：`FR-PUB-06`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `7`, passed: `7` (top: `1`, sub: `6`, time: `0.555s`)
- **日志存根**：`/tmp/csp-test-logs/cv-029.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestPublicationClientSubscriptionEndpoint$
```

### CV-030

- **关联需求**：`FR-AUTH-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.313s`)
- **日志存根**：`/tmp/csp-test-logs/cv-030.log`
- **验证命令**：

```bash
go test -json ./internal/repository/sqlite -run ^TestSettingsRepository_AuthSettings$
```

### CV-031

- **关联需求**：`FR-AUTH-01`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.543s`)
- **日志存根**：`/tmp/csp-test-logs/cv-031.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestAuthSettingsAPIAndAdminModes$
```

### CV-032

- **关联需求**：`FR-AUTH-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.314s`)
- **日志存根**：`/tmp/csp-test-logs/cv-032.log`
- **验证命令**：

```bash
go test -json ./internal/repository/sqlite -run ^TestAuthSettingsPersistAcrossSQLiteReopen$
```

### CV-033

- **关联需求**：`FR-AUTH-02`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.507s`)
- **日志存根**：`/tmp/csp-test-logs/cv-033.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^TestAuthSettingsAPIAndAdminModes$
```

### CV-034

- **关联需求**：`FR-AUTH-03`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `1`, passed: `1` (top: `1`, sub: `0`, time: `0.679s`)
- **日志存根**：`/tmp/csp-test-logs/cv-034.log`
- **验证命令**：

```bash
go test -json ./internal/application/inventory -run ^TestUpdateNodeConnectionPlaintextDirectEdit$
```

### CV-035

- **关联需求**：`FR-AUTH-04`
- **执行类型**：`go`
- **执行结果**：exit `0`, run: `4`, passed: `4` (top: `2`, sub: `2`, time: `0.497s`)
- **日志存根**：`/tmp/csp-test-logs/cv-035.log`
- **验证命令**：

```bash
go test -json ./internal/transport/http -run ^(TestAdminAuthMiddleware_TokenMode_SessionCookie|TestAuthLogoutEndpoint)$
```

### CV-036

- **关联需求**：`FR-UI-01`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `12`, passed: `12` (top: `12`, sub: `0`, time: `4.119s`)
- **日志存根**：`/tmp/csp-test-logs/cv-036.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/features/nodes/nodeView.test.ts src/composables/useResponsiveColumns.test.ts
```

### CV-037

- **关联需求**：`FR-UI-02`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `8`, passed: `8` (top: `8`, sub: `0`, time: `3.151s`)
- **日志存根**：`/tmp/csp-test-logs/cv-037.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/features/nodes/nodeView.test.ts
```

### CV-038

- **关联需求**：`FR-UI-03`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `13`, passed: `13` (top: `13`, sub: `0`, time: `4.242s`)
- **日志存根**：`/tmp/csp-test-logs/cv-038.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/features/policy/PolicyView.test.ts
```

### CV-039

- **关联需求**：`FR-UI-04`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `27`, passed: `27` (top: `27`, sub: `0`, time: `2.757s`)
- **日志存根**：`/tmp/csp-test-logs/cv-039.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/features/publications/publications.test.ts
```

### CV-040

- **关联需求**：`FR-UI-05`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `8`, passed: `8` (top: `8`, sub: `0`, time: `2.902s`)
- **日志存根**：`/tmp/csp-test-logs/cv-040.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/features/nodes/nodeView.test.ts
```

### CV-041

- **关联需求**：`FR-UI-06`
- **执行类型**：`vitest`
- **执行结果**：exit `0`, run: `4`, passed: `4` (top: `4`, sub: `0`, time: `1.058s`)
- **日志存根**：`/tmp/csp-test-logs/cv-041.log`
- **验证命令**：

```bash
npm --prefix web test -- --reporter=json src/locales/messages.test.ts
```

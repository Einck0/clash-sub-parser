# speckit-analyze 增量校验：原始载荷整改

Feature 由 SPECIFY_FEATURE_DIRECTORY=specs/001-inventory 解析；setup-plan 保留旧 plan，执行前基线已保存 /tmp/csp-payload-936716/baseline。无 .specify/extensions.yml，无 hooks。已有能力规格续办，不重建 feature/branch，不覆盖旧需求。当前写入辅助设计补充，spec/plan/tasks 链接同一设计。

| 需求 | 设计 | 任务 | 验收 |
|---|---|---|---|
| FR-RAW-01 | Inventory 内存解析/空 BLOB | 02,05,06 | 正常/部分/失败刷新与节点保全 |
| FR-RAW-02 | Repository Save/三读取接口 | 03,05 | 带正文输入仍存零、历史不返回 |
| FR-RAW-03 | 保留结构化节点及发布引用 | 02,03,06 | pinned/发布/凭据/主订阅/来源不变 |
| FR-RAW-04 | 离线 CLI + 事务清理 | 03,04,06 | dry-run不写、备份核验、幂等、失败回滚 |
| FR-RAW-05 | 显式VACUUM/文件占用报告 | 04,06 | 离线/参数/并发保护、失败区分逻辑提交 |
| FR-RAW-06 | 原Actions/同SHA镜像/审查 | 05,06,07 | 云测+隔离+reviewer，无生产副作用 |

校验结论：无 CRITICAL/HIGH 冲突，无未覆盖需求或孤立任务。既有 spec 涉及 raw 保真/不可变原文的历史描述，以本轮用户明确授权的 FR-RAW-01..06 补充为准；结构化快照不可变性继续成立。Constitution 资产保全/身份/简单明文设计/活规范一致。T-RAW-01 规划检查完成不代表施工或测试门禁通过。未完成任务保持未勾选。

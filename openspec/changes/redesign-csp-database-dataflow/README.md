# Change: redesign-csp-database-dataflow

CSP 数据库与上下游数据流重构规格定义与架构蓝图。保持单 SQLite 主库与 WAL，补齐上游事实层与版本化连接模型。

- **设计审查状态**: `Independent Reviewer PASS` (架构与整改方案已通过独立审查闭环)
- **内存约束验证**: `/tmp/verify_database_dataflow_design.py` 完成 23 项 DDL、外键、前置门禁、IP Risk 统一关联及防泄密约束验证（**注：此仅为内存沙箱 DDL 与契约 scratch 验证，并非业务测试或生产修复**）
- **交付终态**: `STATUS: READY` / `implementation authorized by user; execution contract frozen`
- **实施大包状态**: 任务 3.1 至 7.3 实施已授权，按分包与冻结契约执行，生产代码与业务数据库零修改，等待后续子机施工与审查闭环
- **规范校验**: `openspec validate redesign-csp-database-dataflow --strict` 退出码 0

---

## 边界与未知事实分类说明 (Known Unknowns Categorization)

1. **客观无法恢复的历史未知项 (Permanently Unrecoverable History)**:
   - **旧 Run 失败底层原生报错**: 旧观测记录的底层细节因历史内核未捕获已永久丢失，无法回溯，当前未知真实失败根因，绝不凭空伪造；
   - **真实上游面板软件品牌**: Dogegg 虽然下发固定格式公告条目，但具体后端面板品牌未知，契约建立在经过验证的来源语义证据与规则版本上，不假设面板品牌；
2. **实施阶段完全可测项 (Measurable During Implementation)**:
   - **Payload 磁盘保留天数预算**: 原始 body 长期保留天数需实施前在隔离环境根据实测磁盘写入增长率最终确定；
   - **停写维护短窗真实时长**: 真实停写维护窗口时长需在实施阶段隔离演练实测确定，不假设固定 30 秒，并在上线前由用户确认。

---

## 施工约束守则
本轮工作严格限定于设计方案与 OpenSpec 规范契约修订，生产代码与业务数据库零修改，未执行 `git commit`，等待主脑交付用户审批。

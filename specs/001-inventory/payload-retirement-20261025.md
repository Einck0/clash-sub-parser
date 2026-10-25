# 原始载荷停止持久化与历史清理（2026-10-25）

## 用户结果与授权
用户明确：不保存每次拉取的订阅原始载荷，只保留所属节点信息及主订阅信息。正常使用仍为创建订阅→刷新→查看节点及来源→生成/获取发布；无需新增保留期或快照管理步骤。本轮允许实现、dev 推送、云测和隔离验证，不允许生产换镜像、生产 DELETE/UPDATE/VACUUM 或重启。宿主冗余备份前台已清完，不重复操作。

## 现状取证与决定
HEAD 33264c0；inventory/service.go:584 在刷新事务内写入 fetchResp.Body；sqlite/dataflow.go:54 的 Save 是另一写入口。查询和发布不消费 BodyBlob。subscription_entries.parsed_config_json 实际由 service.go:911 附近的 protocol/server/port/credentials 白名单构造，**并非上游原始片段**；不按调查报告的宽泛 raw 分类误删。publication_payload_refs 与连接版本通过 payload/entry ID 保持来源链。

选择最小充分方案：保留现有表、ID、摘要、抓取元数据、结构化节点快照/分类/覆盖/来源链，body_blob 使用非 NULL 的零长度 BLOB。无新定时任务、无保留期设置、无整表 DELETE、无启动全量清除、无不必要的 schema 重建/触发器。旧字段 BodyBlob 作为兼容输入保留但 Save 忽略；读取返回空正文，不读取历史正文到应用。已经发布的 artifact 自包含，继续可用。无法从原文重新解析未来新字段是用户选择不留原文的自然后果，不影响当前节点凭据及发布。

## 需求及场景
- FR-RAW-01：成功/部分成功刷新持久化 body 长度为 0；解析仍使用本次内存中的响应，节点连接参数与归属关系不丢失；失败/零节点保全旧节点，不创建原文副本。
- FR-RAW-02：repository Save 接受旧调用方带 BodyBlob，但始终保存空 BLOB；GetByID/GetLatest/GetByDigest 不返回历史原文。主订阅配置、primary 标记、userinfo、刷新状态保留。
- FR-RAW-03：每次抓取的结构化 entries、连接版本、名称、分类与纠偏、引用 ID 原样保留。Pinned/发布引用不能成为保留正文的例外。
- FR-RAW-04：新增显式离线维护命令，dry-run 默认只读统计，apply 必须显式选择并提供可核验且非目标本体/硬链接的备份。只清除非空 body，不删除 payload/entries/节点/订阅/发布记录。失败事务回滚；重复执行命中零条。
- FR-RAW-05：物理收缩须单独 --vacuum 且只在 apply/offline 下可用；回收不自动跟随服务启动。报告逻辑清除字节与数据库文件前后大小分别计量；VACUUM 失败明确说明逻辑清理可能已提交，不谎报全部回滚/释放空间。具备离线声明、目标 DB 文件/目录校验、并发锁与打开文件检查，备份 quick_check 核验。不能只凭“确认备份”布尔绕过。
- FR-RAW-06：不改 HTTP/UI 契约与发布语义，不读取/输出订阅正文或凭据到报告。云测最终 SHA 与隔离镜像一致，且无本机编译测试。

具体夹具（非生产实测）：A 两次刷新同一合成订阅，body=0 且节点及 userinfo 正确；B 失败刷新保全旧节点；C 历史库包含 pinned/发布引用正文及连接历史，dry-run 不变、apply 正文归零而全部结构化字段/ID 不变、再次执行零命中；D 历史 artifact 字节及 pinned fetch 选取结果不变；E 错误/同文件备份、并发进程、互斥参数拒绝写入；F 显式 VACUUM 完成后 quick_check=ok、foreign_key_check 无结果，文件占用实测单列。

成功条件：新写入与读取正文均为零；历史清除不丢失上述事实；云端测试和独立审查通过，同 SHA 预构建镜像隔离演练清理及发布有效；生产保持原样。

## 模块职责与接口（冻结）
1. Inventory 写入模块：internal/application/inventory/service.go 与定向测试，internal/domain/dataflow.go 注释。解析入口保持内存响应，刷新事务写空 BLOB，其余字段不变。
2. SQLite 存储模块：internal/repository/sqlite/dataflow.go 及定向测试；新增 payload_cleanup.go/_test.go。Save 忽略 BodyBlob，三读取接口选择空 BLOB。提供：
   - `type PayloadStorageReport struct`，JSON 数值字段：payload_count、nonempty_body_count、body_bytes、cleared_body_count、cleared_body_bytes；以及 retained counts（entries/nodes/subscriptions/publications/refs 可用明确字段）。
   - `InspectPayloadStorage(ctx context.Context, db *sql.DB) (*PayloadStorageReport, error)` 只读；
   - `ClearPayloadBodies(ctx context.Context, db *sql.DB) (*PayloadStorageReport, error)` 事务中检查统计并 UPDATE 非空 body=X''，提交前校验非空正文为零及关键保留表计数不变、FK 检查，不执行 VACUUM。错误 fail closed，缺表失败不伪造零数据。
3. CLI 维护模块：cmd/csp/payload_maintenance.go/_test.go 及 main.go 子命令注册，命令名 `retire-payload-bodies`。复用 SQLite OpenReadOnly/Open；调用上面两函数；负责参数、备份与离线/并发校验、可选 VACUUM 和 JSON 结果。apply 必须 --offline 与 --backup-file；--dry-run/--apply 互斥且必须选一。不调用 migrate 或启动业务服务。路径安全与备份检查失败不得改目标。CLI 文件为模块唯一归属，禁止改现有 maintain.go。
模块只经上述公共接口集成，三个写集合不相交，可并行。主脑协调 Git index/dev 云测与合并隔离证据，不允许子机提交/推送。

## 验证与后续发布
现有 .github/workflows/ci.yml 云端 go vet/go test、前端回归及 runtime image 为门禁；本地仅静态 diff/源码和预构建镜像运行。不新增 CI 系统。预构建镜像以唯一名与合成 DB 演练旧 body→dry-run→apply→vacuum→重复执行及 serve/发布检查，不挂载生产卷或保留热备。生产后续须新授权，停服务并创建且验证本次维护前回滚点→dry-run→明确确认 apply（空间允许才 vacuum）→健康及发布验证；回滚用配对旧镜像与维护前 DB，不能从清空后的文件恢复原文。此文取代旧 R4“保留期未定暂缓”的当前决定，不改写旧失败/完成历史。

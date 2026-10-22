# 2026-10-22 现代前端实施合同

本 Feature 为 006-ui。官方命令使用 SPECIFY_FEATURE_DIRECTORY=specs/006-ui、SPECIFY_FEATURE_NO_PERSIST=1；不覆盖其他 Feature 指针。

## 当前授权与取舍

用户“那就开始做”承接现代底座评估，授权完整前端重构。既有评估的业务、页面与失败语义保持；其中仅分析、不升级样式底座的结论已被撤销。不是只换色/首页试点，完成全部现有八业务页。

冻结：Vue3、Router4、Vite6保留；迁移Tailwind4.3.3 + @tailwindcss/vite4.3.3 + DaisyUI5.7.50(确切版本，锁文件为实际解析结果)。官方peer支持Vite6、Node>=20，现CI Node22足够；保plugin-vue5/Vitest2，无需跟参考Vite8/Router5连锁升级。不采用全仓自动升级器以免越界，手工迁移源与lock。允许仅npm install --package-lock-only --ignore-scripts --no-audit --no-fund解析锁，无本地test/type/build；Go/npm测试编译只能GitHub现ci.yml。官方迁移依据见本轮style-research资料与https://tailwindcss.com/docs/upgrade-guide、https://daisyui.com/docs/upgrade/。

保留7主题名/data-theme/csp_theme，CSS入口import tailwindcss+plugin daisyui，不同时保旧PostCSS/tailwind JS管线。focus变量改color-primary。每模块负责其模板v4/v5类变更(input-bordered/label/form-control/tabs-boxed/shrink等)，不能以永久旧版兼容CSS替代实际迁移。视觉需深浅主题/桌面移动实测；现代浏览器基线Chrome111/Safari16.4/Firefox128。

代码开发提交推dev、云测试/构建/预发布、同SHA纯合成DB隔离预览获授权；**生产部署/数据/DNS/域名/认证配置写未授权**，不挂生产卷。保护全部既有dirty，不清盘。所有子机不commit/push；主脑统一索引/推送。缺外部接口先caller_ping，不开放全仓探索。

## 用户结果与需求 FR-M

- M01：样式底座真实升级，7主题/键盘focus/表单尺寸/弹层/安全区兼容，不引入参考的实时核心控制业务或无关依赖。
- M02：统一单一Router来源，现8路由与旧hash兼容，/probes→/nodes；桌面分组侧栏，移动单行“节点/策略/订阅/更多”，更多提供其余5页。title只一个可见层级(保app名与无障碍heading)，页内无重复大标题横幅。路由懒加载，不把全部页面静态导入。
- M03：节点默认紧凑、健康/地区/基线延迟突出、平台详情展开；健康三态与任务态分开；保分页虚拟化/逻辑ID/来源与历史。选择总数和当前可见数明确，隐藏有效选择保留且发起前明确范围，可清除不可见项。浏览不发真实probe/refresh。URL恢复q/protocol/health与node详情，非法值安全回退，改变筛选重置分页，现稳定ID契约不变。
- M04：组总览紧凑、搜索动作同栏、能力说明按需，校验单一区域区分请求失败/业务阻断/过期；不再大段常驻矩阵。编辑单draft/250ms预览/generation/一次CAS保存，宽屏配置与预览共存、窄屏切换；提取有真实职责的draft/preview子模块。动态有序sources与高级流程不展平，unknown不负向匹配、风险硬约束/WARP软豁免保持；不恢复退役准入CRUD。列表无解析数显示待预览，不新增28个preview请求。
- M05：规则紧凑分类导航/移动分类选择，统计小型化，类型标签nowrap自然高度，保DIRECT/REJECT/PASS与组、顺序、470/14夹具及MATCH最后；维护回填移更多保确认，真实新建201/导出/删除闭环。
- M06：订阅源紧凑摘要/单行独立刷新反馈，基础/高级渐进展开，primary/自动命名/失败保资产/probe_policy/正则/重命名/条目覆盖保持；不通过UI重构改后端来源模型。
- M07：DNS保去横幅现编辑器与17键无损/unknown/outerenabled/CAS，表单分组，补路由/返回/beforeunload离开保护，异步模式切换防旧响应；一draft仅内存不持久敏感内容。
- M08：客户端订阅四目标直链复制下载保持，无强制发布对象；未知/422编译结果不能报下载成功；设置按访问安全/检测/全局筛选/外观分类，现Auth/Token开关保持。不静默隐藏真实URL/凭据已授权查看能力，默认显示或遮蔽依现行为，只改善布局而不另起安全需求。
- M09：概览以现API真实范围和下一步快捷入口展示；加载/错误/真实0区分，不编健康0或虚构检测。全站轻量toolbar/state/Modal/Drawer复用，短错误可追踪requestid，长说明折叠不隐藏阻断。
- M10：中英改动文案一致、7主题旧偏好不清除、localStorage失败不阻塞。只显示偏好可入localStorage，Token继续现Auth管理；DNS/订阅私有URL/凭据草稿不入query/localStorage/日志。Go API/数据库/业务模型不改。

## 公共接口与职责冻结

### UI基础(包A唯一写)

`ui/PageToolbar.vue`：props title?:string, description?:string；slots default(页面控件), actions；title默认不输出重复h1，页面可用sr-only heading；支持wrap/min-w-0。`ui/SectionState.vue`可选，仅真实复用时实现而非强制所有页面改造。

`composables/useUnsavedChanges.ts`：导出useUnsavedChanges(dirty:Ref<boolean>|ComputedRef<boolean>, message?:()=>string)。注册onBeforeRouteLeave及beforeunload；返回confirmDiscard():boolean。dirty false不拦；true确认取消保草稿。域自己的drawerclose调用confirmDiscard；守卫必须可在无Router组件测试中安全注册，不引入全局永久listener。基础机先写并通知模块可消费，模块不写该文件。

`locales/redesign.ts`：导出`uiText(zh:string,en:string):string`，使用同一currentLocale响应式读取(在template/computed调用)，让各域文件维护少量双语新文案，不并发改巨型messages.ts；已有文本继续t(key)。不以双语字符串在界面同时显示代替国际化。App/theme公共原locale文件仅A可改。

api/client.ts现Auth/CSRF/requestID与signal合同不变，域包不改。领域composable owns读写与请求失效，不创建万能缓存/store。

### 域接口(不改变现对外事件)

节点保useNodes/NodeRecord/normalizeNode及任务方法签名，行展示子组件用NormalizedNode和逻辑ID事件；filter URL只非敏感标量，使用router.replace不改全局hash同步。

policy保PolicyEditorSheet props/open/group/allGroups/availableNodes/conflict/cycle及save-group/close/reload等事件，子组件只通过父draft更新，不自保存；既有usePolicy接口不破坏。save payload完整sources+expected_revision，preview失败不自动开emptyfallback。

rules保useRules CRUD与rulesTypes、RuleID/position/category语义，展示选中分类不改底层排序。

subscriptions保useSubscriptions DTO与刷新/primary/probe_policy语义，抽屉guard就地复用，不增CAS到不存在revision合同。

DNS保持GET/validate/PUT expected_revision，只有该字段真实支持CAS，不因通用草稿抽象给所有接口编revision。

### 状态与失败

首次loading不是0；refresh保旧数据+失败说明；成功空才空态。最新query/实体/目标/session的generation才可写UI；关闭或切组使老preview失效。serverConfig/baseRevision与draft分开，422/409/网络错保输入，成功服务确认后resetdirty。queued/running/cancelled与健康分开，停止poll不等于取消任务。认证切换清理旧会话视图并防晚到response回写。

## 模块与可并行写集

A基础：web/package.json/package-lock.json、vite/postcss/tailwind配置、src/style.css、theme/**、App.vue、router/**、navigation.ts、ui/**、composables/useUnsavedChanges*、locales/redesign.ts及公共App/navigation/ui/theme相关tests。不写features；可读Auth导出与原view路由事实、useContentInset/useBodyScrollLock。公共配置/lock/巨型locale消息A独占。
B节点：features/nodes/**与其tests，只读probeTypes/history已有依赖、ui/composables/api/locales、node后端接口由主脑给事实，不写features/probes或全局文件。
C策略：features/policy/**与tests，只读ui/composables/api/locales、nodes/useNodes及settings/GlobalNodeFilterSettings既有接口，不写settings。自身candidate HTTP读取保持现范围。
D规则：features/rules/**与tests，只读ui/composables/api/locales及policyTypes必要目标事实，不写policy。
A/B/C/D写集独立，A公共合同已冻结，可并行；A未落文件时B/C/D可按接口先写consumer，不能独立造另一个guard或locale。编译统一云端。
E输入：features/subscriptions/**与tests，定向只读probe/types、ui、utilsuuid现依赖。F DNS：features/dns/**与tests。G输出设置：features/publications/**、features/settings/**与tests(共享全局筛选此包独占)。H概览：features/dashboard/**与tests。E/F/G/H可第二波并行，不把多个域全链塞给单机；A完成后均只读公共组件，不再跨写。
所有模块外未知/公用生成物/后端变更交主脑；不得扩大到整web/src。文件名按实际职责选择，不按LOC切微任务。

## 验证与场景

云必需jobs实际成功后同SHA镜像合成数据库/独立internal网(无默认出站)+端口+禁两scheduler，不挂生产数据。fixture至少含健康/异常/未知/任务态、实际geo与平台region区别、2订阅primary、多组有序来源/WARP/硬排除、470规则14类、17键复杂DNS。seed只隔离DB，不真实probe/refresh。

必须实际走：8页可达/旧deep link与back；392×872/768/1440×900深浅主题和弹层焦点/底部保存、200%字体；节点筛选URL和隐藏选择/探测确认不发真实外网；订阅编辑保存回读且高级配置保留；组条件修改自动preview→一次save→GET/reopen/actualclash一致与409草稿保留；GlobalAuto三步有序保持；规则470→471→470 MATCH末位；DNS可视/Raw未知键保留→保存GET/实际clash、outerflag、非法/409/离开guard；四导出可用或真实诊断、Clipboardfallback；设置各分类不误写认证；概览error不是0。未实测不勾选，不以HTTP200替代业务。

主脑统一git差异/secret核对→逐文件开发提交pushdev→CI日志；FAIL定向新提交再云复验。全部模块自测凭据在最终SHA云结果汇聚，独立reviewer读源与同SHAjobs，主脑风险核后派Critic真实PNG与流程(非手机实屏)。最后回收owned预览敏感副本，保日志/哈希，提交当前工件并QQ交付。生产上线另授权。

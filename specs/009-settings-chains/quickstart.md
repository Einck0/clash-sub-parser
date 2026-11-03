# Validation guide

1. 主脑精确清单提交并推 dev，现 .github/workflows/ci.yml 的前端类型/测试/构建、Go vet/tests/build及交付契约必须全成功，runtime镜像成功、promote skipped预期。本机不运行这些项目命令，也不本地Docker编译。
2. 拉取同 SHA 云镜像，核 digest/OCIrevision/binary，与 internal Docker network/合成空库、禁 subscription scheduler/probe coordinator、loopback端口隔离运行。准备者交 instance JSON、auth私有路径、测试数据边界与shutdownowner；不挂生产数据、不发真实刷新探测。
3. 合成数据：两订阅、至少 A/B/C 三节点和含 A 的跳板组/含 B,C 的目标组；正常凭据仅example.invalid；原规则含PROCESS-NAME两条和terminalMATCH，记录beforehash/count/order。
4. CSP：三个领域导航可达，修改公共访问地址/拉取方式/检测超时，save→readback；安全token仅合成库测试，旧token清理入口有效；theme/locale本机偏好不混服务端。切分类与离开取消保draft。
5. DNS：逐项主备引导、多行粘贴和YAML映射保存→GET/reopen/export深相等，17已知键+unknownnested；invalidYAML不write，真实带外CAS409保draft；outerfalse/rawtrue导出false但配置保留。
6. Chains：目标订阅→组→node优先级、同级sort、部分overlap(skipA,chainB)、node/group自指、两绑定传递环、失效引用、disable/delete、真实409draft；preview/save→GET/reopen→Clashdialer-proxy一致；其他三个target有链422，无链恢复既有行为。规则beforehash/count/order不变。
7. 工程独审同SHA云jobs与源码、主脑核关键风险后Critic直接read桌面/移动深浅PNG且真实完成上述用户操作。浏览器viewport不等物理手机测试。owner最终精确回收预览容器/网络/合成卷/relay/privateauth，保必要审计，未授权不得部署生产或推进prelease。

# Miyabi 审计与复查清单

> 复核日期：2026-10-02。基线：`v0.2.0 / e79cb1e`。
> 本轮逐项复核原清单 111 项，保留原编号；补充用户反馈第 245 项，共 112 项。已解决项移出待办，仅保留简短结论及代码位置。
> 首次复核只更新审计报告；后续已完成第 135 项目录缓存修复并同步结论。“确认存在”表示代码路径仍在，不代表已经在真实大库或浏览器中测得故障。可选重构不能计作已确认 bug。

## 复核结果

| 状态 | 数量 | 含义 |
| --- | ---: | --- |
| 确认仍存在，待处理 | 21 | 有明确代码依据的边界、状态反馈或行为问题 |
| 可选优化与整理 | 57 | 重复、风格或潜在成本仍在，但收益和方案需验证 |
| 需要用户实测 | 3 | 195、227 为视觉推断；245 为用户反馈，尚未定位 |
| 按现有约定暂缓 | 7 | 不恢复用户已取消的 UI、ARIA 或旧数据兼容要求 |
| 撤回原问题结论 | 11 | 原结论不成立、风险描述过度，或把设计选择当成 bug |
| 已解决或随功能删除失效 | 13 | 移出实施队列，保留回归能力 |

优先处理顺序：

1. **235、167**：STRM token 进入请求日志，以及图片代理缺少地址/体积约束。
2. **161、237**：登录限流的并发和存储边界、可信代理配置及 HTTPS 转发头。
3. **148、149、138**：订阅积压与通知、本地文件身份边界、重复限流。
4. **187、189、192、193、194、206、222、224**：已确认的界面状态和反馈问题；具体视觉效果由用户验收。

### 当前约定

- 只支持当前数据结构，不迁移旧数据库、旧刮削数据、旧检查点或旧任务。当前数据的重启恢复、失败重试、暂停恢复、标准 NFO 导入及索引必须保留。
- 已入库使用紫色 badge；Emby 关闭时隐藏更多设置，并沿用与代理一致的保存方式。
- 不恢复 toast 和媒体库同步/重建按钮的 tooltip，不新增 shadcn 原始组件之外的自定义 ARIA 描述。相关无障碍缺口是暂缓项，不写成“已经修复”。
- 固定圆角、手势按帧更新保留；独立预览缩略图已经回退。保留模糊背景，未经性能记录不把它认定为卡顿根因。
- 不增加番号特例黑名单、不因文件短或只有一个调用方而强制合并、不机械抽取通用组件。

### 本轮验证与限制

- 通过：`go test ./internal/api ./internal/app ./internal/database ./internal/tasks ./internal/library/... ./internal/emby ./internal/netx ./internal/monitor`。
- 通过：前端 `npm run test`，25 个文件 / 141 项测试。相关用例验证任务共享快照、索引、事务、来源隔离、重启恢复、Emby 通知和详情交互逻辑。
- 额外核对依赖实现：slog-gin v1.21.1 的 middleware.go 记录 RawQuery；Gin v1.12.0 的代理解析会在错误时保留已解析前缀。没有读取或输出生产令牌。
- 没有浏览器测试、真实上游/Emby 联调、race 检测或新的性能基准。旧报告中的毫秒数、倍数、对比度和固定积压耗时不再作为本轮结论。
- 第 135 项修复后通过：`go test ./internal/library/... ./internal/app`、`go vet ./internal/library/... ./internal/app`；新增覆盖跨目录过期回收（命中、未命中及请求失败）、目录数/文件总数上限、淘汰后重新读取、超大目录完整读取且不驻留、账号隔离和返回列表修改隔离。保留现有视频位置核验与发布回放回归。限额约束缓存记录数，不代表内存字节硬上限或真实大库性能结论。
- **独立工具链问题仍存在**：本地执行 `web/node_modules/.bin/shadcn --version` 仍报 `ERR_PACKAGE_PATH_NOT_EXPORTED`：MCP SDK 依赖访问 zod 的 `./v3` 导出失败。未安装或调整依赖；这不是应用构建失败，也不意味着需要恢复旧数据兼容。

## 确认仍存在，待处理

- [ ] **138.** ReadMetadata 在 downurl 请求后显式 limiter.Wait，随后又用带 panTransport 限流的 client.http 请求 CDN；无重试时两次 HTTP 请求合计消耗三个令牌。可将 sidecar CDN 读取交给 media client，并保留相同 UA、大小上限和超时。
  - 定位：`internal/pan/metadata.go:78`；`internal/pan/client.go`。
- [ ] **142.** 账号缓存失效时并发 verifyAccount 仍会各自 fetchAccount；refreshTokens 有并发合并，但 RefreshToken 明确拒绝后未标记该凭据版本失效，后续请求仍重复刷新。应区分凭据吊销与暂时网络失败，不能把所有错误都当登出。
  - 定位：`internal/drive/account.go:27`；`internal/drive/token.go`。
- [ ] **148.** 订阅检查仍每轮最多 10 部影片和 5 个演员，由 5 分钟周期触发；到期积压无法在一轮内排空。BatchHandler 与 EnqueueSingle 重复读取订阅，后者每项读取配置；无磁力且未过期的分支更新 checks/时间却不发业务通知。应优先处理积压调度和状态通知，保留请求节流；撤回固定“500 项约 2.8 小时”的估算。
  - 定位：`internal/monitor/check.go:18`；`internal/monitor/batch.go`；`internal/app/app.go`。
- [ ] **149.** 媒体库仍按 created_at DESC/id DESC 排序，Movie schema 没有对应排序索引。另有身份风险：本地 fileID 仅由相对路径哈希生成，不包含扫描根目录；切换根目录但相对路径相同时会命中旧记录，已有文件分支也未刷新名称、大小等字段。跨根目录影响需专门回归，不能沿用旧 local.go 行号。
  - 定位：`internal/library/library.go`；`internal/ent/schema/movie.go`；`internal/library/scan/local_ingest.go:83`。
- [ ] **161.** 登录限流 records 没有全局淘汰或容量限制，陌生 IP 可持续留下条目；check 与 recordFailure 分离，多个同时通过检查的请求会突破单批失败次数阈值。应验证并发预占/结算与过期清理，错误 cause 中写死 24 小时的文案也应统一。
  - 定位：`internal/api/rate_limiter.go:36`；`internal/api/auth.go`。
- [ ] **166.** installFrontend 仍将 fs.Stat 成功的目录交给 FileServer，可列出打包静态目录；带哈希资源未设长缓存头。应明确目录访问策略和静态缓存策略，不能将其夸大为任意服务端目录泄漏。
  - 定位：`internal/api/router.go:143`。
- [ ] **167.** 【优先】/api/image 经 Catalogue.Media 直接调用 FetchMedia，仅检查 https 和非空 host，未限制目标主机/IP，也未设置响应体大小上限；标准 transport 没有 SafeDownload 的地址防护。可用于访问服务器可达的任意 HTTPS 目标并占用大量内存。访问密码启用时需会话，未启用时可直接访问；图片 MIME 检查发生在下载之后。
  - 定位：`internal/javdb/media.go:26`；`internal/catalogue/service.go`；`internal/api/router.go`；`internal/netx/client.go`。
- [ ] **168.** GFriends Filetree.json 下载仍将 maxBytes 设为 0，本地缓存也直接 ReadFile，文件体积无上限。错误包装已修复，仅保留大小约束问题；应按实际索引规模制定上限而非套用头像大小。
  - 定位：`internal/gfriends/client.go:102`。
- [ ] **173.** 代理与 Emby 开关都通过未清理的 50ms setTimeout 聚焦输入，关闭或切换期间可能抢焦点；AccessGate effect 仍依赖整个查询对象，每次渲染重新注册监听。可在条件挂载时聚焦，并收窄 effect 依赖。
  - 定位：`web/src/features/settings/network-section.tsx:39`；`web/src/features/settings/emby-section.tsx`；`web/src/features/access-gate/access-gate.tsx`。
- [ ] **184.** export 仍有 ./data/emby 和硬编码 8080 的默认推导，与 config.Load 按 DataDir/Listen 的推导不一致；应集中默认值。当前正常初始化已传配置，风险主要在空值调用。前端 URL 规范化仍重复；OutboundIP 注释声称优先私网，但成功 UDP 路由路径可能先返回公网地址。
  - 定位：`internal/export/export.go:19`；`internal/config/config.go`；`internal/netx/ip.go`；`web/src/features/settings/emby-url.ts`；`web/src/features/settings/network-section.tsx`。
- [ ] **187.** 订阅设置加载中仍展示 true/preferred 等假默认值（虽已禁用）；网络、Emby、订阅读取错误仍没有直接重试按钮，任务页与媒体库重连文案不同。旧数据源页已删除；Emby 关闭时详情设置不再展示，应删除原“加载中同步头像必然显示开启”的描述。
  - 定位：`web/src/features/settings/subscription-section.tsx:61`；`web/src/features/settings/network-section.tsx`；`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/tasks-section.tsx`。
- [ ] **189.** 订阅状态语义仍不一致：卡片将 waiting/added 都视为已订阅，详情仅把 waiting 视为已订阅，added 展示“重新订阅”；两处强调样式也相反。演员“已订阅演员”按钮实际执行取消，文案未表达动作。应明确状态与动作语义。
  - 定位：`web/src/components/movie/movie-subscribe-button.tsx:13`；`web/src/features/movie-detail/subscription-action.tsx`；`web/src/features/discover/metadata-search-page.tsx`。
- [ ] **192.** 详情骨架仍画三个独立统计块，实际统计为合并 dl；预览圆角与磁力间距也不匹配，磁力骨架重复。加载完成时结构差异已由代码确认，具体跳动程度尚未浏览器复测。
  - 定位：`web/src/features/movie-detail/skeleton.tsx:15`；`web/src/features/movie-detail/hero.tsx`；`web/src/features/movie-detail/previews.tsx`；`web/src/features/movie-detail/magnets.tsx`。
- [ ] **193.** 分类重试 InlineError 未传 retrying，重试中缺少禁用/加载反馈。isLoading/isPending 和骨架圆角的差别仍在，但这两点只属一致性整理，不单独断言功能错误。
  - 定位：`web/src/features/discover/category-filters.tsx:66`；`web/src/features/discover/metadata-search-page.tsx`。
- [ ] **194.** SettingRow 所有描述仍使用 break-all，英文地址以外的普通文字也会被强制断词。控件宽度仍按场景不同；不应强制所有 Input/Select 同宽。
  - 定位：`web/src/features/settings/shared.tsx:45`；`web/src/features/settings/pan-directory-row.tsx`；`web/src/features/settings/subscription-section.tsx`。
- [ ] **206.** 网络、Emby、订阅设置的 mutation 错误仍直接展示 Error.message，fetch 网络异常可能以英文出现；describeApiError 已存在但这些路径尚未使用。优先统一用户错误文案，并核对 inline/toast 是否重复。
  - 定位：`web/src/features/settings/network-section.tsx:51`；`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/subscription-section.tsx`；`web/src/api/client.ts`。
- [ ] **222.** 清理缓存按钮的 busy 仍包含 info.isFetching，后台刷新统计会让按钮禁用；统计刷新按钮仍放在数据目录行。可分开操作中与后台刷新状态，防止交互无故不可用。
  - 定位：`web/src/features/settings/data-section.tsx:28`。
- [ ] **224.** 当前挂载路径仍由 disabled Input 展示，不能聚焦且继承禁用淡化样式。改为只读展示可支持正常聚焦/复制；不声称所有浏览器都绝对无法选取禁用文本。
  - 定位：`web/src/features/settings/pan-directory-row.tsx:33`。
- [ ] **234.** 仍有与 ARIA 无关的结构整理项：FloatingNav 的 ul 直接渲染 Link 而非 li；页码用 href="#"，前后页用 button。PreviewViewer 仍无 DialogTitle，开发态 Radix 提示需浏览器确认；补无障碍标题/自定义描述部分按现有范围暂缓。旧 hover span 问题已删除。
  - 定位：`web/src/components/floating-nav.tsx:25`；`web/src/components/list-pagination.tsx`；`web/src/features/movie-detail/preview-viewer.tsx`。
- [ ] **235.** 【优先】默认 slog-gin v1.21.1 直接将 Request.URL.RawQuery 写入 request.query，NewRouter 未加脱敏；STRM token 仍在 query 中，因此会写入访问日志。已核对依赖 middleware.go 的 query 构造，非仅推测。还应覆盖嵌套 image URL、Referer 与错误 cause；Emby API 调用现用请求头，不再列作 URL token 问题。
  - 定位：`internal/api/router.go:53`；`internal/api/strm.go`；`internal/api/error.go`。
- [ ] **237.** setAuthCookie 仍直接相信 X-Forwarded-Proto，未关联可信代理；config.Load 未校验 TrustedProxies，NewRouter 忽略 SetTrustedProxies 错误。已核对 Gin v1.12.0：解析失败返回已解析的 CIDR 前缀并写入，可能静默使用部分列表。应启动时拒绝无效配置，并按可信代理决定 HTTPS 转发头。
  - 定位：`internal/api/auth.go:139`；`internal/api/router.go`；`internal/config/config.go`。

## 可选优化与整理

- **136.** Activity 仍加载当前来源每个 hash 的最新下载记录，没有历史上限；前端有活动任务时每 5 秒轮询。存在随历史增长的查询成本，但不能直接截断长时间下载、待定位任务或失败记录；先测量，再拆分活动与历史读取。
  - 定位：`internal/offline/projection.go`；`web/src/api/offline.ts`。
- **137.** 批量订阅每项完成后编码并写入包含全部 IDs、FailedIDs 的 payload。全量 JSON 写放大仍存在，但 UI 已共享快照，不再是每个 SSE 连接各查一遍。优化应分离不变输入与检查点，不能简单降低持久化频率而丢失恢复进度。
  - 定位：`internal/monitor/batch.go`；`internal/monitor/batch_tasks.go`。
- **140.** 每次 STRM 请求仍查询索引并申请新的 115 播放地址。更正：GET/Range 走重定向，只有 HEAD 调用 CDN Probe；并非所有请求都探测 CDN。若增加短缓存，必须包含账号/凭据版本和 UA，并处理签名过期。
  - 定位：`internal/api/strm.go`；`internal/strm/relay.go`。
- **145.** ResolveMovieID 仍多次遍历累积候选，matchCandidates 重复 Normalize/IsFormatEquivalent。可缓存规范化结果；需保留层级优先、同 ID 去重和歧义拒绝，尚无当前负载收益数据。
  - 定位：`internal/javdb/movie.go`。
- **146.** selectRoute 非 full 模式仍使用 5ms ticker，存在高频唤醒。可按有效探测开始时间和当前最佳延迟安排 timer；必须保留启动耗时不计入请求延迟的语义。
  - 定位：`internal/javdb/route.go`。
- **147.** DetectLanguage 先将全文转换为 []rune 再截取 15000 字符，大字幕仍有不必要分配。可先有界遍历；约 40 字符 marker 的线性查找是否改 map 应另测，不能把它直接认定为瓶颈。位置已从旧 subtitle/label.go 更新。
  - 定位：`internal/domain/subtitle/label.go`。
- **151.** SSE 首个 changes 事件会将三类 revision 都判为变化并刷新相关查询，手动重连也先失效 tasks。存在首屏刷新合并空间；不能直接删除基线校正，否则查询与订阅建立之间的更新可能漏掉。实际重复请求次数需浏览器确认。
  - 定位：`web/src/features/tasks/task-events.tsx`。
- **152.** 任务、状态、离线及部分设置查询仍使用 refetchOnMount:'always'。这与 staleTime 共存本身不是错误；应结合挂载次数、SSE 断线恢复和缓存时效逐项测量，不统一删除。
  - 定位：`web/src/api/tasks.ts`；`web/src/api/movie-states.ts`；`web/src/api/offline.ts`；`web/src/api/query-client.ts`。
- **153.** 离线提交与扫描成功后仍有 setQueryData、invalidate 和 SSE 刷新交汇。更正：setQueryData 不发网络请求；useOfflineTasks 的影片筛选与来源校验仍有职责，enabled 也不是无效配置。仅评估重复失效调度，不删除跨来源保护。
  - 定位：`web/src/api/offline.ts`；`web/src/api/library.ts`。
- **154.** 媒体库和关键词搜索查询仍未提供 keepPreviousData，翻页会进入无缓存加载态；发现页查询已经使用。路由 intent 预加载仍未接查询 loader。是否预取及保留旧页应按请求量和交互实测决定。
  - 定位：`web/src/api/library.ts`；`web/src/api/discover.ts`；`web/src/router.tsx`。
- **156.** sourceSession 多个读取方法仍重复来源检查、令牌调用与完成后检查。若抽取公共调用须区分读操作和已提交的写操作，不把所有方法机械套入同一个模板。
  - 定位：`internal/drive/session.go`。
- **157.** JavBus MoviePage 与 fetchMagnets 仍有请求头、限流、响应读取及挑战页判断的重复；旧 ensureDetailParams 已只是解析委托。两条路径的 404、重定向和 Accept/Referer 语义不同，只提取相同部分。
  - 定位：`internal/javbus/client.go`。
- **158.** errorMiddleware 与 slog-gin 都会记录失败请求，domain error 的 cause 也可能与 error 字符串重复。需明确访问日志与诊断日志职责；敏感字段脱敏优先于减少日志条数（见 235）。
  - 定位：`internal/api/error.go`；`internal/api/router.go`。
- **159.** authMiddleware 仍直接构造两种 UNAUTHORIZED 响应。可以统一私有响应助手，但必须保留本站会话与 PAN_UNAUTHORIZED 的区别；当前重复未造成已知故障。
  - 定位：`internal/api/auth.go`；`internal/api/error.go`。
- **160.** Emby/网络测试接口仍先加载已保存配置再用请求体替换；订阅批量接口仍单独写 202，feed 复用含 kind 的查询结构。router 的 STRM 方法注册、gin 全局模式及命名整理均是可选重构；代理错误另归 237，不强制新增泛型 handler。
  - 定位：`internal/api/emby.go`；`internal/api/network.go`；`internal/api/subscription.go`；`internal/api/router.go`。
- **162.** Claim 仍先 SELECT 后条件 UPDATE，但已有队列锁、状态条件与资源顺序约束，不能据此判定重复领取。批次列表的近期+活动合并及 registry 检查仍存在；更改单条 SQL 前须证明收益并保留重试/暂停语义。
  - 定位：`internal/tasks/queue.go`；`internal/tasks/pool.go`；`internal/monitor/batch_tasks.go`。
- **163.** Bus、Drive 与 ProxyManager 的订阅/退订 API 风格仍不同；ProxyManager.Unsubscribe 仍遍历通道 map。可在相关模块修改时统一生命周期接口，不作为独立功能故障。
  - 定位：`internal/tasks/bus.go`；`internal/drive/events.go`；`internal/netx/proxy.go`。
- **164.** 部分 goroutine 仍使用 Add/Done，Kind 字符串转换风格不一。ContextLock 未持锁 Unlock 会阻塞，但本轮未发现生产误用路径；不将它写成已发生死锁。wg.Go 和锁误用诊断属于可选整理。
  - 定位：`internal/app/app.go`；`internal/magnet/aggregator.go`；`internal/syncx/lock.go`。
- **165.** 浏览记录裁剪仍是 Count、读取最旧 ID、Delete；可评估单条有界删除，但须保留 viewed_at/id 同序规则与单调时间。当前已有限额，未证明此处为性能瓶颈。
  - 定位：`internal/library/viewed.go`。
- **169.** apiPost/apiPatch/apiPut 的 JSON 请求初始化仍重复，imageURL 也仍位于 client.ts。可以小范围整理，但改参数形状或新增转发层不一定改善维护性，不以调用次数要求全面重写。
  - 定位：`web/src/api/client.ts`。
- **170.** 查询键仍分散：discoverKeys 位于 movie-detail-cache，pan.ts 使用 library/offline 字面量键，subscriptionSettingsKey 命名不同。优先消除实际重复键，不为集中管理引入循环依赖。
  - 定位：`web/src/api/movie-detail-cache.ts`；`web/src/api/pan.ts`；`web/src/api/subscription-settings.ts`。
- **172.** build 仍先 vite build 后 tsc --noEmit；类型错误会较晚暴露，但脚本仍失败，不能说会把类型错误当成功发布。可调整执行顺序。
  - 定位：`web/package.json`。
- **174.** 相对路径、@ 别名、barrel 与常量别名仍混用；MovieGrid 直接导入 DiscoverMovieCard，barrel 同名导出未被它使用。只在相关改动中清理，不把书写风格当运行缺陷。
  - 定位：`web/src/features/settings/tasks-section.tsx`；`web/src/components/movie/index.ts`；`web/src/components/movie/movie-grid.tsx`；`web/src/features/discover/category-content.tsx`。
- **175.** api 的 data/movie 文件名与 maintenance/library 职责仍不完全一致。可按资源整理，短文件和独立接口本身不构成拆分问题。
  - 定位：`internal/api/data.go`；`internal/api/movie.go`；`internal/api/router.go`。
- **176.** HEAD 编排仍经过 api → Relay.Probe → Drive.OpenMedia → pan。可考虑由 Relay 封装 HEAD 行为，但窄接口转发也有隔离价值，不自动删除层。
  - 定位：`internal/api/strm.go`；`internal/strm/relay.go`；`internal/drive/drive.go`。
- **177.** healthcheck 子命令仍先执行完整 config.Load，默认 PublicURL 推导会调用 OutboundIP；checkHealth 仍转发 App.CheckHealth。可把 CLI 所需的监听地址读取与探活实现收拢，避免无关初始化。
  - 定位：`cmd/miyabi/main.go`；`cmd/miyabi/healthcheck.go`；`internal/config/config.go`；`internal/netx/ip.go`。
- **179.** BrowseResults、SearchResults、MetadataSearchPage 仍重复把 query 字段传给 DiscoverResults。是否改成接收 query 对象需权衡组件与 React Query 的耦合，不强制删壳。
  - 定位：`web/src/features/discover/browse-results.tsx`；`web/src/features/discover/metadata-search-page.tsx`；`web/src/features/search/page.tsx`。
- **180.** ActorSubscriptions 仍透传 displayItems/displayQuery 与选择状态到 MovieSubscriptions。可减少 props 穿透；须保持未选演员时展示全部演员新作的 feed。
  - 定位：`web/src/features/subscriptions/actor-subscriptions.tsx`；`web/src/features/subscriptions/page.tsx`。
- **182.** catalogue.MovieDetail 仍在带 release_status 的 Movie 外声明 zone/推荐列表，直接嵌入 domain.MovieDetail 需保留额外字段；domain/library.go、source.go 命名仍宽泛。ScanPayload.Checkpoint 仍为嵌套 JSON 字符串，可在当前任务结构内简化，不引入旧任务兼容。
  - 定位：`internal/catalogue/types.go`；`internal/domain/library.go`；`internal/domain/source.go`；`internal/domain/scan.go`。
- **183.** app.New 的失败分支仍重复释放已创建资源；多源客户端加入后分支变多。当前关闭路径及对应测试存在，可评估局部清理栈，但没有证据支持“当前仍泄漏 worker”。
  - 定位：`internal/app/app.go`；`internal/app/deps_test.go`。
- **188.** scan-status 与 task-progress-state 仍维护两套阶段文案。当前扫描 toast 已显式传 scanStatus，所以原先“同一 toast 用两套文案”不成立；可统一剩余默认标签，保留暂停和自动重试优先级。
  - 定位：`web/src/features/tasks/scan-status.ts`；`web/src/features/tasks/task-progress-state.ts`；`web/src/features/tasks/task-toast.tsx`。
- **190.** success/info 仍直接使用固定色阶，失败 badge 覆盖 destructive，订阅选中使用 success。可以整理语义 token；已入库紫色是明确产品选择，不能当错误改回其他颜色。删除已移除的数据源警示颜色引用，不沿用未经本轮测量的对比度数字。
  - 定位：`web/src/styles/globals.css`；`web/src/components/ui/badge.tsx`；`web/src/features/library/movie-status.tsx`；`web/src/features/subscriptions/subscription-card.tsx`。
- **191.** 筛选 Select 使用 popper，订阅偏好仍使用 item-aligned 默认值，Select Viewport 还存在 position === 'popper' && '' 空表达式。可先清理空表达式，弹出方式由交互验收决定。
  - 定位：`web/src/components/ui/select.tsx`；`web/src/features/settings/subscription-section.tsx`；`web/src/features/discover/category-filters.tsx`。
- **196.** 部分 Button 图标仍重复声明默认 size，Emby/代理保存 spinner 仍有 mr-1.5 与按钮 gap 叠加。仅属样式整理；原数据源页静态 Loader 图标已随页面删除。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/src/components/ui/button.tsx`。
- **198.** 普通元数据链接仍仅在 hover 变灰，与相邻纯文本区别较弱；标签与 EntityRow 行高不同，部分 tracking-normal/换行类风格不一。属于视觉一致性候选，不能把合法 Tailwind 类名差别当 bug。
  - 定位：`web/src/features/movie-detail/metadata-link.tsx`；`web/src/features/movie-detail/metadata.tsx`；`web/src/features/movie-detail/magnets.tsx`。
- **199.** 任务时间、订阅时间和缓存数量仍在各处格式化。三者展示精度/类型不同；只抽取相同格式的 formatter，不要求日期和数字共用同一逻辑。
  - 定位：`web/src/features/settings/tasks-section.tsx`；`web/src/features/subscriptions/subscription-card.tsx`；`web/src/features/settings/data-section.tsx`；`web/src/lib/format.ts`。
- **200.** 网盘头像与演员头像的尺寸 API 不同；容量足够时剩余空间在进度条内与图例各显示一次，演员条骨架与按钮尺寸也不同。是否合并显示需视觉验收，未确认布局故障。
  - 定位：`web/src/features/settings/pan-account.tsx`；`web/src/features/subscriptions/actor-chip.tsx`；`web/src/features/subscriptions/actor-subscriptions.tsx`。
- **202.** 订阅卡片错误仍用原生 title，设置任务进度使用 Tooltip/OverflowTooltip，并可能重复展示同一错误。用户取消的是 toast 和媒体库两个按钮的 tooltip，这里不能误判为回归，也不自动扩展无障碍改动。
  - 定位：`web/src/features/subscriptions/subscription-card.tsx`；`web/src/features/tasks/scan-progress.tsx`。
- **204.** SettingsSection 仍由调用方传图标尺寸，设置路由手写分隔线且关闭返回顶部。图标重复和页面文件位置属于可选整理；“数据源与缓存都用 DatabaseIcon”已失效，刮削设置使用 ScanSearchIcon。
  - 定位：`web/src/features/settings/shared.tsx`；`web/src/routes/settings.tsx`；`web/src/features/settings/scraping-section.tsx`。
- **207.** Emby 与代理的测试连接/保存操作栏仍重复。若后续一起调整可以提取小组件，不要求为两个相似块新增复杂通用接口。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`。
- **208.** Emby 仍逐字段比较 form，代理分开保存 enabled/url 草稿；代理手写 Enter 保存，Emby 没有 form 提交。可以优先统一键盘提交；草稿抽取须保留保存后等待 query 同步的防闪回逻辑。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/tests/emby-settings.test.ts`。
- **209.** 清理缓存与批量入库确认弹窗仍重复 pending/关闭/错误/操作栏结构。可复用相同部分，但保留各操作的失败展示与关闭约束，不引入额外确认流程。
  - 定位：`web/src/features/settings/data-section.tsx`；`web/src/features/subscriptions/page.tsx`。
- **210.** 订阅偏好的三个 Select 仍分别书写。可用轻量配置减少重复；设置请求和失败语义保持原样。
  - 定位：`web/src/features/settings/subscription-section.tsx`。
- **211.** MovieCard 与骨架仍重复卡片外框及比例样式，多个详情触发器仍重复 class。实际卡片已共享 MovieCard，hover 外框已不存在；只保留骨架/外框同步候选，不新增多层 CardShell 包装。
  - 定位：`web/src/components/movie/movie-card.tsx`；`web/src/components/movie/movie-grid-skeleton.tsx`；`web/src/features/library/movie-card.tsx`。
- **213.** CategoryContent 与 CategoryFilters 仍重复派生有效分类；分区选择与 main 分类常量也有重复。可先抽纯计算函数，是否增加 hook/ZoneSelect 按调用复杂度决定。
  - 定位：`web/src/features/discover/category-content.tsx`；`web/src/features/discover/category-filters.tsx`；`web/src/features/discover/metadata-search-page.tsx`。
- **214.** 详情 section 标题仍重复少量 class，预览标题 id 没有引用。可清理未用 id；是否抽 DetailSection 属于可选整理，不能只因相同 HTML 就增加组件。
  - 定位：`web/src/features/movie-detail/previews.tsx`；`web/src/features/movie-detail/recommendations.tsx`；`web/src/features/movie-detail/magnets.tsx`。
- **215.** task-notification-diff 仍本地重写 isScanTask/isBatchTask/isTaskActive/isOfflineTaskActive。可共用纯状态判断，需考虑该模块独立测试与运行时依赖，避免为消除几行重复引入 API 层耦合。
  - 定位：`web/src/features/tasks/task-notification-diff.ts`；`web/src/api/tasks.ts`；`web/src/api/offline.ts`。
- **216.** 三种任务 toast 仍分别构造加载图标和断线文案，但扫描/离线现在额外考虑暂停，已不是完全相同的表达式。可统一文案，抽取时不能让已暂停任务继续转圈。
  - 定位：`web/src/features/tasks/task-toast.tsx`。
- **217.** 目录弹窗仍手写上一页/下一页，ListPagination 已支持无总页数模式。若复用应保留目录统计、正在请求时禁用和不滚动页面；不重新要求自定义 ARIA。
  - 定位：`web/src/features/settings/pan-directory-dialog.tsx`；`web/src/components/list-pagination.tsx`。
- **219.** app、library、offline、drive、strm 仍有多份 115 fixture，部分桩结构重复；不沿用旧的 1500 行统计。评估抽共享 transport/窄接口，保留各包的事务与来源边界断言。
  - 定位：`internal/app/drive_fixture_test.go`；`internal/library/fixture_test.go`；`internal/offline/fixture_test.go`；`internal/drive/drive_fixture_test.go`；`internal/strm/fixture_test.go`。
- **221.** 批量入库成功仍由 page 发“已创建任务”toast，全局任务通知随后也可能出现。两条反馈路径已确认，是否在当前时序重复显示需浏览器验收；失败只在弹窗内展示的旧修复保留。
  - 定位：`web/src/features/subscriptions/page.tsx`；`web/src/features/tasks/task-toast.tsx`。
- **225.** copiedHash 仍保留至下一次复制或组件卸载，无自动恢复；复制失败提示位于 section 底部。是否改短暂反馈、移近按钮属于交互优化，未影响剪贴板实际复制。
  - 定位：`web/src/features/movie-detail/magnets.tsx`。
- **228.** MediaImage 的 className 只作用于前景图，外层尺寸另由父容器约束。可改名 imageClassName 提高清晰度，但属性命名本身不是图片局部显示的已知原因。
  - 定位：`web/src/components/media-image.tsx`。
- **229.** 固定 rounded-2xl 已替代 FRAME_RADIUS 动态计算，缩放已按动画帧更新。仍可整理 React 类型导入、卡片固定预留宽度；不要把跨浏览器 scrollbar CSS 全部当重复代码删除。
  - 定位：`web/src/features/movie-detail/preview-viewer.tsx`；`web/src/features/movie-detail/preview-frame.ts`；`web/src/components/movie/movie-card.tsx`；`web/src/styles/globals.css`。
- **236.** Emby 设置 GET 和更新响应仍返回完整 API Key，页面 password Input 仅遮住显示。该接口位于受保护设置路由，不是无条件匿名泄漏；可评估只返回已配置标记及留空保留旧值的写入协议，属于密钥暴露面收缩。
  - 定位：`internal/api/emby.go`；`internal/emby/service.go`；`web/src/api/emby.ts`；`web/src/features/settings/emby-section.tsx`。
- **238.** prototype_test 仍复制解析/匹配流程，prototypeQueries 没有生产 Queries 的 FC2- 查询变体；测试副本通过不能证明生产解析正确。可迁到真实 JavDB fixture transport；两个语料加载器仍重复，原始语料作为独立预期应保留。
  - 定位：`internal/codeid/prototype_test.go:22`；`internal/codeid/codeid.go`；`internal/codeid/corpus_test.go`；`internal/codeid/random_corpus_test.go`。
- **240.** 继续按真实覆盖收益复核简单 helper/容器与跨包集成测试；不把健康检查、失败预算、恢复或事务测试因篇幅直接删掉。当前尚未逐个证明这些用例可移除，不能写成已确认测试冗余。
  - 定位：`internal/tasks/tasks_test.go`；`cmd/miyabi/healthcheck_test.go`；`internal/app/pipeline_e2e_test.go`；`web/tests/pan-login-poll.test.ts`。

## 需要用户实测

- **195.** 115 登录弹窗关闭立即卸载 LoginContent，目录选择器保留内容直到退出动画结束。代码差异存在，但“动画塌陷”尚未在当前浏览器验证，不作为已复现故障。
  - 定位：`web/src/features/settings/pan-login-dialog.tsx`；`web/src/features/settings/pan-directory-dialog.tsx`。
- **227.** index.html 未提供 favicon、theme-color 或预先设置主题的脚本；暗色首屏闪白需浏览器冷启动确认。更正：缺 favicon 不必然返回 404，当前 SPA fallback 可能返回 index.html。不得把未测试的视觉现象写成已复现。
  - 定位：`web/index.html`；`web/src/main.tsx`；`internal/api/router.go`。
- **245.** 补充用户反馈：部分卡片封面只显示一部分且不恢复，尚无具体影片与浏览器复现定位。应核对返回图片完整性、decode/onLoad 状态及容器尺寸；不能声称独立预览缩略图回退已修复它，也不能直接归因于模糊背景。
  - 定位：`web/src/components/media-image.tsx:69`；`web/src/components/movie/movie-card.tsx`。

## 按现有约定暂缓

- **155.** 卡片模糊背景仍存在；每卡片 matchMedia 的旧 hover hook 已删除。按用户选择保留模糊效果，尚无浏览器性能证据证明它是滚动卡顿原因，不据此要求移除。独立预览缩略图已按用户要求回退，不能重新列为必做优化。
  - 定位：`web/src/components/media-image.tsx`；`web/src/features/movie-detail/previews.tsx`。
- **186.** Emby 已按用户要求与代理一致：开启后展开并保存，关闭立即提交；本地外观、即时订阅设置与文本表单职责不同。不再强制统一所有设置为即时保存，也不恢复已删除的 JavDB 线路设置。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/src/features/settings/subscription-section.tsx`。
- **205.** 撤回强制引入同时附加 aria-label 和 Tooltip 的 IconButton。用户已明确移除 shadcn 原始组件之外的 ARIA 描述，并取消 toast/媒体库动作 tooltip；不能以复用为由重新添加。其他现存 Tooltip 是否抽取单独评估。
  - 定位：`web/src/features/library/scan-button.tsx`；`web/src/features/tasks/task-toast-actions.tsx`。
- **230.** 图标控件缺少自定义可访问名称的事实仍在，但属于用户已决定不补充的自定义 ARIA 范围；不标记“已修复”，也不重新添加标签或 tooltip。旧数据源页和已删除的控件引用移除。
  - 定位：`web/src/components/floating-nav.tsx`；`web/src/features/movie-detail/preview-viewer.tsx`。
- **231.** MediaImage 仍没有 alt，预览按钮没有自定义 ARIA 名称。按当前无障碍范围约定暂不扩展此工作；标准 shadcn 内部行为保留，未把这个选择称为无障碍已达标。
  - 定位：`web/src/components/media-image.tsx`；`web/src/features/movie-detail/previews.tsx`。
- **233.** SettingRow 标题仍是 div，部分输入只提供 placeholder，缺少程序化名称；按用户移除自定义无障碍描述的要求，本轮只记录现状，不建议自动补 label/ARIA。
  - 定位：`web/src/features/settings/shared.tsx`；`web/src/features/search/page.tsx`。
- **244.** 非番号文件名可能符合通用番号模式的问题继续暂缓。用户已拒绝相机前缀、时间戳与分辨率硬编码黑名单；原示例 IMG-XXXX 没有数字，不能据原报告当作已复现样本。后续只在真实样本与通用规则基础上评估。
  - 定位：`internal/codeid/codeid.go:38`。

## 撤回原问题结论

- **141.** 撤回“ValidateSafeURL 只做语法检查”的直接建议。请求前、重定向、直连拨号各有安全职责；走代理时拨号目标是代理，拨号检查不能替代目标 URL 检查。也不能将每个请求固定描述成三次 DNS 查询。
  - 定位：`internal/netx/ssrf.go`。
- **171.** 撤回将回滚后 invalidate 或不同乐观更新策略直接列为问题。Emby 已明确可能先保存再在后续操作报错；重新读取用于校准最终服务端状态。网络设置也有事务/持久化边界，尚无证据证明这些校准多余。
  - 定位：`web/src/api/emby.ts`；`web/src/api/network.ts`；`internal/network/service.go`。
- **178.** 仅因 Source 接口单独放在 source.go 而要求合并不成立；它仍是磁力聚合器的有效依赖边界。旧 netx/probe.go 与 api/javbus.go 已不存在。
  - 定位：`internal/magnet/source.go`。
- **181.** movie-status 和 metadata-link 都有明确业务展示职责；单调用方或文件短不是问题。movie-states 已承担批量状态加载，本轮不建议为减少文件数而合并。
  - 定位：`web/src/features/library/movie-status.tsx`；`web/src/features/movie-detail/metadata-link.tsx`；`web/src/api/movie-states.ts`。
- **197.** 独立详情页与 hover 详情已删除，旧 max-w-7xl 与悬浮详情字号问题失效。剩余空状态高度、嵌套容器 gap/space-y 不能仅凭 class 判为重复间距，需有实际视觉差异再立项。
  - 定位：`web/src/features/library/page.tsx`；`web/src/features/discover/results.tsx`；`web/src/features/movie-detail/dialog.tsx`。
- **201.** cursor-pointer 单独一行或写在基类中运行等价，不是问题。将其统一到全局 CSS 可能改变其他控件行为，无需为了书写位置重构 shadcn 组件。
  - 定位：`web/src/components/ui/tabs.tsx`；`web/src/components/ui/select.tsx`；`web/src/components/ui/switch.tsx`。
- **203.** 访问密码页用 bg-muted、内部页面用 bg-background 是独立页面的设计差异，未发现功能或一致性约束要求两者相同。
  - 定位：`web/src/features/access-gate/access-gate.tsx`。
- **218.** 本地与远端 NFO 均调用 nfo.Decode，已非两套 XML 解码。剩余规范化/缺号回退分别依赖本地媒体番号和远端 sidecar 文件名，差异有业务语义，不强制合并。
  - 定位：`internal/library/scan/local_ingest.go`；`internal/library/scan/nfo.go`；`internal/nfo/movie.go`。
- **220.** 保留事实：清理未使用缓存需确认，退出/取消挂载/取消订阅多为直接执行。但“所有操作统一 ConfirmDialog”是产品决策，不是已证实正确性问题，也与避免多余确认的偏好不符，不据此增加弹窗。
  - 定位：`web/src/features/settings/data-section.tsx`；`web/src/features/settings/pan-section.tsx`；`web/src/features/subscriptions/actor-chip.tsx`。
- **226.** 搜索无关键词时不展示结果、返回按钮无历史时去 /discover 都是有效产品选择。没有证据表明 px-3 构成布局缺陷，不保留为强制待办。
  - 定位：`web/src/features/search/page.tsx`；`web/src/components/page-back-button.tsx`。
- **232.** 撤回旧的原生 <a><button> 嵌套和整卡 Button 包 Checkbox 描述：现在是 div 型 MovieDetailTrigger，内部按钮阻止冒泡，选择 Checkbox 是兄弟节点。卡片与 Checkbox 双 Tab 停靠点仍存在，属于键盘交互选择，不能继续沿用原生嵌套 bug。
  - 定位：`web/src/features/movie-detail/detail-trigger.tsx`；`web/src/components/movie/movie-card.tsx`；`web/src/features/subscriptions/subscription-card.tsx`。

## 已解决或随功能删除失效

- [x] **126.** 工作流 JSON 表达式索引与按通知版本共享的任务快照已实现；索引现在随当前数据库初始化创建，不再描述为旧库迁移。
  - 定位：`internal/database/task_indexes.go`；`internal/app/task_views.go`；`internal/app/task_snapshot_test.go`。
- [x] **127.** UI 通知与工作线程唤醒已分离。修正旧记录：当前 Finish 会 WakePool，这是结束资源占用、让后续任务继续执行所需；不能恢复旧的“Finish 不唤醒”要求。
  - 定位：`internal/tasks/bus.go`；`internal/tasks/queue.go`；`internal/tasks/pool_test.go`。
- [x] **128.** 当前 schema 保留 type/status、重试时间、资源键与历史 type 索引；领取与历史查询的索引测试通过。旧索引迁移已取消。
  - 定位：`internal/ent/schema/task.go`；`internal/database/task_indexes_test.go`。
- [x] **129.** Emby 增量通知只调用 /Library/Media/Updated，没有每批再请求 /Library/Refresh。
  - 定位：`internal/emby/service.go`；`internal/emby/client.go`。
- [x] **130.** MatchMovies 已使用 canonical_code 等值查询与分组匹配；schema hook 维护派生键，当前写入和重启测试通过。
  - 定位：`internal/library/scan/movies.go`；`internal/ent/schema/movie.go`；`internal/database/movie_keys_test.go`。
- [x] **131.** 本地导入已按 32 项分批，文件读取与图片准备在数据库写事务外，资料、文件、字幕及通知在批内提交。
  - 定位：`internal/library/scan/local_ingest.go`；`internal/library/scan/local_batch_test.go`。
- [x] **132.** 收尾按 100 部分页准备导出，再核对版本、批量入队并原子提交；导出已直接读取完整 metadata，不再需要 actors/tags 关联预加载。
  - 定位：`internal/library/scan/reconcile.go`；`internal/library/scan/reconcile_test.go`。
- [x] **133.** 独立 cover 任务、scrape_task_id 查询及专用索引已经删除。当前 scrape 自身保存 metadata_ready、artwork、completed 检查点，本项随架构替换失效。
  - 定位：`internal/tasks/kind.go`；`internal/library/scrape/scrape.go`。
- [x] **134.** 图片保护锁仅覆盖图片生成和检查点保存；下载及发布在锁外，缓存清理保留未完成任务引用。当前扫描与刮削已分池，旧报告的“单个媒体库 worker”描述失效。
  - 定位：`internal/library/scrape/artwork.go`；`internal/maintenance/service.go`；`internal/app/task_workers.go`；`internal/app/cover_lock_test.go`。
- [x] **135.** 目录缓存已限制为最多 128 个目录、合计 20,000 条文件记录，超限淘汰最早写入的列表；45 秒 TTL 到期项在下一次缓存访问或写入时回收，空闲时仍受容量约束。单个超过限额的目录完整读取但不缓存。网络请求仍在锁外，账号隔离、返回列表副本、索引变化后的缓存清空与发布前视频位置核验保留。
  - 定位：`internal/library/scrape/scrape.go`；`internal/library/scrape/dir_cache_test.go`；`internal/library/scrape/publish.go`。
- [x] **139.** 115 sidecar 上传实现已删除，已无逐文件申请 OSS STS token 的生产路径；不再增加 token 缓存。
  - 定位：`internal/pan/client.go`。
- [x] **212.** movie-hover-details 已删除，详情统计栏与悬浮卡片之间的重复不存在。
  - 定位：`web/src/features/movie-detail/hero.tsx`。
- [x] **239.** 旧 115 字幕、旧封面 payload 和历史迁移测试已清理；dir_cache、snapshot、检查点、来源隔离与事务测试仍验证当前功能，不按旧报告要求删除。剩余跨包测试归属统一放入 219/240。
  - 定位：`internal/library/scrape/dir_cache_test.go`；`internal/library/scrape/snapshot_test.go`；`internal/library/scrape/cover_checkpoint_test.go`；`internal/database/sqlite_test.go`。

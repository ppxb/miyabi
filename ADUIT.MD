# Miyabi 审计与复查清单

> 初始审计：2026-09-27，基于提交 `f79e501`。
> 清单整理：2026-09-29；此前已移除 62 条已勾选记录及 63、86、223 三条已失效待办。当前保留 179 条：150 条待处理，29 条已完成、待整体复查。
> 范围：后端 Go（不含 ent 生成代码）与前端；测试仅评估覆盖收益、重复和职责归属。
> 原编号保留，不重新编号；本轮起新完成的条目保留在文档中，勾选并补充实现与验证说明，供全部完成后整体复查。此前清理的记录可通过 Git 历史查看。第 37 项未完成的导出默认值清理保留在第 184 项。
> 本轮仅核对受近期改动影响的条目，并非重新验证全部待办。保留的旧行号仅作定位线索；✔ 表示初审时复核过，不代表当前仍成立。

## 处理顺序与原则

- **优先核实行为与安全问题**：235、237（敏感日志与代理信任）、167（图片抓取边界）、244（非番号误识别）。
- **再处理明显的阻塞与重复工作**：61、129、131、142、148、149；按当前调用链和测试确认影响。
- **结构整理按模块进行**：60、64、65、184；保留任务分池、重启恢复、错误退避和协议兼容语义。
- **前端优先保证交互与可访问性**：186–189、206、220–221、230–234，再处理布局与样式统一。
- **死代码与测试一起核对**：78–83、85、93–109、238–240。只有确认失去生产职责和有效覆盖后才删除；不能只凭文件短、单实现或前端未调用下结论。
- **方案是候选，不是强制指令**：已有公共实现优先复用；历史迁移的删除需先明确支持的升级起点，不能以本机迁完代替兼容策略。
- **新完成的条目不删除**：保留原问题，勾选并记录实现、验证结果及未验证范围；勾选表示实现完成，不表示整体复查通过。混合条目保留已完成与未完成部分的状态说明，全部完成后再统一复查和决定归档。编号互引若指向此前已移除项，可在 Git 历史中追溯。

---

## 一、正确性 / 并发 Bug

- [ ] **244.** `internal/codeid/codeid.go:38-55,59-93` `filenamePattern`：正则前缀匹配规则对个别非番号文件名存在误提取（如相机默认命名的 `VID-20220326`、`IMG-XXXX`、包含分辨率标签的 `MAYLI-1080P` 等被当作番号提取并提交上游查询）→ 建立常见非番号前缀黑名单（如 `VID`, `IMG`, `DSC`, `MOV` 等）与分辨率标记过滤，避免无效请求与限流损耗。

---

## 三、上帝代码

- [x] **58.** `internal/emby/service.go` 集中承担配置持久化、HTTP 请求、批量通知、全库刷新、演员头像同步和后台 STRM 重写，职责过多；需收敛请求构造、认证、启用条件及退出 flush 的重复。
  - 后续完善：Emby 通知改为数据库按目录持久化、版本去重和退避重试（15 秒起，最长 5 分钟），成功后条件删除；手动媒体库/本地扫描触发补发，重启保留待办。事务内导出通知随事务提交，更新期间的旧成功/失败不覆盖新版本。按用户要求静默运行，不新增页面状态或提示；全量 Go 测试通过，未做真实 Emby 联调。
  - 实现：提取 `internal/emby/client.go` 统一 HTTP 请求、X-Emby-Token 认证和错误处理；提取 `actors.go` 独立管理同步定时、单次执行、取消和退出等待。Service 保留配置与通知协调；配置变更取消旧同步，每轮同步使用同一份服务器配置。合并通知 flush，关闭时补发缓冲通知。
  - 验证：`go test ./...` 通过，新增请求格式、同步取消与等待、服务器切换、退出通知回归测试。未运行 race 检测（当前 CGO 关闭），未进行真实 Emby 联调；待整体复查。每批通知后的全库刷新策略仍按第 129 条处理。
- [x] **59.** `internal/library/scan/walker.go:76-263` `Scanner.Run` 约 190 行：参数校验、定向单文件、断点恢复、BFS、限速、NFO、分块入库、进度上报 + 两个闭包；:77 多余别名 → 拆 `runTargetFile` / `walk` / `indexDirectory`。
  - 实现：Run 保留 payload 校验、数据源会话和 scanID 初始化，单次扫描状态由私有 scanRun 承载；runTarget、walk、indexDirectory 分别负责定向扫描、断点与 BFS 遍历、单目录分页及分块入库。复用现有事务与进度保存函数，保持检查点格式、取消和导出锁边界。
  - 验证：library（含 scan/scrape）、app、offline、tasks 测试通过；新增 205 个视频在第二页取消后恢复的回归测试，验证未提交不完整目录、scanID 沿用、跨入库批次文件与进度完整。未进行真实 115 联调，待整体复查。
- [x] **60.** `internal/drive/drive.go:69-90` `Drive`：二维码登录状态机 + token 刷新 + 账号缓存 + 挂载持久化 + 事件 + Session 签发 + 工作计数，含 2 mutex、1 ContextLock、2 singleflight、1 WaitGroup、3 个版本号 → 拆出二维码登录（约 180 行）。
  - 实现：二维码登录状态机提取到 login.go 的私有 qrLogin，集中管理会话、轮询合并、超时、状态变化与凭据兑换；Drive 保留凭据提交、版本更新、账号缓存及挂载一致性。共用原有状态锁和提交锁，保持对外接口及关闭等待语义。
  - 验证：drive、library（含 scan/scrape）、offline、app、api 测试通过；保留轮询共享、取消、过期及旧凭据防覆盖测试，新增二维码生成请求晚返回与断开/新登录交错的回归测试。未运行 race 检测或真实 115 扫码联调，待整体复查。
- [x] **61.** `internal/gfriends/client.go:78-143` `EnsureIndex` 持写锁执行两个镜像的网络下载（各 30s），期间所有 `Lookup` 阻塞；`FetchAvatar`（208-234）复制镜像循环；stale 回退（136-141）把 `loadedAt` 设为 now，一次故障后 7 天不重试。
  - 实现：复用 ContextLock 串行化并发刷新，磁盘读取、下载及索引构建放在读写锁外，短暂加锁替换索引；保留旧索引和磁盘修改时间，下载失败独立退避 5 分钟，取消不计入失败退避。索引与头像共用镜像下载逻辑，索引解析失败继续尝试下一镜像。
  - 验证：gfriends、emby、app 测试通过；新增刷新期间查询、并发请求合并、等待取消、缓存年龄、旧缓存回退、失败退避与恢复、取消后重试及镜像回退测试。未运行 race 检测或真实 GFriends/Emby 联调，待整体复查。
  - 复查修复：索引解析后必须含可用头像条目，远程和磁盘缓存共用校验；无效响应继续尝试下一镜像，全部失败时保留旧内存/磁盘缓存并短期退避。新增空对象、null、空目录和空目标、旧缓存保留及无效磁盘缓存恢复测试；gfriends、emby、app 测试通过。
- [x] **62.** `web/src/features/library/page.tsx` 仍内联扫描状态推导、响应式 Tooltip 按钮和 mutation 回调；错误文案已使用公共 describeApiError。评估提取 LibraryScanButton，以减少页面编排中的业务细节。
  - 实现：提取媒体库专用 LibraryScanButton，集中按钮文案、图标、禁用状态、响应式 Tooltip、扫描提交及通知。页面保留当前数据源的扫描任务匹配与扫描状态，供按钮和空列表共享，并通过 onStarted 回调切回第一页。
  - 验证：现有 Vitest 107 项测试、typecheck、lint 通过；保持原有文案及交互分支。本次未新增组件交互测试，浏览器交互与响应式效果待用户验证，待整体复查。
  - 复查修复：LibraryScanButton 始终挂载，内部切换加载占位或隐藏状态，避免翻页导致 mutation 回调丢失。新增组件结构与回调单元测试，验证各分页查询状态下控制组件位置稳定、hook 保留和成功/失败回调；Vitest 110 项、typecheck、lint 通过，真实浏览器分页交互仍待用户验证。

---

## 四、过度设计

- [x] **64.** `internal/tasks` 的 Service / Queue / Bus / Registry / Pool 及 handler 包装层仍可评估简化。目前已分离媒体库任务池与订阅批量任务池，不能再以“只有单 worker”为由合并 Runner；保留分池、取消、恢复和数据库错误退避语义，重点检查 Registry 锁及 handler 包装是否有实际收益。
  - 实现：Handler 改为包含 Kind、Handle 和可选 Finished 函数的结构体，删除接口适配包装及完成回调类型断言；Registry 继续按值存储并保留读写锁。Service、Queue、Bus、分池执行及其取消、恢复、数据库错误退避保持原有职责。
  - 验证：tasks、app、library（含 scan/scrape）、offline、monitor 测试通过；新增无完成回调、事务提交与回滚测试，确认任务状态和回调数据原子提交，回滚不发布通知或增加版本号。未运行 race 检测，待整体复查。
- [x] **65.** `internal/library/scan/reconcile.go:20,27-46` `embyOpts ...any` 按 string 出现顺序解析 embyDir / publicURL / strmToken，类型不安全；`ProcessScanPageTx` 9 个参数 → 改为 `*Scanner` 方法。
  - 实现：生产入库与收尾逻辑改为 scanRun.processPageTx / reconcileTx，直接使用任务状态和 Scanner 依赖，scanID 统一读取 payload.ScanID。删除 embyOpts ...any 及位置解析，显式传入 export.Config；保留数据源 Session 提交与导出配置锁边界，跨包测试使用的事务入口同步改为类型明确的参数。
  - 验证：library（含 scan/scrape）、offline、app 测试通过，覆盖断点恢复、入库及导出清理回滚、离线任务并发更新；已有导出配置与通知测试同步迁移。未进行真实 115/Emby 联调，待整体复查。
- [x] **66.** `web/src/api/movie-detail-cache.ts`（202 行）+ `discover.ts:160-202`：Query 缓存外再维护 `WeakMap<QueryClient, Map>` 全局卡片索引，订阅 QueryCache 每次 `removed` 全量重建；带引用计数、并发上限 2 的请求队列；`useSyncExternalStore` 手动订阅 → `placeholderData` 从 `getQueriesData` 查找 + queryFn 内约 10 行信号量；`discover.ts:163-165` 的 `prioritize` 多余。
  - 方案修正：JavDB 推荐仅提供基础引用，卡片所需标题等字段必须补全；保留可见性触发、后台两路并发、共享消费者、撤销未开始请求、点击优先及导航请求复用。placeholderData 不能替代跨查询订阅，不按原候选方案强行删除调度和 useSyncExternalStore。
  - 实现：删除额外 WeakMap 卡片索引、永久 QueryCache 订阅和全量重建，直接从 Query 缓存选取最新卡片；集中卡片查询范围与订阅过滤，订阅随组件卸载释放，忽略无关查询移除。列表替换后不再残留已移除卡片，完整详情缓存不混入列表数据。
  - 验证：Vitest 113 项、typecheck、lint 通过，保留并发/优先/取消/导航复用测试，新增列表替换与移除、失效缓存补全、相关事件订阅与释放测试。未进行浏览器交互及真实 JavDB 联调，待整体复查。
  - 复查修复：可见性 request 在初次命中缓存时也保留临时订阅；列表替换/移除导致数据缺失时重新进入同一补全队列，离屏释放订阅与未开始请求。新增缓存消失后补全、双并发限制、共享消费者及移除后立即离屏测试；全量 Vitest 115 项、typecheck、lint 通过。
- [x] **67.** `web/src/api/browse-history-store.ts`（199 行）+ `browse-history.ts`：为"已浏览"徽章做 localStorage 持久化 + 服务端合并 + 50 条批量 + 10s 防抖 + beacon + 5000 截断（:178-183，服务端返回全部，不一致）；测试专用 `unref`（:136-139）进入生产代码；:47 注释引用已不存在的 `REFACTOR_PLAN`；`browse-history.ts:13` 每卡片 effect 调 `init()`，:15 `subscribe` 每次渲染新建 → `useQuery` + 乐观 `useMutation`，整个 store 删除。
  - 核实与取舍：当前服务端同样限制 5000 条，原文“服务端返回全部”已不适用。采用服务端记录与 Query 缓存；用户已接受未提交成功就关闭页面的记录不再跨重启补发，旧 localStorage 待提交内容不再读取。
  - 实现：删除 BrowseHistoryStore、localStorage 合并、延迟批量、退出 beacon 和手动订阅；共享 useQuery 获取记录，useMutation 立即提交并短期重试网络/服务端错误，useMutationState 提供提交中乐观徽章，失败不回滚其他记录。成功后仅更新缓存，保留服务端同步时间，过期后随页面挂载或窗口聚焦同步；查询合并请求期间新确认的记录，避免首次加载覆盖新记录，本地缓存同样限制 5000 条。保持空 ID 和同 ID 进行中提交去重。
  - 验证：初次迁移生产 build 通过；后续减少全量刷新优化经 Vitest 115 项、typecheck、lint 验证，覆盖查询共享、立即提交、并发成功/失败、首次 GET 合并、提交不触发 GET、不延后缓存过期、服务端淘汰替换、5000 条上限及查询/提交重试。浏览器徽章、跨页面及网络切换交互待用户验证，待整体复查。
- [x] **68.** `web/src/lib/pan-login.ts:11-19` `panLoginShouldRetry` 等价于 `retry: 4`，却配 146 行测试解释 react-query 内部行为；115 专用逻辑不应在 `lib/`。
  - 实现：登录查询配置收拢到 pan.ts 的 panLoginOptions，usePanLoginStatus 直接复用；使用 retry: 4 保持首次请求加四次重试的预算，删除 lib/pan-login.ts。保留 1500ms 间隔、禁用重连刷新及错误/终态停止轮询。
  - 验证：测试直接使用生产 options 和 API 查询函数，mock fetch 配合 Vitest 模拟时间，覆盖五次失败后停止、短暂失败保留 scanned、三个终态、waiting→scanned→authorized、URL 编码及空会话不请求。全量 Vitest 112 项、typecheck、lint 通过；未进行真实扫码与浏览器断网联调，待整体复查。
- [x] **69.** `internal/drive/events.go` 整套 eventBus（RWMutex + 自增 ID + map + 取消函数）只有 `library.go:80` 一个订阅者且丢弃取消函数 → 单个 listener 字段。
  - 实现：Drive 保存单个 MountListener，通过 SetMountListener 替换或清除；删除 eventBus、独立锁、ID、map 和取消订阅机制。复用状态锁读取回调后释放，再在原提交锁范围内同步执行；保持挂载失败恢复和清除目录通知语义。
  - 验证：drive、library（含 scan/scrape）、offline、app 测试通过；新增回调替换/清除与读取状态无死锁测试，原有重复挂载、回调拒绝回滚及扫描入队测试通过。未运行 race 检测或真实 115 联调，待整体复查。
- [x] **70.** `internal/netx/ssrf.go:216-236` functional options 只剩 `WithAllowLoopback`（测试开关，且 dialer 仍拦 loopback，所以 `subtitle.WithAllowLoopbackForTesting` 被迫换直连 client）；`WithMaxBytes` 仅测试使用。
  - 实现：删除 DownloadOption、WithMaxBytes、WithAllowLoopback 及字幕 WithAllowLoopbackForTesting；URL 校验不再暴露放行回环地址的参数，SafeDownload 固定 5MB 上限。字幕导出测试使用公开 IP 形式的 URL 与模拟 Transport，不再通过生产开关替换安全客户端；保留生产连接、重定向及代理校验逻辑。
  - 验证：netx、subtitle、app、library（含 scan/scrape）测试通过；实际验证 5MB/超出 1 字节边界，新增直接拨号拦截测试，保留本机 URL、重定向至本机、重定向次数、本地代理及字幕导出覆盖。未进行真实字幕来源联调，待整体复查。
- [x] **71.** `internal/pan/client.go:91-98` 与 `:137-146` Retry-After 两套：`panTransport` 全局 `retryAfter` 窗口 + resty `SetRetryAfter` 各等一遍。
  - 核实：原来的两层等待通常重叠，并非必然双倍；实际风险是共享截止时间延长后，已等待请求未重新检查。
  - 实现：删除 Resty SetRetryAfter 回调，由 panTransport 统一解析并执行共享等待，在退避与限速等待后重新检查截止时间；保留重试次数、条件、普通退避、限速和并发上限。
  - 验证：pan、drive、offline、app 测试通过；使用 testing/synctest 验证退避/限速期间延期、取消释放并发名额，并通过生产 New 客户端验证 Retry-After: 5 只等待 5 秒。原有重试限速、并发上限测试通过；未进行真实 115 限流联调，待整体复查。
  - 复查修复：取消外层 http.Client 总超时，在传输层完成并发/限流等待后创建 35 秒网络 I/O deadline，覆盖响应头和响应体读取，EOF/Close 释放计时器；仍继承调用方取消与截止时间。新增 GET/POST 的 60/120 秒 Retry-After、响应头/体超时与计时器释放测试，pan、drive、offline、app 测试通过。
- [x] **72.** `internal/pan/client.go:59-62`、`play.go:29-36` 用魔法字符串 `"__EMPTY__"` 作 UA 哨兵，在 RoundTrip 中改写 req（违反 RoundTripper 约定），`Header.Del` 后紧跟赋值多余；`pan/offline_test.go` 被迫复制该逻辑 → resty `SetPreRequestHook`。
  - 核实与实现：请求改写此前已迁移到 SetPreRequestHook，RoundTrip 与测试不再复制 UA 改写。本次将三个位置的特殊字符串统一为私有 emptyUserAgentSentinel，保留显式空 UA 不被 Resty 默认值替换的行为。
  - 验证：go test ./internal/pan 通过，已有播放/下载自定义 UA 与空 UA 测试通过；未进行真实播放器与 115 联调，待整体复查。
- [x] **73.** `internal/config/runtime.go` 4 个从未被覆盖的常量包成 struct + `DefaultRuntime()`，`runtime_test.go` 仅断言常量本身 → 普通常量放到使用处。
  - 实现：删除 Runtime、DefaultRuntime、Config.Runtime 及仅断言固定值的 runtime_test.go；同步间隔与提交超时放在 app.go 的私有常量中，媒体库任务池在构建处固定单 worker，保留独立订阅池及底层服务超时注入。四个生产数值保持不变。
  - 验证：config、app、tasks、offline、monitor、cmd/miyabi 测试通过，保留分池、顺序执行和调度行为测试，未新增重复常量断言。待整体复查。
- [x] **74.** `internal/api/access_gate.go` / `auth.go:16-21`：`NewAccessGateService(password, jwtSecret ...string)` 用可变参数模拟可选；`GenerateToken(subject, ttl)` 实参永远是 `"admin"` 与 `defaultTokenTTL`；`VerifyToken` 返回的 `*Claims` 全被 `_` 丢弃，禁用时返回 `(nil, nil)`；`SignToken` / `VerifyToken` / `Claims` / `BadRequest` 仅包内使用却导出。
  - 实现：NewAccessGateService 明确接收 password/jwtSecret，空密钥继续按原规则派生；业务 GenerateToken 固定 admin 和 30 天，VerifyToken 仅返回 error；底层签名/校验、claims 和 badRequest 改为包内私有。过期 Cookie 测试直接用底层签名构造，JWT 格式和校验规则保持不变。
  - 验证：api、app、cmd/miyabi 测试通过，覆盖 Cookie、SSE、STRM、过期/篡改 JWT；新增会话主体和期限、重建后密钥稳定、错误密钥拒绝及禁用门禁签发行为测试。未进行浏览器认证联调，待整体复查。
- [x] **75.** `web/src/components/app-shell.tsx:22-24` 手算 `activeId` 传给 `FloatingNav` → 使用 `Link` 的 `activeProps` + `activeOptions`。
  - 实现：删除 AppShell 的路径订阅与 activeId 传递，FloatingNav 使用 Link.activeProps / inactiveProps 切换完整按钮样式，activeOptions 对首页精确匹配、其余按子路径匹配，并忽略查询参数。
  - 验证：现有 Vitest 112 项、typecheck、lint 通过；本次未新增组件交互测试，导航高亮及移动端样式待用户浏览器验证，待整体复查。
- [x] **76.** `web/src/features/library/movie-card.tsx:14-18` hover 开关三重保护：`open={canHover && open}` + 条件渲染 + `key` 重挂载。
  - 实现：合并 LibraryMovieCard 与内部 Content，删除按设备模式切换的 key；失去悬停能力时条件重置 open，避免重新启用后恢复旧弹层。保留受控展开、详情条件渲染、点击关闭及移动端标题提示。
  - 验证：现有 Vitest 112 项、typecheck、lint 通过；本次未新增组件交互测试，桌面展开→缩小窗口→恢复、点击关闭及触屏标题提示待用户浏览器验证，待整体复查。
- [x] **77.** `internal/magnet/aggregator.go:16,28-35` `logger` 参数两处调用均传 nil；`defaultAggregatorTimeout` 与 `catalogue.aggregatorTimeout` 同为 8s 重复定义。
  - 实现：删除 Aggregator 的 logger 参数、字段及默认回退，直接使用 slog.WarnContext；删除 catalogue 重复定义的 8 秒超时，生产构造传 0 使用聚合器默认值。保留超时参数供测试验证慢来源处理。
  - 验证：magnet、catalogue、app 测试通过，覆盖聚合去重、来源超时、部分失败与缓存行为；未进行真实磁力来源联调，待整体复查。

---

## 五、冗余 / 死代码

### 115 字幕删除（d6a8a0c）残留
- [x] **78.** `internal/library/scan/subtitle.go` 已删除，但 `scan/subtitle_test.go` 仍保留旧命名测试。检查它们现在调用的生产路径和有效断言；失去测试目标的用例可删除，仍覆盖字幕匹配行为的用例应重命名或迁移。与第 239 条一起处理。
  - 已完成：确认 IndexDirectorySubtitles 仅存在于测试专用实现，删除 scan/subtitle_test.go 与 subtitle_fixture_test.go；生产番号识别和影片匹配测试保留。go test ./... 通过，待整体复查。
- [x] **79.** `internal/subtitle/service.go`：`SourcePan`（:25）、`Reader` 接口（:32）、`exportPanTrack`（:114）、`IndexPanTrack`（:209）；`Export` 的 `reader` 参数恒为 nil（`scrape/subtitle_queue.go:131`），`scrape/scrape.go:50` 的 `SubtitleExporter` 签名仍带 `subtitle.Reader`；:69-70 注释 "Subtitles stored with the 115 videos come first" 过时；历史 `pick_code != ''` 行永远 pending → 一次性 `DELETE`。
  - 已完成：删除 SourcePan、IndexPanTrack、exportPanTrack、domain/subtitle.Reader 及 Export 的 reader 参数；在线字幕继续写入 Emby 目录。原描述修正：已导出且文件存在的旧记录原本可复用，未导出的记录才会跳过。现按停用 115 字幕的决定由版本迁移删除旧记录，不删除磁盘文件。在线字幕测试覆盖去重、数量限制、重复导出无下载、缺失重下载及硬字幕跳过；go test ./... 通过，待整体复查。
- [x] **80.** `internal/ent/schema/subtitle.go:10-12,25-28,59`：`file_id`、`pick_code` 字段、`index.Fields("file_id")` 及过时注释 → 删除并重新生成 ent。
  - 已完成：移除字幕 file_id、pick_code 和 file_id 索引并重新生成 Ent；追加第 6 个版本迁移，清理 source=115 或旧文件标识非空的记录和旧列/索引，不重排历史迁移。升级及再次打开测试验证在线记录与本地文件保留，go test ./... 通过；未在用户实际数据库执行，待整体复查。
- [x] **81.** `internal/library/scrape/subtitle_queue.go:15` `SubtitleTask.MetaPayload` 只写不读。
  - 已完成：删除 SubtitleTask.MetaPayload 及入队赋值，任务只携带 MovieID 和 Target；go test ./... 通过，待整体复查。

### 旧快照 / sidecar 残留
- [x] **82.** `internal/library/scrape/cover.go:126-128` 固定 `LocalExport: true`，而 `snapshot.go:124` `Matches` 此时直接 `return true` → `DirectorySnapshot`、`NewDirectorySnapshot`、`Sidecar`、`ObservedDirectory.Sidecar`、`Matches` 比对逻辑，以及 `walker.go:106` 全库 `DirectoryObservations`（每文件名 + SHA1 常驻内存）只服务老数据；每次 cover 仍计算 `nfo.Encode` + 两次 SHA1；`writeSidecars` 名不副实。
  - 已完成：快照仅保留账号、根目录和视频指纹；删除 DirectorySnapshot、Sidecar、LocalExport、全扫描 DirectoryObservations 与远端文件摘要计算。旧 JSON 多余字段不再参与判断，无需数据库列迁移。保留扫描目录内用于番号识别的 NFO 列表、视频变动失效和缓存缺失重建；go test ./... 通过，待整体复查。
- [x] **83.** `internal/library/scrape/cover.go:131,226-233` 已不回写 115，但每个 cover 仍重读 115 NFO（`DirectoryNFO`）、执行 `VerifyCoverOrigin`、逐视频 `sess.Info`（`verifyVideoPositions`），与"减轻网盘负担"目标相反 → 直接使用 payload 中的 `Document/Origin`。
  - 方案修正并完成：扫描阶段保留 NFO 番号识别与内容比对，统一在 scan/nfo.go；每个匹配 NFO 在一次目录处理内最多读取一次，支持别名和多影片目录，明确冲突返回错误，损坏或不可读的可选 NFO 不阻断文件名刮削。单文件定向扫描也检查匹配 NFO。标题、演员和封面改由 JavDB 获取，已有刮削结果仍可复用；删除远端 NFO 元数据/图片复用和封面阶段的重复读取、VerifyCoverOrigin。保留实时视频位置与数据源提交校验；旧未完成封面任务缺少 CoverURL/Artwork 时通过 JavDB 恢复。测试覆盖冲突、单次读取、在线刮削、远端图片变化不重刮、旧任务恢复、缓存目录下视频移动/删除阻止导出；go test ./... 通过，未做真实 115/JavDB 联调，待整体复查。
- [x] **85.** `internal/library/scrape/cover.go:255-264` `exportLocalMedia` 的 DB 兜底永远走不到（`directories()` 保证至少一个视频），且查询不按 source 过滤、`records, _ :=` 吞错。
  - 已完成：删除 exportLocalMedia 中无数据源过滤且吞错的数据库兜底，移除无用 ctx/stem 参数。视频仍由受数据源约束的目录查询提供并逐个校验位置；保留本地单文件、多段视频导出测试，go test ./... 通过，待整体复查。

### 其它死代码（已 grep 确认）
- [x] **87.** `internal/javdb/client.go:291-294`、`route.go:45,102-104,171-173,185-188`：`preferredHost`、`RouteStatus.Manual`、`ensureRoute` 的 ManualRoute 分支在生产中不可达（仅 `route_test.go:43` 使用）；`client.go:168-176` `Route()` 中 `state == nil` 拼 `CachedHost` 分支不可达；`routeState.status.Candidates` 与 `lastProbe` 双份真相（:36、:362）。
  - 方案修正：RouteStatus.Manual 仍用于手动选线、持久化恢复和代理变更策略，保留；仅删除 ensureRoute 的无效手动偏好分支、preferredHost 及 Route 的 CachedHost 兜底。
  - 已完成：routeState 仅保存生效主机、延迟、手动标记和 transport；候选列表仅保存在 lastProbe，公共状态统一组合返回。探测失败保留当前线路并显示最新失败结果，取消不覆盖状态，代理重建继续保留手动选择。
  - 验证：javdb、catalogue、api、app 包测试通过；新增本地 HTTP 模拟测试覆盖手动/自动探测失败、取消、成功切换及候选列表返回，保留缓存恢复与请求失败自动切换测试。未进行浏览器或真实 JavDB 联调，待整体复查。
- [x] **88.** `internal/javdb/client.go:254-260` `getJSON` 的 `language` 参数所有调用点都传 `defaultLanguage`，`transport.go:132-134` `if language == ""` 死代码。
  - 已完成：移除 Client、jsonTransport、transport 和请求构造中的 language 参数，统一在 newRequest 设置 accept-language=defaultLanguage（zh-TW），删除空值兜底；各 API、探测和测试桩同步简化，测试响应按路径索引。
  - 验证：javdb、catalogue、api、app 包测试通过；请求构造测试直接断言 zh-TW 请求头，保留签名、解析、限流重试与线路切换覆盖，git diff --check 通过。未做真实 JavDB 联调，待整体复查。
- [x] **89.** `internal/catalogue/service.go:344-361`、`types.go:77-87` `Facets()`、`Facets`、`FacetItem` 无 API / 生产使用，仅 `TestFacets`。
  - 已完成：确认无生产/API 调用后，删除 Facets()、Facets、FacetItem 与仅验证其固定选项的 TestFacets；标签、演员页面使用的 JavDB main 分类筛选和前端 DISCOVER_ZONES 不受影响。
  - 验证：catalogue、api 包测试通过，git diff --check 通过；未进行浏览器测试，待整体复查。
- [x] **90.** `internal/javdb/magnet.go:81-94` `Find` 中 `JavDBID` 为空走 `ResolveMovieID` 分支不可达（`catalogue/magnets.go:66` 总带 JavDBID）；`magnet.go:54-71` 排序与 `aggregator.go:104-118` 同一规则重复。
  - 已完成：Find 删除按番号回退查询，保留空 JavDB ID 返回空结果；Magnets 仅解码并保留来源顺序，删除预排序。最终排序统一由聚合器负责，保留推断标签、体积、文件数、来源及日期规则；ResolveMovieID 的其他业务调用不变。
  - 验证：javdb、magnet、catalogue、api 包测试通过，覆盖来源顺序、元数据、缺少 ID 不请求及真实 JavDB 适配器经聚合器排序；git diff --check 通过。app 包的 TestEmbyDirectoryAutomaticallyQueuesLocalScan 在本地目录路径断言失败，使用 HEAD 原版 magnet.go 的 Go overlay 对照亦复现，属于已有失败。未进行浏览器或真实 JavDB 联调，待整体复查。
- [x] **91.** `internal/javdb/client.go:52-66` + `internal/catalogue/service.go:56,65`：唯一调用 `app.go:89` 传 `javdb.Options{}`，Timeout / RPS / Burst 负值校验与 `DeviceUUID` 回退永不触发 → `catalogue.New` 不再接收 Options。
  - 方案修正：Timeout 仍由网络连通性 Probe 传入并用于线路超时测试，保留该参数及 New 的默认值和负值校验；持久化 UUID 可能为空或无效，保留 JavDB 构造时的 UUID 校验。
  - 已完成：catalogue.New 删除 Options 入参，直接生成并持久化首次设备 UUID，按数据库设置组装线路与代理配置；删除无人配置的 RequestsPerSecond / Burst 字段及其默认值、负值分支，限速统一使用原常量（2 次/秒、突发 1 次）。调用方和现有测试同步更新。
  - 验证：javdb、catalogue、network、api 包测试及 git diff --check 通过，覆盖 UUID 持久化复用、手动线路恢复、共享选线超时和取消；app 包仅复现第 90 项已确认的 TestEmbyDirectoryAutomaticallyQueuesLocalScan 本地目录路径断言失败。未进行浏览器或真实上游联调，待整体复查。
- [x] **92.** `internal/drive/drive.go:179-184` `ValidateSource`、`internal/drive/session.go:18,70-72` `Session.Version()` 仅测试使用（导致 scrape 的 `mockSession` 也必须实现）；`mount.go:152` `DirectoryPath` 仅包内使用。
  - 已完成：删除 ValidateSource 及其专用测试，移除 Session.Version 接口、实现与 mock 方法，删除 app 中无人调用的 authorizationVersion 测试辅助函数；DirectoryPath 改为包内 directoryPath。会话内部版本字段、请求前后校验及事务锁逻辑不变，保留挂载切换和退出登录后的会话失效测试。
  - 验证：drive、library（含 scan/scrape）、offline、api 包测试及 git diff --check 通过；app 包仅复现第 90 项已确认的 TestEmbyDirectoryAutomaticallyQueuesLocalScan 本地目录路径断言失败。未进行浏览器或真实 115 联调，待整体复查。
- [x] **93.** 零生产调用 / 仅测试使用：`scan/persist.go:22` `ProcessScanPage`、`scan/reconcile.go:20` `ReconcileScan`（只包事务）、`scan/identity.go:127` `HasEligibleVideos`（零调用）；`scrape` 的 `Images()`（:124）、`LockArtwork`（:134）、`Directories`（:272）、`InvalidateDirCache`（:356）、`SetDirTTL`（:368）；`scrape/nfo_source.go:18` `ArtworkOrigin.NFO`（只写不读）；`library/library.go:62` `Service.images`（只赋值）；`library/viewed.go:25` `ViewedMovieIDs(ctx, limit ...int)` 无人传 limit（`api/discover.go:28` 接口也跟着带）；`scrape/subtitle_queue.go:68` `Enqueue` 返回的 bool 无人使用；大量仅包内使用的导出函数（`CanIdentifyVideo`、`IdentifyScanVideos`、`ResolveSingleNFO`、`IndexDownloadedMovie`、`RemoveUnreferencedMovies`、`SaveScanProgress`、`ReportScan`、`SidecarByName`、`FindDirectoryNFO`、`VerifyCoverOrigin`、emby client 的 `PersonItem`）→ 改小写。
  - 第 82–83 项已删除 ArtworkOrigin、SidecarByName、FindDirectoryNFO、VerifyCoverOrigin；ResolveSingleNFO 已替换为扫描阶段 ResolveNFOCodes，不再引用旧符号作为待办。
  - 方案修正：ReportScan 仍被 library/local.go 生产调用；ProcessScanPage、ReconcileScan、IdentifyScanVideos、IndexDownloadedMovie、SaveScanProgress 是跨包回归测试使用的入口，本次保留其导出边界和事务语义，不删除测试或引入替代包装。Directories 保留跨包测试入口，合并原有公私两层方法，生产 Cover 共用同一实现。
  - 已完成：删除 HasEligibleVideos、scrape.Images、LockArtwork、InvalidateDirCache、SetDirTTL 和可变 dirTTL；维护测试改用已有 TryLockArtwork，缓存测试改验实际过期刷新。library.Service.images 移到测试夹具，构造时仍将原缓存传给扫描器；ViewedMovieIDs 固定原 5000 条上限，接口与测试桩同步简化；Enqueue 不再返回无人读取的 bool，测试验证实际队列内容、去重、容量及关闭后拒绝入队；CanIdentifyVideo、RemoveUnreferencedMovies、PersonItem 改为包内符号。
  - 验证：library（含 scan/scrape）、maintenance、emby、offline、api 包测试及 git diff --check 通过；app 包仅复现第 90 项已确认的 TestEmbyDirectoryAutomaticallyQueuesLocalScan 本地目录路径断言失败。未进行浏览器或真实上游联调，待整体复查。
- [x] **94.** 任务业务已迁入 library / monitor。重新检查 `internal/tasks` 与迁移后包的导出边界、包装方法和重复入口；`Service.Registry()` 已被 `app/task_workers.go` 使用，不再属于“仅测试调用”。仅对当前没有外部调用且无接口职责的符号缩小可见性。
  - 已完成：删除仅测试枚举的 Registry.Kinds 及对应断言；NewService 直接组装 Queue，移除仅包内调用的 NewQueue 包装；完成事务提交后直接调用 bus.publish，删除 Changed 包装。任务池通过 Service.Subscribe 订阅原总线，删除重复暴露总线的 Service.Bus，app 与测试同步调整。
  - 保留：Service.Registry、Queue 的业务锁与任务池接口、业务包及 SSE 使用的通知方法；Registry.Get、Handler 构造和 payload 工具仍有跨包调用或测试用途。注册表锁、独立分池订阅、取消恢复及事务提交后通知语义不变。
  - 验证：tasks、library（含 scan/scrape）、monitor、offline、api 包测试及 git diff --check 通过，覆盖数据库错误重试、先订阅后恢复、广播唤醒、退出解绑、完成回调提交与回滚；app 包仅复现第 90 项已确认的 TestEmbyDirectoryAutomaticallyQueuesLocalScan 本地目录路径断言失败。未进行浏览器或真实上游联调，待整体复查。
  - 路径断言复查（第 90–94 项）：失败源于 Windows TEMP 使用 ADMINI~1 短路径，而扫描任务通过 filepath.EvalSymlinks 保存 Administrator 长路径。测试将原始输入直接与规范路径比较造成误报，实际指向同一目录。已修正启动、目录切换和重启断言的预期路径并补充差异诊断，保留原始路径作为配置输入；生产逻辑未修改，app 包全量测试及 git diff --check 通过。
- [x] **95.** Subscription `zone` 字段（schema:38、monitor Item、`AddMovieOptions`、`AddActorOptions`、`UpdateOptions`、API 输入）写入、继承、返回，前后端从不读取；订阅状态 `"error"`（schema:41、`monitor/service.go:34`、前端 subscription-card）从未被设置；`monitor.Status` 与 ent `subscription.Status` 重复。
  - 已完成：删除订阅 zone 的 Ent schema、生成代码、创建/更新选项、演员继承、API 输入输出及前端类型；旧 monitors 导入语句同步移除对 zone 列的依赖。删除重复 monitor.Status 及常量，Item、TargetItem 和调用方统一使用 subscription.Status。
  - 按用户要求不保留旧状态兼容：删除 Ent 与前端的 error 状态枚举、标签及样式分支，移除本次新增的兼容测试；不新增旧状态转换或数据库清理迁移。失败原因仍使用 error 信息字段，检查失败时原有状态及重试策略不变。
  - 验证：database、monitor、api、app 包测试通过，保留原有数据库迁移、演员订阅继承和订阅重试覆盖；前端 118 项测试、typecheck、lint 及 git diff --check 通过。未进行浏览器或真实上游联调，待整体复查。
- [ ] **96.** `internal/subtitle`：`Provider.Name()` 从未通过接口调用（`subtitlecat.go:167` 另写死 `"SubtitleCat"`）；`finder.go:29-37` `WithAllowLoopbackForTesting` 测试开关进入生产结构体；`DecodeToUTF8` 最后的 `ToValidUTF8` 兜底实际走不到。
- [ ] **97.** `internal/netx/proxy.go:88-94` `sameConfig` 蕴含 `sameURL`，`!sameConfig || !sameURL` 实为 `!sameConfig`，与注释"只在生效代理变化时广播"相反（Enabled=false 改 URL 也重建 tls-client）→ 条件改为 `!sameURL`；:73-79 `resolve` 中 `!Enabled ||` 冗余；:131-133 `parseProxyURL` 的 `raw == ""` 分支死代码。
- [ ] **98.** `internal/netx/ssrf.go:59-60` 255.255.255.255 分支已被 :58 `ip4[0] >= 240` 覆盖；:103 多余中间变量；:191-193 `timeout <= 0` 兜底唯一调用传固定 20s；:157/:176 `ips` 为空时返回 `(nil, nil)`。
- [ ] **99.** `internal/codeid/codeid.go:132-144` `Layers.stripPrefix` 与 `:247-262` `Candidates` 两套"去前缀"逻辑，正则（`layerNumeric` vs `numericPattern`）已分歧；`Parse`（60-62）与 `Layers`（111-116）去噪流程重复；:344 `UnpaddedNumericCandidate` 仅包内使用；:350-362、:329-334 冗余分支；:95-101 正则 var 块散落；:180、:187 对恒为单元素的 `layers[0]` 循环。
- [ ] **100.** `internal/gfriends/client.go:21-23,29` `DefaultFastlyURL`、`DefaultRawURL`、`CacheExpiration`、`FileTree` 无外部使用却导出；:42-44 nil httpClient 默认值死代码；:127-128 写缓存错误被静默吞掉。
- [ ] **101.** `domain.ZoneAll` 的使用范围、`LocalMovie` 的 JSON 标签，以及 `domain.ScanPayload` 与 `scan.Payload` 别名是否仍有必要待核实。`KindInternal` 和旧 tasks 层 ScanPayload 别名已清理，不再作为待办。
- [ ] **102.** `internal/library/scrape/scrape.go:155` `fileScope` 与 `internal/database/scope.go:10` `LibraryFiles` 一字不差；魔法字符串 `"local"` 在 `library.go:118,120,202,204`、`scan/local.go:235-236` 硬编码，`Movies` 与 `MatchingMovies` 的 scope 计算重复 → `domain.LocalAccountID` + `s.libraryScope()`。
- [ ] **103.** `internal/app/app.go` 的 EmbyDir 兜底与 `config.Load` 重复；核实直接调用 App.New 的测试和调用方需求后，统一配置初始化职责。旧的 Emby Config 读取、activePublicURL 及代理非空判断描述已失效。默认导出路径与 PublicURL 一并按第 184 条处理。
- [ ] **104.** `internal/config/config.go:50` `MIYABI_EMBY_URL` 未文档化旧别名；:101-119 `validate` 重复 Trim、EmbyDir 空检查不可能触发；日志级别 `logging.Validate` 校验一次、main 中 `UnmarshalText` 再解析一次 → Config 直接存 `slog.Level`，删除 `internal/logging`（28 行）。
- [ ] **105.** `internal/database/viewed.go` 的 `migrateViewedMovies` 为全部旧浏览记录写入同一时间，可能丢失原有顺序。核实旧数组顺序及保留策略；迁移已纳入版本管理，不能因当前开发数据库迁完就删除或重排迁移步骤。若要修正已升级数据，应新增迁移并明确可恢复的信息。
- [ ] **106.** ✔ 前端零引用 UI：`components/ui/popover.tsx`、`components/ui/dropdown-menu.tsx`；`ui/textarea.tsx` 仅被零引用的 `InputGroupTextarea` 使用；零引用导出 `AvatarBadge` / `AvatarGroup` / `AvatarGroupCount`、`CardFooter` / `CardAction`、`TabsContent`、`tabsListVariants`、`badgeVariants`、`InputGroupText`、`SelectLabel` / `SelectSeparator`、`BreadcrumbEllipsis`。
- [ ] **107.** 前端零引用 / 无意义项：
  - `api/auth.ts:45-57` `useAccessGateLogout` 无调用（后端 `POST /auth/logout` 随之无人使用），`LoginResponse.expires_at` 不读；
  - `auth.ts:20`、`discover.ts:134`、`:160`、`:235` 的 `enabled` 参数从未传入；
  - `client.ts:53` `apiPut` 的 `options`（keepalive/signal）是播放记录删除残留；
  - `features/tasks/task-toast.tsx:17` ToastID 再导出无人使用；`browse-history-store.ts:196` `getPendingCount` 仅测试；
  - `lib/source.ts:5` `sameSource` 零调用，而同源判断在 `api/library.ts:57-64`、`api/offline.ts:68-86`、`features/library/page.tsx:33-40`、`task-notification-diff.ts:79-84` 手写 4 遍；`SourceScope` 的 `directory_id` 分支用不到；
  - `api/tasks.ts:74` `isBatchTask` 外部无人使用；`library.ts:40` `movieLists` 未使用；`discover.ts:7` `discoverKeys` 再导出未使用；
  - `api/tasks.ts:28-38` ScanTask 10 个前端不读字段（`current_path`、`directories_*`、`files_scanned`、`removed_*` 等）；
  - `task-notification-diff.ts:18` `NotificationEntry.playable` 是删除播放器后的遗留命名（实为"已入库"）。
- [ ] **108.** 后端 `catalogue/service.go:398-401` 已随列表计算 `state` 与 `library_id`，前端从不读取 `DiscoverMovie.state`（`api/discover.ts:47,66`），见第 150 条。
- [ ] **109.** 重新评估前端未调用的接口是否仍承担运维、外部调用或手动恢复职责：`GET /discover/movies/:id/offline`、`POST /library/scan/local`、`POST /auth/logout`。本地扫描接口已支持后台入队与配置路径校验；“前端未调用”本身不能作为删除理由。
- [ ] **110.** 删除属性后残留空行：`ui/input-group.tsx:13`（原 `role="group"`）、`ui/breadcrumb.tsx:57/68/82`（原 role / aria-hidden / aria-current）、`ui/dialog.tsx:62-64` 删除了 `sr-only` 文案、`search/page.tsx:67`、`settings/pan-section.tsx:40/62/76`、`pan-directory-row.tsx:34`；`internal/domain/video.go:30-31` 双空行（唯一 gofmt 报告）。
- [ ] **111.** `cn` 导入不统一：`ui/hover-card.tsx:2`、`ui/popover.tsx:2`、`ui/dropdown-menu.tsx:2` 直接 `from 'cn'`，其余 `@/lib/utils`；`dropdown-menu.tsx` 未过 oxfmt（双引号、长行）。

---

## 六、过度防御

- [ ] **112.** `internal/api/auth.go:35/58/70/76/83/117`、`strm.go:64` 的 `gate == nil` 生产不可能成立；`rate_limiter.go:37/74/98` `limiter != nil` 与方法内 `l == nil` 双重防御；`accessLoginHandler` gate 为 nil 返回 `Success: true` 死路径 → 测试统一传 `NewAccessGateService("")`。
- [ ] **113.** `internal/drive/token.go:59-105` `withPanToken` 中 `current.closed` 检查 3 次、`ctx.Err()` 3 次 + `refreshTokens` 入口再一次 → 合并为最多 2 次的循环。
- [ ] **114.** `internal/drive/session.go` 每次调用执行 `sourceState` 3~4 次（显式一次、`withPanSourceToken` 内一次、重试一次、`checkSource` 一次）。
- [ ] **115.** `internal/pan/client.go:48-50` `maxInFlight <= 0`、:127-129 `baseTransport == nil`、`SetRetryAfter` 中 `resp == nil` / `wait > 0`、:241-244 可直接 `return result, result.err()`；`auth.go:99-115` `Status == nil`、`case 0`、`default` 三处返回同值；`upload.go:44` 局部变量名 `init`。
- [ ] **116.** `internal/strm/relay.go:35-37,78-80` `relay.drive == nil`、:65 `source.URL != ""`、`downloadURL != ""`（pan 层已保证）。
- [ ] **117.** `internal/javbus/client.go:149,214` `errors.Is(ctx.Err(), Canceled) || errors.Is(ctx.Err(), DeadlineExceeded)` 等价 `ctx.Err() != nil`；:70 `rate.Every(time.Second/time.Duration(defaultRate))` 即 `rate.Limit(1)`；:297 `c.client != nil`；`parser.go:57` `Contains("/doc/driver-verify") || Contains("driver-verify")`；`parser.go:128,132` 显式零值。
- [ ] **118.** `internal/javdb/route.go:277` `utf8.Valid`；`route.go:217-256`、`client.go:369-389` 解析到 `map[string]any` 再逐层断言 → typed struct；`route.go:49` 类型 `probe` 与方法 `c.probe` 同名。
- [ ] **119.** `internal/catalogue/state.go:60` `source == nil` 在 :32 已判断；`service.go:163-168` nil 检查；`service.go:379` `time.Now().In(time.Local)`；`magnets.go:80-90` 先投影 URI 再剥离。
- [ ] **120.** `internal/magnet/aggregator.go:75,84-86` Hash 小写化、`Sources` 为空兜底（各来源已保证）；`picker.go:108`、`quality.go:158` `m.HD || hasTag(m, MagnetTagHD)`（根源：`domain.Magnet` 同时用 bool 与 Tags 表达 HD/字幕）；`picker.go:105` 对已聚合结果再次 `ApplyInference`。
- [ ] **121.** `internal/library/scrape`：`Close` 的 `service == nil`、`subtitleQueue != nil`；`newSubtitleQueue` 的 concurrency / capacity / logger 兜底（唯一调用传默认值）；`Enqueue` 的 `q.ctx != nil`、`q.logger != nil`；`worker` 的 `!ok`（channel 从不关闭）；`processCover` 与 `process` 两次判断 `subtitles != nil`；`ExportLocalMovie` 的 `record == nil` + `notifiers ...MediaNotifier` 表示可选；`scrape.go:339` `dirCache == nil`、:343 `ttl <= 0`；`findAvatar` 的 `s.db == nil`；`reconcile.go:112` `if images != nil`；`reconcile.go:58` 多余 `var err error`。
- [ ] **122.** `internal/image` singleflight + Stat 检查 + `publishImage` 读回比较三层叠加 → 保留原子 rename + Stat；`internal/maintenance` 每次循环检查 `ctx.Err()`、检查 DB 文件 `IsRegular`。
- [ ] **123.** `internal/monitor/service.go:108` `checking`、`internal/offline/service.go:52` `syncing` 互斥锁：生产中仅由一个 `RunPeriodic` 串行调用；`offline/add.go:20,30` `catalogue != nil` 两次；`sync.go:254` `library != nil` 只为测试；`sync.go:76` 重复过滤 `DirectoryID`、:94-96 每条检查 `ctx.Err()`、:154-159 身份校验冗余；`monitor/service.go:160,176` `limit > 0` / `max(page, 1)`（API 绑定已保证）；`tasks/pool.go:34` `logger == nil`。
- [ ] **124.** `internal/offline/projection.go` 已使用独立 OfflineDownload 表，不再有旧 JSON hash 分组。当前空 hash 会使整次 Activity 投影报错，而 schema 已要求 hash 非空；核实遗留坏数据的实际入口，再决定移除冗余检查或采用记录日志并跳过的策略。
- [ ] **125.** 前端：`access-gate.tsx:14,25` `sessionUnlocked`（onSuccess 已写入缓存）；`lib/math.ts:3` 处理 -0；`api/client.ts:66,71,80,106` 四处 `typeof window === 'undefined'`（SPA，且 :18 直接使用 `window.location.origin`）；`components/error-state.tsx:34` 外层重复判断 `retry.onRetry`；`task-progress-state.ts:21` 手写 clamp（`lib/math` 已有）；`lib/search-schema.ts:3` `errorMessage` 参数无意义；`stores/settings.ts:17` `partialize` 多余。

---

## 七、性能

- [ ] **126.** `internal/library/workflow.go` 的 metadataGroups 仍通过 payload 中的 `scan_task_id` 聚合 scrape/cover 子任务；`internal/api/sse.go` 的多个连接可能重复计算同一任务快照。快照已移出 task 表，删除“大快照逐行解析”的旧描述。通过查询计划和负载数据评估 parent_id 列/索引，以及共享快照或通知合并。
- [ ] **127.** `internal/tasks/bus.go:69-75` `Notify()` 同时唤醒 pool：扫描分页进度（`scan/persist.go:171,230`、`reconcile.go:151`）、`SaveSubscriptionBatch`、`ChangeOffline`（每 30s）都会让 pool 空跑 Claim；`queue.go:66` Claim 自身又 Notify → 拆分 `WakePool()` 与 `NotifyUI()`。
- [ ] **128.** `internal/ent/schema/task.go:37-42` `index.Fields("status", "created_at")` 的 `created_at` 从未用于过滤 / 排序，热点查询均为 `type + status + ORDER BY id` → `index.Fields("type", "status")`，删除单列 `type` 索引。
- [ ] **129.** ✔ `internal/emby/service.go` 的 `sendBatch` 每批 `/Library/Media/Updated` 增量通知后仍调用 `client.refresh`（`/Library/Refresh` 全库扫描），扫描期间约每 3 秒一次 → 删除。
- [ ] **130.** `internal/library/scan/movies.go:29` `MatchMovies` 每页 100 个 `LIKE '%code'` 以 OR 连接 → 全表扫描；:38-45 codes × records 双重循环；:56 `slices.Contains` 去重 O(n²) → 增加 `canonical_code` 索引列 + 等值 IN，去重用 map。
- [ ] **131.** `internal/library/scan/local.go` 已通过后台任务执行，但仍逐文件开事务并调用 MatchMovies；图片解码/重编码在 SQLite 写事务内；Emby 通知现已同事务持久化并在提交后发送。评估批量匹配、缩短事务；通知事务语义已修复。
- [ ] **132.** `internal/library/scan/reconcile.go:94-142` 全量扫描加载所有已刮削影片的 files / actors / tags；每部影片 `images.Exists` 3 次 stat + `ExportLocalMovie` stat / 写文件均在 SQLite 写事务内，长时间占写锁；:139 刮削任务逐条 `Create().Exec` → 事务外导出 + `CreateBulk`。
- [ ] **133.** `internal/library/scrape/scrape.go` 每个刮削任务仍用 cover payload 的 `scrape_task_id` 执行 Exist 查询，评估关联列或索引。旧的“扫描时从 task 表批量读取元数据快照”已随快照迁入 movie 解决；与第 126 条共同处理任务关联查询。
- [ ] **134.** `internal/library/scrape/cover.go:45-53` `service.artwork.Lock` 覆盖整个 `processCover`（含 JavDB 封面下载、115 列目录、`sess.Info`、本地导出）→ 所有 cover 串行，刮削期间 `ClearCache` 永远返回"忙" → 只锁"保存图片到提交引用"一段。
- [ ] **135.** `internal/library/scrape/scrape.go:71-73,320-353` `dirCache` 过期条目永不清理，每条存整个目录列表；`processCover:120` 注释 "Re-read the directory after scraping" 实际拿到 45s 内缓存。
- [ ] **136.** `internal/offline/projection.go` 的 Activity 仍读取作用域内每个 hash 的最新下载记录且不设历史上限，再关联扫描任务与文件索引；已不再逐条解码离线 payload。`web/src/api/offline.ts` 仍在 SSE 之外每 5 秒轮询活动任务。评估数据量、事件断线兜底需求和重复查询成本。
- [ ] **137.** `internal/monitor/batch.go` 批量每处理一条就重新编码整个 payload（含全部 IDs）写回并 Notify（触发第 126 条的全连接重查）→ 只更新计数或按 N 条 / T 秒保存。
- [ ] **138.** `internal/pan/metadata.go:81-86` `ReadMetadata` 显式 `limiter.Wait` + `panTransport` 每次尝试再 Wait，加上 `DownloadURL`，一次 NFO/图片读取消耗 3 个令牌（上限 4 req/s）并占用 2 个 inFlight 槽之一，与 `OpenMedia` 注释"CDN 不走限流"矛盾 → 删除显式 Wait，CDN 读取改用 `client.media`，`Client.limiter` 字段随之可删。
- [ ] **139.** `internal/pan/upload.go:86-97` 每个 sidecar 请求一次 `/open/upload/get_token`；OSS `PutObject` 经 `panTransport` 消耗 115 限流令牌并占 inFlight 槽 → STS token 按过期时间缓存，OSS 用独立 client。
- [ ] **140.** `internal/api/strm.go:33-39` Emby / 播放器每次 HEAD、GET、Range 都走 DB + downurl（限流 4rps / 2 并发）+ CDN HEAD → 按 `(pickCode, UA)` 做短 TTL 缓存。
- [ ] **141.** `internal/netx/ssrf.go:103-119,152,206` 同一主机 3 次 DNS 解析（请求前 `ValidateSafeURL`、每次重定向、拨号时），拨号那次已覆盖 DNS 重绑定 → `ValidateSafeURL` 只做语法校验。
- [ ] **142.** `internal/drive/account.go` 缓存未命中无 singleflight，并发 `Open()` 同时请求 `/open/user/info`；`accountCacheMu` 可并入 `d.mu`；`internal/drive/token.go:13-57` refresh_token 被吊销后不标记凭证失效，每个请求都发起一次注定失败的刷新。
- [ ] **144.** `internal/catalogue/service.go:239-251` `MovieSummary`、`:254-264` `BrowseMovies` 走完整 `projectMovies → MovieStates`（2 次 DB 查询）后丢弃投影结果。
- [ ] **145.** `internal/javdb/movie.go:105-141,147-174` `ResolveMovieID` 每次查询前对累积的数百候选全量 `matchCandidates`，`codeid.Normalize` 每次最多 8 个正则，`IsFormatEquivalent` 两边重复 Normalize → 预计算 `normCode`。
- [ ] **146.** `internal/javdb/route.go:112` 非 full 模式用 5ms ticker 轮询（最长 60s，每秒唤醒 200 次）→ 按最早截止时间的单个 `time.Timer`。
- [ ] **147.** `internal/subtitle/label.go:89-100` `[]rune(text)` 将最多 10MB 文本整体转换（约 40MB 分配）后再截取前 15000 个；`containsRune` 在 40 元素切片中线性查找 → range 计数 + map。
- [ ] **148.** `internal/monitor/check.go:48-56` 每轮只查 10 部影片 / 5 个演员后等 5 分钟，500 个订阅约 2.8 小时才查完；`service.go:137` `EnqueueSingle` 每条读一次 config、新建 Picker；`BatchHandler` 先 `Subscription.Get` 一次，`EnqueueSingle` 再 Get 一次；`check.go:94-108` "继续等待"分支更新 checks 却不发 `NotifyMonitorChanged`。
- [ ] **149.** `internal/library/library.go:222` 分页 `ORDER BY created_at DESC, id DESC`，movie 表无 `(created_at, id)` 索引；`scan/local.go:223-227` fileID 取 `sha256(rel)`，不同扫描根目录下相对路径相同会冲突，已存在记录不更新 path。
- [ ] **150.** 前端列表已带 `state`，每个 `MovieStateBadge` 再批量 `POST /api/discover/movie-states`，形成"列表 → 状态"瀑布 → 用列表数据 seed（`initialData` + `initialDataUpdatedAt`），或后端从列表 DTO 删除 state。
- [ ] **151.** `web/src/features/tasks/task-events.tsx:115-123` 连接后首个 `changes` 事件因 `revisions` 为 undefined，把 library / offline / movie-states / subscriptions 四类全部失效 → 每次打开页面重复请求一次；:38 `reconnect` 中的 `invalidateQueries` 多余 → 首个事件仅作基线，断线重连后才强制刷新。
- [ ] **152.** 复核 `refetchOnMount: 'always'` 与 staleTime 的组合：`web/src/api/tasks.ts`、`movie-states.ts`、`subscriptions.ts`、设置类查询等。全局默认值现位于 `api/query-client.ts`；JavBus 设置查询已删除。按事件同步和重连恢复需求决定刷新策略，不应仅因 staleTime 与 always 共存就机械删除。
- [ ] **153.** `web/src/api/offline.ts:66-88` `useAddOffline` 成功后 `setQueryData` + `invalidate` + SSE 再失效，同一数据刷新三次；:48-57 `useOfflineTasks` 在 select 中重复过滤（后端已按数据源返回），与全局查询共用 key 使其 `enabled` 形同虚设；`library.ts:56-70` 两个分支最终都 invalidate。
- [ ] **154.** `useLibraryMovies`、`useSearchMovies` 未使用 `keepPreviousData`，翻页闪骨架；路由未注入 queryClient context、无 loader，`defaultPreload: 'intent'` 只预加载 chunk 不预取数据。
- [ ] **155.** `web/src/components/media-image.tsx:60-65` 每张封面额外渲染一张 `scale-125 blur-xl brightness-75` 背景图，一页 20 个大半径模糊滤镜；`features/library/use-desktop-hover.ts:14` 每卡片每次 snapshot 调 `matchMedia` 并各自注册监听 → 模块级缓存 `MediaQueryList`。

---

## 八、低效 / 非最佳实践

- [ ] **156.** `internal/drive/session.go:79-207` 8 个 `sourceSession` 方法同一模板（`sourceState → withPanSourceToken → checkSource`），约 130 行 → 泛型 `call[T]`。
- [ ] **157.** `internal/javbus/client.go:128-191` 与 `:193-242` `ensureDetailParams` / `fetchMagnets` 请求构造、UA/Cookie、301/302、`LimitReader`、Cloudflare 判断、状态码判断逐行重复 → `c.get(ctx, url, headers)`。
- [ ] **158.** `internal/api/error.go:76-119` 与 sloggin 各记录一次失败日志；attrs 中 `cause` 与 `error` 内容重复。
- [ ] **159.** `internal/api/auth.go` 的 authMiddleware 仍直接构造鉴权错误响应，可考虑统一内部处理方式，但必须保留 `code: "UNAUTHORIZED"` 及缺失/失效令牌的语义。该错误码用于区分本站会话与 115 账号失效，不能直接改成不带 code 的通用错误响应。
- [ ] **160.** api handler 零碎问题：`network.go:42`、`emby.go:41` test handler 先查已保存配置再被 body 覆盖（前端总带 body），两者写法相同可抽泛型 helper；`subscription.go:141-145` 手写 `c.JSON(202)` 未用 `accepted`；`subscription.go:155` feed 复用带 `Kind` 字段的 query 结构；`router.go:45-49` if/else 等价且忽略错误；`router.go:70-73` 4 条 STRM 路由可 `Match` 合并；`task.go:16`、`sse.go:10` 参数名 `tasks`、`offline.go` 参数名 `offline` 遮蔽包名；`router.go:42` 构造函数中设置全局 `gin.SetMode`。
- [ ] **161.** `internal/api/rate_limiter.go`：`records` 仅在同一 IP 再访问或登录成功时删除（换 IP 无限增长）；`check` 与 `recordFailure` 不在同一把锁（并发可超过 maxFailures）；`ErrTooManyLoginAttempts` 文案写死"24 小时"且作为 Cause 拼出矛盾日志 → `allow(ip)` 一次加锁完成检查与预占。
- [ ] **162.** `internal/tasks/queue.go:47-68` Claim 先 SELECT 再条件 UPDATE + 重试循环 → 单条 `UPDATE … WHERE id=(SELECT … LIMIT 1) RETURNING`；`batch.go:66-76` `listBatchTasks` 两次查询内存合并；`pool.go:76-79` `registry.Get` 分支不可达。
- [ ] **163.** 订阅 API 三种风格：tasks 返回 `(ch, unsubscribe)`、drive 返回 `func()`、netx 返回 ch + 单独 `Unsubscribe`（`proxy.go:99-117` O(n) 遍历）→ 统一 `(<-chan struct{}, func())`。
- [ ] **164.** Go 1.27：`app.go:223-235`、`internal/network/service.go`、`aggregator.go:53-62` 可用 `wg.Go`；`internal/syncx/lock.go:47` 对未加锁的锁 `Unlock` 会永久阻塞（`sync.Mutex` 会 panic）；`KindX.String()` 与 `string(KindX)` 混用。
- [ ] **165.** `internal/library/viewed.go:91-110` Count + 查最旧 + Delete 三步 → 单条 `DELETE … WHERE id NOT IN (SELECT id … ORDER BY viewed_at DESC LIMIT ?)`。
- [ ] **166.** `internal/api/router.go:139-158` `installFrontend` 中 `fs.Stat` 对目录也成功，FileServer 会列出 `/assets/` 目录清单；带哈希的静态资源无长缓存头。
- [ ] **167.** `internal/javdb/media.go:29-43` `FetchMedia` 接受任意 https 主机且 body 无大小上限，`/image` 成为登录后任意抓取器 → 限定 JavDB CDN 域名或复用 `netx.SafeDownload` + 上限。
- [ ] **168.** `internal/gfriends/client.go:236` `errors.Join(errors.New(...), lastErr)` → `fmt.Errorf("fetch avatar: %w", lastErr)`；:113 Filetree 读取无大小限制。 第 61 项已将头像下载错误改为 fmt.Errorf 包装；Filetree 大小限制仍待处理。
- [ ] **169.** `web/src/api/client.ts:30-61` `apiPost` / `apiPatch` / `apiPut` JSON 体逻辑重复 3 遍 → `sendJSON(method, path, body)`；`apiGet(path, undefined, signal)` 出现 15 次 → `apiGet(path, { query, signal })`；:85-89 `imageURL` 不属于请求客户端。
- [ ] **170.** 查询键的组织和命名仍不统一：discoverKeys 位于 `web/src/api/movie-detail-cache.ts`，`pan.ts` 存在 library/offline 字面量键，subscriptionSettingsKey 与其他配置键风格不同。评估集中管理或按 API 模块导出，避免重复定义与循环依赖；已删除 JavBus API 文件的引用不再适用。
- [ ] **171.** `web/src/api/network.ts` 更新失败时同时回滚和 invalidate，核实是否需要两者；Emby、订阅设置与网络设置的乐观更新策略不同，应按失败恢复和交互需求明确规则，而不是机械要求所有设置采用同一方式。JavBus 设置 API 已删除。
- [ ] **172.** `web/package.json` build 脚本 `vite build && tsc --noEmit`，类型检查应在前。
- [ ] **173.** `web/src/features/settings/network-section.tsx:39` `setTimeout(() => inputRef.current?.focus(), 50)` 不清理 → `autoFocus`；`access-gate.tsx:26` effect 依赖整个 `config` 查询对象 → 依赖 `config.refetch`。
- [ ] **174.** 导入风格混用：`tasks-section.tsx:7`、`scan-progress.tsx:8` 用 `@/features/settings/shared`，同目录其他文件用 `./shared`；`components/movie/index.ts` barrel 仅部分使用（`recommendations.tsx:5-6` 相邻两行一个走 barrel 一个直连，barrel 导出的 `DiscoverMovieCard` 无外部使用）；`DISCOVER_PAGE_SIZE as PAGE_SIZE`（`category-content.tsx:6`、`discover/page.tsx:7`）、`DISCOVER_ZONES as zones`（`category-filters.tsx:15`）别名不一致。

---

## 九、过度拆分

- [ ] **175.** `internal/api` 中部分文件命名与资源职责不一致：`data.go` 对应 maintenance、`movie.go` 对应 library，HealthChecker 与 handler 分散。按资源和依赖关系评估整理；task/sse/health 等文件短不等于需要合并。JavBus handler 文件已删除。
- [ ] **176.** `internal/drive/drive.go:195-198` `Drive.OpenMedia` 与 `internal/strm/relay.go:77-82` `Relay.Probe` 纯透传，一次 HEAD 穿过 api → strm → drive → pan 三层空转发；api 层自行编排两次调用 → strm 持窄接口并提供 `Head()`。
- [ ] **177.** `cmd/miyabi/healthcheck.go` 一行转发 `app.CheckHealth`（应属 CLI）；healthcheck 子命令执行完整 `config.Load`，其中 `netx.OutboundIP()` 做一次 UDP 拨号。
- [ ] **178.** `internal/magnet/source.go` 只有 Source 接口，评估是否适合与同职责类型合放；如它是清晰的聚合器依赖边界，可保留。原 netx/probe.go 和 api/javbus.go 已删除，不能再作为待清理文件。
- [ ] **179.** `DiscoverResults` 三处调用（`discover/browse-results.tsx:14-23`、`metadata-search-page.tsx:104-113`、`search/page.tsx:103-112`）是相同的"拆 query 字段传 props"适配 → 直接接收 `UseQueryResult` + `emptyTitle`，删除 `BrowseResults` / `SearchResults` 壳组件。
- [ ] **180.** `web/src/features/subscriptions/actor-subscriptions.tsx` 把 displayItems/displayQuery、选择状态等透传给 MovieSubscriptions，后者又在 page 中另一分支渲染。可将结果列表统一放在 page 层。注意演员页未选单个演员时仍应展示“全部演员新作”feed，不能回退到普通影片订阅列表。
- [ ] **181.** 评估 `features/library/movie-status.tsx`、`features/movie-detail/metadata-link.tsx` 的单调用方拆分是否改善可读性；删除无意义 Fragment 等局部冗余。`api/movie-states.ts` 已合并状态加载与 hook，不再是原来的 17 行壳文件。不要仅按文件行数合并模块。

---

## 十、臃肿 / 重复（后端）

- [ ] **182.** `catalogue.MovieDetail` 重新声明 `domain.MovieDetail` 的字段而非内嵌；`domain/library.go` 实际存放 MovieSummary / Media / OfflineSubmission，`ScanProgress` 位于 `source.go`，文件名与内容不符；`ScanPayload.Checkpoint` 是 JSON 中嵌套 JSON 字符串 → 结构化字段。
- [ ] **183.** `internal/app/app.go` 初始化失败分支仍重复列出资源 Close；此前 scrape worker 清理遗漏已修复。评估以清理栈或局部辅助方式减少重复，必须保持资源逆序关闭，并明确哪些对象在构造成功前已有后台工作；不强制改成半初始化 App。
- [ ] **184.** 导出默认值仍有重复：`internal/export/export.go` 保留 `./data/emby` 和硬编码 8080 的 defaultPublicURL，与 config.Load 推导不一致；app 也有 EmbyDir 兜底。统一在配置入口确定，底层导出使用明确配置。另核实前端 emby-url.ts、network-section.tsx 的 URL scheme/host 格式化重复，以及 netx/ip.go 注释与实现的差异。
- [ ] **185.** 磁力排序规则（字幕 > 高清 > 体积 > 文件数）在 `javdb/magnet.go:54-71` 与 `magnet/aggregator.go:104-118` 各一份。

---

## 十一、UI 一致性

- [ ] **186.** 设置页 5 种保存语义：立即生效无提示（外观、隐私、JavDB 线路 `data-source-section.tsx:56`）；立即生效且每项 toast（`subscription-section.tsx:45` 改一个 Select 弹一次）；代理开启需手动保存、关闭立即保存（`network-section.tsx:36-59`）；Emby 开关也需点保存（`emby-section.tsx:155-171`）→ 统一：开关 / 下拉即时保存，文本表单才有保存按钮，toast 策略一致。
- [ ] **187.** 加载态不一致：Skeleton（data-section）、纯文本（`tasks-section.tsx:23`）、裸 spinner（`pan-section.tsx:55`）、直接显示假默认值（`subscription-section.tsx:61` `movie_auto_download ?? true`、`emby-section.tsx:32` `sync_actors ?? true`，加载中显示为开启）。错误态：tasks / data 带重试，emby:246、network:152、subscription:138 无重试；`data-source-section.tsx:147` 文案"点击重试"却未接 `onRetry`；同一"任务读取失败"在 `library/page.tsx:248` 用 `retryLabel="重新连接"`、`tasks-section.tsx:25` 未用且文案不同 → `SettingsSection` 接收 query 统一处理。
- [ ] **188.** 同一扫描阶段两套文案：`features/tasks/scan-status.ts:3-18`（扫描中 / 核对中 / 刮削中 / 写入元数据）vs `task-progress-state.ts:5-12`（正在扫描文件 / 正在核对媒体库 / 正在刮削元数据 / 正在写回元数据）→ 共用一张 label 表。
- [ ] **189.** ✔ 订阅按钮三处实现且语义冲突：`components/movie/movie-subscribe-button.tsx:14` `subscribed = waiting || added`，`movie-detail/subscription-action.tsx:11` 只算 `waiting`（`added` 显示"重新订阅"）；"已订阅"卡片用 `variant="default"` 实心，详情 / 演员按钮用 `outline`，颜色相反；`discover/metadata-search-page.tsx:118-146` 演员按钮文案"已订阅演员"点击即取消订阅。
- [ ] **190.** 硬编码颜色 / 语义色挪用：`library/movie-status.tsx:10` `[--destructive:oklch(0.577_0.245_27.325)]` 覆盖暗色 token；`ui/badge.tsx:18` library 变体写死 `bg-violet-600/dark:bg-violet-500`，hover 仍是 `violet-600/80`；`data-source-section.tsx:216` `text-orange-600 dark:text-orange-400`；`subscription-card.tsx:151` `data-checked:text-white`；`globals.css:17,19` `--color-success` / `--color-info` 直接指 `emerald-500` / `sky-500` 不分亮暗，白字配 emerald-500 对比度约 2.5:1；`ring-success` / `bg-success`（`subscription-card.tsx:130,151`）把"成功"色当"选中"色（全站其他选中用 primary）→ `:root/.dark` 定义 `--success`、`--library` 及 `*-foreground`。
- [ ] **191.** Select 弹出方式混用：6 处显式 `position="popper"`，`subscription-section.tsx:88/107/128` 使用默认 `item-aligned`；`ui/select.tsx:81` `position === 'popper' && ''` 死表达式 → 默认值改为 popper。
- [ ] **192.** 详情页骨架与实际不符：`movie-detail/skeleton.tsx:15-19` 统计区画成 3 个 `h-20 rounded-2xl` 块（实际 `hero.tsx:58` 是一条带分隔线的 `dl`）；:30 预览 `rounded-2xl` vs `previews.tsx:32` `rounded-xl`；:36 磁力 `space-y-2` vs `magnets.tsx:91` `space-y-4`；磁力骨架在 `magnets.tsx:79-83` 与 `skeleton.tsx:36-40` 逐字重复；主栅格 `lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]` 在 `hero.tsx:11` 与 `skeleton.tsx:7` 各写一遍。
- [ ] **193.** 圆角 / 状态处理不一致：`SelectTrigger` 为 `rounded-4xl`，taxonomy 骨架 `rounded-full`，`library/page.tsx:55` 按钮占位 `rounded-4xl`；同一 taxonomy 查询 `category-filters.tsx:55` 用 `isLoading`、`metadata-search-page.tsx:90` 用 `isPending`；`category-filters.tsx:62` 的 `InlineError` 未传 `retrying`。
- [ ] **194.** SettingRow 控件宽度不一：emby / network Input 未设宽度，`pan-directory-row.tsx:36` `sm:w-64`，`data-source-section.tsx:98` `sm:w-72`，subscription `w-32`；`settings/shared.tsx:45` 对所有描述 `break-all`，会从中间截断英文单词。
- [ ] **195.** 两个弹窗关闭动画处理相反：`pan-login-dialog.tsx:53` `{open ? <LoginContent/> : null}` 立即卸载导致退出动画塌陷；同目录 `pan-directory-dialog.tsx:39` 特意保持挂载。
- [ ] **196.** Button 内图标：多数显式 `className="size-4"`（默认已是 size-4），subscriptions / actor-chip 不写；`emby-section.tsx:240`、`network-section.tsx:144` `mr-1.5 size-3.5` 与 Button 的 `gap-1` 叠加成双倍间距；`data-source-section.tsx:201` "未测速"使用不转的 `LoaderCircleIcon`（全站代表加载中）。
- [ ] **197.** 页面尺寸不一：`movie-detail/page.tsx:18` `max-w-7xl` + 额外 padding，其他页 `max-w-6xl`，推荐卡片与发现卡片尺寸不同；空状态 `library/page.tsx:129` `min-h-0 flex-1 py-12` vs discover 默认 `min-h-80`；行内空文案 `text-sm`（previews:24、recommendations:25、subscription-action:16）vs `text-xs leading-5`（movie-hover-details:51）；`AppPage` 已 `gap-6`，`discover/page.tsx:19`、`category-content.tsx:24`、`results.tsx:38` 又叠 `space-y-6` / `gap-6`。
- [ ] **198.** 链接 / 文本样式：`metadata-link.tsx:24` 链接 hover 变灰且无下划线，与不可点文本（`metadata.tsx:84`）无区分，`magnets.tsx:67` 却用 `underline underline-offset-4`；`magnets.tsx:155` 用 `wrap-break-word`，其他处用 `break-words`；`metadata.tsx:50-51` 标签行 `dt` 与 `EntityRow`（:88-93）行高不一；`tracking-normal` 冗余（`hero.tsx:24` 等）。
- [ ] **199.** 日期 / 数字格式化三种：`tasks-section.tsx:37` `toLocaleString`、`subscription-card.tsx:17` Intl 实例、`data-section.tsx:21` 另建 `numberFormat` → 统一到 `lib/format.ts`。
- [ ] **200.** Avatar 尺寸：`pan-account.tsx:10` 用 `className="size-12"`，`actor-chip` 用 `size="lg"`；演员条"全部新作"按钮（`actor-subscriptions.tsx:78` `px-4 py-3`）与 ActorChip（约 64px）高度不一，骨架又是 `h-16 w-64`；`pan-account.tsx:47` 与 :58 "剩余容量"显示两遍。
- [ ] **201.** `cursor-pointer` 添加方式不统一：`ui/tabs.tsx:56`、`ui/select.tsx:37`、`ui/switch.tsx:17` 单独一行，button / checkbox 写在基类 → `globals.css` base 层统一。
- [ ] **202.** 错误信息展示：`subscription-card.tsx:68` 用原生 `title`，`scan-progress.tsx:52` 用 `OverflowTooltip`；`scan-progress.tsx:37` 状态 Tooltip 挂在不可聚焦的 `<p>` 上，键盘无法触发，且与下方错误行重复。
- [ ] **203.** `features/access-gate/access-gate.tsx:46` 背景 `bg-muted`，其余页面 `bg-background`。
- [ ] **204.** `SettingsSection` 的 `icon={<XIcon className="size-4" />}` 重复传 9 次 → 接收 `LucideIcon` 内部控制尺寸；"数据源"与"数据与缓存"都用 `DatabaseIcon`（`data-source-section.tsx:87`、`data-section.tsx:45`）；`routes/settings.tsx:28-44` 手写 8 个 `<Separator />`，无 `features/settings/page.tsx`（与其他页结构不一致），最长页面却 `showBackTop={false}`。

---

## 十二、UI 复用（重复粘贴）

- [ ] **205.** `<TooltipTrigger asChild><Button size="icon">` 组合在 settings 约 9 处、`floating-nav`、`back-top-button`、`preview-viewer` 关闭、library 扫描、`movie-subscribe-button` 等处重复 → 抽 `IconButton({ label, ... })`，同一 label 同时用于 `aria-label` 与 `TooltipContent`；`data-section.tsx:60-61` 的 `title` 一并统一。
- [ ] **206.** `web/src/api/client.ts` 已有 describeApiError，订阅错误提示也已集中处理；设置页 emby-section、network-section、subscription-section 仍直接显示 Error.message，网络错误可能显示英文 Failed to fetch。接入公共错误文案，并为每个操作明确 inline 或 toast 的唯一展示位置。
- [ ] **207.** "测试连接 + 保存"操作栏 `emby-section.tsx:227-244` 与 `network-section.tsx:126-148` JSX 完全相同 → `settings/shared.tsx` 增加 `SettingsActions`。
- [ ] **208.** "服务端值 + 本地草稿"两种实现：emby 用 `form | null` 手工比较 7 个字段（`emby-section.tsx:48-57`），network 用两个 `useState<T | null>`（`network-section.tsx:26-33`）；emby 无 `<form>` 回车不提交，network 手写 `onKeyDown`（:120）→ 抽 `useDraft(serverValue)` + `<form onSubmit>`。
- [ ] **209.** 确认弹窗 `data-section.tsx:101-139` 与 `subscriptions/page.tsx:200-243` 结构一致（pending 时禁止关闭、`showCloseButton={!pending}`、InlineError、取消 + loader 确认）→ 抽 `ConfirmDialog`。
- [ ] **210.** `subscription-section.tsx:79-136` 三个 Select 块只差字段与选项 → 配置数组 map 或 `PreferenceSelect`。
- [ ] **211.** 卡片结构重复：`Card size="sm" className="h-full gap-0 overflow-hidden py-0"` + `aspect-3/2` + `CardContent className="space-y-2 p-3"` 在 `components/movie/movie-card.tsx:60-73` 与 `movie-grid-skeleton.tsx:23-25` 各写一遍（`overflow-hidden` 与 Card 默认重复，`gap-0 py-0 p-3` 覆盖 size="sm"）；卡片链接 class 三处（`movie-card.tsx:18`、`library/movie-card.tsx:42`、`recommendations.tsx:59`）；`MovieCover` 外框三处（`movie-card.tsx:61-64` 的 flex 居中无效、`movie-hover-details.tsx:26`、`hero.tsx:12`）→ `MovieCardShell` / `MovieCardLink` / MovieCover 自带比例框。
- [ ] **212.** 影片信息栏 `movie-detail/hero.tsx:44-73`（MovieStats）与 `library/movie-hover-details.tsx:34-46` 重复判断与格式化 release_date / duration / rating → `movieStats(movie)`。
- [ ] **213.** discover 分类派生 `category-content.tsx:18-21` 与 `category-filters.tsx:27-30` 逐字相同；分区 Select `category-filters.tsx:35-53` 与 `metadata-search-page.tsx:69-88` 两份；`mainOptions` 一处用常量 `MAIN_CATEGORY`、一处用字面量 `'main'`（:43）→ `useDiscoverTaxonomy` + `ZoneSelect({ allowAll })`。
- [ ] **214.** 详情页 section 标题 `<section className="space-y-4"><h2 className="text-xl font-semibold tracking-normal">` 在 `magnets.tsx:63`、`previews.tsx:20`、`recommendations.tsx:23` 重复 → `DetailSection`；`previews.tsx:20` 的 `id="movie-previews-title"` 无人引用。
- [ ] **215.** `features/tasks/task-notification-diff.ts` 仍重复定义 isScanTask/isBatchTask/isTaskActive，以及 `api/offline.ts` 已有的 isOfflineTaskActive。通知判定循环也有重复。评估共用状态判断并提取真正重复的通知条件，避免再创建只作转发的层；原 lib/offline-state.ts 已删除。
- [ ] **216.** `features/tasks/task-toast.tsx:42,71,123` 三处 `icon: options.waiting ? undefined : <LoaderCircleIcon…/>` → 并入 `taskToastOptions`；"等待同步"scan 改标题、offline / batch 改描述，处理不一致。
- [ ] **217.** `settings/pan-directory-dialog.tsx:83-109` 手写上一页 / 下一页且无 aria-label，现成 `ListPagination` 已支持 `hasMore`、可不传 totalPages 和 `scrollToTop={false}`。
- [ ] **218.** 本地 NFO 解码（`scan/local.go`）与 `scan/nfo.go` 的 `readNFO` 两套 → 抽 `decodeNFO(body, fallbackName)` 共用。
- [ ] **219.** 测试桩复制 5 份约 1500 行：`internal/app/drive_fixture_test.go`（371）、`internal/library/fixture_test.go`（367）、offline（363）、drive（292）、strm（95），app 与 library 两份几乎相同 → `internal/pan/pantest`，或像 `strm/fixture_test.go` 那样嵌入接口只覆盖需要的方法。

---

## 十三、UI 合理性

- [ ] **220.** 破坏性操作确认策略倒置：风险最低的"清理未使用缓存"有确认弹窗；退出 115 登录（`pan-section.tsx:78`）、取消挂载目录（`pan-directory-row.tsx:62`）、取消演员订阅（`actor-chip.tsx:93`）、取消影片订阅（`subscription-card.tsx:111`）均一键执行 → 统一使用 ConfirmDialog。
- [ ] **221.** 批量入库失败已只在弹窗内展示。成功时，`subscriptions/page.tsx` 的“已创建任务”toast 与 task-toast.tsx 的任务通知仍可能同时出现；评估保留一条可持续更新的任务通知，避免重复反馈。
- [ ] **222.** `data-section.tsx:28` `busy` 包含 `info.isFetching`，每次后台刷新清理按钮闪一下禁用；刷新统计按钮放在"数据目录"行（:56），位置与作用不符。
- [ ] **224.** `pan-directory-row.tsx:35` 用 `disabled` Input 展示当前路径（opacity-50 变淡、无法选中复制）→ `readOnly`。
- [ ] **225.** `movie-detail/magnets.tsx:48,55` "已复制"状态永不恢复；:108 复制失败提示在整个 section 底部，远离按钮。
- [ ] **226.** `search/page.tsx:31-38` 无关键词时下方完全空白；`components/page-back-button.tsx:14` ghost 按钮 `px-3` 使返回图标比标题右缩 12px，:17 兜底写死 `/discover`。
- [ ] **227.** `web/index.html`：主题由 next-themes 在 JS 执行后才加 `.dark`，暗色用户首屏闪白；无 favicon（无 `public/` 目录，会 404）；无 `color-scheme` / `theme-color` meta → head 内联脚本读取 `localStorage['miyabi-theme']` 提前设 class。
- [ ] **228.** `components/media-image.tsx` 的 `className` 只作用于前景图，命名误导 → `imageClassName`。
- [ ] **229.** `components/movie/movie-card.tsx:65` `max-w-[calc(100%-3.5rem)]` 与订阅按钮尺寸隐式耦合，无浮层按钮的卡片也被限宽；`preview-viewer.tsx:37` `FRAME_RADIUS = 18` 按 `--radius` 手算；`preview-viewer.tsx:116` `scrollbar-none` 后的 `[&::-webkit-scrollbar]:hidden` 冗余；`preview-viewer.tsx:24-28` 重复 import react；`globals.css:116-134` 手写 scrollbar 规则。

---

## 十四、可访问性

- [ ] **230.** 纯图标控件无可访问名称：`components/floating-nav.tsx:44-52` 主导航 5 个 Link（lucide 默认 aria-hidden，Tooltip 只提供 describedby）；`back-top-button.tsx:16-24`；`preview-viewer.tsx:96-98` 关闭 X；`search/page.tsx:61-63` 提交、:76-84 清空；`library/page.tsx:59-90`（移动端文字 `hidden sm:inline`）；`pan-section.tsx:36,58`（无 Tooltip）、:72；`pan-directory-row.tsx:42,57`；`data-source-section.tsx:131`；`pan-directory-dialog.tsx:88,97`；`actor-chip.tsx:62`（Switch）、:74、:88；`task-toast-actions.tsx:15`；`appearance-section.tsx:22` 主题 Tab 只有图标。
- [ ] **231.** `components/media-image.tsx:61-80` 所有 `<img>` 无 `alt`（模糊背景图应 `alt="" aria-hidden`）；`previews.tsx:28`、`preview-viewer.tsx:121` 缩略图按钮无 `aria-label`。
- [ ] **232.** ✔ 交互元素嵌套：`components/movie/movie-card.tsx:15-34` `<Link>` 内经 `coverOverlay` 渲染 `MovieSubscribeButton`（`<a><button/></a>`）；`subscription-card.tsx:86-116` 同样；选择模式整卡 Button + Checkbox 两个 Tab 停靠点，且用 `hover:bg-transparent hover:text-current` 抵消 ghost 样式 → 参照 `recommendations.tsx:84-100` 将按钮作为 Link 兄弟节点。
- [ ] **233.** 表单无名称：`settings/shared.tsx:44` SettingRow 标题是 `<div>` 而非 `<label>`，设置页所有 Switch / Input / Select 无可访问名称（emby 6 个 Input）；`search/page.tsx:65` 搜索框、`access-gate.tsx:68` 密码框仅 placeholder；`category-filters.tsx:41/71/93` 三个 `SelectTrigger` 无 `aria-label`（`common-filter-select.tsx:25` 有）。
- [ ] **234.** 其他：`preview-viewer.tsx:90-94` `DialogContent` 缺 `DialogTitle`（Radix 控制台报错）；`ui/pagination.tsx:9` nav 无 aria-label，:49-85 Prev/Next 移动端文字 hidden 且无 aria-label；`floating-nav.tsx:26-29` `<ul>` 直接子元素不是 `<li>`、nav 无 aria-label、`nav:25` 的 gap 无效；`list-pagination.tsx:44-53` 页码用 `<a href="#">` 而 Prev/Next 是 `<button>`；`empty-state.tsx:20` 颜文字无 `aria-hidden`；`movie-hover-details.tsx:38-41` `aria-label` 加在无 role 的 `<span>` 上被忽略。

---

## 十五、安全（顺带发现）

- [ ] **235.** 核查日志中的敏感查询参数：`internal/api/router.go` 请求日志可能记录 STRM URL 的 `token`。Emby client 已统一使用 X-Emby-Token 请求头，网页登录 URL token 通道也已删除，不再列为待办。对实际日志入口统一脱敏，并验证错误日志和代理日志是否仍会泄漏。
- [ ] **236.** `GET /api/settings/emby` 明文返回 APIKey → 打码返回，提交时回填。
- [ ] **237.** `internal/api/auth.go:164` 无论代理是否可信都信任 `X-Forwarded-Proto`；`router.go:45-49` `SetTrustedProxies` 错误被忽略，解析失败时部分列表静默生效 → 在 `config.Load` 校验。

---

## 十六、测试臃肿

- [ ] **238.** `internal/codeid/prototype_test.go` 以 `prototypeResolve` / `prototypeMatch` 重写一遍 `javdb.ResolveMovieID`，`prototypeQueries` 已与生产 `Queries` 分歧，文件头注释过时——测的是副本而非生产代码 → 迁到 javdb 包基于 fixture transport 测真实实现；`corpus_test.go:18` 与 `random_corpus_test.go:12` 语料加载器几乎相同 → `loadSamples(t, file, n)`。
- [ ] **239.** 随生产路径清理重新评估相关测试：scan/subtitle_test.go 的旧命名与断言、scrape/dir_cache_test.go、subtitle 中 115 导出路径等。UploadSidecar 已删除；snapshot_test.go 已覆盖 movie 快照，不应按旧快照测试整体删除。保留断点恢复、快照失效、历史封面任务 JSON 兼容等有效回归测试，按当前生产调用逐项决定删除或迁移。
  - 第 82–83 项已删除远端目录快照和写回校验专用测试/基准，保留并更新电影快照、视频指纹、数据源隔离、断点和事务测试；旧 ArtworkOrigin 格式测试改为旧封面任务通过 JavDB 恢复的集成测试。
  - 第 78–80 项已删除旧 115 字幕扫描测试和专用实现，将混合导出测试改为在线字幕测试，并增加旧字幕存储迁移测试；这部分不再作为待办，其余测试归属与覆盖评估保留。
- [ ] **240.** 复核低收益或重复测试：runtime 常量、简单 domain helper、任务容器基本操作、健康检查重复路径，以及 app 包中实际测试 library/scrape 的用例归属。pan 客户端测试已增加真实请求路径覆盖，不能沿用“未测 New”的旧结论；前端测试已迁移为 `.test.ts`，pan-login-poll.test.ts 的失败预算与恢复行为应保留，是否缩减需按实际覆盖收益判断。

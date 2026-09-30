# Miyabi 审计与复查清单

> 初始审计：2026-09-27，基于提交 `f79e501`。
> 清单整理：2026-09-30；本轮移除复查通过的第 112–125 项，共 14 条。当前保留 111 条，均为待处理项。
> 范围：后端 Go（不含 ent 生成代码）与前端；测试仅评估覆盖收益、重复和职责归属。
> 原编号保留，不重新编号；完成项经复查确认后可移除，已提交的历史记录可通过 Git 追溯。第 37 项未完成的导出默认值清理保留在第 184 项。
> 本轮复查范围为第 112–125 项的实现、生产调用链与回归覆盖；其余待办和历史已移除条目不在本轮复查范围内。保留的旧行号仅作定位线索；✔ 表示初审时复核过，不代表当前仍成立。

## 本轮复查结论（2026-09-30）

- 结论：第 112–125 项全部通过复查，未发现本轮修复范围内的问题遗漏、新增回归或需要进一步重构的实现；删除完成记录及空章节，保留有效回归测试。本次仅修改审计文档。
- 核对重点：必需依赖的生产初始化、令牌刷新次数与来源快照、备用域名损坏后的健康线路保留、磁力校验与推断归属、字幕关闭等待、离线任务身份与终态保护、历史数据迁移校验，以及前端登录缓存时序与错误组件布局。必要边界检查保留，不按原始审计建议机械删除。
- 验证：Go 全量测试、go vet、前端全部 22 个测试文件 / 124 项测试、TypeScript、oxlint、oxfmt 格式检查、Vite 生产构建及 git diff --check 均通过。未进行浏览器或真实上游联调；CGO_ENABLED=0，未运行 race 检测。
- 保留问题：shadcn CLI 因现有依赖引用 zod/v3 启动失败（ERR_PACKAGE_PATH_NOT_EXPORTED），用户要求保留依赖现状，尚未修复。应用直接导入 cn 已通过类型检查和构建。
- 已接受的行为边界：不猜测性重排此前已迁移的浏览记录；影片资料与实时状态仍分别请求。第 244 项继续暂缓。

## 处理顺序与原则

- **优先核实行为与安全问题**：235、237（敏感日志与代理信任）、167（图片抓取边界）、244（非番号误识别）。
- **再处理明显的阻塞与重复工作**：129、131、142、148、149；按当前调用链和测试确认影响。
- **结构整理按模块进行**：184；保留任务分池、重启恢复、错误退避和协议兼容语义。
- **前端优先保证交互与可访问性**：186–189、206、220–221、230–234，再处理布局与样式统一。
- **死代码与测试一起核对**：238–240。只有确认失去生产职责和有效覆盖后才删除；不能只凭文件短、单实现或前端未调用下结论。
- **方案是候选，不是强制指令**：已有公共实现优先复用；历史迁移的删除需先明确支持的升级起点，不能以本机迁完代替兼容策略。
- **完成后复查再清理**：完成时记录实现、验证结果及未验证范围；经复查确认后可删除。混合条目只移除已解决部分，保留剩余问题；历史编号可通过 Git 追溯。

---

## 一、正确性 / 并发 Bug

- [ ] **244.** `internal/codeid/codeid.go:38-55,59-93` `filenamePattern`：正则前缀匹配规则对个别非番号文件名存在误提取（如相机默认命名的 `VID-20220326`、`IMG-XXXX`、包含分辨率标签的 `MAYLI-1080P` 等被当作番号提取并提交上游查询）→ 建立常见非番号前缀黑名单（如 `VID`, `IMG`, `DSC`, `MOV` 等）与分辨率标记过滤，避免无效请求与限流损耗。
  - 暂缓：按用户要求撤回本次特例过滤及对应测试，不增加相机前缀、时间戳或分辨率硬编码规则；后续如处理，应先评估更通用的方案。已有通用候选降级规则保持不变。

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
- [ ] **145.** `internal/javdb/movie.go:105-141,147-174` `ResolveMovieID` 每次查询前对累积的数百候选全量 `matchCandidates`，`codeid.Normalize` 每次最多 8 个正则，`IsFormatEquivalent` 两边重复 Normalize → 预计算 `normCode`。
- [ ] **146.** `internal/javdb/route.go:112` 非 full 模式用 5ms ticker 轮询（最长 60s，每秒唤醒 200 次）→ 按最早截止时间的单个 `time.Timer`。
- [ ] **147.** `internal/subtitle/label.go:89-100` `[]rune(text)` 将最多 10MB 文本整体转换（约 40MB 分配）后再截取前 15000 个；`containsRune` 在 40 元素切片中线性查找 → range 计数 + map。
- [ ] **148.** `internal/monitor/check.go:48-56` 每轮只查 10 部影片 / 5 个演员后等 5 分钟，500 个订阅约 2.8 小时才查完；`service.go:137` `EnqueueSingle` 每条读一次 config、新建 Picker；`BatchHandler` 先 `Subscription.Get` 一次，`EnqueueSingle` 再 Get 一次；`check.go:94-108` "继续等待"分支更新 checks 却不发 `NotifyMonitorChanged`。
- [ ] **149.** `internal/library/library.go:222` 分页 `ORDER BY created_at DESC, id DESC`，movie 表无 `(created_at, id)` 索引；`scan/local.go:223-227` fileID 取 `sha256(rel)`，不同扫描根目录下相对路径相同会冲突，已存在记录不更新 path。
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
- [ ] **168.** `internal/gfriends/client.go:236` `errors.Join(errors.New(...), lastErr)` → `fmt.Errorf("fetch avatar: %w", lastErr)`；:113 Filetree 读取无大小限制。 头像下载错误已改为 fmt.Errorf 包装；Filetree 大小限制仍待处理。
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
- [ ] **184.** 导出默认值仍有重复：`internal/export/export.go` 保留 `./data/emby` 和硬编码 8080 的 defaultPublicURL，与 config.Load 推导不一致；app 的 EmbyDir 兜底已删除。统一在配置入口确定，底层导出使用明确配置。另核实前端 emby-url.ts、network-section.tsx 的 URL scheme/host 格式化重复，以及 netx/ip.go 注释与实现的差异。

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
  - 已删除远端目录快照和写回校验专用测试/基准，保留并更新电影快照、视频指纹、数据源隔离、断点和事务测试；旧 ArtworkOrigin 格式测试改为旧封面任务通过 JavDB 恢复的集成测试。
  - 已删除旧 115 字幕扫描测试和专用实现，将混合导出测试改为在线字幕测试，并增加旧字幕存储迁移测试；这部分不再作为待办，其余测试归属与覆盖评估保留。
- [ ] **240.** 复核低收益或重复测试：runtime 常量、简单 domain helper、任务容器基本操作、健康检查重复路径，以及 app 包中实际测试 library/scrape 的用例归属。pan 客户端测试已增加真实请求路径覆盖，不能沿用“未测 New”的旧结论；前端测试已迁移为 `.test.ts`，pan-login-poll.test.ts 的失败预算与恢复行为应保留，是否缩减需按实际覆盖收益判断。

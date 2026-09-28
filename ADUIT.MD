# Miyabi 代码审计清单

> 审计日期：2026-09-27（基于提交 `f79e501`）
> 范围：后端 Go（不含 ent 生成代码）+ 前端 web/src 全量非测试文件，测试文件仅检查臃肿问题。
> 标记说明：✔ 表示已亲自打开源码复核；其余已通过 grep / 通读确认。编号全局连续，修复后勾选即可。

---

## 修复优先级

- **P0（直接影响使用）**：1、2、3、10、8、9、13、11、12、7
- **P1（结构性，一改清一片）**
  - 37 + 4/5/6：统一导出配置
  - 41 + 82~85 + 126/133：快照移出 task 表
  - 78~81：删除字幕残留并重新生成 ent
  - 64 + 39 + 40：精简任务框架
  - 44：认证只保留 cookie
- **P2（前端）**
  - 抽 IconButton / ConfirmDialog / useDraft / 订阅按钮 hook
  - 统一设置页保存与错误处理
  - 删掉 66、67 两个手写��存
- **其余**：死代码与过度防御，按包批量清理

---

## 一、正确性 / 并发 Bug

- [x] **1.** ✔ `web/pnpm-lock.yaml` 仍残留 `@vidstack/react`、`hls.js`（7fe1cba 只改了 package.json），Dockerfile `pnpm install --frozen-lockfile` 必然失败 → 本地 `pnpm install` 重新生成锁文件。
- [x] **2.** ✔ `internal/library/scan/walker.go:98`、`:156` 断点续扫每次生成新 scanID，却只遍历剩余目录；`reconcile.go:47` 的 `ScanIDNEQ` 会把重启前已��目录的文件判为过期删除，连带影片与刮削结果 → scanID 持久化进 payload，续扫沿用。
- [x] **3.** ✔ `internal/library/scrape/export.go:76` `RewriteSTRM` 仅在"没有 token"时追加，token 轮换后旧值保留 → 全部 401。应解析 fileID 后用 `STRMContent` 整行重生成；`"/api/strm/play/"` 在 `export.go:39`、`:70`、`scan/local.go:26` 三处硬编码需收敛。
- [x] **4.** 导出配置数据竞争：`internal/emby/service.go:200` `UpdateConfig` 在 HTTP goroutine 无锁写 `scan.Scanner` / `scrape.Service` 的 `embyDir/publicURL/strmToken`，任务 worker 同时在读（`cover.go:266`、`walker.go:115`）。
- [x] **5.** 导出目录双来源：`internal/app/app.go:142-154` 用环境变量 `cfg.EmbyDir`，emby 用数据库中的 `LocalDir`（`emby/service.go:70`），修改其一后 Emby 通知路径错误；`translatePath` 兜底返回 `mediaPath/<basename>`（`emby/service.go:474`）也是错误路径，应跳过。
- [x] **6.** `internal/app/app.go:152`、`internal/emby/service.go:205` 裸 goroutine 执行 `RewriteSTRM`，不受 ctx / WaitGroup 管理，会与导出任务并发写同一批 .strm。
- [x] **7.** ✔ `internal/magnet/aggregator.go:92` + `internal/catalogue/magnets.go:33`：关闭的 JavBus 返回 `nil, nil` 被计为成功，JavDB 失败时返回空列表不报错，并被缓存 1 分钟（`catalogue/cache.go:85`）；monitor 当作"无磁力"处理甚至标记 stale → 区分"跳过"与"成功为空"，部分失败不入缓存。
- [x] **8.** ✔ `internal/netx/ssrf.go:134` SSRF 拨号检查同时作用于代理地址，`127.0.0.1:7890` 等本机 / 局域网代理被拦 → 开代理后字幕下载全部失败。仅在直连时检查。
- [x] **9.** ✔ `internal/netx/client.go:39` "Direct" 客户端克隆 `http.DefaultTransport`，自带 `ProxyFromEnvironment`，Docker 设了 `HTTPS_PROXY` 时 115 流量走代理；`netx/client_test.go` 靠 `Setenv` 清空环境变量掩盖了问题 → 显式 `transport.Proxy = nil`。
- [x] **10.** ✔ `internal/pan/client.go:147` resty 对 POST 同样重试：`/open/refreshToken` 超时重试会用已轮换的旧 refresh_token → 凭证丢失；`add_task_urls` 重试返回 10008 被当成冲突；`upload/init` 同理 → 只对 GET / 429 重试，或对 token/离线/上传请求关闭重试。
- [x] **11.** ✔ `internal/monitor/batch.go:102` 批量任务重启后从 index 0 重跑，`Processed` 在旧值上继续累加 → 进度 >100 触发 schema `Range(0,100)` 校验，之后进度永远保存失败 → 从 `payload.Batch.Processed` 处继续。
- [x] **12.** ✔ `internal/tasks/pool.go:66`、`:87` Claim / Finish 遇到一次数据库临时错误（SQLITE_BUSY）就 `return err` → errgroup 取消 → `App.Run` 关闭整个 HTTP 服务 → 记录日志并退避重试，仅 ctx 结束时退出。
- [x] **13.** ✔ `web/src/api/client.ts:104` 所有非 `/api/auth/` 的 401 都会 `clearAuthToken()` 并派发 `miyabi:unauthorized`，而 115 登录失效（`pan.ErrUnauthorized`）、Emby API Key 错误也返回 401 → 仅在后端 `code: "UNAUTHORIZED"`（访问密码失效）时触发，或 115 失效改用独立状态码。
- [x] **14.** `web/src/api/movie-state-cache.ts:73` 用 `initialData` 伪造 `not_in_library`，加载中 / 失败时被当成真实数据 → `features/subscriptions/subscription-card.tsx:56` 可对已入库影片点"入库" → 改 `placeholderData`，`resetMovieStates` 同理。
- [x] **15.** ✔ `web/src/api/subscriptions.ts:52` 订阅列表固定 `limit: 100` 只取第 1 页，`useSubscription`（:57-63）用 `find` 查找 → 第 101 条起显示"未订阅"，订阅页也显示不全。
- [x] **16.** ✔ `internal/library/scrape/cover.go:130-149` 同一影片分布在多个 115 目录时，每个目录各导出一次到同一 `EmbyMovieDir` → `stem.strm` 互相覆盖、cd 编号错乱；`ExportLocalMovie`（`export.go:150`）不排序 → 先收集全部视频、排序后统一导出一次。
- [x] **17.** ✔ `internal/export/export.go:39`（原 `library/scrape/export.go:26`）`EmbyMovieDir` 直接用原始 code 作目录名，含 `/` `\` `:` 时产生嵌套或非法路径 → 使用 `nfo.FileStem(code)`。
- [x] **18.** ✔ `internal/library/scan/local.go:177` 本地 NFO 未做 `codeid.Normalize` 且无空值回退，`SetCode("")` 违反 NotEmpty；同段多处 `_ =` 吞错（含 `:244`）。
- [x] **19.** ✔ `internal/monitor/subscribe.go:51`、`actor.go:71` 遇 `ent.IsConstraintError` 无限递归自身，外键约束也会命中 → 栈溢出风险 → 只重查一次。
- [x] **20.** ✔ `internal/monitor/actor.go:37-69` + `check.go:140-143` 空作品演员 `SeenMovieIDs` 为空，被当成"尚无基线"，首批新作只记基线不生成订阅 → 增加显式"已初始化"标记。
- [x] **21.** ✔ `internal/emby/service.go:147-161` `ScheduleActorSync` 的 `Timer.Stop` 挡不住已在运行的同步 → 多个 `SyncActorAvatars` 并发；找不到头像的演员每轮都重查 GFriends / DB / JavDB → `atomic.Bool` 单飞 + 负缓存。
- [x] **22.** ✔ `internal/pan/play.go:92-95` + `internal/strm/relay.go:47,77-81`：`DownloadURL` 用播放器 UA 签名，`OpenMedia` HEAD 探测时空 UA 被替换成 `"Miyabi/1.0"` → 签名不匹配 403；回退 `PlayURL` 时同样不一致 → UA 由 strm 统一决定并透传。
- [x] **23.** ✔ `internal/strm/relay.go:47-49` `DownloadURL` 的所有错误（含 Unauthorized、SourceChanged、ctx 取消）都被吞掉并回退 `PlayURL` → 仅在"无直链"时降级。
- [x] **24.** ✔ `internal/drive/account.go:54-74` 账号缓存未绑定 `credentialVersion`，旧凭证的在途 fetch 可能在重新登录后写回旧账号 → 60s 内误清新账号挂载。
- [x] **25.** ✔ `internal/netx/proxy.go:83-97` `Update` 的 Load / Store 非原子；`internal/app/network.go` 的 `UpdateNetwork` 先写 DB 再更新内存，不在同一把锁内；`Normalize` 调用两次。
- [ ] **26.** `internal/catalogue/cache.go:113-121` 上游成功后 `persistActiveRoute` 写库失败，会让本次成功的数据变成错误返回且不入缓存 → 记日志后忽略。
- [ ] **27.** ✔ `internal/domain/error.go:132-143` `PublicMessage` 无公开消息时返回 `err.Error()` → 500 把 SQL、文件路径、validator 原文（如 `Key: 'discoverSearchQuery.Query' Error:...`）返回前端 → Unexpected/Internal 一律返回默认文案。
- [ ] **28.** `internal/api/movie.go:72` `req, _ := bindJSON(...)` 忽略绑定失败，`c.Errors` 已挂 400 仍去扫默认目录。
- [ ] **29.** `internal/api/subscription.go:171-179` PUT 返回用户原始输入，而非 `normalized()` 后实际保存的值（如 `check_time` 为空存 "00:00" 却返回 ""）。
- [ ] **30.** `internal/subtitle/service.go:87-105` 既无 PickCode 也无 SourceURL 的 pending 记录不进任何分支，永远不清理 → 增加 default 分支删除。
- [ ] **31.** `web/src/features/tasks/task-events.tsx:127-135` SSE 遇 401 后变为 CLOSED，每 3 秒无限重连且不会触发未授权处理。
- [ ] **32.** `web/src/features/settings/pan-login-dialog.tsx:92,102-107` `refreshed` ref 整个会话只置位一次，第二张二维码过期不再自动刷新，但 :117 文案仍显示"正在刷新" → 按 `session.id` 重置。
- [ ] **33.** ✔ `web/src/features/settings/data-section.tsx:93` 按钮文案"清理所有缓存"，实际（:109 标题、:39 toast）只清理未使用的图片缓存 → 改为"清理未使用缓存"。
- [ ] **34.** `web/src/api/browse-history-store.ts:158-175` visibilitychange 触发 `flush()` 未返回时 pagehide 又用 beacon 发同一批 → 重复 POST；beacon 发送后不清 pending，下次启动再发一次。
- [ ] **35.** `internal/javbus/parser.go:19,203-214` 正则允许 `PB`，switch 中无 PB 分支，倍率落到 1。
- [ ] **36.** `internal/emby/service.go:443-475` `translatePath` 中 `strings.HasPrefix(rel, "..")` 会把 `..foo` 目录误判为越界；方法未使用 receiver。
- [x] **241.** ✔ `internal/library/scan/reconcile.go:58-71`、`persist.go:197-210`：用户在 115 删除影片后重新扫描，仅删除了 SQLite 的 `File` 与无引用 `Movie` 记录，未清理 `EmbyDir` 本地对应的影片目录（`.strm`、`.nfo`、海报等），且未向 Emby 发送删除/变更通知，导致 Emby 媒体库残留死链幽灵影片。
- [x] **242.** ✔ 废除 JavBus 强制绑定代理限制，移除冗余的手动设置项，改为全自动连通性健康探测与自适应降级：删除后端 `/api/settings/javbus`（连同 `internal/api/javbus.go`）及前端 JavBus 开关与 API；`javbus.Client` 内部自适应维护连通性探测与熔断状态，网络可达时自动聚合磁力，不可达时 0ms 静默跳过（`ErrSkipped`），代理配置变更时自动重探自愈（一并解决第 175、178、223 条）。



---

## 二、架构不合理

- [x] **37.** 导出配置 `(embyDir, publicURL, strmToken)` 复制在 `scan.Scanner`、`scrape.Service`、`library.Service`（仅透传）、`emby.Service`、`app.go` 共 5 处，靠 `SetEmbyExport` / `SetSTRMToken` / `SetSTRMExporters` 扇出同步（4~6 的根源）；scrape 另有 `defaultEmbyDir = "./data/emby"` 与写死 8080 端口的 `defaultPublicURL()`（`export.go:18-22`），与 `config.Load` 推导不一致 → 新建 `internal/export`（或并入 strm），持有��一 `atomic.Pointer[ExportConfig]` 并负责 STRM/NFO/图片目录布局与重写。
- [ ] **38.** 依赖方向反转：emby 导入 `library/scrape`（为 `RewriteSTRM`），scrape 又通过 `MediaNotifier` 回调 emby；`internal/database/indexes.go:8` 导入 tasks���仅为常量）；`internal/app/deps_test.go:17-18` 的 business 列表漏了 emby、subtitle，因此未拦截。
- [ ] **39.** `internal/tasks` 通用包内含业务逻辑：`SubscriptionBatchPayload` / `EnqueueSubscriptionBatch` / `SaveSubscriptionBatch` / `BatchTaskInfo`（batch.go）、`EnsureScanTask` / `EnqueueScan` / `EnqueueFreshScan` / `ScanTaskInfo` / scrape-cover 折叠规则（workflow.go:219-248）、各业务 payload JSON 路径常量（payload.go:10-24）→ 分别迁回 monitor、library。
- [ ] **40.** offline 下载记录存于 task 表（`KindOffline`，从不进入 pool），依赖 JSON payload + 手写表达式索引（`database/indexes.go`）查询；`scan/persist.go:17` 跨包用字符串 `"file_ids"` 调 `SetPayloadField` 改写 offline payload，而 offline 自身整体重编码 payload，二者前提矛盾；各处 `StatusQueued` 判断（add.go:53、sync.go:38/213、projection.go:86/145）对 offline 永不成立 → 独立 ent 表。
- [ ] **41.** cover 快照存于已完成 cover 任务的 payload（`scrape/snapshot.go:153-190`、`scrape.go:167-169`），导致 task 表永远不能清理、每次扫描需对全表 `json_extract` → 快照 / `videos_fingerprint` 存到 movie 行。
- [ ] **42.** `internal/config/runtime.go:24` `TaskPoolWorkers: 1` 硬编码：订阅批量（每条 sleep 1.5-3s，500 条约 30 分钟）与 scan/scrape/cover 共用唯一 worker，阻塞所有扫描（含离线完成后的 `EnqueueTargetedScan`）→ 批量任务独立执行。
- [ ] **43.** `POST /api/library/scan/local`（`internal/api/movie.go:70-84`）在 HTTP 请求中同步 `WalkDir` 并写库，绕过"扫描必须串行"的队列约束，路径任意，前端无入口 → 删除或改为入队任务并限制在 EmbyDir 内。
- [ ] **44.** 认证三通道并存：HttpOnly cookie（`api/auth.go:97`）+ JSON 返回 token 存 localStorage 加 Bearer（`web/src/api/client.ts:63-96`、`auth.ts:34-36`）+ `?token=` query（`auth.go:155`）。EventSource / img / beacon 已依赖 cookie，localStorage 这份多余且抵消 HttpOnly 的防 XSS 作用 → 只保留 cookie。
- [ ] **45.** HTTP 客户端三套栈用法混乱：resty（pan、subtitle、javdb/media）、tls-client（javdb、javbus）、net/http（emby、gfriends）；`app.go:125-134` 手工复刻 `NewRestyClient` 的动态代理逻辑；"有 manager 用 NewRestyClient 否则 Direct"在 `subtitlecat.go`、`xunlei.go`、`javdb/media.go` 各写一遍（media.go:18-20 先建 Direct 再覆盖）；`SafeDownload`、`OpenMedia` 只取原始流却用 resty → netx 提供 `NewTransport(manager)`，nil 安全。
- [ ] **46.** javdb 与 javbus 重复实现：`httpClient` 接口、watchProxy 重建 fingerprint client（javbus 用 RWMutex、javdb 用 atomic）、Close 中冗余双重判断、导出的 `Probe`（`javbus/client.go:31-34,244-288`、`javbus/probe.go` vs `javdb/client.go:204-233,409-414`、`transport.go:16-19`）→ netx 提供 `ProxiedFingerprintClient`。
- [ ] **47.** `internal/javbus/cache.go` 与 `internal/catalogue/cache.go` 两套缓存；javbus 版"上限 512"只清过期项，实际无上限；`get` 在 RUnlock 后再 Lock 删除存在 TOCTOU；外层已按影片缓存磁力 → 删除 javbus 缓存。
- [ ] **48.** `internal/catalogue/types.go:89-102` `Provider` 11 方法接口仍返回 `javdb.RouteStatus`，未真正解耦；`NewWithProvider` 仅测试使用，`magnets.go:63-65` 的 `aggregator == nil` 分支只为它存在且行为与生产不一致；`javbus` 字段是具体类型、`javdb` 是接口。
- [ ] **49.** 同一份路由信息 4 个结构体：`javdb.RouteStatus`、`routeResult`（字段完全一致，`client.go:341-345` 逐字段搬运）、`catalogue.RouteStatus`、`catalogue.persistedRoute`；`catalogue.Service.route` 镜像 javdb 路由状态（`service.go:48-49,153-158,295-319,407-430`）。
- [ ] **50.** 迁移分散：`database/sqlite.go:53,61` `dropPlayerStorage` / `migrateSubscriptions` + `app.go:62` `MigrateViewedMovies`，每次启动做 pragma / sqlite_master 探测且无版本记录 → `PRAGMA user_version` 有序迁移列表。
- [ ] **51.** `internal/app/network.go` 的 `NetworkService` 是业务服务却放在组合根包，`Network(ctx)` 的 error 恒为 nil，:75 重复传超时；`internal/netx/probe.go` 的 `NetworkTestResponse`（含 javdb/javbus 字段）是 API DTO 却放在通用网络包。
- [ ] **52.** `internal/app/app.go:135-150` 构造后 Set* 注入：`SetGFriends`、`SetMediaFetcher`、`SetSubtitles`、`SetMediaNotifier`、`SetPacing(scan.DefaultPacing)`、`SetSTRMToken`、`SetSTRMExporters`，存在时序耦合；`SetPacing` 传入的就是唯一生产值 → 非循环依赖走构造参数。
- [ ] **53.** 错误体系：`KindInternal` 从未被构造（仅 `api/error.go:72` 作日志标签）；`HasKind` 接口与 `*Error` 两套分类，`IsKind`（error.go:119）只认 `*Error`、`KindOf`（:146）两者都认，行为不一致；`KindBusy` 注释写 409/503 实际只映射 409；登录限流应返回 429。
- [ ] **54.** 前端为绕开 node test 不支持 `@/` 别名，把模块拆成"纯逻辑 + hooks"两半：`movie-detail-cache.ts`/`discover.ts`、`movie-state-cache.ts`/`movie-states.ts`、`browse-history-store.ts`/`browse-history.ts`、`task-progress-state.ts`/`task-progress.tsx`、`lib/offline-state.ts`（5 行），并为 `.ts` 后缀导入开启 `allowImportingTsExtensions` → 使用 package.json `"imports"`（`#/*`）两端原生支持，或换 vitest（需自行安装）。
- [ ] **55.** 前端 discover 浏览状态放在内存 zustand（`web/src/stores/discover.ts`），而 library / search 用 URL search params；`metadata-search-page.tsx:47-51` 用 effect 双向同步；订阅页 tab / 选中演员（`subscriptions/page.tsx:26-27`）用本地 useState，返回即丢失 → 统一 `validateSearch`。
- [ ] **56.** 前端错误处理位置不统一：`api/subscriptions.ts` 在 hook 内 toast，其余在组件；"115 登录已失效"映射在 `library/page.tsx:72-76` 与 `movie-detail/magnets.tsx:192-196` 各写一遍；"query.error → invalidate account"在 `pan.ts:121-125`、`library.ts:71-76`、`offline.ts:90-94`、`pan.ts:140-144` 重复 → `QueryCache/MutationCache({ onError })` 统一。
- [ ] **57.** pan 包 json tag 语义混用：`pan.File`、`FilePage`、`Account` 带 API 输出 tag，被 `api/pan.go` 直接返回；`pan.OfflineTask` 的 tag 却是 115 线上字段名（如 `percentDone`）。

---

## 三、上帝代码

- [ ] **58.** `internal/emby/service.go`（645 行）：配置持久化（三处合并 defaultPublicURL：:67-76、:168、:180）+ HTTP client + 批量通知 worker + 全库刷新 + 演员头像同步（GFriends/JavDB 回退 + DB 查询）+ STRM token 与导出器扇出 + 后台 RewriteSTRM。5 个 HTTP 方法（:233、:381、:420、:495、:544）各自拼 URL/header，API key 同时放 header 与 query（泄露到日志）；`Enabled && ServerURL != "" && APIKey != ""` 重复 6 次；worker 退出 flush（:329-339）复制一份 → 拆 client / notifier / actors / config。
- [ ] **59.** `internal/library/scan/walker.go:76-263` `Scanner.Run` 约 190 行：参数校验、定向单文件、断点恢复、BFS、限速、NFO、分块入库、进度上报 + 两个闭包；:77 多余别名 → 拆 `runTargetFile` / `walk` / `indexDirectory`。
- [ ] **60.** `internal/drive/drive.go:69-90` `Drive`：二维码登录状态机 + token 刷新 + 账号缓存 + 挂载持久化 + 事件 + Session 签发 + 工作计数，含 2 mutex、1 ContextLock、2 singleflight、1 WaitGroup、3 个版本号 → 拆出二维码登录（约 180 行）。
- [ ] **61.** `internal/gfriends/client.go:78-143` `EnsureIndex` 持写锁执行两个镜像的网络下载（各 30s），期间所有 `Lookup` 阻塞；`FetchAvatar`（208-234）复制镜像循环；stale 回退（136-141）把 `loadedAt` 设为 now，一次故障后 7 天不重试。
- [ ] **62.** `web/src/features/library/page.tsx:32-98` 扫描状态推导 + 响应式 Tooltip 按钮 + mutation + 错误映射内联在页面 → 抽 `LibraryScanButton`。
- [ ] **63.** `web/src/features/settings/data-source-section.tsx` 把 JavDB 线路与 JavBus 开关两个无关功能塞在一个组件（靠 `// JavDB`、`// JavBus` 注释分块，:36、:45）→ 拆 `JavDBRouteRow`、`JavBusRow`。

---

## 四、过度设计

- [ ] **64.** `internal/tasks`：单 worker DB 队列拆成 Service / Queue / Bus / Registry / Pool 5 个类型 + `PoolQueue` / `PoolBus` 单实现接口；`Registry` 仅启动时注册却用 RWMutex，`Kinds()` 多余排序；`Handler` / `FinishedHook` / `funcHandler` / `funcHookHandler` / `HandleFunc` / `FinishedFunc` 多层包装，4 处注册全部带 finished；errgroup + size + Claim 重试循环均为多 worker 准备；`library.Finished`、`monitor.BatchFinished` 只返回常量 Change → 合成一个 Runner。
- [ ] **65.** `internal/library/scan/reconcile.go:20,27-46` `embyOpts ...any` 按 string 出现顺序解析 embyDir / publicURL / strmToken，类型不安全；`ProcessScanPageTx` 9 个参数 → 改为 `*Scanner` 方法。
- [ ] **66.** `web/src/api/movie-detail-cache.ts`（202 行）+ `discover.ts:160-202`：Query 缓存外再维护 `WeakMap<QueryClient, Map>` 全局卡片索引，订阅 QueryCache 每次 `removed` 全量重建；带引用计数、并发上限 2 的请求队列；`useSyncExternalStore` 手动订阅 → `placeholderData` 从 `getQueriesData` 查找 + queryFn 内约 10 行信号量；`discover.ts:163-165` 的 `prioritize` 多余。
- [ ] **67.** `web/src/api/browse-history-store.ts`（199 行）+ `browse-history.ts`：为"已浏览"徽章做 localStorage 持久化 + 服务端合并 + 50 条批量 + 10s 防抖 + beacon + 5000 截断（:178-183，服务端返回全部，不一致）；测试专用 `unref`（:136-139）进入生产代码；:47 注释引用已不存在的 `REFACTOR_PLAN`；`browse-history.ts:13` 每卡片 effect 调 `init()`，:15 `subscribe` 每次渲染新建 → `useQuery` + 乐观 `useMutation`，整个 store 删除。
- [ ] **68.** `web/src/lib/pan-login.ts:11-19` `panLoginShouldRetry` 等价于 `retry: 4`，却配 146 行测试解释 react-query 内部行为；115 专用逻辑不应在 `lib/`。
- [ ] **69.** `internal/drive/events.go` 整套 eventBus（RWMutex + 自增 ID + map + 取消函数）只有 `library.go:80` 一个订阅者且丢弃取消函数 → 单个 listener 字段。
- [ ] **70.** `internal/netx/ssrf.go:216-236` functional options 只剩 `WithAllowLoopback`（测试开关，且 dialer 仍拦 loopback，所以 `subtitle.WithAllowLoopbackForTesting` 被迫换直连 client）；`WithMaxBytes` 仅测试使用。
- [ ] **71.** `internal/pan/client.go:91-98` 与 `:137-146` Retry-After 两套：`panTransport` 全局 `retryAfter` 窗口 + resty `SetRetryAfter` 各等一遍。
- [ ] **72.** `internal/pan/client.go:59-62`、`play.go:29-36` 用魔法字符串 `"__EMPTY__"` 作 UA 哨兵，在 RoundTrip 中改写 req（违反 RoundTripper 约定），`Header.Del` 后紧跟赋值多余；`pan/offline_test.go` 被迫复制该逻辑 → resty `SetPreRequestHook`。
- [ ] **73.** `internal/config/runtime.go` 4 个从未被覆盖的常量包成 struct + `DefaultRuntime()`，`runtime_test.go` 仅断言常量本身 → 普通常量放到使用处。
- [ ] **74.** `internal/api/access_gate.go` / `auth.go:16-21`：`NewAccessGateService(password, jwtSecret ...string)` 用可变参数模拟可选；`GenerateToken(subject, ttl)` 实参永远是 `"admin"` 与 `defaultTokenTTL`；`VerifyToken` 返回的 `*Claims` 全被 `_` 丢弃，禁用时返回 `(nil, nil)`；`SignToken` / `VerifyToken` / `Claims` / `BadRequest` 仅包内使用却导出。
- [ ] **75.** `web/src/components/app-shell.tsx:22-24` 手算 `activeId` 传给 `FloatingNav` → 使用 `Link` 的 `activeProps` + `activeOptions`。
- [ ] **76.** `web/src/features/library/movie-card.tsx:14-18` hover 开关三重保护：`open={canHover && open}` + 条件渲染 + `key` 重挂载。
- [ ] **77.** `internal/magnet/aggregator.go:16,28-35` `logger` 参数两处调用均传 nil；`defaultAggregatorTimeout` 与 `catalogue.aggregatorTimeout` 同为 8s 重复定义。

---

## 五、冗余 / 死代码

### 115 字幕删除（d6a8a0c）残留

- [ ] **78.** `internal/library/scan/subtitle.go` 整个文件：`IndexDirectorySubtitles` 零生产调用，仅 `subtitle_test.go` 使用 → 两个文件一起删除。
- [ ] **79.** `internal/subtitle/service.go`：`SourcePan`（:25）、`Reader` 接口（:32）、`exportPanTrack`（:114）、`IndexPanTrack`（:209）；`Export` 的 `reader` 参数恒为 nil（`scrape/subtitle_queue.go:131`），`scrape/scrape.go:50` 的 `SubtitleExporter` 签名仍带 `subtitle.Reader`；:69-70 注释 "Subtitles stored with the 115 videos come first" 过时；历史 `pick_code != ''` 行永远 pending → 一次性 `DELETE`。
- [ ] **80.** `internal/ent/schema/subtitle.go:10-12,25-28,59`：`file_id`、`pick_code` 字段、`index.Fields("file_id")` 及过时注释 → 删除并重新生成 ent。
- [ ] **81.** `internal/library/scrape/subtitle_queue.go:15` `SubtitleTask.MetaPayload` 只写不读。

### 旧快照 / sidecar 残留

- [ ] **82.** `internal/library/scrape/cover.go:126-128` 固定 `LocalExport: true`，而 `snapshot.go:124` `Matches` 此时直接 `return true` → `DirectorySnapshot`、`NewDirectorySnapshot`、`Sidecar`、`ObservedDirectory.Sidecar`、`Matches` 比对逻辑，以及 `walker.go:106` 全库 `DirectoryObservations`（每文件名 + SHA1 常驻内存）只服务老数据；每次 cover 仍计算 `nfo.Encode` + 两次 SHA1；`writeSidecars` 名不副实。
- [ ] **83.** `internal/library/scrape/cover.go:131,226-233` 已不回写 115，但每个 cover 仍重读 115 NFO（`DirectoryNFO`）、执行 `VerifyCoverOrigin`、逐视频 `sess.Info`（`verifyVideoPositions`），与"减轻网盘负担"目标相反 → 直接使用 payload 中的 `Document/Origin`。
- [x] **84.** `internal/library/scrape/cover.go:304` `UploadSidecar` 零生产调用，仅 `library_concurrency_test.go:78` 测一条已不存在的上传路径。
- [ ] **85.** `internal/library/scrape/cover.go:255-264` `exportLocalMedia` 的 DB 兜底永远走不到（`directories()` 保证至少一个视频），且查询不按 source 过滤、`records, _ :=` 吞错。

### 其它死代码（已 grep 确认）

- [ ] **86.** ✔ `internal/javbus/cache.go:9,45,60` `detailCacheEntry.html` 存整张详情页（最大 2MB × 512 条）从未被读取。
- [ ] **87.** `internal/javdb/client.go:291-294`、`route.go:45,102-104,171-173,185-188`：`preferredHost`、`routeResult.Manual`、`ensureRoute` 的 ManualRoute 分支在生产中不可达（仅 `route_test.go:43` 使用）；`client.go:168-176` `Route()` 中 `state == nil` 拼 `CachedHost` 分支不可达；`routeState.status.Candidates` 与 `lastProbe` 双份真相（:36、:362）。
- [ ] **88.** `internal/javdb/client.go:254-260` `getJSON` 的 `language` 参数所有调用点都传 `defaultLanguage`，`transport.go:132-134` `if language == ""` 死代码。
- [ ] **89.** `internal/catalogue/service.go:344-361`、`types.go:77-87` `Facets()`、`Facets`、`FacetItem` 无 API / 生产使用，仅 `TestFacets`。
- [ ] **90.** `internal/javdb/magnet.go:81-94` `Find` 中 `JavDBID` 为空走 `ResolveMovieID` 分支不可达（`catalogue/magnets.go:66` 总带 JavDBID）；`magnet.go:54-71` 排序与 `aggregator.go:104-118` 同一规则重复。
- [ ] **91.** `internal/javdb/client.go:52-66` + `internal/catalogue/service.go:56,65`：唯一调用 `app.go:89` 传 `javdb.Options{}`，Timeout / RPS / Burst 负值校验与 `DeviceUUID` 回退永不触发 → `catalogue.New` 不再接收 Options。
- [ ] **92.** `internal/drive/drive.go:179-184` `ValidateSource`、`internal/drive/session.go:18,70-72` `Session.Version()` 仅测试使用（导致 scrape 的 `mockSession` 也必须实现）；`mount.go:152` `DirectoryPath` 仅包内使用。
- [ ] **93.** 零生产调用 / 仅测试使用：`scan/persist.go:22` `ProcessScanPage`、`scan/reconcile.go:20` `ReconcileScan`（只包事务）、`scan/identity.go:127` `HasEligibleVideos`（零调用）；`scrape` 的 `Images()`（:124）、`LockArtwork`（:134）、`Directories`（:272）、`InvalidateDirCache`（:356）、`SetDirTTL`（:368）；`scrape/nfo_source.go:18` `ArtworkOrigin.NFO`（只写不读）；`library/library.go:62` `Service.images`（只赋值）；`library/viewed.go:25` `ViewedMovieIDs(ctx, limit ...int)` 无人传 limit（`api/discover.go:28` 接口也跟着带）；`scrape/subtitle_queue.go:68` `Enqueue` 返回的 bool 无人使用；大量仅包内使用的导出函数（`CanIdentifyVideo`、`IdentifyScanVideos`、`ResolveSingleNFO`、`IndexDownloadedMovie`、`RemoveUnreferencedMovies`、`SaveScanProgress`、`ReportScan`、`SidecarByName`、`FindDirectoryNFO`、`VerifyCoverOrigin`、emby 的 `Ping` / `RefreshLibrary` / `ListPersonsWithoutAvatar` / `UploadPersonAvatar` / `SyncActorAvatars` / `PersonItem`）→ 改小写。
- [ ] **94.** `internal/tasks/service.go:34` `Service.Info`、`workflow.go:110` `WorkflowInfo`、`Service.Registry()` 仅测试使用；`Queue.Lock/Unlock`、`EnsureScanTask`、`ListWorkflows`、`WorkflowInfos`、`ScanTaskInfo`、`BatchTaskInfo`、`offline.UpdateTask` 仅包内使用却导出；Service 包装方法与包级函数两套对外 API。
- [ ] **95.** Subscription `zone` 字段（schema:38、monitor Item、`AddMovieOptions`、`AddActorOptions`、`UpdateOptions`、API 输入）写入、继承、返回，前后端从不读取；订阅状态 `"error"`（schema:41、`monitor/service.go:34`、前端 subscription-card）从未被设置；`monitor.Status` 与 ent `subscription.Status` 重复。
- [ ] **96.** `internal/subtitle`：`Provider.Name()` 从未通过接口调用（`subtitlecat.go:167` 另写死 `"SubtitleCat"`）；`finder.go:29-37` `WithAllowLoopbackForTesting` 测试开关进入生产结构体；两个 Provider 构造中 `if proxyManager != nil … else …` 复制；`DecodeToUTF8` 最后的 `ToValidUTF8` 兜底实际走不到。
- [ ] **97.** `internal/netx/proxy.go:88-94` `sameConfig` 蕴含 `sameURL`，`!sameConfig || !sameURL` 实为 `!sameConfig`，与注释"只在生效代理变化时广播"相反（Enabled=false 改 URL 也重建 tls-client）→ 条件改为 `!sameURL`；:73-79 `resolve` 中 `!Enabled ||` 冗余；:131-133 `parseProxyURL` 的 `raw == ""` 分支死代码。
- [ ] **98.** `internal/netx/ssrf.go:59-60` 255.255.255.255 分支已被 :58 `ip4[0] >= 240` 覆盖；:103 多余中间变量；:195 `RestyOptions{Timeout}` 在 transport 中不生效；:191-193 `timeout <= 0` 兜底唯一调用传固定 20s；:157/:176 `ips` 为空时返回 `(nil, nil)`。
- [ ] **99.** `internal/codeid/codeid.go:132-144` `Layers.stripPrefix` 与 `:247-262` `Candidates` 两套"去前缀"逻辑，正则（`layerNumeric` vs `numericPattern`）已分歧；`Parse`（60-62）与 `Layers`（111-116）去噪流程重复；:344 `UnpaddedNumericCandidate` 仅包内使用；:350-362、:329-334 冗余分支；:95-101 正则 var 块散落；:180、:187 对恒为单元素的 `layers[0]` 循环。
- [ ] **100.** `internal/gfriends/client.go:21-23,29` `DefaultFastlyURL`、`DefaultRawURL`、`CacheExpiration`、`FileTree` 无外部使用却导出；:42-44 nil httpClient 默认值死代码；:127-128 写缓存错误被静默吞掉。
- [ ] **101.** `domain.KindInternal`；`domain.ZoneAll` 仅测试使用；`LocalMovie` json tag 无用；`domain.ScanPayload` 另被 `scan.Payload`（`scan/types.go:9`）与 `tasks.ScanPayload`（`workflow.go:33`）起别名，一个类型三个名字。
- [ ] **102.** `internal/library/scrape/scrape.go:155` `fileScope` 与 `internal/database/scope.go:10` `LibraryFiles` 一字不差；魔法字符串 `"local"` 在 `library.go:118,120,202,204`、`scan/local.go:235-236` 硬编码，`Movies` 与 `MatchingMovies` 的 scope 计算重复 → `domain.LocalAccountID` + `s.libraryScope()`。
- [ ] **103.** `internal/app/app.go:53-55` EmbyDir 默认值与 `config.go:36-39` 重复且 validate 已保证非空；:138-141 `embySvc.Config()` 的 err 恒为 nil、`activePublicURL` 判断与 :152 条件恒真；:126 `network.ProxyManager() != nil` 恒真。
- [ ] **104.** `internal/config/config.go:50` `MIYABI_EMBY_URL` 未文档化旧别名；:101-119 `validate` 重复 Trim、EmbyDir 空检查不可能触发；日志级别 `logging.Validate` 校验一次、main 中 `UnmarshalText` 再解析一次 → Config 直接存 `slog.Level`，删除 `internal/logging`（28 行）。
- [ ] **105.** `internal/library/viewed.go:115-162` `MigrateViewedMovies` 一次性历史迁移，数据迁完即可删除（且所有 ID 使用同一 `now`，原顺序丢失）。
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
- [ ] **109.** 后端接口前端从未调用：`GET /discover/movies/:id/offline`（`router.go:118`，连同 `offlineTasksHandler`、`offline.Service.Tasks`）、`POST /library/scan/local`（:93）、`POST /auth/logout`。
- [ ] **110.** 删除属性后残留空行：`ui/input-group.tsx:13`（原 `role="group"`）、`ui/breadcrumb.tsx:57/68/82`（原 role / aria-hidden / aria-current）、`ui/dialog.tsx:62-64` 删除了 `sr-only` 文案、`search/page.tsx:67`、`settings/pan-section.tsx:40/62/76`、`pan-directory-row.tsx:34`；`internal/domain/video.go:30-31` 双空行（唯一 gofmt 报告）。
- [ ] **111.** `cn` 导入不统一：`ui/hover-card.tsx:2`、`ui/popover.tsx:2`、`ui/dropdown-menu.tsx:2` 直接 `from 'cn'`，其余 `@/lib/utils`；`dropdown-menu.tsx` 未过 oxfmt（双引号、长行）。

---

## 六、过度防御

- [ ] **112.** `internal/api/auth.go:35/58/70/76/83/117`、`strm.go:64` 的 `gate == nil` 生产不可能成立；`rate_limiter.go:37/74/98` `limiter != nil` 与方法内 `l == nil` 双重防御；`accessLoginHandler` gate 为 nil 返回 `Success: true` 死路径 → 测试统一传 `NewAccessGateService("")`。
- [ ] **113.** `internal/drive/token.go:59-105` `withPanToken` 中 `current.closed` 检查 3 次、`ctx.Err()` 3 次 + `refreshTokens` 入口再一次 → 合并为最多 2 次的循环。
- [ ] **114.** `internal/drive/session.go` 每次调用执行 `sourceState` 3~4 次（显式一次、`withPanSourceToken` 内一次、重试一次、`checkSource` 一次）。
- [ ] **115.** `internal/pan/client.go:48-50` `maxInFlight <= 0`、:127-129 `baseTransport == nil`、`SetRetryAfter` 中 `resp == nil` / `wait > 0`、:241-244 可直接 `return result, result.err()`；`play.go:91-116` `OpenMedia` 错误分支 `RawBody() != nil`；`auth.go:99-115` `Status == nil`、`case 0`、`default` 三处返回同值；`upload.go:44` 局部变量名 `init`。
- [ ] **116.** `internal/strm/relay.go:35-37,78-80` `relay.drive == nil`、:65 `source.URL != ""`、`downloadURL != ""`（pan 层已保证）。
- [ ] **117.** `internal/javbus/client.go:149,214` `errors.Is(ctx.Err(), Canceled) || errors.Is(ctx.Err(), DeadlineExceeded)` 等价 `ctx.Err() != nil`；:70 `rate.Every(time.Second/time.Duration(defaultRate))` 即 `rate.Limit(1)`；:297 `c.client != nil`；`parser.go:57` `Contains("/doc/driver-verify") || Contains("driver-verify")`；`parser.go:128,132` 显式零值。
- [ ] **118.** `internal/javdb/route.go:277` `utf8.Valid`；`route.go:217-256`、`client.go:369-389` 解析到 `map[string]any` 再逐层断言 → typed struct；`route.go:49` 类型 `probe` 与方法 `c.probe` 同名。
- [ ] **119.** `internal/catalogue/state.go:60` `source == nil` 在 :32 已判断；`service.go:163-168` nil 检查；`service.go:379` `time.Now().In(time.Local)`；`magnets.go:40` `JavBus()` 永不返回 error；`magnets.go:80-90` 先投影 URI 再剥离。
- [ ] **120.** `internal/magnet/aggregator.go:75,84-86` Hash 小写化、`Sources` 为空兜底（各来源已保证）；`picker.go:108`、`quality.go:158` `m.HD || hasTag(m, MagnetTagHD)`（根源：`domain.Magnet` 同时用 bool 与 Tags 表达 HD/字幕）；`picker.go:105` 对已聚合结果再次 `ApplyInference`。
- [ ] **121.** `internal/library/scrape`：`Close` 的 `service == nil`、`subtitleQueue != nil`；`newSubtitleQueue` 的 concurrency / capacity / logger 兜底（唯一调用传默认值）；`Enqueue` 的 `q.ctx != nil`、`q.logger != nil`；`worker` 的 `!ok`（channel 从不关闭）；`processCover` 与 `process` 两次判断 `subtitles != nil`；`ExportLocalMovie` 的 `record == nil` + `notifiers ...MediaNotifier` 表示可选；`scrape.go:339` `dirCache == nil`、:343 `ttl <= 0`；`findAvatar` 的 `s.db == nil`；`reconcile.go:112` `if images != nil`；`reconcile.go:58` 多余 `var err error`；`ResolveSingleNFO` 永远返回 nil error。
- [ ] **122.** `internal/image` singleflight + Stat 检查 + `publishImage` 读回比较三层叠加 → 保留原子 rename + Stat；`internal/maintenance` 每次循环检查 `ctx.Err()`、检查 DB 文件 `IsRegular`。
- [ ] **123.** `internal/monitor/service.go:108` `checking`、`internal/offline/service.go:52` `syncing` 互斥锁：生产中仅由一个 `RunPeriodic` 串行调用；`offline/add.go:20,30` `catalogue != nil` 两次；`sync.go:254` `library != nil` 只为测试；`sync.go:76` 重复过滤 `DirectoryID`、:94-96 每条检查 `ctx.Err()`、:154-159 身份校验冗余；`monitor/service.go:160,176` `limit > 0` / `max(page, 1)`（API 绑定已保证）；`tasks/pool.go:34` `logger == nil`。
- [ ] **124.** `internal/offline/projection.go:81-83` + `latestOfflineTasks` 的 json_type 分组专门让格式错误的 hash 可见，结果一条坏记录让整个 Activity 报错 → 跳过并记日志。
- [ ] **125.** 前端：`access-gate.tsx:14,25` `sessionUnlocked`（onSuccess 已写入缓存）；`lib/math.ts:3` 处理 -0；`api/client.ts:66,71,80,106` 四处 `typeof window === 'undefined'`（SPA，且 :18 直接使用 `window.location.origin`）；`components/error-state.tsx:34` 外层重复判断 `retry.onRetry`；`task-progress-state.ts:21` 手写 clamp（`lib/math` 已有）；`lib/search-schema.ts:3` `errorMessage` 参数无意义；`stores/settings.ts:17` `partialize` 多余。

---

## 七、性能

- [ ] **126.** `internal/tasks/workflow.go:183-203` `metadataGroups` 的 `sqljson.ValueIn($.scan_task_id)` 无索引，对所有历史 scrape/cover 行（payload 含大快照）逐行解析 JSON，task 表从不清理；`internal/api/sse.go:36-43` 每个 SSE 连接每次通知各执行一遍 `tasks.List`（6+ 查询），通知来源包括每个扫描分页、每次离线进度、每个任务结束 → 增加 `parent_id` 列并建索引；Bus 端计算一次快照（版本号 + 去抖）后广播。
- [ ] **127.** `internal/tasks/bus.go:69-75` `Notify()` 同时唤醒 pool：扫描分页进度（`scan/persist.go:171,230`、`reconcile.go:151`）、`SaveSubscriptionBatch`、`ChangeOffline`（每 30s）都会让 pool 空跑 Claim；`queue.go:66` Claim 自身又 Notify → 拆分 `WakePool()` 与 `NotifyUI()`。
- [ ] **128.** `internal/ent/schema/task.go:37-42` `index.Fields("status", "created_at")` 的 `created_at` 从未用于过滤 / 排序，热点查询均为 `type + status + ORDER BY id` → `index.Fields("type", "status")`，删除单列 `type` 索引。
- [ ] **129.** ✔ `internal/emby/service.go:403` `sendBatch` 每批 `/Library/Media/Updated` 增量通知后无条件调用 `RefreshLibrary`（`/Library/Refresh` 全库扫描），扫描期间约每 3 秒一次 → 删除。
- [ ] **130.** `internal/library/scan/movies.go:29` `MatchMovies` 每页 100 个 `LIKE '%code'` 以 OR 连接 → 全表扫描；:38-45 codes × records 双重循环；:56 `slices.Contains` 去重 O(n²) → 增加 `canonical_code` 索引列 + 等值 IN，去重用 map。
- [ ] **131.** `internal/library/scan/local.go:147-256` 本地扫描逐文件开事务，每次 `MatchMovies` 全表匹配；图片解码 / 重编码（:196-198）在 SQLite 写事务内；`NotifyUpdated`（:252）在提交前发出；整体在 HTTP 请求内同步执行。
- [ ] **132.** `internal/library/scan/reconcile.go:94-142` 全量扫描加载所有已刮削影片的 files / actors / tags；每部影片 `images.Exists` 3 次 stat + `ExportLocalMovie` stat / 写文件均在 SQLite 写事务内，长时间占写锁；:139 刮削任务逐条 `Create().Exec` → 事务外导出 + `CreateBulk`。
- [ ] **133.** `internal/library/scrape/snapshot.go:153-190` 每次扫描按 500 一组对 task 表 `json_extract + GROUP BY`；每个刮削任务对全部 cover 任务执行一次 JSON `Exist`。
- [ ] **134.** `internal/library/scrape/cover.go:45-53` `service.artwork.Lock` 覆盖整个 `processCover`（含 JavDB 封面下载、115 列目录、`sess.Info`、本地导出）→ 所有 cover 串行，刮削期间 `ClearCache` 永远返回"忙" → 只锁"保存图片到提交引用"一段。
- [ ] **135.** `internal/library/scrape/scrape.go:71-73,320-353` `dirCache` 过期条目永不清理，每条存整个目录列表；`processCover:120` 注释 "Re-read the directory after scraping" 实际拿到 45s 内缓存。
- [ ] **136.** `internal/offline/projection.go:26-44,190-201` `Activity` 刻意不设历史上限，每次解码全部 hash 最新记录 + 对所有关联扫描执行 `Workflows`（走第 126 条的全表 JSON）+ 查文件索引；`web/src/api/offline.ts:44` 在 SSE 之外还 `refetchInterval: 5000` 轮询。
- [ ] **137.** `internal/tasks/batch.go:35-48` 批量每处理一条就重新编码整个 payload（含全部 IDs）写回并 Notify（触发第 126 条的全连接重查）→ 只更新计数或按 N 条 / T 秒保存。
- [ ] **138.** `internal/pan/metadata.go:81-86` `ReadMetadata` 显式 `limiter.Wait` + `panTransport` 每次尝试再 Wait，加上 `DownloadURL`，一次 NFO/图片读取消耗 3 个令牌（上限 4 req/s）并占用 2 个 inFlight 槽之一，与 `OpenMedia` 注释"CDN 不走限流"矛盾 → 删除显式 Wait，CDN 读取改用 `client.media`，`Client.limiter` 字段随之可删。
- [ ] **139.** `internal/pan/upload.go:86-97` 每个 sidecar 请求一次 `/open/upload/get_token`；OSS `PutObject` 经 `panTransport` 消耗 115 限流令牌并占 inFlight 槽 → STS token 按过期时间缓存，OSS 用独立 client。
- [ ] **140.** `internal/api/strm.go:33-39` Emby / 播放器每次 HEAD、GET、Range 都走 DB + downurl（限流 4rps / 2 并发）+ CDN HEAD → 按 `(pickCode, UA)` 做短 TTL 缓存。
- [ ] **141.** `internal/netx/ssrf.go:103-119,152,206` 同一主机 3 次 DNS 解析（请求前 `ValidateSafeURL`、每���重定向、拨号时），拨号那次已覆盖 DNS 重绑定 → `ValidateSafeURL` 只做语法校验。
- [ ] **142.** `internal/drive/account.go` 缓存未命中无 singleflight，并发 `Open()` 同时请求 `/open/user/info`；`accountCacheMu` 可并入 `d.mu`；`internal/drive/token.go:13-57` refresh_token 被吊销后不标记凭证失效，每个请求都发起一次注定失败的刷新。
- [x] **143.** ✔ `internal/catalogue/magnets.go:67` 无论 JavBus 是否启用都先调 `CatalogueDetail`（可能连带最多 5 次 Tags），monitor 批量检查时 JavDB 限流请求翻倍 → 仅在 JavBus 可达时查询（`service.javbus != nil && service.javbus.Available()`）。
- [ ] **144.** `internal/catalogue/service.go:239-251` `MovieSummary`、`:254-264` `BrowseMovies` 走完整 `projectMovies → MovieStates`（2 次 DB 查询）后丢弃投影结果。
- [ ] **145.** `internal/javdb/movie.go:105-141,147-174` `ResolveMovieID` 每次查询前对累积的数百候选全量 `matchCandidates`，`codeid.Normalize` 每次最多 8 个正则，`IsFormatEquivalent` 两边重复 Normalize → 预计算 `normCode`。
- [ ] **146.** `internal/javdb/route.go:112` 非 full 模式用 5ms ticker 轮询（最长 60s，每秒唤醒 200 次）→ 按最早截止时间的单个 `time.Timer`。
- [ ] **147.** `internal/subtitle/label.go:89-100` `[]rune(text)` 将最多 10MB 文本整体转换（约 40MB 分配）后再截取前 15000 个；`containsRune` 在 40 元素切片中线性查找 → range 计数 + map。
- [ ] **148.** `internal/monitor/check.go:48-56` 每轮只查 10 部影片 / 5 个演员后等 5 分钟，500 个订阅约 2.8 小时才查完；`service.go:137` `EnqueueSingle` 每条读一次 config、新建 Picker；`BatchHandler` 先 `Subscription.Get` 一次，`EnqueueSingle` 再 Get 一次；`check.go:94-108` "继续等待"分支更新 checks 却不发 `NotifyMonitorChanged`。
- [ ] **149.** `internal/library/library.go:222` 分页 `ORDER BY created_at DESC, id DESC`，movie 表无 `(created_at, id)` 索引；`scan/local.go:223-227` fileID 取 `sha256(rel)`，不同扫描根目录下相对路径相同会冲突，已存在记录不更新 path。
- [ ] **150.** 前端列表已带 `state`，每个 `MovieStateBadge` 再批量 `POST /api/discover/movie-states`，形成"列表 → 状态"瀑布 → 用列表数据 seed（`initialData` + `initialDataUpdatedAt`），或后端从列表 DTO 删除 state。
- [ ] **151.** `web/src/features/tasks/task-events.tsx:115-123` 连接后首个 `changes` 事件因 `revisions` 为 undefined，把 library / offline / movie-states / subscriptions 四类全部失效 → 每次打开页面重复请求一次；:38 `reconnect` 中的 `invalidateQueries` 多余 → 首个事件仅作基线，断线重连后才强制刷新。
- [ ] **152.** `refetchOnMount: 'always'` 滥用：`api/tasks.ts:66`、`movie-state-cache.ts:75`（每卡片挂载）、`subscriptions.ts:44`；`data.ts:22`、`emby.ts:29`、`javbus.ts:17`、`network.ts:29`、`subscription-settings.ts:28` 的 `staleTime: 15_000` 与 `'always'` 共存使 staleTime 无意义；`main.tsx:14` 全局默认 30s 被几乎所有查询覆盖。
- [ ] **153.** `web/src/api/offline.ts:66-88` `useAddOffline` 成功后 `setQueryData` + `invalidate` + SSE 再失效，同一数据刷新三次；:48-57 `useOfflineTasks` 在 select 中重复过滤（后端已按数据源返回），与全局查询共用 key 使其 `enabled` 形同虚设；`library.ts:56-70` 两个分支最终都 invalidate。
- [ ] **154.** `useLibraryMovies`、`useSearchMovies` 未使用 `keepPreviousData`，翻页闪骨架；路由未注入 queryClient context、无 loader，`defaultPreload: 'intent'` 只预加载 chunk 不预取数据。
- [ ] **155.** `web/src/components/media-image.tsx:60-65` 每张封面额外渲染一张 `scale-125 blur-xl brightness-75` 背景图，一页 20 个大半径模糊滤镜；`features/library/use-desktop-hover.ts:14` 每卡片每次 snapshot 调 `matchMedia` 并各自注册监听 → 模块级缓存 `MediaQueryList`。

---

## 八、低效 / 非最佳实践

- [ ] **156.** `internal/drive/session.go:79-207` 8 个 `sourceSession` 方法同一模板（`sourceState → withPanSourceToken → checkSource`），约 130 行 → 泛型 `call[T]`。
- [ ] **157.** `internal/javbus/client.go:128-191` 与 `:193-242` `ensureDetailParams` / `fetchMagnets` 请求构造、UA/Cookie、301/302、`LimitReader`、Cloudflare 判断、状态码判断逐行重复 → `c.get(ctx, url, headers)`。
- [ ] **158.** `internal/api/error.go:76-119` 与 sloggin 各记录一次失败日志；attrs 中 `cause` 与 `error` 内容重复。
- [ ] **159.** `internal/api/auth.go:115-141` `authMiddleware` 绕过 errorMiddleware 手写两套中文消息与 `"code":"UNAUTHORIZED"`，`VerifyToken` 的公开消息被丢弃 → `c.Error(err); c.Abort()`。
- [ ] **160.** api handler 零碎问题：`network.go:42`、`emby.go:41` test handler 先查已保存配置再被 body 覆盖（前端总带 body），两者写法相同可抽泛型 helper；`subscription.go:141-145` 手写 `c.JSON(202)` 未用 `accepted`；`subscription.go:155` feed 复用带 `Kind` 字段的 query 结构；`router.go:45-49` if/else 等价且忽略错误；`router.go:70-73` 4 条 STRM 路由可 `Match` 合并；`task.go:16`、`sse.go:10` 参数名 `tasks`、`offline.go` 参数名 `offline` 遮蔽包名；`javbus.go:25` Update 后重复调 `JavBus()`；`router.go:42` 构造函数中设置全局 `gin.SetMode`。
- [ ] **161.** `internal/api/rate_limiter.go`：`records` 仅在同一 IP 再访问或登录成功时删除（换 IP 无限增长）；`check` 与 `recordFailure` 不在同一把锁（并发可超过 maxFailures）；`ErrTooManyLoginAttempts` 文案写死"24 小时"且作为 Cause 拼出矛盾日志 → `allow(ip)` 一次加锁完成检查与预占。
- [ ] **162.** `internal/tasks/queue.go:47-68` Claim 先 SELECT 再条件 UPDATE + 重试循环 → 单条 `UPDATE … WHERE id=(SELECT … LIMIT 1) RETURNING`；`batch.go:66-76` `listBatchTasks` 两次查询内存合并；`pool.go:76-79` `registry.Get` 分支不可达。
- [ ] **163.** 订阅 API 三种风格：tasks 返回 `(ch, unsubscribe)`、drive 返回 `func()`、netx 返回 ch + 单独 `Unsubscribe`（`proxy.go:99-117` O(n) 遍历）→ 统一 `(<-chan struct{}, func())`。
- [ ] **164.** Go 1.27：`app.go:223-235`、`network.go:82-84`、`aggregator.go:53-62` 可用 `wg.Go`；`internal/syncx/lock.go:47` 对未加锁的锁 `Unlock` 会永久阻塞（`sync.Mutex` 会 panic）；`KindX.String()` 与 `string(KindX)` 混用。
- [ ] **165.** `internal/library/viewed.go:91-110` Count + 查最旧 + Delete 三步 → 单条 `DELETE … WHERE id NOT IN (SELECT id … ORDER BY viewed_at DESC LIMIT ?)`。
- [ ] **166.** `internal/api/router.go:139-158` `installFrontend` 中 `fs.Stat` 对目录也成功，FileServer 会列出 `/assets/` 目录清单；带哈希的静态资源无长缓存头。
- [ ] **167.** `internal/javdb/media.go:29-43` `FetchMedia` 接受任意 https 主机且 body 无大小上限，`/image` 成为登录后任意抓取器 → 限定 JavDB CDN 域名或复用 `netx.SafeDownload` + 上限。
- [ ] **168.** `internal/gfriends/client.go:236` `errors.Join(errors.New(...), lastErr)` → `fmt.Errorf("fetch avatar: %w", lastErr)`；:113 Filetree 读取无大小限制。
- [ ] **169.** `web/src/api/client.ts:30-61` `apiPost` / `apiPatch` / `apiPut` JSON 体逻辑重复 3 遍 → `sendJSON(method, path, body)`；`apiGet(path, undefined, signal)` 出现 15 次 → `apiGet(path, { query, signal })`；:85-89 `imageURL` 不属于请求客户端。
- [ ] **170.** 查询键无统一管理：`discoverKeys` 定义在缓存实现文件 `movie-detail-cache.ts:5-13`；`pan.ts:65-66` 字面量 `['library']`、`['offline']`；`javbus.ts:35` 字面量 `['discover', 'movie']`；`subscriptionSettingsKey` 与 `xxxKeys.config` 命名混用，`dataKeys` 未导出 → `api/query-keys.ts`。
- [ ] **171.** `web/src/api/javbus.ts:37-42`、`network.ts:50-55` onError 先回滚再 invalidate，二者取一；且仅这两个设置做乐观更新，emby / subscription-settings 未做（订阅设置 Switch 需等服务端返回且保存期间整块禁用）。
- [ ] **172.** `web/package.json` build 脚本 `vite build && tsc --noEmit`，类型检查应在前。
- [ ] **173.** `web/src/features/settings/network-section.tsx:39` `setTimeout(() => inputRef.current?.focus(), 50)` 不清理 → `autoFocus`；`access-gate.tsx:26` effect 依赖整个 `config` 查询对象 → 依赖 `config.refetch`。
- [ ] **174.** 导入风格混用：`tasks-section.tsx:7`、`scan-progress.tsx:8` 用 `@/features/settings/shared`，同目录其他文件用 `./shared`；`components/movie/index.ts` barrel 仅部分使用（`recommendations.tsx:5-6` 相邻两行一个走 barrel 一个直连，barrel 导出的 `DiscoverMovieCard` 无外部使用）；`DISCOVER_PAGE_SIZE as PAGE_SIZE`（`category-content.tsx:6`、`discover/page.tsx:7`）、`DISCOVER_ZONES as zones`（`category-filters.tsx:15`）别名不一致。

---

## 九、过度拆分

- [ ] **175.** `internal/api`：`task.go`（21 行）、`sse.go`、`health.go`、`javbus.go`、`data.go` 各二三十行；`data.go` 实为 maintenance、`movie.go` 实为 library；`HealthChecker` 定义在 `router.go` 而 handler 在 `health.go` → 按资源合并。
- [ ] **176.** `internal/drive/drive.go:195-198` `Drive.OpenMedia` 与 `internal/strm/relay.go:77-82` `Relay.Probe` 纯透传，一次 HEAD 穿过 api → strm → drive → pan 三层空转发；api 层自行编排两次调用 → strm 持窄接口并提供 `Head()`。
- [ ] **177.** `cmd/miyabi/healthcheck.go` 一行转发 `app.CheckHealth`（应属 CLI）；healthcheck 子命令执行完整 `config.Load`，其中 `netx.OutboundIP()` 做一次 UDP 拨号。
- [ ] **178.** `internal/magnet/source.go`（13 行单接口）、`internal/netx/probe.go`（14 行）、`internal/api/javbus.go`（28 行两个 handler）。
- [ ] **179.** `DiscoverResults` 三处调用（`discover/browse-results.tsx:14-23`、`metadata-search-page.tsx:104-113`、`search/page.tsx:103-112`）是相同的"拆 query 字段传 props"适配 → 直接接收 `UseQueryResult` + `emptyTitle`，删除 `BrowseResults` / `SearchResults` 壳组件。
- [ ] **180.** `web/src/features/subscriptions/actor-subscriptions.tsx` 接收 `displayItems` / `displayQuery`（:17-22 手写 query 子集类型）/ `selecting` / `selected` / `onSelect` / `disabled` 后原样转给 `MovieSubscriptions`（:98-109），`MovieSubscriptions` 在两处渲染 → page 统一渲染一次；`page.tsx:43` 可简化为 `view === 'actors' && selectedActorID !== null ? feed : movies`。
- [ ] **181.** 单调用方小文件：`features/library/movie-status.tsx`（18 行，外层 `<>` 包单个三元）、`features/movie-detail/metadata-link.tsx`、`api/movie-states.ts`（17 行）。

---

## 十、臃肿 / 重复（后端）

- [ ] **182.** `catalogue.MovieDetail` 重新声明 `domain.MovieDetail` 的字段而非内嵌；`domain/library.go` 实际存放 MovieSummary / Media / OfflineSubmission，`ScanProgress` 位于 `source.go`，文件名与内容不符；`ScanPayload.Checkpoint` 是 JSON 中嵌套 JSON 字符串 → 结构化字段。
- [ ] **183.** `internal/app/app.go:56-123` 每个失败分支手工复制一串 `Close()`，且不完整（maintenance / emby 失败时未关闭已启动 subtitleQueue 的 scrapeSvc）；`Close()`（:268-285）的 nil 检查从未用于半构造 App → 先 `a := &App{}`，出错统一 `a.Close()`。
- [ ] **184.** URL scheme 正则前端 3 份（`settings/emby-url.ts:15,42`、`network-section.tsx:175`）+ `formatHost` 另一正则 → `hasScheme` / `withDefaultScheme` 入 lib；后端 `scrape/export.go:20-21` `defaultPublicURL()` 写死 8080、每次导出触发 `OutboundIP()` UDP 拨号，与 `config.go:41-46` 推导重复；`netx/ip.go:11-19` 注释"优先私网段"与 UDP 探测实现不符。
- [ ] **185.** 磁力排序规则（字幕 > 高清 > 体积 > 文件数）在 `javdb/magnet.go:54-71` 与 `magnet/aggregator.go:104-118` 各一份。

---

## 十一、UI 一致性

- [ ] **186.** 设置页 5 种保存语义：立即生效无提示（外观、隐私、JavDB 线路 `data-source-section.tsx:56`）；立即生效且每项 toast（`subscription-section.tsx:45` 改一个 Select 弹一次、JavBus `data-source-section.tsx:78`）；代理开启需手动保存、关闭立即保存（`network-section.tsx:36-59`）；Emby 开关也需点保存（`emby-section.tsx:155-171`）→ 统一：开关 / 下拉即时保存，文本表单才有保存按钮，toast 策略一致。
- [ ] **187.** 加载态不一致：Skeleton（data-section）、纯文本（`tasks-section.tsx:23`）、裸 spinner（`pan-section.tsx:55`）、直接显示假默认值（`subscription-section.tsx:61` `movie_auto_download ?? true`、`emby-section.tsx:32` `sync_actors ?? true`，加载中显示为开启）。错误态：tasks / data 带重试，emby:246、network:152、subscription:138、javbus:173 无重试；`data-source-section.tsx:147` 文案"点击重试"却未接 `onRetry`；同一"任务读取失败"在 `library/page.tsx:248` 用 `retryLabel="重新连接"`、`tasks-section.tsx:25` 未用且文案不同 → `SettingsSection` 接收 query 统一处理。
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
- [ ] **206.** settings mutation `onError: error => toast.error(error instanceof Error ? error.message : '…')` 重复 7 遍（`data-source-section.tsx:81`、`emby-section.tsx:105,143`、`network-section.tsx:51,80,98`、`subscription-section.tsx:47`），fallback 永不触发，网络异常时原样显示 "Failed to fetch" → 将 `api/subscriptions.ts:38` 的 `describeError` 提到 `client.ts` 统一使用。
- [ ] **207.** "测试连接 + 保存"操作栏 `emby-section.tsx:227-244` 与 `network-section.tsx:126-148` JSX 完全相同 → `settings/shared.tsx` 增加 `SettingsActions`。
- [ ] **208.** "服务端值 + 本地草稿"两种实现：emby 用 `form | null` 手工比较 7 个字段（`emby-section.tsx:48-57`），network 用两个 `useState<T | null>`（`network-section.tsx:26-33`）；emby 无 `<form>` 回车不提交，network 手写 `onKeyDown`（:120）→ 抽 `useDraft(serverValue)` + `<form onSubmit>`。
- [ ] **209.** 确认弹窗 `data-section.tsx:101-139` 与 `subscriptions/page.tsx:200-243` 结构一致（pending 时禁止关闭、`showCloseButton={!pending}`、InlineError、取消 + loader 确认）→ 抽 `ConfirmDialog`。
- [ ] **210.** `subscription-section.tsx:79-136` 三个 Select 块只差字段与选项 → 配置数组 map 或 `PreferenceSelect`。
- [ ] **211.** 卡片结构重复：`Card size="sm" className="h-full gap-0 overflow-hidden py-0"` + `aspect-3/2` + `CardContent className="space-y-2 p-3"` 在 `components/movie/movie-card.tsx:60-73` 与 `movie-grid-skeleton.tsx:23-25` 各写一遍（`overflow-hidden` 与 Card 默认重复，`gap-0 py-0 p-3` 覆盖 size="sm"）；卡片链接 class 三处（`movie-card.tsx:18`、`library/movie-card.tsx:42`、`recommendations.tsx:59`）；`MovieCover` 外框三处（`movie-card.tsx:61-64` 的 flex 居中无效、`movie-hover-details.tsx:26`、`hero.tsx:12`）→ `MovieCardShell` / `MovieCardLink` / MovieCover 自带比例框。
- [ ] **212.** 影片信息栏 `movie-detail/hero.tsx:44-73`（MovieStats）与 `library/movie-hover-details.tsx:34-46` 重复判断与格式化 release_date / duration / rating → `movieStats(movie)`。
- [ ] **213.** discover 分类派生 `category-content.tsx:18-21` 与 `category-filters.tsx:27-30` 逐字相同；分区 Select `category-filters.tsx:35-53` 与 `metadata-search-page.tsx:69-88` 两份；`mainOptions` 一处用常量 `MAIN_CATEGORY`、一处用字面量 `'main'`（:43）→ `useDiscoverTaxonomy` + `ZoneSelect({ allowAll })`。
- [ ] **214.** 详情页 section 标题 `<section className="space-y-4"><h2 className="text-xl font-semibold tracking-normal">` 在 `magnets.tsx:63`、`previews.tsx:20`、`recommendations.tsx:23` 重复 → `DetailSection`；`previews.tsx:20` 的 `id="movie-previews-title"` 无人引用。
- [ ] **215.** `features/tasks/task-notification-diff.ts:4-14` 重复定义 `isScanTask` / `isBatchTask` / `isTaskActive`（`api/tasks.ts:70-80` 已有），:55-56 第三份 `isOfflineTaskActive`（`lib/offline-state.ts`、`api/offline.ts:9` 已有）；三个循环（:78-103、:105-122、:125-147）通知判定条件完全复制 → 纯函数移至 `lib/task-state.ts`，抽 `shouldNotify()`。
- [ ] **216.** `features/tasks/task-toast.tsx:42,71,123` 三处 `icon: options.waiting ? undefined : <LoaderCircleIcon…/>` → 并入 `taskToastOptions`；"等待同步"scan 改标题、offline / batch 改描述，处理不一致。
- [ ] **217.** `settings/pan-directory-dialog.tsx:83-109` 手写上一页 / 下一页且无 aria-label，现成 `ListPagination` 已支持 `hasMore`、可不传 totalPages 和 `scrollToTop={false}`。
- [ ] **218.** 本地 NFO 解码（`scan/local.go`）与 `ReadNFO` 两套 → 抽 `decodeNFO(body, fallbackName)` 共用。
- [ ] **219.** 测试桩复制 5 份约 1500 行：`internal/app/drive_fixture_test.go`（371）、`internal/library/fixture_test.go`（367）、offline（363）、drive（292）、strm（95），app 与 library 两份几乎相同 → `internal/pan/pantest`，或像 `strm/fixture_test.go` 那样嵌入接口只覆盖需要的方法。

---

## 十三、UI 合理性

- [ ] **220.** 破坏性操作确认策略倒置：风险最低的"清理未使用缓存"有确认弹窗；退出 115 登录（`pan-section.tsx:78`）、取消挂载目录（`pan-directory-row.tsx:62`）、取消演员订阅（`actor-chip.tsx:93`）、取消影片订阅（`subscription-card.tsx:111`）均一键执行 → 统一使用 ConfirmDialog。
- [ ] **221.** 批量入库失败错误双显示：hook onError 已 toast（`api/subscriptions.ts:168`），弹窗又显示 `InlineError`（`subscriptions/page.tsx:215`）；成功时 `page.tsx:90` toast + 任务通知 toast 两个。
- [ ] **222.** `data-section.tsx:28` `busy` 包含 `info.isFetching`，每次后台刷新清理按钮闪一下禁用；刷新统计按钮放在"数据目录"行（:56），位置与作用不符。
- [ ] **223.** `data-source-section.tsx:70` 代理未开启时用 error toast 拦截 JavBus 开关 → 直接禁用开关并在描述中说明原因。
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

- [ ] **235.** `internal/api/router.go:52` sloggin 默认记录 query，`.strm` URL 带 `?token=<STRMToken>`（`scrape/export.go:41`）→ Emby 每次播放把 token 明文写进 info 日志；emby API key 放在 query 同样进日志；`auth.go:155` `extractToken` 接受 `?token=` JWT 而前端从不使用 → 删除该分支并对 sloggin 做 query 脱敏。
- [ ] **236.** `GET /api/settings/emby` 明文返回 APIKey → 打码返回，提交时回填。
- [ ] **237.** `internal/api/auth.go:164` 无论代理是否可信都信任 `X-Forwarded-Proto`；`router.go:45-49` `SetTrustedProxies` 错误被忽略，解析失败时部分列表静默生效 → 在 `config.Load` 校验。

---

## 十六、测试臃肿

- [ ] **238.** `internal/codeid/prototype_test.go` 以 `prototypeResolve` / `prototypeMatch` 重写一遍 `javdb.ResolveMovieID`，`prototypeQueries` 已与生产 `Queries` 分歧，文件头注释过时——测的是副本而非生产代码 → 迁到 javdb 包基于 fixture transport 测真实实现；`corpus_test.go:18` 与 `random_corpus_test.go:12` 语料加载器几乎相同 → `loadSamples(t, file, n)`。
- [ ] **239.** 随死代码一并删除的测试：`scan/subtitle_test.go`（`IndexDirectorySubtitles`）、`library_concurrency_test.go:55`（`UploadSidecar`）、`scrape/dir_cache_test.go`（`InvalidateDirCache`）、`scrape/snapshot_test.go:27` `TestSnapshotMatches`（老快照路径）、`scan/walker_feature_test.go:10`（只测 JSON 往返，且未覆盖第 2 条断点 bug）、`subtitle` 中测 pan 导出的部分、`database/sqlite_test.go:87` 相关断言。
- [ ] **240.** 低价值测试：`config/runtime_test.go`（断言常量）、`domain/value_test.go`（5 行泛型函数）、`tasks_test.go` 的 `TestRegistryAndHandlers` / `TestBusSubscriptionsAndRevisions`（测 map / channel 本身）、`netx` 的 `TestFingerprintClientBuildsWithAndWithoutProxy`（只验证构造不报错）、`TestOutboundIP`（依赖环境）；`TestCheckHealth` 在 `app_test` 与 `cmd/miyabi/healthcheck_test.go` 重复；`api/jwt_test.go` 6 个测试走同一失败路径可表驱动；`app/task_test.go`、`task_payload_test.go` 测 tasks/scrape 行为却放在 app 包；`pan/client_test.go` 手工拼 resty 配置而非测试 `New()` 的真实配置（第 10 条因此未被发现）；`web/tests/pan-login-poll.test.mjs` 146 行测 `retry: 4` 的等价实现。

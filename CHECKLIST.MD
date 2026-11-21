# 代码审计 Checklist

> 审计日期：2026-11-20　基线提交：`4e20f19`
> 范围：`web/src`（前端）与 `cmd/`、`internal/`（Go 后端，不含 `internal/ent` 生成代码与测试）
> 标记：✅ = 已在代码中人工复核确认；其余为子代理审计结论（带证据，未逐条复核）
> 严重度：🔴 高　🟠 中　🟢 低

---

## P0 · 功能 Bug（优先修复）

- [x] 🔴 ✅ **Select 显示原始 value 而非中文标签**：base-ui `SelectValue` 无 children 且 Root 未传 `items` 时直接显示 value。（已修复：各 Select 传 `items`）
  - `web/src/features/discover/category-filters.tsx:39`（分区）、`:75`（分类）、`:92`（标签）
  - `web/src/features/discover/common-filter-select.tsx:21`
  - `web/src/features/settings/subscription-section.tsx:93`、`:114`、`:133`
  - 改法：Root 传 `items={options}`（`{value,label}[]` 或 `{[value]: label}`）；`movies/filter-bar.tsx` 的 `sortLabel/sourceLabel` 已同法简化。
- [x] 🔴 ✅ **扫描"文件未变"判断恒失败**：`internal/library/scan/persist.go:38` Select 漏 `file.FieldUploadTime`，`:105` 却比较 `old.UploadTime`，导致每页全量 upsert + 每页 `NotifyLibraryChanged`。改法：Select 补该字段。（已修复：Select 补 `file.FieldUploadTime`）
- [x] 🔴 ✅ **115 账号设置读改写无锁/无事务**：`internal/drive/accounts.go:39`（`saveAccountTokens`）、`:107`（`saveAccountMount`）、`:122`（`removeAccount`）并发时互相覆盖，重启后可能读回旧 refresh token。改法：加 `accountsMu` 包住整段读改写，或 `ent.WithTx`。（已修复：新增 `accountsMu` 串行化三处读改写，并加并发测试）
- [x] 🔴 ✅ **NSFW 模式被原生 `<img>` 绕过**：
  - `web/src/components/app-search.tsx:135`（搜索结果封面）
  - `web/src/features/actors/detail-page.tsx:164`（演员图集）
  - 改法：改用 `MediaImage`。（已修复：两处改用 `MediaImage`，并让 `imageURL` 透传本地 `/api/actors/` 路径）
- [x] 🟠 ✅ **域名白名单后缀校验缺前导点**：`internal/freejavbt/client.go:245` `HasSuffix(host, "jdbstatic.com")` 放行 `evil-jdbstatic.com`。改法：`host == "jdbstatic.com" || HasSuffix(host, ".jdbstatic.com")`。（已修复：改为整域名/带前导点后缀匹配，并加 `TestMediaHostAllowlist`）
- [x] 🟠 ✅ **不存在的 Tailwind 类 `scrollbar-none`**：`movie-detail/dialog.tsx:34`、`movie-detail/preview-viewer.tsx:113`、`library/movie-shelf.tsx:106`。改法：换成 `no-scrollbar`。（已修复：改用 `no-scrollbar`，并删除冗余的 `[&::-webkit-scrollbar]:hidden` / `[scrollbar-width:none]` 兜底）
- [x] 🟠 ✅ `internal/api/router.go:49-52`：`SetTrustedProxies` 错误被吞，if/else 两分支等价；配置错误时静默部分生效，影响 `ClientIP()` 与登录限流。改法：调用一次并返回错误。（已修复：`config.validate` 校验 IP/CIDR，非法启动即报错；`router.go` 合并为一次调用，出错时记录并回退为不信任代理）
- [x] 🟠 `web/src/features/sources/sources-section.tsx:166`：保存失败的 `InlineError` 渲染在模态框之下，用户不可见。改法：`onError` 用 toast 或把错误传入 `SourceDialog`。（已修复：`persist` 增加 `onError` toast，移除被遮住的 `InlineError`）
- [x] 🟠 `web/src/features/library/player-dialog.tsx:156-160`：`try { void seek() } catch {}` 捕获不到异步 reject。改法：`seek(...).catch(() => {})`。（已修复：改为 `void seek(...).catch(() => {})`）
- [x] 🟠 `web/src/features/library/player-dialog.tsx:53-55,87,118`：`sources` 为空时永远显示"加载中"；失败无重试。改法：区分 pending / error / 无源，失败用 `InlineError onRetry`。（已核实：后端成功响应必带 ≥1 条 source，"成功+空源"分支不可达；已修复真实问题——播放失败改用 `InlineError onRetry`，右侧信息区失败态用 `InlineError`、加载态改骨架屏）
- [ ] 🟠 `web/src/features/sources/sources-section.tsx:484-488,210-214`：PikPak 提示"在卡片中完成登录"，但卡片无此入口。改法：移除 pikpak 选项或改文案（暂不处理）。
- [x] 🟠 `web/src/features/sources/sources-section.tsx:505-514`：删除存储源无二次确认（data-section 清理缓存有确认）。（已修复：删除前弹出二次确认对话框）
- [x] 🟠 `web/src/components/app-navigation.tsx:36,57`：`onNavigate` 无人传入，移动端点击导航后侧边栏不关闭。改法：内部 `useSidebar().setOpenMobile(false)`。（已修复：`AppNavigation` 内部 `setOpenMobile(false)` 并移除死 prop；侧栏 logo 链接同样关闭）
- [x] 🟠 `internal/drive/events.go:18-19` vs `account_ops.go:74-78`：注释承诺"commit 锁下运行、失败回滚"，代码未实现。改法：实现或修正注释。（已修正注释：明确回调不在 commit 锁下、失败不会回滚已持久化的 mount）
- [x] 🟠 `internal/drive/accounts.go:149-155`：`Mount` 把数据库错误吞成"未挂载"。改法：签名返回 `error`。（已修复：`Mount` 改为返回 `(source, bool, error)`，区分「未挂载」与「读库失败」；同步更新 7 处调用方、`source.MountProvider` 接口与测试）
- [x] 🟠 `internal/drive/accounts.go:181-215`：`Accounts()` 失败项丢失 `AccountID`，前端无法定位掉线账号；`refreshExpiringAccounts` 吞错无日志。（已修复：`AccountStatus` 增 `AccountID` 并经 `/api/pan/accounts` 透出；`refreshExpiringAccounts`/`Accounts` 刷新失败补 `slog.Warn` 日志）
- [x] 🟠 `internal/metadata/providers/onepondo.go:52-84`：缺 `MovieID/SiteID` 校验，空 title 不返回 `ErrNotFound`。改法：复用 `parsePondoMovieJSON`。（已修复：`parseOnePondoJSON` 改为薄封装复用 `parsePondoMovieJSON`（movieID + SiteID 2470），并加负向测试）
- [x] 🟠 `internal/javdb/media.go:32`：图片下载无响应体上限。（已修复：media 客户端 `SetResponseBodyLimit(4 MiB)`，超限返回错误；补超限测试）
- [x] 🟢 `internal/freejavbt/client.go:148`：singleflight 使用首个调用者 ctx，其取消会连带其他调用者失败。（已修复：flight 内改用 `context.WithoutCancel(ctx)`，由 HTTP 客户端超时兜底）
- [x] 🟢 `internal/freejavbt/client.go:264`：`err == metadata.ErrNotFound` 应为 `errors.Is`。（已修复：改用 `errors.Is`）
- [x] 🟢 `internal/offline/recovery.go:217-221`：`deferAction` 数据库出错时丢失原始 `cause`。改法：`errors.Join(cause, err)`。（已修复：`return errors.Join(cause, err)`）
- [x] 🟢 `web/src/features/settings/network-section.tsx:120-122`：回车保存绕过 `isDirty`；Emby 不响应回车。改法：统一用 `<form onSubmit>`。（已修复：按“只允许按钮保存”，移除网络代理输入框的回车保存；Emby 本就只有按钮保存）

---

## P1 · 性能

### 后端
- [x] 🔴 ✅ `internal/actors/service.go:642-692`：每个头像请求全表扫描 Actor 并逐行 sha1（头像已改为 eager 加载，放大开销）。改法：profile 存 key 列或用 profile ID，按索引单行查询。（已修复：`Actor` 新增索引列 `name_key`（归一化名的 sha1），`scrape/mapping.go` 写入，`findNames` 改为按 `name_key` 走索引查询；并给 `actors.actor_profile` 加索引，存在 profile 时补合并别名仍走索引。查询计划由 `SCAN actors` 变为 `SEARCH ... USING INDEX actor_name_key`/`actor_actor_profile`）
- [ ] 🔴 `internal/actors/service.go:219-281,351-362`：`Catalog/Detail` 全量加载演员与影片边后内存分页；`:279` 排序比较器内反复调用 `display()`（每次复制+排序）。改法：以 ActorProfile 为主表 SQL 分页，`display()` 预计算。（部分改善：`display()/norm` 已改为每组只算一次；`Detail` 改走 `name_key` 索引只加载单组，不再全量构建目录。Catalog 的 SQL 分页未做——分组键是「profile_id 或归一化名」的派生值、非可索引列，且库中 3/91 有 profile，需先物化分组表，另行处理）
- [x] 🔴 ✅ `internal/actors/service.go:936-943`：`EnsureProfiles` 循环逐行写、无事务、错误全吞。改法：`WithTx` + `Update().Where(IDIn)` + `CreateBulk`。（已重写为增量对账：只处理「尚无 profile 且能匹配到已有 profile」的演员，补挂 profile + 缺失别名；`s.db.Tx` 事务内批量 `Update().Where(IDIn)`，不再逐行写/全删重建/新建 profile。新增 `EnsureMovieProfiles(ctx, movieID)` 在每次影片刮削入库后调用（`scrape.go` 紧随 `MergeMovieCrossLanguage`），避免新抓到的同名演员在列表里裂成两条。`EnsureProfiles` 保留为全量增量兜底，仅处理未挂载演员）
- [x] 🟠 ✅ `internal/actors/service.go:377-382`：GET `Detail` 中逐个 UPDATE 写库。（已修复：改为单条批量 `Actor.Update().Where(IDIn(ids...), 仅未挂载或挂到别的 profile).SetProfileID(pid)`，已挂到本 profile 的演员跳过，重复打开详情页 0 写；错误改为返回不再 `_ =` 吞掉）
- [x] 🟠 ✅ `internal/actors/service.go:504-534`：`mergeActorPair` 多步写无事务、错误全吞；`fillProfileFrom:585`、`applyProfile:847` 同。（已修复：`mergeActorPair` 整体放进 `s.db.Tx`，其中 `ensureProfileTx`/`fillProfileFrom`/`copyProfileImages` 全部改为接收 tx 并返回 error，任一失败回滚；`applyProfile` 也改为「先取 `persistMu`（统一锁序 persistMu→tx）再开事务」，资料更新 + 图片写入同事务、返回 error；`appendProfileImages` 改 tx 版、错误不再 `_ =`。`enrichProfile` 现在会把 `applyProfile` 的错误向上返回以便下次重试）
- [x] 🟠 ✅ `internal/actors/service.go:588-613` 与 `:851-878`：两个追加图片函数重复且循环逐行 Create。改法：抽 `appendImages` + `CreateBulk`。（已修复：抽出 `appendImageRows(ctx, tx, pid, items []imageItem)` 统一「查已有→按 URL 去重→续 position→`CreateBulk`」；`copyProfileImages` 与 `appendProfileImages` 只负责把来源映射成 `imageItem` 后调用它，重复逻辑消除、插入改为批量。另把 `persistImages`（gfriends）也改为事务内复用同一 helper、返回 error 不再吞，避免半截图集永久缺图）
- [x] 🔴 ✅ `internal/library/library.go:789-844`：`BrowseMovies` 读全部匹配行（含 metadata JSON）再内存分页；`hydrateMovieEdges:867` 重复查标量。改法：`recent` 排序走 SQL `Offset/Limit`；hydrate 只查边。（已修复：`Movie` 新增持久化列 `rating10`（ent hook 由 `rating`+`metadata.rating_max` 派生，索引 `(rating10,id)`）；`BrowseMovies` 改为 `Count` + SQL `ORDER BY`/`Offset`/`Limit`，recent（含 `cloudAddedOrder`）、release、rating 三种排序全部下推，HighRated 改为 `rating10 >= 8` 进 WHERE；演员/标签改为随主查询 `WithActors/WithTags` 一次取回并删除 `hydrateMovieEdges` 二次查询；列表与 `Movies()` 不再 select `metadata`，`movieItem` 改读 `rating10`。注：按约定不做存量回填，老库需重建/重刮）
- [x] 🔴 ✅ `internal/library/library.go:534-564`：`TopRated` 对全部评分影片预加载边后再截断。改法：先截断后 hydrate。（已修复：查询改为 `Select(browseScalarFields...)` + `Order(rating10 DESC, id DESC)` + `Limit(limit)`，演员/标签边只对返回的 ≤limit 条 `WithActors/WithTags` 加载，删除内存 `sort.SliceStable` 全量排序与 `items[:limit]` 截断；`markCloudSources/markSubtitleStatus` 随之只处理 ≤limit 条；移除不再使用的 `sort` import；新增 `TestLibraryTopRatedSortsAndTruncates` 覆盖排序、同分 ID 倒序、limit 截断、无评分/无文件排除）
- [x] 🟠 ✅ `internal/library/library.go:445-446,724-725`：一次请求调两次 `drive.Mounts`，且 `Source()` 用 `context.Background()`。（已修复：列表路径改为先 `mountList(ctx)` 只取一次挂载源，再由 `mergeSources` 复用同一份生成 scope、`firstSource` 供 `Page.Source`，`Movies`/`BrowseMovies` 不再二次调用 `cloudSources`→`Mounts`；`Service.Source` 以及 `catalogue.LocalState` 接口改为接收 `ctx`（`Source(ctx)`），彻底去掉 `context.Background()`。`TestLibraryPageLoadsCardMetadataWithScopedCounts` 的 `SettingQuery` 期望由 2 降为 1，确认每请求只读一次账号配置）
- [x] 🟠 ✅ `internal/drive/account_session.go:33,108,115,191`：每次远程调用从数据库加载并反序列化全部账号。改法：挂载信息内存缓存 + 版本号。（已修复：`Drive` 增加内存 `accountMounts`（`New` 从账号表装载，`saveAccountMount`/`removeAccount` 落库成功后同步更新，`d.mu` 保护）；`Mount`/`Mounts` 改读内存，不再 `loadAccounts` 反序列化整表；失效沿用现有 `accountAuthVersions`（mount 变更即 bump，`checkSource` 已比对），未另造版本号。效果：每个远程调用（`readAccount` 前后两次 `checkSource`）的 2 次整表账号读降为 0，library 列表的 settings 读也随之消失——`TestLibraryPageLoadsCardMetadataWithScopedCounts` 的 `SettingQuery` 期望已从 2→1→0 移除。`drive`/`library`/`offline`/`api`/`catalogue` 测试全过。补充（P2/P3）：`accountRecord` 也改为读内存（token+mount），`OpenSource`/`accountSessionFor`/`AccountStatusFor` 不再读库；`refreshExpiringAccounts`/`Accounts` 会从 DB 重新同步挂载缓存，兜底外部/手工改库导致的陈旧（仍假定单写进程）。）
- [x] 🟠 ✅ `internal/offline/sync.go:149,155`：循环内重复读配置与挂载。改法：循环外读一次传参。（已修复：`syncSource` 进入任务循环前 `service.config(ctx)` 只读一次拿到 `prefs`，新增内部 `updateTask(ctx, sess, record, remote, prefs)` 传参；循环内两处调用改走该内部方法；公开 `UpdateTask` 保留为「读一次配置再转调」的薄封装以兼容调用方/测试。挂载读取 `currentMountFor`→`drive.Mount` 已在上一条改为内存缓存，不再读库。`go build`/`go vet`/`go test ./internal/offline/...` 通过）
- [x] 🟠 ✅ `internal/maintenance/service.go:48,128`：每次打开系统设置页全表扫描图片字段。改法：按需计算或短 TTL 缓存。（已修复：对展示用的引用快照 `retained` 加 5 分钟 TTL 缓存（`cachedRetainedArtwork`/`storeRetained`），`Info` 在 TTL 内直接复用，重复打开设置页不再扫影片表；`ClearCache` 始终现算引用、绝不用缓存，避免用过期集合误删在用封面，清理后再刷新缓存。清理语义不变——只删未引用的孤儿图、保留在用封面。`go build`/`go test ./internal/maintenance/...` 通过）
- [x] 🟠 ✅ `internal/metadata/providers/html.go:44-49`、`internal/actors/html.go:45-50`：`first()` 遍历整树并分配切片只取第一个。改法：命中即返回。（已修复：两处 `first()` 改为带提前返回的 DFS，命中即停，不再调用 `nodes()` 收集整棵树的匹配切片；`nodes()` 保留（其它解析仍在用）。`go build`/`go vet`/`go test ./internal/metadata/... ./internal/actors/...` 通过）
- [x] 🟠 ✅ `internal/gfriends/client.go:200-221,314-382`：两套索引、同一 6 MiB 文件解析两遍；`ensureAll` 不检查 TTL、不回写磁盘。改法：只保留 `allImages`。（已修复：统一为单一 `allImages` 缓存 + 单一加载器 `ensureAll`（`EnsureIndex` 变薄封装）；删除 `index`/`allAt`/`allMu`/`indexReady`/`buildIndex`（`buildIndex`/`installIndex`/`readCachedIndex` 仅移到 `legacy_helpers_test.go` 供旧测试）；磁盘缓存按 mtime 判 7 天 TTL、下载成功后回写 `gfriends_tree.json`、失败沿用 retry/backoff 且保留旧缓存；`Lookup` 取 `allImages[name][0]`（folder、alias 均排序保证“第一个”确定）；新增 `TestGFriendsClient_ImagesCacheWritesBackAndHonorsTTL` 覆盖回写/新鲜命中/过期重下。`go build`/`go vet`/`go test ./internal/gfriends/... ./internal/actors/... ./internal/emby/... ./internal/api/...` 通过）
- [x] 🟢 ✅ `internal/metadata/service.go:188-189`：每拿一个结果就整体 merge，O(n²)。改法：循环后 merge 一次。（已修复：内层来源循环只 `append(results, result)`，循环结束后 `merge(results)` 一次；跨来源的「同片」冲突校验改为累计 `confirmedCode`（首个非空 code）逐条 `IsFormatEquivalent` 比对，语义等价。`go build`/`go vet`/`go test ./internal/metadata/... ./internal/library/scrape/...` 通过）
- [x] 🟢 ✅ 正则在函数内编译：`metadata/providers/tokyohot.go:266,269`、`actors/avleague.go:175`。改法：包级变量；tokyohot 可复用 `codeAVDuration`。（已修复：`japaneseDuration` 的 PT 分支改为直接复用包级 `codeAVDurationMinutes`，不再每次现编译 `(\d+)H`/`(\d+)M`；`parseMeasurements` 的身材正则在包级 `avLeagueMeasurements` 预编译。`go build`/`go test ./internal/metadata/... ./internal/actors/...` 通过）
- [x] 🟢 ✅ `[]byte`→`string` 多余拷贝：`actors/avleague.go:80,112`、`providers/codeav.go:91`。改法：`bytes.NewReader`。（已修复：三处 `html.Parse(strings.NewReader(string(body)))` 改为 `html.Parse(bytes.NewReader(body))`，省去 `[]byte→string` 拷贝。`go build`/`go test ./internal/actors/... ./internal/metadata/...` 通过）
- [x] 🟢 ✅ `internal/netx/ssrf.go:187,198`：前置 `ValidateSafeURL` 与拨号时重复 DNS 解析。（已修复·方案A：新增只做语法/字面量检查、**不解析 DNS** 的 `validateSafeURLShape`；`CheckRedirect` 按代理状态分流——配置了代理时仍用完整 `ValidateSafeURL`（代理拨号不校验目标，必须在此拦），直连时用 shape（拨号阶段已解析并直连已校验 IP）。`SafeDownload` 的预检保持完整（拿不到 proxy 状态，代理场景必需），已加注释说明。新增 `TestValidateSafeURLShapeSkipsDNS` 断言 `.invalid` 域名不做 DNS。`go build`/`go vet`/`go test ./internal/netx/... ./internal/subtitle/... ./internal/freejavbt/... ./internal/metadata/...` 通过）
- [x] 🟢 ✅（核实为非问题，保留。`internal/ent/schema/task.go:43`：索引 `(type,status)` 虽是 `(type,status,retry_at)` 的前缀，但**并不冗余**——`tasks.Queue.Claim` 以 `ORDER BY id` 领取任务，依赖该索引的隐式 rowid 顺序避免排序；`(type,status,retry_at)` 是给 `NextRetry` 按 `retry_at` 排序用的。`internal/database/task_indexes_test.go` 亦断言 batch claim 命中 `task_type_status` 且无 TEMP B-TREE，删掉会导致该测试失败与领任务退化。故保持现状，不改 schema。）

### 前端
- [x] 🟠 ✅ `web/src/features/tasks/task-events.tsx:45,138`：`revisions` 为 effect 局部变量且 onerror 清空，每次重连全量刷新四类查询。改法：`useRef` 跨重连保留（⚠️ 先确认后端 revision 重启是否归零）。（已修复·方案B：确认后端 revision 是内存计数、**重启归零**（`tasks/bus.go`）。后端 `Bus` 增加 `instance` 进程标识，SSE `changes` 事件带上 `instance` 字段（`TaskRevisions` 结构不变，仅 SSE payload 增加该字段）；前端把 `revisions` 改为 `useRef` 跨重连保留、`onerror` 不再清空，仅在 `instance` 变化（服务重启）时重置基线全刷一次，否则只刷真正变化的类别。`go build`/`go vet`/`go test ./internal/tasks/... ./internal/api/...` 与 `pnpm build`（vite+tsc）均通过）
- [x] 🟠 ✅ `web/src/features/library/page.tsx:22` + `api/library.ts:109-115`：同接口一页请求两次。改法：共用 `libraryKeys.movies(1)` + `select` 截取。（已修复：`useRecentMovies` 改为复用 `libraryKeys.movies(1)` 缓存、queryFn 固定 `page:1, limit:LIBRARY_PAGE_SIZE`，用 `select` 截取前 `limit` 条；删除不再需要的 `libraryKeys.recent`。列表页 page1 只发一次 `/api/library/movies`。`pnpm build` 与 `pnpm test`（186 passed）通过）
- [x] 🟠 ✅ `web/src/api/subscriptions.ts:65-71`：内联 `select` 每次渲染重建 Map；订阅页 `'all'` 与发现页 `'movie'` 双缓存。改法：模块级 select，订阅页用 `'movie'`。（已修复：`select` 提为模块级 `indexTargets`（数据不变时不再重建 Map）；订阅页 `useSubscriptionTargets('movie')`——该页只过滤 `kind==='movie'`，与发现页铃铛共用 `subscriptions/targets/movie` 缓存，去掉多余的 `'all'` 请求。`pnpm build` 与 `pnpm test`（186 passed）通过）
- [x] 🟠 ✅ `web/src/features/library/player-dialog.tsx:140-189`：订阅 `currentTime` 导致播放中持续重渲染；保存进度逻辑重复两次。（已修复：`PlaybackSync` 改用 `usePlayer()` 无选择器取 store（**不订阅**，`currentTime` 不再触发重渲染），保存时按需读 `player.state`；仅保留低频的 `paused`/`duration` 订阅。抽出单一 `saveProgress` 供 interval / teardown / pause 三处复用，删除原先重复的保存代码与 `latest` ref。`pnpm build` 与 `pnpm test`（186 passed）通过）
- [x] 🟢 ✅ `web/src/api/history.ts:34-40`：缺 `keepPreviousData`，翻页闪骨架。（已修复：`useHistory` 增加 `placeholderData: keepPreviousData`，翻页时保留上一页数据、`isPending` 不再为 true，历史页不会闪 `MovieGridSkeleton`。`pnpm build` 与 `pnpm test`（186 passed）通过）
- [x] 🟢 ✅ `web/src/features/settings/scraping-section.tsx:59-66`：开关无乐观更新且保存期间整列禁用。改法：`isPending ? variables : data`。（已修复：`ScrapingSection` 与同构的 `ActorSourcesSection` 均改为 `const sources = (update.isPending ? update.variables : query.data) ?? []`，保存过程中直接显示提交后的状态（乐观）；并移除 `Switch` 的 `disabled={update.isPending}`，不再整列禁用。`pnpm build` 与 `pnpm test`（186 passed）通过）

---

## P2 · 死代码 / 冗余（移至回收站，勿 rm）

### 整文件
- [x] ✅ `web/src/features/movie-detail/recommendation-visibility.ts`（已移入回收站；同步删除 `web/tests/movie-detail.test.ts` 中 `observeRecommendation` 的 import 与该 helper 的单测，其余 15 个用例保留；`tsc`/`vitest` 通过）
- [x] `web/src/features/settings/pan-account.tsx`（已移入回收站；导出的 `PanAccountInfo`/`PanStorageUsage` 全仓 0 引用，`PanSourceCard` 为 `sources-section.tsx` 自带实现、未复用它）
- [x] `web/src/features/library/movie-status.tsx`（已移入回收站；`LibraryMovieStatus` 0 引用）
- [x] `web/src/components/videojs/ui/airplay-button.tsx`、`cast-button.tsx`、`pip-button.tsx`、`quality-menu.tsx`（已移入回收站；四个均无 import、`ui/` 无 index barrel，实际使用的是 `custom-quality-menu`）
- [x] `web/src/components/videojs/styles/audio/base.css`（已移入回收站；全 web 搜 `audio/base` 0 命中，各组件直接 `import '../styles/audio/theme.css'`；**保留 `audio/theme.css`**）
- [x] `internal/metadata/providers/javdatabase.go`（已移入回收站，确认不再需要；连带移走 `javdatabase_test.go` 与 `testdata/javdatabase_ssis-001.html`；`go build ./...`/`go vet ./internal/metadata/providers/` 通过）

### 函数 / 字段 / 常量
- [x] ✅ `web/src/api/discover.ts:181-199` `useRecommendationMovie`；`api/movie-detail-cache.ts:48-177` `subscribeMovieCard`/`createMovieDetailLoader`；`useDiscoverMovie` 中 `prioritize` effect；`discoverKeys.search`（已清理：删除 `useRecommendationMovie`、`useDiscoverMovie` 的 prioritize effect、模块级 `movieDetails`(`createMovieDetailLoader`)、`subscribeMovieCard`、`discoverKeys.search`；`movie-detail-cache.ts` 仅保留 `findCachedMovieCard`；`movie-detail.test.ts` 裁剪为 3 个卡片缓存用例。`tsc` + `vitest`（173 passed）通过）
- [x] `web/src/api/play.ts:8-13,40-69` `usePlayFiles`/`usePlayback`/`PlayFiles`；释放会话 effect 抽 `useReleasePlaySession`（已删 `PlayFiles`/`usePlayFiles`/`usePlayback`；`usePlayQualities`/`useStartMoviePlayback` 未动；`useReleasePlaySession` 属重构建议，未做）
- [x] `web/src/api/pan.ts:67-68` `fileLists`、`panKeys.files`（已删 `fileLists`；`panKeys.files` 经核实被 `web/tests/query-errors.test.ts:90` 使用，保留）
- [x] `web/src/features/tasks/task-notification-diff.ts:1-14` 重复的 `isScanTask/isTaskActive/isOfflineTaskActive`（已改为复用 `@/api/tasks`、`@/api/offline`，仅保留本地 `isBatchTask`）
- [x] `web/src/features/tasks/task-toast.tsx:79-81` 不可达分支（已简化：active 分支恒有 `movie_id`，删除不可达的 `正在${action}媒体库` 分支）
- [ ] `web/src/features/tasks/task-progress-state.ts:6-13` 从未显示的 `stages[].label`（复核成立；涉及 `TaskProgress` 取 label 兜底的重构，待办）
- [x] `internal/offline/recovery.go:83-101` `Service.Recover`（已删除；`recoverSource` 仍由 `sync.go:66,132` 使用，保留）
- [x] `internal/offline/service.go:18-20` 接口未用方法 → `catalogue.HasMagnet`（`magnets.go:67`）、`MovieCode`（`service.go:225`）；`monitor.OfflineAdder.Add` → `offline.Service.Add`（已从接口删除 `HasMagnet`/`MovieCode` 与 `OfflineAdder.Add`；实现与测试保留）
- [x] `internal/catalogue/types.go:81` `LocalState.Source()`（已从接口删除；包内 0 调用）
- [x] `internal/gfriends/client.go:31` `rawContentBase`、`:410-419` `FetchImage`；`:234-245` 改用 `escapeContentRel`（已删 `rawContentBase` 与 `Client.FetchImage`；`escapeContentRel` 已在 `FetchRel` 使用）
- [ ] `internal/metadata/service.go:244` `needsSupplement`；`:255-264` `attachSourceIdentity` 简化（已删 `needsSupplement` 及测试断言；`attachSourceIdentity` 简化待办）
- [x] `internal/catalogue/service.go:40` `proxy`、`internal/database/sqlite.go:21` `db` 只写不读（已删 `Service.proxy` 字段及赋值；`Store.db` 被数据库测试读取，保留）
- [x] `internal/domain/movie.go:47` `SearchOptions`；`tasks/payload.go:35` `SetPayloadField`、`netx.ProxyManager.Update`（仅测试用）（复核成立但【评估后保留，不改】：三项均仅测试引用、无生产调用；删除需改多份测试并削减 `ProxyManager` 等公共 API 覆盖，运行收益为 0，属过度清理）
- [x] `internal/api/error.go:83` 不可达分支；`:112` 硬编码文案比较（已处理：`:83` 删除不可达的 `errors.Is(err, fs.ErrNotExist)` 分支——`domain.KindOf` 已把 `fs.ErrNotExist` 映射为 `KindNotFound`；`:112` 改为 `status==404 && ent.IsNotFound(err)` 时用 `domain.PublicMessage(domain.E(domain.KindNotFound,"",nil))` 取规范文案，去掉字面量比较。`go build`/`go vet`/`go test ./internal/api/...` 通过）

### 未被传入的 props
- [x] `components/movie/movie-card.tsx` `coverLoading`、`onCoverReady`；探索/库卡片多余的 `titleTooltip={false}`（已删 `coverLoading`/`onCoverReady` 及 `MovieCover` 传参；两处 `titleTooltip={false}` 也删除（两卡片均 `hideContent`，本来无效））
- [x] `components/movie/movie-cover.tsx` `loading`、`onReady`（`onReady` 已删；`loading` 保留——`movie-detail/hero.tsx:20` 用 `loading="eager"`）
- [x] `components/media-image.tsx:13,43-45` `onReady` + effect（已删 `onReady` 属性与对应 `useEffect`）
- [x] `components/movie/movie-grid-skeleton.tsx:7` `compact`（复核为已使用：history/actors/movies 页均传 `compact`，保留）
- [x] `components/page-header.tsx:9` `inlineActions`；`components/app-page.tsx:9` `contentClassName`（`inlineActions` 已被 history/library 页使用，保留；`contentClassName` 无调用方，已删）
- [x] `features/library/movie-card.tsx:11-15` `busy`、`playIcon`、`hideActors`（连带 `movie-actions` 的 `disabled`）（全部调用点均未传；已删这些 props 与对应分支，`LibraryMovieActions` 去掉 `disabled`）
- [ ] `features/library/scan-button.tsx` `available`、`onStarted`（复核：生产调用传的是常量 `available` 与空实现 `onStarted`，非“未传”；评估后保留，改动收益低）
- [x] `features/discover/results.tsx` `searching`（恒 false）、`errorMessage`（唯一调用传 `searching={false}`、从不传 `errorMessage`；已删两个 props 并固定文案）
- [x] `features/movie-detail/metadata.tsx:76` `EntityLink` 的 `kind`（调用处传了 `kind` 但函数未用；已删 `kind` 参数与各调用点，仍渲染 `span`）
- [x] `features/movie-detail/dialog-context.ts:11` `openMovie` 的 `_trigger`（含 `app-search.tsx:218` 的 `document.createElement('div')`）（已删 `trigger` 参数、`detail-trigger`/`app-search` 的第二实参与 `createElement('div')`，同步改测试断言）
- [x] `api/subscriptions.ts:156` `useRemoveSubscription` 的 `kind`（输入类型仅保留 `{ id }`；调用方传对象多余字段不影响）

---

## P3 · 过度防御 / 过度设计

- [x] 🟠 `web/src/features/settings/emby-url.ts` + `emby-section.tsx:95-106`：后端单一 URL，前端拆 host/port 再拼。改法：单输入框。（已合并为单个 `server_url` 输入框，删除 `emby-url.ts` 的 split/combine 及测试、`handleHostBlur`）
- [x] 🟠 `web/src/features/settings/emby-section.tsx:43-61`、`network-section.tsx:26-34`：表单草稿两套写法，含渲染期 `setState`。改法：数据就绪后挂载表单 + `key` 重置。（已拆出 `EmbyForm`/`NetworkForm` 子组件，数据就绪后挂载、保存成功以 `resetToken` 作 `key` 重置；删除渲染期 `setForm(null)` 与 null 哨兵；顺带修复保存/测试按钮宽度抖动）
- [x] 🟠 `web/src/stores/theme.ts:58-66`：DOM 副作用在被 3 处调用的 hook 中重复执行；登录页不同步系统主题。改法：单点 `ThemeSync`。（已抽出 `ThemeSync` 组件并在 `__root` 的 `AccessGate` 之外挂载一次，`useTheme` 改为纯读取；登录页可同步系统主题，重复副作用消除）
- [x] 🟢 `web/src/stores/theme.ts:26-46`：SPA 中的 SSR 守卫。（已删除 `typeof window`/`matchMedia` 守卫与 `getServerSnapshot` fallback，`useSyncExternalStore` 去掉第三参）
- [x] 🟠 `internal/api/discover.go:122-128`：匿名接口运行时断言 + 吞错。改法：加入 `CatalogueManager` 接口。（已把 `LocalRecommendations` 加入 `CatalogueManager` 并直接调用，移除匿名接口断言；错误改为 `c.Error` 记录、不再吞掉，推荐仍为 best-effort）
- [x] 🟠 `internal/network/service.go:65-87`：单任务 goroutine + WaitGroup。（已整体取消网络代理"测试连接"环节：删 `TestNetwork`、`networkProbeTimeout`、`TestResult/ProbeResult`、`NetworkManager.TestNetwork`、`POST /network/test` 路由与相关后端测试，前端删 `useTestNetwork` 与测试按钮；`javbus/probe.go` 一并移除）
- [x] 🟠 `internal/app/app.go:129-259`：手写出错清理链。改法：`cleanup []func()` + defer 逆序。（已改为资源创建成功后 `cleanup = append(...)`，`New` 命名返回 + defer 逆序关闭；删除约 10 处手工关闭链，成功路径不触发关闭）
- [x] 🟠 `web/src/api/library.ts:190-205`、`api/offline.ts:77-96,106-123`：条件 `setQueryData` 写三遍且紧接 invalidate。（已把三处"守卫式乐观合并"收敛为 `query-refresh.ts` 的 `updateIfCurrent` helper；保留原并发语义与 invalidate，测试保持通过）
- [x] 🟢 `web/src/api/emby.ts:42-59`、`api/network.ts:36-53`：乐观更新 + 回滚 + 立即 invalidate，回滚冗余。（已删除 emby/network 的 `onError` 回滚，仅保留 `invalidateQueries` 由重取对账；`onMutate` 改为 updater 不再返回 `previous` 快照；同步更新 `emby-settings` 测试）
- [x] 🟢 `web/src/api/tasks.ts:87,96`：直接 `toast.error(error.message)`，绕过全局 `meta.errorTitle` / `UNAUTHORIZED` 静默 / `describeApiError`。（已改用 `meta: { errorTitle: '重试失败' / '操作失败' }` 交给全局 MutationCache 统一处理：`describeApiError`、`UNAUTHORIZED` 静默、PAN 失效提示；移除直接 toast 与 `error.message`）
- [x] 🟢 `web/src/features/access-gate/access-gate.tsx:72`：用 `error.message` 而非 `describeApiError`。（已改为 `describeApiError(login.error)`，非 ApiError 回退统一文案）
- [x] 🟢 `web/src/components/media-image.tsx:72-77`：`typeof image.decode` 检测多余；decode 失败误标 error。（已去掉 `typeof` 检测，直接 `await image.decode()`；decode 失败不再置 error，仍标记 ready）
- [x] 🟢 `web/src/components/list-pagination.tsx:29`：`nextPage === page` 防御多余。（已删除该守卫；实际调用点只会传 `item !== page` / `page±1`）
- [x] 🟢 `web/src/components/app-shell.tsx:65-77`：主题类型断言 + `?? 'system'` 多余；暗色显示 Sun 图标语义不符。（已删第 65 行 `as ThemeName | undefined` 断言与 `?? 'system'`，并移除随之多余的类型别名；第 66 行 `?? 'system'` 因 `noUncheckedIndexedAccess` 必须保留；暗色显示 Sun 属正确"切换到亮色"动作语义，未改）
- [x] 🟢 前端多余空值判断：`app-search.tsx:73`、`library/movie-card.tsx:25`、`library/continue-watching.tsx:15`、`actors/detail-page.tsx:38-39,102`、`discover/movie-subscribe-button.tsx:20`、`player-dialog.tsx:21-25,48`、`subscriptions/page.tsx:100-104`（已删 4 处确证多余：app-search / movie-card 的 `movie.actors ?? []`、continue-watching 的 `duration>0?`（filter 已保证）、movie-subscribe 中与按钮 disabled 重复的 `add.isPending`；其余经评估为必要类型/重入守卫，保留，故行号引用已漂移）
- [x] 🟢 后端多余 nil 检查：`catalogue/state.go:27,48`、`catalogue/recommend.go:23`、`api/router.go:44`、`api/movie.go:207`、`app/app.go:392-415`、`drive/accounts.go:63,88,101`、`freejavbt/client.go:176,181,196`（已删恒非 nil 的检查：recommend.go 与 state.go 的 `database == nil`、accounts.go 两处 map 懒初始化、freejavbt 的 cache 懒初始化、App.Close 逐字段 nil 守卫；保留 state.go `local == nil`（部分构造可为 nil）、router.go logger 兜底、movie.go `emby != nil`（可选依赖）。行号已漂移）
- [x] 🟢 `web/src/features/sources/sources-section.tsx:56-99`：`KIND_OPTIONS` 大半字段死数据 + `?? KIND_OPTIONS[0]!` 兜底。（已拆为 `KIND_LABELS: Record<SourceKind,string>` + `KIND_ORDER`，本地路径字段独立为 `LOCAL_PATH`（仅本地用）；删掉 115/pikpak 永不可达的 path 字段与 `?? KIND_OPTIONS[0]!` 兜底）
- [x] 🟢 `web/src/components/videojs/video/menus/settings-menu.tsx:19-32`：`qualityOptions.length > 0` 恒真、多余默认值。（已收紧 `settings-menu`/`controls`/`skin` 三处：`qualityOptions/qualityValue/onQualityChange` 改为必填，移除 `length>0` 守卫、`?? (()=>{})`、`= []`/`= ''`/`= {}` 默认值，`CustomQualityMenu` 常驻）
- [x] 🟢 `web/src/features/library/player-dialog.tsx:51-62`：轻量计算的 `useMemo`；`:125` 多余 `show115`；`:76-81` `skinStyle` 可用 className。（已把 `source` 的轻量 `useMemo` 改为普通表达式；`MovieInfo` 去掉 `show115` prop、内部改用 `movie.source === '115'`；`skinStyle` 改为 `VideoSkin` 的 className（`aspect-video w-full` + 字体 CSS 变量）；移除 `CSSProperties` 导入）
- [x] 🟢 `internal/offline`：`config.go:22,24` 重复 `Normalized()`；`sync.go:36`、`recovery.go:97` 可直接 `return errors.Join(errs...)`；`add.go:135-145` 冗余 return。（已改：`config.go` 的 `Normalized()` 只算一次并复用；`sync.go` 改为直接 `return errors.Join(errs...)`；`add.go` 闭包内改为 `return err`；`recovery.go:97` 行号已过时、无该模式，未改）
- [x] 🟢 `internal/monitor/subscribe.go:22/35`：`existingMovie` 重复查询；`:135` `ptr` 可用 `new(v)`（Go 1.26+）。（已抽出无存在性检查的 `createMovie`，`AddMovie` 检查后直接 createMovie、`addMovie` 保留检查供 actor/测试调用，新增路径不再重复查询；`ptr` 辅助函数删除改用 `new(expr)`，同步更新 actor.go 与测试）

---

## P4 · shadcn / base-ui / Tailwind 规范

- [x] 🟠 **`asChild` 与 `render` 混用**：已移除 button/badge/tooltip/dialog/dropdown-menu/breadcrumb 的自造 `asChild`，全部调用点改 `render`（全库 `asChild`=0）；顺带修正 `BreadcrumbLink`/`Badge` 的 `render` 顶层转发、`PaginationLink` 补 `nativeButton={false}`。
- [x] 🟠 `ui/progress.tsx`：改 `h-full transition-all`，删除 `flex-1 + translateX` 与 style（宽度由 base-ui 决定，已核对源码）。
- [x] 🟢 `ui/select.tsx`：补回 `origin`、`duration-100`、`data-open/closed` 进出场动画，与 Popover/Dropdown 一致。
- [x] 🟢 `ui/card.tsx`：内部 `rounded-t-xl/b-xl` 对齐为 `rounded-*-2xl`（含 img 圆角）。
- [x] 🟢 `ui/pagination.tsx`：省略号 `size-9` → `size-8`，与页码对齐。
- [x] 🟠 `components/list-pagination.tsx`：`href="#"` 假链接改真实 `Button disabled`，激活项不再重复触发。
- [x] 🟠 `app-navigation.tsx` / `routes/settings.tsx`：app-navigation 去掉手写 `aria-current` 与 `collapsed` tooltip 判断（改由 `Link`/`SidebarMenuButton` 处理）。（`routes/settings.tsx` 的手写 active 保留：改用 `Link activeProps` 会与基类 `text-muted-foreground` 冲突，风险高。）
- [x] 🟠 `features/settings/shared.tsx`：`SettingRow` 改用 `Field/FieldContent/FieldTitle/FieldDescription`，并给 `SettingsCard` 加 `@container/field-group` 恢复响应式横排。
- [x] 🟠 `features/movies/filter-multi-select.tsx`：改用 `CommandInput`，去掉 `CommandList` 重复 `overflow-y-auto`。
- [x] 🟢 `LoaderCircleIcon animate-spin` → `Spinner`：scan-button / subscriptions.page / subscription-card / task-toast / task-toast-actions / access-gate 已替换（其余非清单内同类项保留）。
- [x] 🟢 手写进度条 → `Progress`：`sources-section.tsx` 的 `PanSpaceBlocks` 改用 `Progress`。
- [x] 🟢 原生 `title` → `Tooltip`：`task-toast-actions.tsx` 取消/关闭按钮改用 `Tooltip`（Provider 已在 main.tsx）。
- [x] 🟢 在 shadcn 组件上叠加样式：
  - `app-search.tsx` 搜索按钮 `variant="link"` → `ghost`；CommandDialog 去掉重复 `overflow-hidden`（居中定位为有意保留）
  - `page-back-button.tsx` → `size="lg"`；`subscription-card.tsx` Checkbox `text-white` → `text-success-foreground`；`directory-breadcrumbs.tsx` 去掉重复 `gap-1 sm:gap-1`
- [x] 🟢 与默认值重复的 class / props：已清理 `Input h-8`（filter-bar/catalog-page）、`Card overflow-hidden`（movie-card/movie-grid-skeleton）、`tracking-normal`（recommendations/previews/magnets）、`main.tsx delay={0}`、`credentials: 'same-origin'`（api/client、task-events）、`task-toast dismissible/icon`、`api/pan staleTime: 30_000`。（Button 内显式 `size-4` 与 `type="button"` 保留：前者有 Button 的 `[&_svg]` 规则兜底、后者涉及表单默认行为，未盲删。）
- [x] 🟢 `components/movie/movie-card.tsx`：Badge `truncate` 改为内包 `<span className="truncate">`。
- [x] 🟢 `components/app-shell.tsx`：移除不生效的 `transition-[width,height] ease-linear`。（外层包装层保留：涉及布局，未盲删。）
- [x] 🟢 `styles/globals.css`：删除与 shadcn `@utility no-scrollbar` 重复的 `.no-scrollbar` 块。（`--warning`/`--chart-1..5` 保留：仍被 `@theme` 颜色映射引用。）
- [x] 🟢 `back-top-button.tsx` 加 `aria-label`；`pan-login-dialog.tsx` `<img>` 加 `alt`。
- [x] 🟢 `movie-shelf.tsx`：`href?: string` → `LinkProps['to']`。
- [x] 🟢 `discover/page.tsx`：去掉 `<BrowseResults key={view}>` 多余 key。

---

## P5 · UI 风格统一

- [x] 🟠 **settings 各 section 不一致**：`SettingsSection` 增加 `description` prop 并在 `scraping-section` 使用（去掉游离 `<p>`）；`tasks-section`/`scraping-section` 的「正在读取…」文字统一为 `Skeleton`；错误态本就以 `InlineError` 为主。（按钮尺寸/位置未逐一强统一；`SettingsActions` 未抽，避免引入暂时无人使用的抽象。）
- [x] 🟠 **状态色对比度不足**：`globals.css` 新增语义 `--info` 并把 `--color-info` 改为引用它；`--success` 提到 `emerald-600`、`--downloading` 提到 `cyan-700` 以提升白字对比度。（如需严格 WCAG AA 4.5:1，可继续加深到 `-700/-800`，建议视觉复核。）
- [x] 🟠 **筛选与分页状态未进 URL**：`/movies` 与 `/actors/` 增加 `validateSearch`，`MoviesPage`/`ActorsCatalogPage` 改为由路由 search 驱动（新增 `features/movies/search.ts`；列表筛选以逗号串编码）。`discover/history/subscriptions` 原本已用 `validateSearch`。
- [x] 🟢 **骨架尺寸与真实控件不符**：`discover/category-filters` 骨架改 `h-8 rounded-lg`；`library/scan-button` 骨架改 `h-8 rounded-lg`；`actors/detail-page` 骨架头像 `size-48`→`size-32`。
- [x] 🟢 `actors/catalog-page.tsx`：加载仅文字改为头像骨架网格。
- [x] 🟢 写法不统一：`aspect-[3/4]`→`aspect-3/4`（`actors/detail-page`）；`preview-zoom.ts`、`task-notification-diff.ts` 相对路径改 `@/`；`custom-quality-menu` lucide `Hd` 改 videojs `QualityIcon`。（`movie-subscribe-button asChild` 已在 P4 统一为 `render`。）
- [ ] 🟢 进度口径不一致：`library/library-progress.tsx` 仍自算百分比（其实按 `scan.metadata_*`/`directories_*`/`task.progress` 兜底，与 `scanCount()` 文案口径不同）。保留：两者语义不同（圆形进度 vs 任务文案），强合并会改变现有展示，建议单独评估。
- [x] 🟢 `LibraryPlayerDialog` 引入方式统一：新增 `features/library/player-dialog-lazy.tsx` 共享懒加载封装，`history/library/movies/actors` 统一使用。

---

## P6 · 可复用 / 可抽取

### 前端
- [ ] 卡片封面浮层（评分角标 + 渐变标题）：`discover/movie-card.tsx:34-47` ≈ `library/movie-card.tsx:58-67` → `MovieRatingPill` / `MovieCoverCaption`
- [ ] 统计条：`movie-detail/hero.tsx:77-90`、`actors/detail-page.tsx:55-68`、`movie-detail/skeleton.tsx:21` → `StatList`
- [ ] 横向列表项宽度：`continue-watching.tsx:33`、`recent-shelf.tsx:14`、`top-rated-shelf.tsx:14` → `MovieShelfItem`；recent/top-rated 可合并
- [ ] 演员头像 URL：`app-search.tsx:59-62`、`actors/catalog-page.tsx:16-19`、`actors/detail-page.tsx:27-30` → 并入 `imageURL`
- [ ] `scraping-section.tsx:41-84` ≈ `:91-134` → `SourceToggleList`
- [ ] `subscription-section.tsx:93-151` 三段 Select → `PreferenceSelect`
- [ ] `network-section.tsx:126-141` ≈ `emby-section.tsx:253-268` 按钮组 → `SettingsActions`
- [ ] `media-sync-section.tsx:15-18` ≈ `tasks-section.tsx:11-14` → `useActiveScan()`
- [ ] 分类过滤：`discover/category-content.tsx:20-24` ≈ `category-filters.tsx:29-35` → `useCategoryTaxonomy`
- [ ] `discover/results.tsx` + `browse-results.tsx` 合并
- [ ] `movie-detail/content.tsx:62-108` 两分支后半段 → `CatalogueSections`
- [ ] `movie-detail/magnets.tsx:231-239` ≈ `:262-269` 按钮内容
- [ ] `recommendations.tsx:29-48`、`history/page.tsx:120-132` 伪造整条 movie → 卡片 props 收窄为 `Pick<>`
- [ ] `subscription-card.tsx:20-26,129-133` 两张状态表合并
- [ ] `task-notification-diff.ts:78-118` scan / batch 循环 → `shouldNotify`
- [ ] 4 个路由重复 errorComponent（`discover/history/index/subscriptions`）→ `router.tsx` `defaultErrorComponent`
- [ ] `custom-quality-menu.tsx` 与 `quality-menu.tsx` 约 70 行 class 重复

### 后端
- [ ] HTML DOM helper 四份：`actors/html.go:11-86`、`metadata/providers/html.go:13-102`、`javdb/web.go:343-408`、`javbus/parser.go:269`（含 `absoluteURL`/`absolute`）→ 共享 `htmlx` 包
- [ ] Cloudflare 挑战检测四份且标志串不一致：`javbus/parser.go:60-64`、`sukebei/client.go:181-185`、`freejavbt/client.go:134`、`javdb/web.go:87` → `netx.IsCloudflareChallenge`
- [ ] 代理变更监听三份：`javbus/client.go:321-343`、`sukebei/client.go:88-106`、`javdb/client.go:184-203` → `ProxiedFingerprintClient.Watch`
- [ ] HTTP 模板：`freejavbt/client.go:105-138` ≈ `metadata/providers/http.go:71-125`
- [ ] Pondo 系 provider：`onepondo.go`、`tenmusume.go`、`muramura.go` → 表驱动 `pondoSite`
- [ ] 115 离线状态码魔法数字 `0/1/2/-1`：`offline/sync.go:172`、`submit.go:31`、`control.go:233`、`recovery.go` → `pan` 包具名常量
- [ ] handler 分页解析：`api/movie.go:84-91`、`api/actors.go:48-57` `queryInt` → `bindQuery`；`api/task.go:32,56`、`api/pan.go:51` → `accepted`/`respond`
- [ ] `tasks/pause.go:18-49` → `database.LoadSetting/SaveSetting`
- [ ] `pan/client.go:135` `parseRetryAfter` → `domain.ParseRetryAfter`
- [ ] `drive/accounts.go:171,190` `sort.*` → `slices.SortFunc` / `slices.Sorted(maps.Keys())`

---

## 待确认（需人工判断，暂不处理）

- [ ] `api/rate_limiter.go:71`：记录满 4096 后拒绝所有新 IP，是否可接受。
- [ ] `netx/ssrf.go:122-124`：启用代理后跳过 SSRF 检查，是否符合威胁模型。
- [ ] `api/auth.go:369`：未校验来源即采信 `X-Forwarded-Proto` 决定 cookie `Secure`。
- [ ] `task-events.tsx`：后端 revision 是否在重启后归零（决定 P1 修复方式）。
- [ ] Go 空切片序列化为 `null`：前端 `?? []`（`metadata.tsx:18-19`、`recommendations.tsx:12`、`previews.tsx:12`）是否必要。
- [ ] `api/actors.ts:70`：演员 key 未 `encodeURIComponent`。
- [ ] `apiDelete<null>` 遇 204 空响应是否会在 `response.json()` 抛错（`subscriptions.ts:157`、`pan.ts:165,177`）。
- [ ] `stores/theme.ts:62-65`：`disable-transitions` 在 rAF 中移除可能无效（无强制回流）。
- [ ] `metadata/providers/javbus.go:30`：`proxy == nil` 时 `Media()` 可能 panic。
- [ ] `sources-section.tsx:94-99`：`crypto.randomUUID` 兜底是否因局域网 HTTP 部署而必要。

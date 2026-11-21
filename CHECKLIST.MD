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
- [x] 🟢 与默认值重复的 class / props：已清理 `Input h-8`（filter-bar/catalog-page）、`Card overflow-hidden`（movie-card/movie-grid-skeleton）、`tracking-normal`（recommendations/previews/magnets）、`main.tsx delay={0}`、`credentials: 'same-origin'`（task-events；`api/client` 保留——有测试断言 `credentials === 'same-origin'`）、`task-toast dismissible/icon`、`api/pan staleTime: 30_000`。（Button 内显式 `size-4` 与 `type="button"` 保留：前者有 Button 的 `[&_svg]` 规则兜底、后者涉及表单默认行为，未盲删。）
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
- [x] 卡片封面浮层（评分角标 + 渐变标题）：抽出 `components/movie/movie-cover-overlay.tsx` 的 `MovieRatingPill` / `MovieCoverCaption`，`discover/movie-card` 与 `library/movie-card` 复用；同时移除探索卡已废弃的订阅分支（`allowSubscribe`/`subscribable`/`MovieSubscribeButton`）——`features/discover/movie-subscribe-button.tsx` 已无引用，已移出项目到 `backups/removed-20261121-1350/`（可恢复）。
- [x] 统计条：抽出 `components/stat-list.tsx` 的 `StatList` / `StatListSkeleton`，`movie-detail/hero`、`actors/detail-page`、`movie-detail/skeleton` 均复用（value 类名统一为 `…whitespace-nowrap tabular-nums sm:text-base`）。
- [x] 横向列表项宽度：抽 `features/library/movie-shelf-section.tsx` 的 `MovieShelfItem` + `MovieShelfSection`；`continue-watching`/`recent-shelf`/`top-rated-shelf` 复用。`recent`/`top-rated` 的展示逻辑已由 `MovieShelfSection` 合并，二者仅剩「取 hook + 渲染」薄封装；`continue-watching` 同时去掉了伪造整条 `LibraryMovie`。
- [x] 演员头像 URL：三处重复 helper 删除，调用处直接 `avatar ? imageURL(avatar) : undefined`（`imageURL` 已内置 `/api/actors/` 直通，无需再判）。
- [x] `scraping-section.tsx`：抽出 `SourceToggleList`（sources/names/descriptions + pending/error/retry + onToggle），`ScrapingSection` 与 `ActorSourcesSection` 仅剩取 hook + 传参。
- [x] `subscription-section.tsx` 三段 Select → 抽出 `PreferenceSelect({ value, options, disabled, onChange })`，字幕/高清/无码各自只传项与字段。
- [x] `settings/shared.tsx` 新增 `SettingsActions`（右对齐按钮行容器），`network-section` 与 `emby-section` 的保存/测试按钮组复用。
- [x] 新增 `features/tasks/use-active-scan.ts` 的 `useActiveScan()`（tasks + scans + active），`media-sync-section`、`tasks-section` 复用。
- [x] 分类过滤：`MAIN_CATEGORY`/`UNSUPPORTED_CATEGORIES` 移入 `discover/constants.ts`，新增 `discover/use-category-taxonomy.ts` 的 `useCategoryTaxonomy(zone, categoryID?)`，`category-content`、`category-filters` 复用。
- [x] `discover/results.tsx` + `browse-results.tsx` 合并为单个 `BrowseResults`（query + 展示），`results.tsx` 已移出项目到 `backups/removed-20261121-1430/`。
- [x] `movie-detail/content.tsx` 两分支后半段抽 `CatalogueSections`（容器 + InlineError + Hero + Plot + Previews，`children` 放各自追加段）。
- [x] `movie-detail/magnets.tsx` 提交按钮内容（pending/submitted 图标 + label）抽 `SubmitLabel`，单账号按钮与多账号下拉触发器复用。
- [x] `recommendations.tsx`、`history/page.tsx` 伪造整条 movie → 卡片 props 收窄：`LibraryMovieCard` 用 `LibraryMovieCardData`、`DiscoverMovieCard` 用 `DiscoverMovieCardData`，`referenceMovie` 只造所需字段，`HistoryCard` 适配器删除、`LibraryMovieActions` 收窄为 `Pick<LibraryMovie,'id'|'code'>`。
- [ ] `subscription-card.tsx:20-26,129-133` 两张状态表合并
- [x] `task-notification-diff.ts` 抽 `shouldNotify({ active, old, id, dismissed, announced, initialized, extra })`，scan / batch / offline 三处循环统一守卫。
- [x] 路由重复 errorComponent（实际 6 处：`discover/movies/actors.index/index/subscriptions/history`）→ 统一到 `router.tsx` 的 `defaultErrorComponent`（用 `error.message`，兜底「页面加载失败」），各路由已移除。
- [x] ~~`custom-quality-menu.tsx` 与 `quality-menu.tsx` 约 70 行 class 重复~~：`quality-menu.tsx` 不存在；相关 class 重复此前已清理，无需再抽常量。

### 后端
- [x] ~~HTML DOM helper 四份 → 共享 `htmlx` 包~~：**暂不处理（C，人工决定）**。真实为 3 套 helper（`actors/html.go`、`metadata/providers/html.go`、`javdb/web.go`）+ `absoluteURL`（`actors/avleague.go`），调用点近 ~400，且 `text()` 三者语义分叉（providers 跳过隐藏节点、javdb 不折叠空白），统一回归风险高、收益低；如需再抽走 B 方案（仅抽机械 helper，保留各自 `text`）。
- [x] Cloudflare 挑战检测：新增 `netx.IsCloudflareChallenge(body []byte)`（3 个标志），`javbus`/`sukebei` 薄封装转调，`freejavbt`/`javdb` 的 inline 判断改用之（顺带补上 `challenge-platform`，更稳健）。
- [x] 代理变更监听：新增 `netx.ProxiedFingerprintClient.Watch(stop, onSignal)`，`javbus`/`sukebei`/`javdb` 统一使用。**并按决定移除 javbus 的探活/可用性机制**（`Available/setAvailable/probe/runHealthLoop`），`Find` 不再因「不可用」跳过、失败按普通源交由 aggregator 标记 partial；`JavBusSource` 接口去掉 `Available()`，`catalogue` 不再据此判断。
- [x] ~~HTTP 模板：`freejavbt/client.go` ≈ `metadata/providers/http.go`~~：**暂不处理（C，人工决定）**。二者形似但策略差异大（providers 有 `HTTPError`+`Retry-After`→冷却与 `admit` 准入、Cookie、16MiB；freejavbt 为 `KindUpstream` 直接报错、另有 Cloudflare 检测），合并需把错误/冷却/头/CF/限额全参数化，回归风险高、收益低。
- [x] Pondo 系 provider：采用 B 方案——保留三个具体类型（及测试），抽出共享 `fetchPondoJSON(ctx, client, base, code, providerID, siteID, stripPrefix)`，`onepondo`/`tenmusume`/`muramura` 的 `Fetch` 各自仅传站点参数；解析仍复用 `parsePondoMovieJSON`。
- [x] 115 离线状态码魔法数字 `0/1/2/-1`：`pan` 包新增 `OfflineStatusWaiting/Running/Done/Failed`，`offline` 的 `sync/submit/control/recovery/projection` 非测试代码全部改用常量（测试保留字面量）。
- [x] handler 分页解析：`api/movie.go`(libraryBrowse) 与 `api/actors.go`(actorList) 改用 `bindQuery`（`form:"page,default=1"` 等），删除 `queryInt`；`api/task.go` 两个 handler 改用 `accepted`。（`api/pan.go:51` 原本已是 `respond`，无需改。）
- [x] `tasks/pause.go`：`LibraryPaused` 改用 `database.LoadSetting[bool]`、`SetLibraryPaused` 改用 `database.SaveSetting`（移除手写 query/upsert 与 `encoding/json`/`setting` 导入）。
- [x] `pan` 的本地 `parseRetryAfter` 删除，`client.go`/`metadata.go` 三处改用 `domain.ParseRetryAfter(header, time.Now())`（移除 `strconv`/`strings` 导入）。
- [x] `drive/accounts.go`：`sort.Slice` → `slices.SortFunc`（`cmp.Compare`），「取 map 键 + `sort.Strings`」→ `slices.Sorted(maps.Keys(store))`；移除 `sort` 导入。

---

## R1 · 优化后复查（2026-11-21）

> 复查范围：`4e20f19..f7f49c4`（本轮优化提交）；状态已对照 `d96c790` 校正。
> 方法：5 个只读子代理分区审查 + 主线程抽查。✅ = 已人工对照代码复核；其余为子代理结论（带证据，未逐条复核）。
> 验证：`go build ./...`、`go vet`、相关包 `go test` 通过；前端未跑构建/测试。

### 🔴 高（回归 / 数据一致性）

- [x] ✅ `internal/api/error.go:111`：**自定义 404 文案被覆盖（本轮回归）**。基线仅在文案为「内部服务错误」时替换；现改为 `ent.IsNotFound(err)`，其 `errors.As` 会穿透 `domain.E` 包装，`library/detail.go:118`「影片不在当前媒体库中」、`strm/relay.go:139`「视频索引不存在…」均被改写为「资源不存在」。改法：条件改为 `domain.KindOf(err) == domain.KindUnexpected && ent.IsNotFound(err)`，文案直接写字面量。（已修复：仅 `KindUnexpected + ent.IsNotFound` 时用「资源不存在」，domain 分类的 NotFound 保留自定义文案；新增两条回归测试）
- [x] ✅ ~~`internal/ent/schema/actor.go:33`：**`name_key` 无存量回填**~~ **按决定不处理**：项目明确不兼容老数据库，老库需重建/重刮（与 `rating10` 不回填为同一约定）；新库由刮削写入路径生成 `name_key`，无需启动回填。
- [x] ✅ `internal/drive/accounts.go:228`、`:298`：**mount 内存缓存可被旧快照覆盖**。`Accounts()`/`refreshExpiringAccounts` 不持 `accountsMu` 即 `loadAccounts`，随后 `syncAccountMounts(store)` 整表替换；与 `saveAccountMount` 并发时会回滚新 mount。改法：load+sync 放进 `accountsMu`，或去掉周期 sync 只由写路径维护。（已修复：抽出 `loadAccountsAndMounts`，在 `accountsMu` 内完成 load+sync，两处调用改用之；`go build`/`go test ./internal/drive/` 通过）
- [x] ✅ `internal/actors/service.go:1076-1114`：**`reconcileActorProfiles` 出错仍 `tx.Commit()`**，部分提交，事务形同虚设；循环内逐 profile 查别名（N+1）+ 逐条 `Create`。改法：出错立即 `return err` 回滚；别名一次 `HasActorWith(IDIn(...))` 查全 + `CreateBulk`。（已修复：事务内任一错误立即 `return err` 由 defer 回滚；别名改为一次 `WithActor` 批量查询 + `CreateBulk`；`go build`/`go test ./internal/actors/` 通过）
- [x] ✅ `web/src/features/movies/filter-bar.tsx:44-53`、`actors/catalog-page.tsx:31-38`：**搜索框吞末尾空格（本轮回归）**。防抖后 trim 写入 URL，渲染期同步又把 trim 值写回输入框；输入 "abc " 停顿 300ms 空格消失。改法：仅当 `value.q !== query.trim()` 时回写。（已修复：URL 值与输入 trim 相同（自身回显）时只更新 `syncedQ`、不回写 `query`；`tsc`/`oxlint`/`oxfmt`/`vitest` 通过）
- [x] ✅ `web/src/features/discover/page.tsx:48`：**切 Tab 短暂显示上一个 Tab 的结果（本轮回归）**。P4 删除了 `<BrowseResults key={view}>`，但 `useDiscoverMovies` 带 `keepPreviousData`（`api/discover.ts:117`）。改法：恢复 `key={view}` 或 `isPlaceholderData` 时显示骨架。（已修复：恢复 `key={view}`，切换 released/upcoming 时重新挂载、显示加载骨架）

### 🟠 中

后端
- [x] ~~`magnet/aggregator.go`：JavBus 移除探活后不可达时拖累结果~~ **评估后暂不处理（见 D6）**。（`613fcad` 已部分修复：CF/driver-verify 改返回 `domain.Blocked`，聚合按「无结果」处理；连接失败/超时仍计为 failure。是否把连接错误也降级为「无结果」需产品决定，且更合理的方案是 D6 的聚合层熔断/冷却，避免掩盖真实网络故障。）
- [x] ✅ `netx/cloudflare.go`：`IsCloudflareChallenge` 的 `challenge-platform` 特征可能误判正常页面。（`d96c790` 新增 `IsCloudflareInterstitial`，仅 freejavbt/projectjav 改用；`javdb/web.go:88`、`sukebei/client.go:171`、`javbus/parser.go:62` 仍用旧函数，**两套判定并存**。）改法：评估后统一为一个函数，删除另一套。（已修复：删除 `IsCloudflareChallenge`，`javdb`/`sukebei`/`javbus` 全部改用 `IsCloudflareInterstitial`，仅保留单一判定；同步更新 javbus 测试用例）
- [x] ✅ `offline/add.go:84`：`defer service.warmMetadata(code)` 在报错返回路径也执行，白白刮削。改法：仅在提交成功后显式调用；`SetMetadata`（`app.go`）可改为 `offline.New` 构造参数。（已修复：移除 `defer`，改为 `recordSubmission` 成功后再预热）
- [x] ~~`offline/sync.go:166`：`updateTask` 每条任务仍调用 `currentMountFor`→`drive.Mount`；本轮只把 prefs 提到循环外~~ **评估后不处理**：`drive.Mount` 已改为内存缓存（P1 修复后为 O(1)，非读库），且完成时需重新读取挂载以检测「轮询期间挂载被替换」（`TestOfflineSyncRespectsSourceChangesDuringRemotePolling` 依赖此语义）；改成循环外算一次会破坏该正确性。
- [x] ~~`drive/accounts.go:188,197`：`accountRecord`/`Mount` 改读内存后 error 恒为 nil、ctx 未用，约 10 处调用方多写无效 `if err != nil`~~ **评估后保留**：`Mount` 是跨包接口（`source.MountProvider` 等）且调用点 10+；保留 error 返回便于未来缓存未命中/回源读库，删除收益低、改动面大。
- [x] `actors/service.go:378` 与 `:776`：`loadGroup`/`findNames` 按 `MatchKeyEQ(normalizeName(seed.Name))` 反查 profile 逻辑重复，后者吞 `perr/lerr`。改法：seed 查询 `WithProfile` 带出外键，抽同一 helper 并返回错误。（已修复：新增 `linkedProfileID` 统一反查并返回错误；`loadGroup`/`findNames` 复用，`findNames` 不再吞错）
- [x] `ent/schema/watch_history.go:26`：`settings` 用 `json.RawMessage` + `playback/history.go:48` 手写编解码，读改写无事务。改法：`field.JSON("settings", &domain.PlaybackSettings{})`。（已修复：schema 改为 typed JSON；`domain.PlaybackSettings` 实现 `driver.Valuer`/`sql.Scanner`；`Resume`/`existingSettings` 不再手写编解码；`SaveProgress` 的“读旧值+合并+upsert”放入同一事务。`go build`/`go test ./internal/playback ./internal/api ./internal/database ./internal/library` 通过）

前端
- [x] `features/movies/page.tsx:28,41`：`moviesFilterFromSearch(search)` 每次渲染新建对象，`useCallback([filters])` 失效、筛选栏防抖计时器被重置。改法：`useMemo(..., [search])`。（已修复：`filters` 改用 `useMemo([search])`）
- [x] `api/library.ts:94-99,112-121`：`useRecentMovies` 与 `useLibraryMovies(1)` 共用 `libraryKeys.movies(1)` 却各写一份 queryFn；内联 `select` 每次渲染重算。改法：抽 `libraryMoviesOptions(page)`（`queryOptions`），select 提为模块级。（已修复：抽出 `libraryMoviesOptions(page)`；`selectRecentMovies` 提为模块级，`useRecentMovies()` 复用同一 query 选项）
- [x] `settings/network-section.tsx:72`：关闭代理时保存 `normalizeProxyInput(form.url)`（未保存的输入），与 emby 的 `{ ...config, enabled: false }` 不一致。（已修复：关闭时改为保存 `{ ...config, enabled: false }`，保留已持久化的 URL）
- [x] `settings/scraping-section.tsx:48-68`：`SourceToggleList` 9 个参数，两调用处 pending/error/retry/toggle 仍逐字重复。改法：传 `{ query, update, names, descriptions, errorText }`，内部处理乐观值/重试/切换。（已修复：`SourceToggleList` 改为接收 `query/update/names/descriptions/errorText`，内部处理乐观值、重试与切换，两调用处只传 hook）
- [x] `tasks/task-notification-diff.ts:61-80`：`shouldNotify` 7 参数，`dismissed/announced/initialized` 三处原样透传，指纹比较重复三遍。改法：`diffTaskNotifications` 内闭包 `notify(id, active, old, fp, extra?)`。（已修复：删除模块级 `shouldNotify`，改为 `diffTaskNotifications` 内闭包 `shouldNotify(id, active, old, extra?)`，三处调用收敛；`vitest` 通过）
- [x] settings 各 section 仍不统一：加载态（emby/network 渲染 `null`、scraping/tasks `Skeleton`、subscription 渲染禁用控件）、错误态（仅部分有 `onRetry`）、按钮尺寸（`sm` 与默认混用）。（已统一：emby/network/subscription 加载态改 `Skeleton`、错误态统一 `InlineError onRetry`；media-sync 同步按钮改 `size="sm"`；与 tasks/scraping/magnet-sources/data 现有范式一致）

### 🟢 低

组件重复 / 复用
- [x] `library/player-dialog.tsx:465-486` `MovieStats` 手写一份 `StatList`（缺 `sm:text-base`）；评分格式化与 `movie-detail/hero.tsx` 重复。（已复用 `StatList`；评分格式化文案仍各写一处，属可接受的小重复）
- [x] `library/player-dialog.tsx:79` 用 JS `useScrollFade`，`movie-detail/dialog.tsx:34` 用 shadcn `scroll-fade` 工具类，二选一。（已统一为 `useScrollFade`：`movie-detail/dialog.tsx`、`app-sidebar.tsx` 均改用 hook，与 `player-dialog`/`app-search` 一致；移除 `scroll-fade`/`scroll-fade-y` 类）
- [x] `history/page.tsx:59-76` 与 `library/continue-watching.tsx:15-29` 「历史条目→卡片」重复且进度算法不一致（前者 clamp+round）。改法：抽 `historyToCard(item)`。（已修复：新增 `features/library/history-card.ts` 的 `historyToCard`，两处复用；进度统一为 clamp 0-100 + round；`tsc`/`oxlint`/`oxfmt`/`vitest` 通过）
- [x] `components/list-pagination.tsx:36-46` 重写了 `PaginationLink`（后者已 0 调用）；`data-active` 无样式使用。（已修复：`ui/pagination.tsx` 的 `PaginationLink` 改为与 `PaginationPrevious/Next` 一致的按钮形态（接收 Button props + `isActive`），去掉无用的 `data-active` 与 `<a>`/`nativeButton` 渲染；`ListPagination` 页码改用 `PaginationLink`；`tsc`/`oxlint`/`oxfmt`/`vitest` 通过）
- [x] `components/stat-list.tsx:13-16,37` `StatListSkeleton` 复制容器 class。（已修复：容器与单项 class 提为模块常量 `listClass`/`itemClass`，`StatList` 与 `StatListSkeleton` 共用；`tsc`/`oxlint`/`oxfmt` 通过）
- [x] `library/page.tsx:22,26` 仍手写 `filter(isScanTask)` + `find(isTaskActive)`，未用 `useActiveScan`。（已修复：改用 `useActiveScan()` 取 `tasks/scans/active`，`active` 即进度任务；`latestScan` 基于 `scans` 查找，语义不变；`tsc`/`oxlint`/`oxfmt`/`vitest` 通过）
- [x] `ensureProfileTx`（`actors/service.go:1135`）≈ `ensureProfile`；`domain.ActorNameKey` 与 `actors.Key` 重复 sha1。（已修复：合并为单个包级 `ensureProfile(ctx, db *ent.Client, ...)`，事务内传 `tx.Client()`；删除 `actors.Key`，URL key 统一用 `domain.ActorNameKey`（归一化幂等，结果不变，且与 `name_key` 列同源）；同步测试；`go build`/`go vet`/`go test ./internal/actors/... ./internal/api/...` 通过）

样式 / shadcn
- [x] `components/movie/movie-cover-overlay.tsx:18` `bg-gradient-to-t` 为 v3 名（v4 `bg-linear-to-t`）；discover/library 卡片副标题样式不一（`text-[11px] text-white/60` vs `text-xs text-white/75`），`MovieCoverCaption` 可接收 `subtitle` 统一。（已修复：`bg-linear-to-t` 已是 v4 写法；`MovieCoverCaption` 新增 `subtitle`，统一为 `mt-0.5 truncate text-xs text-white/75`，discover 发行日期与 library 演员均改传 `subtitle`）
- [x] `settings/shared.tsx:10` 手写 `@container/field-group`，应用 `FieldGroup`；`:52` 视口断点 `sm:gap-6` 与容器查询混用；`:56` 覆盖 `FieldDescription` 默认样式。（已修复：`SettingsCard` 改用 `FieldGroup`（自带 `@container/field-group`，`gap-8` 保持原间距）；`sm:gap-6` 改为容器查询 `@md/field-group:gap-6`；`FieldDescription` 去掉 `text-xs leading-5` 覆盖、仅保留 `wrap-break-word`，描述字号随 shadcn 默认变为 `text-sm`）
- [x] `components/app-search.tsx:126-133` 外层 `span` 包 `MediaImage` 的 `div`（非法嵌套）、`bg-muted` 重复；`image` 为空分支可交给 `MediaImage` 占位。（已修复：结果行与缩略图容器 `span`→`div`，去掉重复 `bg-muted`；无图分支交给 `MediaImage` 自带占位，移除 `FilmIcon`）
- [x] `settings/emby-section.tsx:240`、`network-section.tsx:132` 常驻 `LoaderCircleIcon invisible` 占位，与全站 `Spinner` 不一致。（已修复：两处改为 `Spinner`，保存中显示、空闲时 `invisible` 占位以避免按钮宽度跳动）
- [x] `settings/scraping-section.tsx:79-84` 单个 `Switch` 外多余 `div`。（已随 `SourceToggleList` 重构移除多余包裹）
- [x] 待确认：`components/page-back-button.tsx` 改 `size="lg"` 后由 `h-11 text-base` 变为 `h-9 text-sm`，是否有意。（已决定恢复大尺寸：还原为基线的 `h-11 gap-2 px-3 text-base`）
- [x] 待确认：`videojs/ui/container.tsx:22` 删除 `focus-visible:outline-media-ring` 改 `outline-none`，键盘用户无焦点提示；可改为 `focus({ preventScroll: true, focusVisible: false })`。（已决定：播放器容器保持不显示焦点框，维持现状）

死代码 / 过度防御
- [x] `player-dialog.tsx:483`、`hero.tsx:76` `stats.length === 0` 恒假（评分恒 push）。（已删除两处恒假守卫）
- [x] `player-dialog.tsx:49` `movieId === null` 多余（调用方均经 `LazyLibraryPlayerDialog` 过滤）。（已删除，`LibraryPlayerDialog` 的 `movieId` 改为 `number`）
- [x] `discover/browse-results.tsx:27` `!movies.data` 多余。（已改为 `movies.isPending`）
- [x] `discover/category-filters.tsx:104`、`common-filter-select.tsx:31` placeholder 永不显示（value 恒有 `'all'`）。（已移除两处 placeholder）
- [x] `emby-section.tsx:129` `if (form.enabled)` 恒真；`network-section.tsx:99` 不可达分支；`:121-122` `form.enabled &&`、`type="text"` 多余。（已清理：emby 去掉恒真判断；network 去掉不可达分支、冗余 autoFocus 条件与 `type="text"`）
- [x] `sources-section.tsx:185,188,200,208` 确认后先关弹窗，`save.isPending` 守卫不可达；`:392-397` 重新拼回 `option` 对象。（已修复：删除确认框去掉 4 处不可达的 `save.isPending` 守卫（编辑弹窗保存中删除按钮已禁用，确认后先关框再删）；删除重新拼出的 `option` 对象，直接用 `KIND_LABELS[kind]`/`LOCAL_PATH`，路径字段写 `path`）
- [x] `tasks/task-events.tsx:39,127-130` `instanceRef` 多余（instance 已随 revisions 存入 ref）；`task-toast.tsx:61-83` `active` 恒 false、`description: undefined` 多余。（已修复：删除 `instanceRef`，改为 `stored?.instance === next.instance ? stored : undefined` 判定重启；`task-toast` 删除恒 false 的 `active`。**更正**：`description: undefined` 并非冗余——sonner 对同 id toast 做属性合并，它用于清掉重试前失败 toast 的描述，保留并补注释）
- [x] `components/ui/progress.tsx:19,38` `percent` 变量仅剩判空用途；`app-shell.tsx:64` `const current = theme` 无意义别名。（已改用 `value == null` 判定并删除别名）
- [x] `tasks/pause.go:21-25` `!found` 分支冗余；`javbus/client.go:257` `fingerprint == nil` 不可能；`api/router.go:50` TrustedProxies 回退已被 `config.validate` 覆盖。（已处理：`pause.go` 的 `!found` 分支、`javbus` 的 `fingerprint == nil` 此前已移除。**更正**：`router.go` 的回退并不冗余——gin `prepareTrustedCIDRs` 出错时返回已解析的部分列表并设为可信代理，`SetTrustedProxies(nil)` 用于避免半截配置生效，保留不改）
- [x] `subscription-section.tsx:49` `items={[...options]}` 每次渲染复制；`:51` `next ?? ''` 写空串；三处 `as MagnetPreferences[...]`。改法：`PreferenceSelect<T extends string>` 泛型。（已修复：`PreferenceSelect<T extends string>` 泛型；`items` 直接传只读数组不再复制；`null` 时不触发 `onChange`；三处 `as MagnetPreferences[...]` 移除）

薄封装 / 过度设计
- [ ] `library/recent-shelf.tsx`、`top-rated-shelf.tsx` 各两行、唯一调用方，可内联用 `MovieShelfSection`（可选）。
- [ ] `offline/sync.go:150` 导出 `UpdateTask`、`providers/onepondo.go:46` `parseOnePondoJSON` 仅测试使用。
- [ ] `providers/tenmusume.go:41` `fetchPondoJSON` 7 参数，`stripPrefix` 仅 1pondo 用。改法：调用方传 movieID 或 `pondoSite{base, provider, siteID}`。
- [ ] `actors` `persistImages` 已持 `persistMu`，为单条 `CreateBulk` 另开事务多余。
- [ ] `app.go:172-178` cleanup 栈与 `metadata.Close`/`catalogue.Close` 重复关闭（幂等无 bug，所有权不清）。

其他
- [ ] `actors/detail-page.tsx:142` 改用 `MediaImage` 后丢失 `alt`；`MediaImage` 需透传 `alt`。
- [ ] `routes/actors.index.tsx:7-10` `validateSearch` 写在路由文件，与 `features/*/search.ts` 惯例不一致。
- [ ] `settings/pan-login-dialog.tsx:27,55`、`pan-directory-dialog.tsx:25,31` `children as ReactElement` 断言，props 直接声明 `ReactElement`。
- [ ] 待确认：`player-dialog.tsx:108,120` 保存的清晰度已不在列表时，每次打开都等列表请求，且旧值永不清除。
- [ ] 待确认：`api/discover.go:124` `c.Error(recErr)` 后仍 200，能否被记录取决于 sloggin；`source/source.go:140` 任一账号 Mount 出错使 `LibrarySources` 整体失败，启动扫描全部跳过，是否本意。

---

## D · 设计优化（2026-11-21）

> 建议顺序：D1、D2 优先（R1 高优先级问题的根因）→ D4、D5、D8（改动小、收益明确）→ D3 单独评估立项。

### 系统设计

- [ ] **D1 实体数据存在 KV 设置表中**：115 账号整份 map 存于 `pan.accounts`（`drive/accounts.go:17,32`），存储源整份 `[]Source` 存于设置（`source/source.go:74`）。改一个账号需全局 `accountsMu` 读改写，再叠加内存缓存同步，R1 高「mount 缓存被旧快照覆盖」即源于此。改法：改为 ent 实体 `PanAccount` / `StorageSource`，行级更新，去掉全局锁并简化缓存。
- [ ] **D2 派生列缺统一的存量回填机制**：`rating10` 用 hook、`name_key` 在 `scrape/mapping.go:128` 写入、`canonical_code` 用 hook，均无回填，老库需重刮（R1 高「name_key 无存量回填」）。改法：`database.Open` 后执行极简「数据版本号 + 幂等回填函数列表」；`name_key` 改由 hook 维护。
- [ ] **D3 演员分组键为派生值而非实体**：分组键 =「profile_id 或归一化名」，导致 Catalog 无法 SQL 分页（P1 第 49 条），并衍生 `reconcileActorProfiles`/`EnsureMovieProfiles`/`EnsureProfiles`/`loadGroup`/`findNames` 多套对账与反查（R1 高 #4、中「loadGroup/findNames 重复」）。改法：演员入库即挂 profile（无匹配则新建），分组键统一为 `profile_id`，Catalog 以 `ActorProfile` 为主表分页；对账逻辑与 `name_key` 随之收敛。改动面最大，需单独评估。
- [ ] **D4 组装根 `app/app.go` 简化**：
  - 17 个 provider 手动 new、单独登记 Close，又按另一份顺序传入 `metadata.New`，两份列表不一致（R18/OnePondo/CodeAV/JavDB/JavBus 不在 cleanup 中）。改法：provider 列表单一来源，由 `metadata.Service` 统一持有与关闭。
  - 5 处 setter 注入：`SetMetadata`、`SetMountProvider` 仅是构造顺序问题，调换顺序改构造参数；`SetScanHandler`/`SetLocalScanHandler`（source↔library 互相回调）可改由 API 层编排（保存源后直接调 library 入队）。
  - `New()` 含副作用：启动账号刷新 goroutine、入队全部扫描、STRM 重写（`app.go:93,270-295`）。改法：移到 `Run()`，`New` 只做组装。
- [ ] **D5 三套「来源开关」实现**：`metadata.sources`（`metadata/service.go:60`）、演员来源（`actors/service.go:113`）、`magnet.sources`（`catalogue/magnet_settings.go:64`）各自一套类型/读取/排序/API；前端 `settings/magnet-sources-section.tsx` 复制了 `SourceToggleList` 的逻辑。改法：后端抽通用开关列表 helper，前端统一一个组件。
- [ ] **D6 上游源健康检测**：`613fcad` 仅把挑战页映射为 `domain.Blocked`，连接失败/超时仍每次等满超时。改法：在磁力聚合层加所有源共用的熔断（连续失败 N 次 → 冷却 T 分钟），可参考 providers 的 `admit` 冷却；不恢复 javbus 单源探活。

### 功能设计

- [ ] **D7 每次启动全量入队扫描（待确认是否有意）**：`app.go:270-292` 启动即为全部云盘与本地源入队扫描，账号多、库大时启动即大量调用 115 API。改法：若非有意，改为仅补扫未完成任务或交由定时任务。
- [ ] **D8 搜索框 ↔ URL 同步逻辑两份**：`movies/filter-bar.tsx:44-53` 与 `actors/catalog-page.tsx:31-38` 各写一份「防抖 + 渲染期回写」，并共有吞末尾空格 bug（R1 高）。改法：抽 `useDebouncedSearch` hook，一处修复。
- [ ] **D9 settings 各 section 状态处理约定**：加载/错误/保存各自实现（R1 中「settings 各 section 仍不统一」）。改法：统一约定「加载 → Skeleton、错误 → `InlineError onRetry`、单一按钮尺寸」，不新增抽象，避免过度设计。

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

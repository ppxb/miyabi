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
- [ ] 🟢 `internal/netx/ssrf.go:187,198`：前置 `ValidateSafeURL` 与拨号时重复 DNS 解析。
- [ ] 🟢 `internal/ent/schema/task.go:43`：索引 `(type,status)` 是 `(type,status,retry_at)` 的前缀，冗余。

### 前端
- [ ] 🟠 ✅ `web/src/features/tasks/task-events.tsx:45,138`：`revisions` 为 effect 局部变量且 onerror 清空，每次重连全量刷新四类查询。改法：`useRef` 跨重连保留（⚠️ 先确认后端 revision 重启是否归零）。
- [ ] 🟠 `web/src/features/library/page.tsx:22` + `api/library.ts:109-115`：同接口一页请求两次。改法：共用 `libraryKeys.movies(1)` + `select` 截取。
- [ ] 🟠 `web/src/api/subscriptions.ts:65-71`：内联 `select` 每次渲染重建 Map；订阅页 `'all'` 与发现页 `'movie'` 双缓存。改法：模块级 select，订阅页用 `'movie'`。
- [ ] 🟠 `web/src/features/library/player-dialog.tsx:140-189`：订阅 `currentTime` 导致播放中持续重渲染；保存进度逻辑重复两次。
- [ ] 🟢 `web/src/api/history.ts:34-40`：缺 `keepPreviousData`，翻页闪骨架。
- [ ] 🟢 `web/src/features/settings/scraping-section.tsx:59-66`：开关无乐观更新且保存期间整列禁用。改法：`isPending ? variables : data`。

---

## P2 · 死代码 / 冗余（移至回收站，勿 rm）

### 整文件
- [ ] ✅ `web/src/features/movie-detail/recommendation-visibility.ts`（同步修改 `web/tests/movie-detail.test.ts`）
- [ ] `web/src/features/settings/pan-account.tsx`（或在 `PanSourceCard` 中复用它）
- [ ] `web/src/features/library/movie-status.tsx`
- [ ] `web/src/components/videojs/ui/airplay-button.tsx`、`cast-button.tsx`、`pip-button.tsx`、`quality-menu.tsx`
- [ ] `web/src/components/videojs/styles/audio/base.css`（⚠️ `audio/theme.css` 有引用，保留）
- [ ] `internal/metadata/providers/javdatabase.go`（未注册；或在 `app.go` 注册）

### 函数 / 字段 / 常量
- [ ] ✅ `web/src/api/discover.ts:181-199` `useRecommendationMovie`；`api/movie-detail-cache.ts:48-177` `subscribeMovieCard`/`createMovieDetailLoader`；`useDiscoverMovie` 中 `prioritize` effect；`discoverKeys.search`
- [ ] `web/src/api/play.ts:8-13,40-69` `usePlayFiles`/`usePlayback`/`PlayFiles`；释放会话 effect 抽 `useReleasePlaySession`
- [ ] `web/src/api/pan.ts:67-68` `fileLists`、`panKeys.files`
- [ ] `web/src/features/tasks/task-notification-diff.ts:1-14` 重复的 `isScanTask/isTaskActive/isOfflineTaskActive`
- [ ] `web/src/features/tasks/task-toast.tsx:79-81` 不可达分支
- [ ] `web/src/features/tasks/task-progress-state.ts:6-13` 从未显示的 `stages[].label`
- [ ] `internal/offline/recovery.go:83-101` `Service.Recover`
- [ ] `internal/offline/service.go:18-20` 接口未用方法 → `catalogue.HasMagnet`（`magnets.go:67`）、`MovieCode`（`service.go:225`）；`monitor.OfflineAdder.Add` → `offline.Service.Add`
- [ ] `internal/catalogue/types.go:81` `LocalState.Source()`
- [ ] `internal/gfriends/client.go:31` `rawContentBase`、`:410-419` `FetchImage`；`:234-245` 改用 `escapeContentRel`
- [ ] `internal/metadata/service.go:244` `needsSupplement`；`:255-264` `attachSourceIdentity` 简化
- [ ] `internal/catalogue/service.go:40` `proxy`、`internal/database/sqlite.go:21` `db` 只写不读
- [ ] `internal/domain/movie.go:47` `SearchOptions`；`tasks/payload.go:35` `SetPayloadField`、`netx.ProxyManager.Update`（仅测试用）
- [ ] `internal/api/error.go:83` 不可达分支；`:112` 硬编码文案比较

### 未被传入的 props
- [ ] `components/movie/movie-card.tsx` `coverLoading`、`onCoverReady`；探索/库卡片多余的 `titleTooltip={false}`
- [ ] `components/movie/movie-cover.tsx` `loading`、`onReady`
- [ ] `components/media-image.tsx:13,43-45` `onReady` + effect
- [ ] `components/movie/movie-grid-skeleton.tsx:7` `compact`
- [ ] `components/page-header.tsx:9` `inlineActions`；`components/app-page.tsx:9` `contentClassName`
- [ ] `features/library/movie-card.tsx:11-15` `busy`、`playIcon`、`hideActors`（连带 `movie-actions` 的 `disabled`）
- [ ] `features/library/scan-button.tsx` `available`、`onStarted`
- [ ] `features/discover/results.tsx` `searching`（恒 false）、`errorMessage`
- [ ] `features/movie-detail/metadata.tsx:76` `EntityLink` 的 `kind`（且名不副实，渲染 span）
- [ ] `features/movie-detail/dialog-context.ts:11` `openMovie` 的 `_trigger`（含 `app-search.tsx:218` 的 `document.createElement('div')`）
- [ ] `api/subscriptions.ts:156` `useRemoveSubscription` 的 `kind`

---

## P3 · 过度防御 / 过度设计

- [ ] 🟠 `web/src/features/settings/emby-url.ts` + `emby-section.tsx:95-106`：后端单一 URL，前端拆 host/port 再拼。改法：单输入框。
- [ ] 🟠 `web/src/features/settings/emby-section.tsx:43-61`、`network-section.tsx:26-34`：表单草稿两套写法，含渲染期 `setState`。改法：数据就绪后挂载表单 + `key` 重置。
- [ ] 🟠 `web/src/stores/theme.ts:58-66`：DOM 副作用在被 3 处调用的 hook 中重复执行；登录页不同步系统主题。改法：单点 `ThemeSync`。
- [ ] 🟢 `web/src/stores/theme.ts:26-46`：SPA 中的 SSR 守卫。
- [ ] 🟠 `internal/api/discover.go:122-128`：匿名接口运行时断言 + 吞错。改法：加入 `CatalogueManager` 接口。
- [ ] 🟠 `internal/network/service.go:65-87`：单任务 goroutine + WaitGroup。
- [ ] 🟠 `internal/app/app.go:129-259`：手写出错清理链。改法：`cleanup []func()` + defer 逆序。
- [ ] 🟠 `web/src/api/library.ts:190-205`、`api/offline.ts:77-96,106-123`：条件 `setQueryData` 写三遍且紧接 invalidate。
- [ ] 🟢 `web/src/api/emby.ts:42-59`、`api/network.ts:36-53`：乐观更新 + 回滚 + 立即 invalidate，回滚冗余。
- [ ] 🟢 `web/src/api/tasks.ts:87,96`：直接 `toast.error(error.message)`，绕过全局 `meta.errorTitle` / `UNAUTHORIZED` 静默 / `describeApiError`。
- [ ] 🟢 `web/src/features/access-gate/access-gate.tsx:72`：用 `error.message` 而非 `describeApiError`。
- [ ] 🟢 `web/src/components/media-image.tsx:72-77`：`typeof image.decode` 检测多余；decode 失败误标 error。
- [ ] 🟢 `web/src/components/list-pagination.tsx:29`：`nextPage === page` 防御多余。
- [ ] 🟢 `web/src/components/app-shell.tsx:65-77`：主题类型断言 + `?? 'system'` 多余；暗色显示 Sun 图标语义不符。
- [ ] 🟢 前端多余空值判断：`app-search.tsx:73`、`library/movie-card.tsx:25`、`library/continue-watching.tsx:15`、`actors/detail-page.tsx:38-39,102`、`discover/movie-subscribe-button.tsx:20`、`player-dialog.tsx:21-25,48`、`subscriptions/page.tsx:100-104`
- [ ] 🟢 后端多余 nil 检查：`catalogue/state.go:27,48`、`catalogue/recommend.go:23`、`api/router.go:44`、`api/movie.go:207`、`app/app.go:392-415`、`drive/accounts.go:63,88,101`、`freejavbt/client.go:176,181,196`
- [ ] 🟢 `web/src/features/sources/sources-section.tsx:56-99`：`KIND_OPTIONS` 大半字段死数据 + `?? KIND_OPTIONS[0]!` 兜底。
- [ ] 🟢 `web/src/components/videojs/video/menus/settings-menu.tsx:19-32`：`qualityOptions.length > 0` 恒真、多余默认值。
- [ ] 🟢 `web/src/features/library/player-dialog.tsx:51-62`：轻量计算的 `useMemo`；`:125` 多余 `show115`；`:76-81` `skinStyle` 可用 className。
- [ ] 🟢 `internal/offline`：`config.go:22,24` 重复 `Normalized()`；`sync.go:36`、`recovery.go:97` 可直接 `return errors.Join(errs...)`；`add.go:135-145` 冗余 return。
- [ ] 🟢 `internal/monitor/subscribe.go:22/35`：`existingMovie` 重复查询；`:135` `ptr` 可用 `new(v)`（Go 1.26+）。

---

## P4 · shadcn / base-ui / Tailwind 规范

- [ ] 🟠 **`asChild` 与 `render` 混用**：`ui/button.tsx:47,61-66`、`ui/badge.tsx:39,52,56`、`ui/tooltip.tsx:13-36`、`ui/dialog.tsx:12-48`、dropdown-menu、breadcrumb 自行加入 Radix 式 `asChild`（外部 13 处 `asChild` / 26 处 `render`）。改法：统一 `render`，原语恢复上游 base-nova。
- [ ] 🟠 `ui/progress.tsx:35-41`：用 `flex-1 + translateX` 抵消 base-ui 自带 width。改法：`h-full transition-all`，删 style。
- [ ] 🟢 `ui/select.tsx:82`：删掉了上游进出场动画，与 Popover/Dropdown 不一致。
- [ ] 🟢 `ui/card.tsx:14,27,76`：外层 `rounded-2xl`，内部仍 `rounded-*-xl`。
- [ ] 🟢 `ui/pagination.tsx:92`：省略号 `size-9` 与页码 `size-8` 不齐。
- [ ] 🟠 `components/list-pagination.tsx:44-51`：页码为 `href="#"` 假链接、禁用仅视觉。改法：`Button disabled` 或 TanStack `Link search`。
- [ ] 🟠 `components/app-navigation.tsx:36-61`、`routes/settings.tsx:31-60`：手写激活态，重复 `Link` 的 `data-status` / `aria-current` 与 `SidebarMenuButton` 的 tooltip 逻辑。
- [ ] 🟠 `features/settings/shared.tsx:34-58`：`SettingRow` 未用 `Field`，标题不关联控件（无障碍）。
- [ ] 🟠 `features/movies/filter-multi-select.tsx:40-47`：手写搜索框，应用 `CommandInput`；`CommandList` 重复 `overflow-y-auto`。
- [ ] 🟢 用 `LoaderCircleIcon animate-spin` 而非 `Spinner`：`library/scan-button.tsx:61`、`subscriptions/page.tsx:276`、`subscriptions/subscription-card.tsx:84`、`tasks/task-toast.tsx:86,132,200`、`tasks/task-toast-actions.tsx:31`、`access-gate/access-gate.tsx:50,74`
- [ ] 🟢 手写进度条而非 `Progress`：`sources-section.tsx:331-348`
- [ ] 🟢 原生 `title` 而非 Tooltip：`tasks/task-toast-actions.tsx:27,49`
- [ ] 🟢 在 shadcn 组件上叠加样式：
  - `components/app-search.tsx:100-104` `variant="link"` 后全覆盖 → `variant="ghost"`
  - `components/app-search.tsx:251` 覆盖 CommandDialog 定位 + 重复 `overflow-hidden`
  - `components/page-back-button.tsx:14` `h-11 px-3 text-base` → `size="lg"`
  - `subscriptions/subscription-card.tsx:122` Checkbox 叠 `text-white` → `text-success-foreground`
  - `settings/directory-breadcrumbs.tsx:27` `gap-1 sm:gap-1`
- [ ] 🟢 与默认值重复的 class / props：
  - Button 内图标 `size-4`：`pan-login-dialog.tsx:157`、`pan-directory-dialog.tsx:93,104,116,118`、`data-section.tsx:63`、`movies/filter-bar.tsx:145`、`access-gate.tsx:74`、`back-top-button.tsx`
  - `<Button type="button">`（约 10 处：back-top-button、error-state、page-back-button、pagination 等）
  - Input `h-8`：`movies/filter-bar.tsx:69`、`actors/catalog-page.tsx:42`
  - Card `overflow-hidden`：`movie/movie-card.tsx:32`、`movie-grid-skeleton.tsx:23`
  - `tracking-normal`：`recommendations.tsx:17`、`previews.tsx:21`、`magnets.tsx:87`
  - `main.tsx:22` `delay={0}`
  - `api/client.ts:79`、`task-events.tsx:75` `credentials: 'same-origin'`
  - `task-toast.tsx` `dismissible: true`、`icon: undefined`、`description: undefined`
  - `api/pan.ts:115,129` `staleTime: 30_000`（同全局默认）
- [ ] 🟢 `components/movie/movie-card.tsx:38`：inline-flex Badge 上 `truncate` 无省略号 → 内包 `<span className="truncate">`。
- [ ] 🟢 `components/app-shell.tsx:23-24,44,54-58`：冗余包装层与不生效的 `transition-[width,height]`。
- [ ] 🟢 `styles/globals.css:264-274`：`.no-scrollbar` 与 shadcn `@utility no-scrollbar` 重复；`--warning`、`--chart-1..5` 未使用。
- [ ] 🟢 `back-top-button.tsx:16-24`：纯图标按钮缺 `aria-label`；`pan-login-dialog.tsx:141` `<img>` 缺 `alt`。
- [ ] 🟢 `movie-shelf.tsx:15,98`：`href?: string` 丢失路由类型 → `LinkProps['to']`。
- [ ] 🟢 `discover/page.tsx:49`：`<BrowseResults key={view}>` 多余 key。

---

## P5 · UI 风格统一

- [ ] 🟠 **settings 各 section 不一致**：加载（Skeleton / 文字 / 无）、错误（toast / InlineError）、按钮尺寸（`sm` / 默认）与位置（右上 / 右下）。约定：查询失败 InlineError、变更失败 toast、加载 Skeleton；抽 `SettingsActions`；`SettingsSection` 增加 `description` prop（替代 `scraping-section.tsx:47-49,97-99` 游离 `<p>`）。
- [ ] 🟠 **状态色对比度不足**：`styles/globals.css:174-180` `downloading`（白/cyan-400 ≈1.8:1）、`success`（白/emerald-500 ≈2.5:1）；`--color-info:20` 未走语义变量。
- [ ] 🟠 **筛选与分页状态未进 URL**：`features/movies/page.tsx:24-25`、`features/actors/catalog-page.tsx:22-24`（discover/history 已用 `validateSearch`）。
- [ ] 🟢 **骨架尺寸与真实控件不符**：`discover/category-filters.tsx:61-63`（`h-9 rounded-full` vs `h-8 rounded-lg`）、`library/scan-button.tsx:27`（同）、`actors/detail-page.tsx:80`（`size-48` vs `size-32`）。
- [ ] 🟢 `actors/catalog-page.tsx:48`：加载仅文字，其他列表为骨架。
- [ ] 🟢 写法不统一：`aspect-[3/4]`（`actors/detail-page.tsx:94,169`）vs `aspect-3/2`；`preview-zoom.ts:1`、`task-notification-diff.ts` 相对路径 vs `@/`；`movie-subscribe-button.tsx:26` `asChild` vs `magnets.tsx` `render`；`custom-quality-menu.tsx` lucide `Hd` 图标混入 videojs 图标集。
- [ ] 🟢 进度口径不一致：`library/library-progress.tsx:10-20` 自算 vs `task.progress` + `scanCount()`。
- [ ] 🟢 `LibraryPlayerDialog` 在 history 直接导入，movies/actors 懒加载且各写一份。

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

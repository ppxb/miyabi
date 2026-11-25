# 流程与架构评估 Checklist

> 评估日期：2026-11-25　基线提交：`d37ef43`
> 范围：`cmd/`、`internal/`（Go 后端，不含 `internal/ent` 生成代码与测试）与 `web/src`（前端）
> 标记：✅ = 已在代码中人工复核确认；其余为子代理评估结论（带证据，未逐条复核、未实际运行验证）
> 严重度：🔴 高　🟠 中　🟢 低
> 上一版审计清单（2026-11-20）已备份至 `backups/CHECKLIST.MD.20261125.bak`

---

## P0 · 流程跑不通或结果不对（优先修复）

- [x] 🔴 ✅ **本地源"自动刮削"模式必然失败**：本地 scrape 任务的 `Source.AccountID` 为 `local`，但刮削无条件走 115 会话，查不到账号即返回 `ErrUnauthorized`。另外本地文件行的 `root_id` 固定为 `local`，而任务里的 `Source.Directory.ID` 是目录路径，`database.LibraryFiles(source)` 匹配不到文件，`Finished` 也就标不了 Failed：影片一直停在 Pending，每次扫描都再排一个必败任务。
  - 链路：`internal/library/library.go:351`（`StartLocalSourceScanModeIncremental`）→ `internal/library/local.go:154`（入队 scrape）→ `internal/library/scrape/scrape.go:146`（`drive.OpenSource`）→ `internal/drive/session.go:44`
  - （已修复：本地影片既入 Miyabi 库，也导出到 Emby 目录）
    - `scrape/scrape.go`：本地源不开 115 会话，事务改用 `ent.WithTx`；新增 `movieFiles`，本地按账号划定文件范围，`prepareMetadata`、`Finished` 都改用它，失败时能正确标 Failed。
    - `scrape/publish.go`：新增 `publishLocal`，写元数据、封面、Done 状态并导出 .strm/NFO/封面；115 发布的落库加通知事务抽成共用的 `commitPublished`。两种情况只入库不导出：影片同时有 115 文件（Emby 目录归 115 导出）、影片文件本身在 Emby 目录内（避免把用户的 .strm 改成指向自己）；不导出时保留原 `MetadataSnapshot`。
    - `api/router.go`：`/api/strm/play/` 遇到 `local-` 前缀的文件 ID 时转给 `localStreamHandler` 读硬盘（支持 Range，令牌策略相同），.strm 格式、`RewriteSTRM` 不变。
    - `scan/local_ingest.go`：Emby 目录回导时，.strm 内容解析出的 `local-` ID 只当引用，不把视频行的 path 改写到 .strm 上。
    - `domain/source.go`：新增 `LocalFileIDPrefix`。
    - 测试：`internal/app/local_scrape_test.go`（本地扫描 → 刮削 → 导出 → Emby 回导全流程；Emby 目录内的本地源不被重导出）、`internal/api/strm_test.go`（`TestSTRMLocalFileIDStreamsFromDisk`）。两处保护都做过反向验证。
  - 遗留（低）：
    - 本地扫描在发布期间并发新增分盘时，可能漏导一个 cd，下次扫描补上。
    - 数据库为空、先回导 Emby 目录时，.strm 会短暂自建一条本地行；同次启动随后的本地源扫描会纠正。
- [x] 🟠 ✅ **播放直链缓存键缺 UA 和授权版本，且不淘汰**（复核后由 🔴 降为 🟠）：`internal/playback/files.go:278` 的键为 `accountID + "|" + pickCode`。115 直链与 UA 绑定（依据代码推断：取直链时原样透传浏览器 UA、连空 UA 都特意保留，`internal/pan/play.go:29`、`internal/pan/client.go:256`；未用真实账号验证），浏览器随后被 302 直连 CDN（`internal/playback/proxy.go:33`）。
  - 实际影响：只有不同浏览器或设备在 2 分钟内播放同一 115 文件时，后打开的一方拿到别人 UA 的直链而播放失败，2 分钟后恢复。
  - 另外两点影响很小：命中缓存前仍会打开 115 会话、校验文件归属，权限没有被绕过；过期条目原先读到时只忽略不删，每个文件残留一条小记录。
  - 对比：`internal/strm/relay.go:47` 的键包含授权版本和 UA。
  - （已修复：最小方案）
    - `playback/files.go`：`playSourceSession` 增加 `AuthorizationVersion()`，缓存键改为 `授权版本:账号:pickCode:UA`，与 STRM 中转一致。
    - `playback/service.go`：`cachedDownloadURL` 读到过期条目时从 map 删除。
    - 测试：`internal/playback/files_test.go` 新增 `TestResolveSpecsCacheIsScopedToUserAgentAndAuthorization`（换 UA、换授权版本都不命中缓存，过期条目被删除），两处改动都做过反向验证。
  - 遗留：两套 115 直链解析的收敛仍见 P2 第 1 条。
- [x] 🔴 ✅ **开了访问密码但未配 `MIYABI_STRM_TOKEN` 时 MPV/STRM 返回 401**（复核后由 🟠 升为 🔴）：`internal/config/config.go:76` 中 token 可选且不校验、不派生；token 为空时导出的 .strm 与 MPV 链接都不带 `?token=`（`internal/export/export.go:51`、`internal/api/mpv_link.go:25`），而开门禁后 `strmAuthorized`（`internal/api/strm.go:60`）只认 token 或 cookie，外部播放器带不了 cookie。
  - 实际影响：Emby 播放 .strm、"用 MPV 播放"、MPV 外挂字幕（`/api/subtitle-file/`）全部 401。README 的 docker run / Compose 示例都配了访问密码（并"强烈建议开启门禁"），却都没有 `MIYABI_STRM_TOKEN`，按文档部署并接入 Emby 必然踩中。
  - （已修复：开门禁且未配 token 时从访问密码派生固定 token，与 JWT 密钥的做法一致）
    - `config/config.go`：`Load` 中 `MIYABI_STRM_TOKEN` 为空且设置了访问密码时，用 HMAC-SHA256（独立盐 `miyabi-strm-token`，与 JWT 派生互不相关）派生 32 位十六进制 token。导出、播放鉴权、MPV 链接都读 `cfg.STRMToken`，一处生效。
    - 已有 .strm 无需手动处理：启动时 `StartStartupSTRMRewrite`（`internal/app/app.go:296`）会按当前 token 改写；改密码后重启同样自动更新。显式配置 token 的行为不变。
    - `README.md`：`MIYABI_STRM_TOKEN` 默认值改为"空，开启门禁时由访问密码派生固定 Token"。
    - 测试：`internal/config/config_test.go` 新增 `TestLoadDerivesSTRMTokenForProtectedAccess`（派生稳定、不同密码不同、显式配置优先、未开门禁为空）；开门禁时凭 token 放行已有 `internal/api/strm_test.go` 覆盖。
  - 说明：派生 token 会明文写进 .strm 文件，能读媒体目录者可凭它播放（不能登录或管理），与手动配置 token 相同。
- [x] 🟠 ✅ **扫描任务复用不区分模式**：`internal/library/workflow.go:209` 的 `ensureScanTask` 只按"来源 + 是否重建"复用，不比较 `Incremental` / `MetadataOnly`。`Incremental` 只对 115 源生效（`internal/library/scan/reconcile.go:188`，增量只给 Pending 影片排刮削），`MetadataOnly` 只对本地源生效（`internal/library/local.go:141`）。
  - 实际影响（只在同源扫描排队或运行的窗口内）：
    - 启动增量同步（`internal/library/library.go:362`）期间手动点"扫描"（`library.go:234`），复用了增量任务，全量扫描该做的修复（已完成影片换文件、加分盘、封面缺失）静默不做。
    - 刚改本地源刮削模式并保存，这一轮仍按旧模式执行；改成"仅使用本地数据"后仍会发起刚关掉的在线刮削。
  - （已修复：可复用的任务必须覆盖请求的全部工作，否则按最新请求处理）
    - `library/workflow.go`：排队中的同源任务直接改为请求的模式（`MetadataOnly` 以最新为准，`Incremental` 只从增量放宽为全量、不会收窄），不重复排任务、不重复遍历 115 目录；正在运行的任务只有 `MetadataOnly` 相同且（它是全量或请求就是增量）时才复用，否则新排一个排队任务（扫描池只有 1 个 worker，会在当前任务后执行）。调用方持有队列锁，排队任务在改写期间不会被领取。
    - 测试：`internal/library/scan_reuse_test.go` 新增 `TestScanReuseRespectsRequestedMode`（排队增量被全量请求放宽且不被收窄回去、运行中的增量不顶替全量、运行中的全量覆盖增量、本地模式以最新设置为准、运行中的旧模式不被复用），换回旧行为后三个子测试全部失败。
- [ ] 🟠 **已完成影片重新刮削时被降级为 Pending，失败后变 Failed**：`internal/library/scrape/mapping.go:263` 的 `SaveMovieMetadata` 固定设 Pending；`internal/library/scrape/scrape.go:202` 对已完成影片也重存同一份文档；`publish.go` 里又 `SetMetadata` 一次，元数据写两次。
  - 改法：已完成且非重建时跳过保存；状态由调用方决定。
- [ ] 🟠 **Emby 目录以外的通知永远清不掉**：`internal/emby/notifications.go:291` 的 `enqueueNotification` 不检查路径是否在 LocalDir 下；路径不匹配时只累加重试次数，记录永不删除，最长每 5 分钟重试。MediaPath 为空时原始本地路径会直接发给 Emby（`internal/emby/service.go:231`）。
  - 改法：入队时过滤掉不在 LocalDir 下的路径，配置不匹配的记录直接删除。
- [ ] 🟢 ✅ **订阅自动下载时磁力聚合两次，可能误报找不到磁力**（推断，未验证）：`internal/monitor/check.go:118` 取一次，`internal/offline/add.go:50` 的 `resolveMagnet` 再取一次。来源不稳定时两次结果可能不一致，返回 `ErrMagnetNotFound`。
  - 改法：内部调用方传入已选中的磁力，跳过二次校验（与 P1 磁力缓存一并处理）。
- [ ] 🟢 **（待确认）按影片 ID 添加订阅把番号当 JavDB ID 用**：`internal/catalogue/service.go:337` 把 `item.ID` 设为番号，`internal/monitor/subscribe.go:25` 的 `AddMovie` 及 `catalogue/magnets.go:94` 会把它拼进 `/api/v4/movies/{id}`（`internal/javdb/movie.go:28`）。前端 `useAddSubscription` 已无调用方。
  - 先确认 JavDB 是否接受番号作为 ID，再决定修（先 `ResolveMovieID`）还是删。

---

## P1 · 冗余操作与不必要的链路

### 影响大

- [ ] 🔴 ✅ **刮削时整棵查询树反复刷新**：每刮完一部影片后端至少发两次 `NotifyLibraryChanged`（`internal/library/scrape/scrape.go:239`、`internal/library/scrape/publish.go:136`）；前端看到库版本变化就刷新 `libraryKeys.all`、`actorKeys.all`、影片状态和离线任务（`web/src/features/tasks/task-events.tsx:130-133`）。SSE 每 300ms 推一次（`internal/api/sse.go:13`），批量刮削期间当前页面接口被持续重拉。
  - 改法：后端合并为每部影片一次、按秒级节流；前端刮削进行中只刷新任务，任务结束或间隔 ≥2s 再刷新 `libraryKeys.all`。
- [ ] 🔴 ✅ **文件没变也通知 Emby**：`internal/export/media.go:20` 的 `WriteMedia` 返回"是否有变化"，但 `internal/library/scrape/publish.go:103` 丢弃该返回值，随后无条件 `NotifyUpdatedTx`；`internal/library/scan/reconcile.go:122` 同样丢弃，全量对账时对每部可复用影片都发通知。
  - 改法：只在 `changed` 为真时通知；重试场景在 payload 里记标记兜底。
- [ ] 🔴 **启动时 Emby 目录被处理三遍**：
  1. `ScheduleLocalScan`（`internal/app/app.go:290`，保存 Emby 设置时 `internal/emby/service.go:158` 也会触发）把自己导出的目录全量再导入：`internal/library/scan/local_ingest.go:65` 重新编码 3 张 JPEG（含 Lanczos 缩放），`:166` 数据库→NFO→数据库来回写，`:214` 每目录发一条 Emby 通知；全程持有 `exportMgr.WithConfig` 全局锁（`internal/library/library.go:285`），卡住所有刮削发布。
  2. STRM 改写再遍历一遍（`internal/app/app.go:296`、`internal/export/export.go:66`）。
  3. 对账再逐片读一遍；增量模式下对账算出的 `reusable` 结果用不上（`internal/library/scan/reconcile.go:154-189`），只剩"修复导出文件"的隐含副作用，且同时持有封面锁和导出配置锁。
  - 不确定：JPEG 重编码后字节可能变化，导致下次对账改写 poster.jpg 形成循环。
  - 改法：本地回导只在数据库为空或用户手动触发时执行，已关联完成影片的文件跳过；记录上次 STRM 改写用的 PublicURL 和 token，未变化则跳过；增量模式跳过对账修复，或拆成显式、分批释放锁的修复任务。
- [ ] 🔴 ✅ **磁力缓存经常失效**：`internal/magnet/aggregator.go:192` 只要有来源失败或被拦即标记 partial；`internal/catalogue/magnets.go:53` 遇 partial 返回 `ErrDoNotCache`。一个来源长期被拦或冷却，1 分钟缓存就形同虚设，叠加 P0 的二次聚合，远程请求翻倍。
  - 改法：partial 结果也缓存，TTL 15–30 秒；内部调用方复用已选中的磁力。

### 影响中

- [ ] 🟠 **JavBus 详情页无缓存，同一番号抓两次**：`internal/javbus/client.go:149` 注释称页面由元数据和磁力共用，但无缓存或 singleflight；`internal/metadata/providers/javbus.go:52` 和 `javbus.Find`（`client.go:128`）各自调用 `MoviePage`。
  - 改法：照 `internal/freejavbt/client.go:44-57` 加小容量 TTL 缓存 + singleflight。
- [ ] 🟠 **一轮离线同步重复分页拉取 115 任务列表**：`internal/offline/sync.go:94` 先完整走一遍 `WalkOfflinePages`，`internal/offline/control.go:149` 的 `lookupRemoteTask` 再走一遍，`control.go:236`、`internal/offline/submit.go:61` 还会再走。115 客户端限速为每 250ms 一个请求、最多 2 并发（`internal/pan/client.go:34`）。
  - 改法：`syncSource` 把本轮观察到的 `map[hash]pan.OfflineTask` 传给 `recoverSource` / `advance`，命中即不再拉取。
- [ ] 🟠 **自动换源队头阻塞，且每次换源至少两轮**：`internal/offline/recovery.go:103`、`:124` 处理完第一个需动作的任务就 `return`；`internal/offline/control.go:216` 移除旧任务后只改状态为 `actionSubmit`，等下一轮 30 秒同步才提交。N 个卡住任务约需 N×60 秒。
  - 改法：`return` 改 `continue` 或每轮设预算；`removeCurrent` 成功后在同一次 `advance` 里直接 `resumeSubmission`。
- [ ] 🟠 **每次云盘扫描都重新下载所有 NFO**：`internal/library/scan/identity.go:117` 的 `readNFO` 调 `sess.Read`（最多 2MB），不受扫描节流控制，只用来取番号。
  - 改法：先查已有文件记录，未变化的跳过。
- [ ] 🟠 **发布时逐视频调 `sess.Info`**：`internal/library/scrape/publish.go:84`、`:140-150`（`verifyVideoPositions`）。`Directories` 刚列过同目录，新增的只有 `WithinSource` 检查。
  - 改法：每个目录检查一次。
- [ ] 🟠 **未开自动下载但已找到磁力的订阅，每天永久重查**：`internal/monitor/check.go:137` 用 `nextDaily`，不走 `nextMovieCheck` 的 stale 策略，每天一次完整多来源聚合。
  - 改法：复用 `nextMovieCheck`，或找到磁力后停止轮询、等用户手动入库。

### 影响小

- [ ] 🟢 **离线任务 SSE 与轮询重复**：`web/src/api/offline.ts:58` 活跃时每 5 秒轮询，而后端 `internal/offline/sync.go:217` 已经 `NotifyOfflineChanged()` 经 SSE 推送。
  - 改法：仅在 `useTaskConnection().status !== 'connected'` 时启用 `refetchInterval`。
- [ ] 🟢 **刷新范围过宽**：
  - `web/src/features/library/player-dialog.tsx:445` 关播放器刷新整个 `libraryKeys.all`；
  - `web/src/api/subtitles.ts:105`、`:114` 字幕保存/删除刷新整个 `libraryKeys.all`；
  - `web/src/api/library.ts:170`、`:179`，`web/src/api/tasks.ts:85`、`:94` 操作成功后主动刷新 `taskKeys.all`，SSE 又推一次快照，任务列表被拉两次。
  - 改法：只刷新受影响的 key；SSE 已连接时操作成功后不再刷新 tasks。
- [ ] 🟢 ✅ **任务池惊群 + 每次 Claim 读暂停设置**：`internal/tasks/queue.go` 的 `Finish` → `WakePool` 唤醒全部 4 个 worker，每个都要拿全局锁、读一次 `tasks.library_paused`（`internal/tasks/pause.go:52` 的 `runnableKinds`），再跑 `Claim` 和 `NextRetry`。
  - 改法：暂停状态缓存在内存（`SetLibraryPaused` 时更新）；按 kind 只唤醒相关池。
- [ ] 🟢 **离线同步无唤醒通道**：`internal/app/app.go:319` 传入的 wake 为 `nil`（monitor 有 `Pending()`），提交或取消后最多 30 秒才看到状态更新。
  - 改法：给离线服务加 Pending 通道，提交和控制操作时发信号。
- [ ] 🟢 **零碎重复读取**：
  - `internal/drive/account_session.go:119-132` 的 `readAccount` 每次调用 115 查 3 次 PanAccount；
  - 离线同步 `config()` 每轮读两次（`internal/offline/sync.go:67`、`internal/offline/recovery.go:81`）；
  - `AddByCode` 先调 `Mounts`，`addForAccount` 再调一次 `Mount`；
  - `CatalogueMagnetsByCode` 查内存缓存前先查一次数据库（`internal/catalogue/magnets.go:45`）；
  - 手动扫描用 `drive.Mounts`（含未启用账号，`internal/library/library.go:222`），增量同步用 `s.sources`（`:359`），每个请求重复调用 `Mounts` / `source.List` / 逐账号 `Mount`（`:387-431`）；
  - `internal/library/workflow.go:154-170` 对每条带复用引用的扫描记录各查一次，payload 解码两次。

---

## P2 · 架构收敛（同一功能多套实现）

- [ ] 🟠 **两套 115 直链解析和缓存**：`internal/playback/files.go:256-275` 与 `internal/strm/relay.go:82-123` 都实现了"先直链、失败再转码"，缓存各一套。
  - 改法：收敛到 `drive` 层，或让 playback 复用 `strm.Relay.StreamURL`（顺带修 P0 第 2 条）。
- [ ] 🟠 **scrape 任务入队有三套实现，去重规则各不相同**：`internal/library/scan/reconcile.go:210-263`、`internal/library/local.go:164-176`（逐条先查再插，无事务、未拿队列锁）、`internal/library/scan/movie_actions.go:53-128`。
  - 改法：收敛成一个函数。
- [ ] 🟢 **本地与云盘导入各有一套影片匹配逻辑**：`internal/library/scan/local_ingest.go:113-159`（逐条 Create）与 `internal/library/scan/movies.go:105`（批量建并合并别名）。
- [ ] 🟢 **重复类型与转换**：
  - `library.LocalScanTarget` 与 `source.LocalTarget` 字段相同，`internal/app/app.go:116-126` 专门转换；可移到 domain 包。
  - `localMetadataJob`（`internal/library/local.go:18-27`）重复定义 `scrape.MetadataPayload`。
  - `internal/javdb/model.go:80` 的 `HTTPError`、`internal/javdb/retry.go:17` 的 `parseRetryAfter` 与 `internal/domain/retry.go:17`、`:41` 重复。
- [ ] 🟢 **生产代码无调用方**：
  - 完全无调用：`libraryScope`（`internal/library/library.go:333`）、`catalogue.HasMagnet`（`internal/catalogue/magnets.go:74`）、`catalogue.MovieCode`（`internal/catalogue/service.go:242`）、`offline.Service.Add`（`internal/offline/add.go:19`）、`magnet.Aggregator.Find`（`internal/magnet/aggregator.go:86`）。
  - 仅测试使用：`StartLocalSourceScan` / `StartLocalSourceScanMode`（`internal/library/local.go:56-64`）、`scan.ReconcileScan`（`internal/library/scan/reconcile.go:31`，另建导出 Manager 绕开共享锁）、`scan.ProcessScanPage`（`internal/library/scan/persist.go:19`）、`drive.Drive.Source`（`internal/drive/drive.go:130`）。
  - 前端从不调用的路由：`GET /api/play/files`、`GET /api/play/:id`（`internal/api/router.go:145`、`:148`）。外部客户端是否在用无法确认。

---

## 核查后排除（不是问题）

- 任务视图与观看历史无 N+1：`internal/library/workflow.go:29-75`、`internal/monitor/batch_tasks.go:73-104` 为固定次数批量查询；`internal/app/task_views.go:33-50` 按版本号缓存快照；`internal/playback/history.go:136` 用 `IDIn` 批量查询。
- `web/src/api/movie-detail-cache.ts` 只从 react-query 缓存里查数据，不是重复缓存。
- 三层元数据缓存（catalogue 内存、MetadataCache 表、freejavbt 内部缓存）服务不同层，不重复。
- pan / drive 分层合理：drive 负责 token 刷新、授权版本与目录归属校验，分页只在 drive 实现；仅 `Drive.OpenMedia` 是纯转发。
- 三套 HTTP 客户端有理由：tls-client 指纹客户端过反爬，resty 用于 115 限速与字幕站，其余用标准库 + SafeDownloadClient。
- 离线完成后不会重复扫描：完成标记与定向扫描同事务写入，以 `ScanTaskID` 去重（`internal/offline/sync.go:259-283`）。
- monitor 每 5 分钟 tick 只做两次到期订阅查询，不访问远端。
- 鉴权路径基本统一：页面走 cookie + JWT，外部播放器走 `strmAuthorized`（`internal/api/strm.go:60`），唯一缺口见 P0 第 3 条。

---

## 建议处理顺序

1. P0 第 1–4 条：用户可直接遇到的错误，改动都小。
2. P1 影响大的四条：通知合并与节流、`WriteMedia` 有变化才通知、启动三遍处理去重、磁力 partial 短 TTL 缓存。
3. P1 影响中的：JavBus 缓存、离线同步单次拉取、换源队头阻塞。
4. P2 收敛与死代码清理。

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
- [x] 🟠 ✅ **已完成影片重新刮削时被降级为 Pending，失败后变 Failed**：全量扫描会给视频变化或封面缺失的已完成影片排非重建刮削（`internal/library/scan/reconcile.go:236-247`）。`prepareMetadata` 沿用已存元数据后仍调用 `SaveMovieMetadata`（`internal/library/scrape/scrape.go:244`），后者固定写 Pending（`internal/library/scrape/mapping.go:85`），使 `Finished` 对已完成影片的保护（`ScrapeStatusNEQ(Done)`）失效。
  - 实际影响：任务进行中详情页显示"待刮削"并计入待刮削数；发布阶段任一失败（视频已移动、索引变化重试耗尽、115 登录过期、封面重新下载失败）都会把它标成"刮削失败"。此后增量同步不处理它，Emby 目录回导也不会恢复它，下次全量扫描还会重新向元数据源请求。顺带白做：清空并重建演员、标签，跨语言演员合并，库变化通知。
  - 原描述"元数据写两次"不成立：发布阶段的 `SetMetadata` 仍需要，因为封面阶段可能向元数据追加备选图片。
  - （已修复）
    - `scrape/scrape.go`：`prepareMetadata` 对已完成且非重建的影片只保存任务断点，不调用 `SaveMovieMetadata`，也不做演员合并和库变化通知，状态保持 Done。需要联网获取元数据的分支（待刮削、失败重试、重建）不变。
    - 测试：`internal/app/scrape_completed_test.go` 新增 `TestFailedReexportKeepsCompletedMovie`（已完成影片走非重建刮削、在发布阶段注入失败，断言全程保持 Done 且影片记录未被修改）。换回修复前的代码，测试复现了"进行中变 Pending、失败后变 Failed"。
- [x] 🟢 ✅ **Emby 目录以外的通知永远清不掉**（复核后由 🟠 降为 🟢）：`internal/emby/notifications.go:25` 的 `enqueueNotification` 不检查路径是否在 LocalDir 下；路径不匹配时只累加重试次数，记录永不删除，最长每 5 分钟重试。MediaPath 为空时原始本地路径会直接发给 Emby（`internal/emby/service.go:229` 的 `translatePath`）。
  - 触发来源：本地源扫描对每个视频所在目录发通知（`internal/library/scan/local_ingest.go:218-222`），本地源不在 Emby 目录下时必中；改 LocalDir/MediaPath 后排队中的旧记录也会残留。
  - 实际影响：用户无感知（不打日志、不影响正常通知）。每个本地影片目录残留一条记录，每 5 分钟查询加写事务一次；手动扫描或保存 Emby 设置时 `RetryPending` 把它们全部置为到期，flush 每次只取 50 条，正常通知会被推迟几个 3 秒周期。
  - （已修复：统一过滤，MediaPath 为空时同样只通知 LocalDir 内的路径）
    - `emby/notifications.go`：新增 `inLocalDir`（Abs + Rel，LocalDir 自身算在内，跨盘符或 LocalDir 为空判为不在内）。`enqueueNotification` 对 LocalDir 以外的路径直接返回、不入队；`flushNotifications` 遇到不在当前 LocalDir 下的记录按 revision 匹配直接删除，不再算作失败重试。
    - 测试：`internal/emby/notifications_test.go` 新增 `TestNotificationsOutsideLocalDirAreDropped`（MediaPath 为空和非空两种：目录外路径不入队、旧配置遗留记录被删除、只有目录内路径发给 Emby），分别关掉入队过滤和 flush 删除做反向验证，两处都会失败。原有两个测试（`TestNotificationTransactionsAndConcurrentUpdates`、`actors_lifecycle_test.go` 的 `TestServiceCloseFlushesBufferedNotifications`）原先用空 LocalDir，补上 LocalDir（生产环境来自 `cfg.EmbyDir`，非空）。`go test ./...` 中 `internal/` 全部通过。
  - 遗留（极低）：
    - flush 先取配置快照再查询。如果恰好在两步之间改了 LocalDir，并且新目录下的记录也已入队，这条记录会按旧目录被删掉（以前是记失败后重试）。窗口为微秒级，未处理。
    - 用户自己把本地源目录加进 Emby 媒体库时，不再通知 Emby 刷新这些目录（按方案统一过滤）。
- [x] 🟢 ✅ **订阅自动下载时磁力聚合两次，可能误报找不到磁力**：`internal/monitor/check.go:118` 取一次，`internal/offline/add.go:50` 的 `resolveMagnet` 再取一次。来源不稳定时两次结果可能不一致，返回 `ErrMagnetNotFound`。
  - 复核：两次都走 `CatalogueMagnetsByCode`（订阅创建时总会写 Code，`internal/monitor/subscribe.go:49`）。结果完整时第二次命中 1 分钟缓存，不会误报；结果为 partial 时返回 `ErrDoNotCache`、不进缓存（`internal/catalogue/magnets.go:53`），第二次重新远程聚合。选中的磁力只来自某个这次失败的来源时，就会误报。
  - 实际影响：定时检查（`check.go:146`）显示"加入 115 失败"，1 小时后重试；手动"入库"（`internal/monitor/batch.go:59`）直接把错误返回给用户。只要结果是 partial，每次提交都多一轮远程聚合。
  - （已修复：与 P1 磁力缓存拆开处理）
    - `offline/add.go`：`resolveMagnet` 收到 code（即 `AddByCode`，调用方只有 monitor 的上述两处，hash 刚从同一番号的聚合结果中选出）时直接返回规范化番号，不再拉磁力列表。`Add`、`AddForAccount`（API 上用户提交的 hash）不传 code，仍校验。monitor 与接口签名未改。
    - 测试：`internal/offline/concurrency_test.go` 新增 `TestOfflineAddByCodeTrustsPickedMagnet`（hash 不在目录磁力列表中，`AddByCode` 仍提交并以规范化番号落库）；换回旧代码后测试复现"磁力链不属于当前影片"。原有 `TestOfflineAddRejectsUnknownMagnetBeforeSubmission` 继续覆盖不传 code 时的校验。`go test ./internal/...` 全部通过。
  - 后续（2026-11-25）：订阅自动下载已整体移除，`Add`、`AddByCode` 及 `resolveMagnet` 的免校验分支一并删除；离线下载只剩 API 的 `AddForAccount`，始终校验磁力归属，本条问题随之不复存在。
- [x] 🟢 ✅ **按影片 ID 添加订阅把番号当 JavDB ID 用**：`internal/catalogue/service.go:337` 把 `item.ID` 设为番号，`internal/monitor/subscribe.go:25` 的 `AddMovie` 及 `catalogue/magnets.go:94` 会把它拼进 `/api/v4/movies/{id}`（`internal/javdb/movie.go:28`）。前端 `useAddSubscription` 已无调用方。
  - 复核：JavDB 影片 ID 是 `ZY5eq` 这类短编码，与番号不同（`ResolveMovieID` 专门经搜索页把番号换成 ID，`internal/javdb/websearch.go:39`），整条 `AddMovie` 链路没有调用它。界面上没有手动订阅影片的入口，属于潜在 bug；演员派生的影片订阅走 `addMovie`，不受影响。
  - （已修复：按产品方向"订阅只支持演员、JavDB 磁力只在有 JavDB ID 时取"处理，不修这条链路而是删掉）
    - 订阅只收演员：`api/subscription.go` 创建接口只接受 `kind=actor`；删除 `monitor.AddMovie`、`retry`（仅手动重订阅会用到）、`catalogue.MovieSummary` 及 `Discoverer.MovieSummary`；`existingMovie` 简化为已存在就原样返回。前端 `useAddSubscription` 保留（以后加演员订阅按钮用），固定 `kind: 'actor'`。
    - 磁力只按番号查：删除按影片 ID 查磁力的路径（`catalogue` 的 `Magnets`、`CatalogueMagnets`、`HasMagnet`、`cachedMagnets`、`MovieCode`，它们会把 ID 当 JavDB ID 请求 JavDB 详情），`monitor`、`offline` 接口同步收窄。`/api/discover/movies/:id/magnets` 增加可选 `javdb_id`，无合法番号时返回 400；`catalogue/magnets.go` 优先用请求的 `javdb_id`，其次用库内元数据记录的 JavDB ID，都没有则 JavDB 跳过（不算失败）；缓存键带上 JavDB ID。`CatalogueMagnetsByCode` 改为返回副本。monitor 只按订阅 Code 查磁力。
    - 前端：`DiscoverMovie` 补 `sources` 类型；`movie-detail-cache.ts` 新增 `findCachedJavdbID`（扫描所有缓存卡片，详情响应不含 JavDB）；`useDiscoverMagnets` 带上 `javdb_id`，查询键含之。
    - JavDB 限速或 403 不影响其他来源：聚合器已满足（`ErrBlocked` 视为空结果，其他失败只标 partial），未改。
    - 测试：`internal/api/subscription_test.go` 新增 `TestSubscriptionCreateOnlyAcceptsActors`；`internal/api/discover_magnets_test.go` 新增 `TestDiscoverMagnetsLooksUpByCodeOnly`；`internal/catalogue/catalogue_test.go` 新增 `TestMagnetsByCodeQueriesJavDBOnlyWithAnID`（无 ID 不查 JavDB、仅番号的缓存不复用给带 ID 的请求、库内 ID 生效、全程不请求 JavDB 详情）；`web/tests/movie-detail.test.ts` 新增 JavDB ID 取自任一缓存卡片的用例。原有 catalogue、monitor、api 测试迁移到按番号查询，删除已无意义的"详情失败后重试"用例。`go test ./internal/...`、`npx vitest run`（172 个）、`npm run typecheck` 全部通过；子代理只读 review 未发现问题。
  - 遗留：
    - 订阅页的影片卡片没有目录卡片缓存，带不上 javdb_id，只有影片已入库时 JavDB 磁力才参与（符合设计）。
    - 界面上仍没有订阅演员的入口。
    - "订阅只展示演员新作、取消自动下载和入库"另作任务，待出删改清单后实施。

---

## P1 · 冗余操作与不必要的链路

### 影响大

- [x] 🟠 ✅ **刮削时整棵查询树反复刷新**（复核后由 🔴 降为 🟠）：每刮完一部影片后端至少发两次 `NotifyLibraryChanged`（`internal/library/scrape/scrape.go:239`、`internal/library/scrape/publish.go:136`）；前端看到库版本变化就刷新 `libraryKeys.all`、`actorKeys.all`、影片状态和离线任务（`web/src/features/tasks/task-events.tsx:130-133`）。SSE 每 300ms 推一次（`internal/api/sse.go:13`），批量刮削期间当前页面接口被持续重拉。
  - 改法：后端合并为每部影片一次、按秒级节流；前端刮削进行中只刷新任务，任务结束或间隔 ≥2s 再刷新 `libraryKeys.all`。
  - 复核：`invalidateQueries` 只重拉页面上正在用的查询；`refreshQueries` 对同一 key 最多一个请求在途、一个排队（`web/src/api/query-refresh.ts:19-24`），不会堆积。实际是当前影片库/演员页每部影片重拉约两次。P0 第 5 条修复后，已完成影片的重导出只通知一次（`scrape.go:256-258`）。
  - （已修复：只改前端，后端不变）
    - `web/src/features/tasks/task-events.tsx`：影片库和演员（`libraryKeys.all`、`actorKeys.all`）的刷新改为 2 秒节流：空闲后的第一次变化立即刷新，2 秒冷却期内的后续变化合并成冷却结束时的一次补刷。冷却用计时器实现，不读系统时钟，时钟回拨不影响。影片状态、离线、订阅仍立即刷新。SSE 重连/卸载时如有待补刷立即执行（重连前修订号已记录，新连接不会再判为变化）。
    - 后端两次通知保留：`NotifyLibraryChanged` 只是内存计数加一（`internal/tasks/bus.go:97`）。`scrape.go:260` 那次覆盖"元数据已存、发布前遇到暂停或可重试失败"的窗口，删掉后这段时间界面看不到新元数据；节流后两次通知在前端合并，没有额外代价。
    - 测试：`web/tests/task-events.test.tsx` 新增 `library bursts refresh at once, then once per interval, without delaying offline views`（首次立即刷新、窗口内合并为一次补刷、离线不受节流、重连时补刷不丢）。换回旧代码，或单独去掉重连补刷，测试都会失败。`npx vitest run`（168 个）、`npm run typecheck` 通过；子代理只读 review 未发现 bug。
  - 遗留（低）：
    - 批量刮削中途 SSE 重连时，重连补刷加上新连接的首次刷新，2 秒内会刷两次，只多不少。
    - 其他来源触发的库变化（扫描、手动操作）同样受 2 秒节流，最多延迟 2 秒显示。
- [x] 🟠 ✅ **文件没变也通知 Emby**（复核后由 🔴 降为 🟠）：`internal/export/media.go:20` 的 `WriteMedia` 返回"是否有变化"，但 `internal/library/scrape/publish.go:103` 丢弃该返回值，随后无条件 `NotifyUpdatedTx`；`internal/library/scan/reconcile.go:122` 同样丢弃，全量对账时对每部可复用影片都发通知。
  - 改法：只在 `changed` 为真时通知；重试场景在 payload 里记标记兜底。
  - 复核：真问题只在对账。每次手动全量扫描，每部已完成影片都向 Emby 发一条 `/Library/Media/Updated`（每 3 秒 50 条，每批成功后还会排一次演员同步）；增量同步只处理待刮削影片，不发这批通知。刮削发布只在新刮削、重建、换文件、缺封面时执行，文件基本都会变，多发的很少，不改。无条件通知原是有意为之：事务失败后重试时文件已完整，`changed` 为假，按变化通知会丢。
  - （已修复：只改对账，刮削发布不变）
    - `domain/scan.go`：扫描 payload 新增 `ExportedCodes`，记录对账改写过导出的番号，事务失败后保留到重试。
    - `scan/reconcile.go`：`prepareReconcile` 记下 `ExportLocalMovie` 返回有变化的番号；`WriteMedia` 中途出错时仍会报告已改写的文件，同样记下。每批有新增时存一次断点；出错或取消提前退出时，用 `context.WithoutCancel` 补存。`reconcileTx` 只通知这些番号，然后清空列表并标为完成；原来按可复用影片逐部通知的逻辑删除。增量模式下修复的导出现在也会通知（以前漏通知）。
    - 测试：`internal/library/scan/reconcile_test.go` 新增 `TestReconcileNotifiesOnlyRewrittenExports`（首次导出两部都通知；再次对账只通知被破坏后修复的那部；完成后列表清空）、`TestReconcileSavesExportsRewrittenBeforeFailure`（第二部写完封面后 NFO 写入失败，两部的番号都已存进断点）。原有 `TestReconcileRetryNotifiesExportsWrittenBeforeFailedCommit` 继续覆盖"事务失败后重试仍通知"。反向验证：换回旧代码，第一个新测试失败；去掉批末存盘，重试测试失败；换回 review 前的版本（没有出错补存，出错时也不记番号），第二个新测试失败。`go test ./internal/...` 全部通过；子代理只读 review 发现的"批内出错丢通知"已按上述补存修复。
  - 遗留（低）：
    - 文件已写入、断点未存时进程崩溃，这些影片的通知会丢，Emby 要等自身扫描才能发现。
    - 以前手动全量扫描能顺带让 Emby 把每个目录刷新一遍，改后需在 Emby 里自行扫描。
    - `go build ./...` 会被 `backups/actor-name-key-20261120-163048/tmp-verify/` 里的旧临时脚本卡住（引用的字段已不存在），与本条无关，未处理。
- [x] 🔴 ✅ **启动时 Emby 目录被处理三遍**：
  1. `ScheduleLocalScan`（`internal/app/app.go:290`，保存 Emby 设置时 `internal/emby/service.go:158` 也会触发）把自己导出的目录全量再导入：`internal/library/scan/local_ingest.go:65` 重新编码 3 张 JPEG（含 Lanczos 缩放），`:166` 数据库→NFO→数据库来回写，`:214` 每目录发一条 Emby 通知；全程持有 `exportMgr.WithConfig` 全局锁（`internal/library/library.go:285`），卡住所有刮削发布。
  2. STRM 改写再遍历一遍（`internal/app/app.go:296`、`internal/export/export.go:66`）。
  3. 对账再逐片读一遍；增量模式下对账算出的 `reusable` 结果用不上（`internal/library/scan/reconcile.go:154-189`），只剩"修复导出文件"的隐含副作用，且同时持有封面锁和导出配置锁。
  - 不确定：JPEG 重编码后字节可能变化，导致下次对账改写 poster.jpg 形成循环。
  - 改法：本地回导只在数据库为空或用户手动触发时执行，已关联完成影片的文件跳过；记录上次 STRM 改写用的 PublicURL 和 token，未变化则跳过；增量模式跳过对账修复，或拆成显式、分批释放锁的修复任务。
  - 复核（✅）：循环成立。回导对每部已导出影片解码 `poster.jpg`/`fanart.jpg`，按 JPEG 质量 90 重新编码进按内容哈希命名的缓存（`internal/image/cache.go:82-129`），并把影片封面改指向新缓存（`local_ingest.go:192-196`）；随后增量同步的对账发现缓存和 `poster.jpg` 不一致，写回新字节。用同版本 imaging（v1.6.2）实测：平滑渐变图重编码 2 次后字节稳定，接近照片的噪点图 12 次后仍在变化（合成图，未用真实封面）。后果：封面画质随启动次数下降，缓存每次启动约增加"影片数 × 3 张"，每部影片每次启动通知 Emby 两次；回导还会用 NFO 重写数据库（重建演员、标签）。STRM 改写只读全部 .strm、内容变了才写，影响小。
  - （已修复：回导跳过已完成的自有导出；增量对账只补缺失的导出；STRM 改写不变）
    - `scan/local_ingest.go`：`prepareMedia` 拆出 `identifyMedia`（文件 ID、是否 Miyabi 导出、相对路径）。新增 `settledExports`：Miyabi 自己导出的 .strm（能解析出播放地址），关联的影片已完成且封面缓存存在时，跳过读 NFO、解码封面、重写元数据、更新封面和通知 Emby，字幕照常索引，本地文件行的名称、大小、路径照常更新。封面锁只在有未跳过、带封面的条目时获取。用户配置的本地源（真实视频）不受影响。按用户确认，不再读入 Emby 写回 NFO 的元数据改动。
    - `scan/reconcile.go`：增量模式下，已完成影片只在导出目录不存在时导出（如换了 Emby 目录、手动删了导出目录），已存在的导出不读图片、不逐个比较，修复留给手动全量扫描。增量模式本来就只给待刮削影片排刮削，不用可复用结果。
    - 测试：`internal/library/scan/local_batch_test.go` 新增 `TestLocalImportSkipsCompletedExportsWithCachedArtwork`（封面已缓存：标题、封面不变，不通知，不读 NFO；未缓存：照旧从导出恢复）；`internal/library/scan/reconcile_test.go` 新增 `TestIncrementalReconcileOnlyRestoresMissingExports`（增量补齐缺失目录、不改写已存在的导出，全量扫描修复损坏的导出）。`TestLocalScannerBatchesQueriesAndPreservesRemoteFileIdentity` 的文件查询期望从每批 1 次改为 2 次（多一次已完成导出查询，仍按批固定）。反向验证：换回旧 `local_ingest.go`，第一个新测试失败；增量也总是修复（修改前的行为）或增量一律跳过，第二个新测试分别失败。`go test ./internal/...` 全部通过；子代理只读 review 未发现正确性问题，指出的"换 Emby 目录后增量不再补导出"已按上面的目录存在判断处理。
  - 遗留（低）：
    - 增量模式仍按页查询已完成影片，并对每部 `stat` 一次导出目录，不再读图片。
    - 单个导出文件损坏（目录还在）要等手动全量扫描修复。
    - 多分段影片在增量扫描中删掉一个分段后，残留的 `cd2.strm` 查不到文件行，每次启动仍按旧逻辑导入：重写这部影片的元数据并通知 Emby，直到全量扫描清掉它。因为增量不再写回封面，这种情况不会再形成劣化循环。
- [x] 🔴 ✅ **磁力缓存经常失效**：`internal/magnet/aggregator.go:192` 只要有来源失败或被拦即标记 partial；`internal/catalogue/magnets.go:53` 遇 partial 返回 `ErrDoNotCache`。一个来源长期被拦或冷却，1 分钟缓存就形同虚设，叠加 P0 的二次聚合，远程请求翻倍。
  - 改法：partial 结果也缓存，TTL 15–30 秒；内部调用方复用已选中的磁力。
  - （已修复：按用户决定改为统一长 TTL + 空结果短 TTL，不做来源/年龄分级）
    - `catalogue/service.go`：磁力缓存 `64 条 / 1 分钟` → `512 条 / 6 小时`；新增 `magnetsEmptyTTL = 5 分钟`，并更新缓存注释。
    - `catalogue/magnets.go`：不再因 partial 丢弃缓存，聚合结果（含部分来源失败、被拦、熔断冷却）一律缓存 6 小时；0 条磁力时按 5 分钟缓存；全部来源失败仍是 error、不缓存。
    - `magnet/aggregator.go`：删除"部分来源失败且未找到磁力"时返回 error 的分支（按用户决定，方案 A）。该情况改为 0 条结果 + `partial=true` + nil error，于是走 5 分钟空结果缓存、前端显示"暂无磁力"；每个失败来源仍逐条记 `slog.Warn`。只剩"所有活动来源都失败"返回 error。
    - `catalogue/cache.go`：`responseCache` 的 load 回调可返回条目级 TTL（正数覆盖默认 TTL，0 用默认），`cachedJavDB` 透传；`ErrDoNotCache` 机制保留但磁力不再使用。
    - `catalogue/magnet_settings.go`：磁力来源开关变更后 `service.magnets.reset()`。原来 1 分钟 TTL 下未处理，长 TTL 会让已关闭来源的磁力继续显示。
    - 测试：`cache_test.go` 新增 `TestResponseCachePerValueTTLExpiresSooner`（5 分钟条目到期后重载，默认 TTL 条目不受影响）与 `TestResponseCacheResetDropsInFlightLoads`（reset 之前开始的加载完成后不写回）；`javbus_test.go` 原 `TestMagnetsOptionalSourceFailureDoesNotCachePartialResults` 改写为 `TestMagnetsOptionalSourceFailureIsCachedUntilReset`（partial 命中缓存、失败来源不重查，reset 后恢复来源被重新查询）；`magnet_settings_test.go` 新增 `TestMagnetSourceSettingsInvalidateCachedMagnets`（开关注销缓存，关闭来源后返回空且不再查询）；新增 `magnets_cache_test.go`：`TestMagnetsEmptyResultsExpireAfterEmptyTTL`（一个来源失败、另一个返回 0 条：无 error，5 分钟到期重查）、`TestMagnetsTotallyFailedLookupsAreNotCached`（全失败返回 error、不缓存）；`magnet/aggregator_test.go` 原 `TestAggregatorEmptyResultWithPartialFailureFails` 改写为 `TestAggregatorEmptyResultWithPartialFailureReportsPartial`。反向验证：去掉 `magnets.reset()`、恢复 partial 不缓存、空结果改回默认 TTL、恢复聚合器返回 error，四个新测试分别失败（已实测）。`go test ./internal/...` 全部通过，`gofmt -l` 无输出。子代理只读 review 未发现缓存并发、锁序或 TTL 传参问题。
  - 遗留（中）：
    - 瞬时失败（单次超时）也会让该来源的磁力缺席 6 小时；熔断冷却只有 5 分钟，但冷却结束后也要最多等 6 小时才会重新查询该来源（没有主动刷新入口）。这是统一长 TTL 的代价。
    - 离线自动换源：部分来源失败且该番号在所有健康来源都没有磁力时，现在按"暂无其他符合偏好的磁力"把任务标记为 `Exhausted`（`internal/offline/control.go:66-73`）；以前是返回 error、等下一轮重试。只有该番号本来就没有任何磁力时才会触发，已有磁力可换源的下载不受影响。
    - 前端没有"刷新磁力"入口，用户无法提前绕过缓存，只能等 TTL 到期或重启进程（缓存是进程内存，重启即失）。
    - 部分来源被拦或冷却时，其结果会以"缺该来源"的形态缓存 6 小时；来源恢复后最多延迟 6 小时补齐。

### 影响中

- [x] 🟠 ✅ **JavBus 详情页无缓存，同一番号抓两次**：`internal/javbus/client.go:149` 注释称页面由元数据和磁力共用，但无缓存或 singleflight；`internal/metadata/providers/javbus.go:52` 和 `javbus.Find`（`client.go:128`）各自调用 `MoviePage`。
  - 改法：照 `internal/freejavbt/client.go:44-57` 加小容量 TTL 缓存 + singleflight。
  - （已修复：client 内缓存详情页 + singleflight）
    - `internal/javbus/client.go`：Client 增加 5 分钟 / 256 条详情页缓存（`detailCacheTTL`、`detailCacheCap`）。`MoviePage` 先查缓存，未命中用 `singleflight.Group.DoChan` 合并同番号请求，Do 内二次查缓存；只缓存成功且非空的 body，错误和 404 不缓存。等待者用 `select` 响应自身 ctx 的取消/超时；请求本身使用发起调用者的 ctx（首调用者取消会让等待者一起失败，错误不缓存可自愈，已加注释）。缓存满时只在插入新 key 时淘汰一条，刷新已有 key 不误删。
    - 两条路径共用同一实例（`internal/app/app.go:128` 创建，`:147` 给 catalogue、`:162` 给 metadata provider），同一番号的元数据刮削和磁力查询只发一次详情请求；磁力 ajax 仍每次查询。
    - 测试：`internal/javbus/client_test.go` 原 `TestClient_FindUsesFreshDetailParameters` 改写为 `TestClient_SharesDetailPageBetweenMetadataAndMagnets`（MoviePage 后 Find 只 1 次详情请求；TTL 内重复 Find 详情仍 1 次、ajax 递增；TTL 过期后重新抓取并使用新 gid）；新增 `TestClient_ConcurrentDetailFetchesShareOneRequest`（4 并发只 1 次详情请求）与 `TestClient_WaitingCallerHonorsItsOwnDeadline`（leader 阻塞时等待者 1 秒超时返回，释放后仍只 1 次请求）。`go test ./internal/javbus/`、`go vet` 通过；受影响包 `./internal/metadata/...`、`./internal/catalogue/`、`./internal/magnet/`、`./internal/app/` 通过；子代理只读 review 及增量复核未发现正确性问题。
  - 遗留（低）：
    - 首调用者取消会让同番号等待者一起收到取消错误；错误不缓存，下次重试可恢复。
    - 只缓存成功页面，不存在的番号（404/空 body）每次仍会请求。
    - 缓存 key 为原始 code，依赖调用方先规范化；当前元数据 provider 和 catalogue 磁力路径都会规范化。
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

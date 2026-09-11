# Miyabi 项目审计报告

- 审计模型：Claude Fabel 5.1
- 审计日期：2026-09-10
- 基线提交：`24b8925 fix: stabilize catalogue imports and playback feedback`
- 范围：Go 后端（`cmd/`、`internal/`，不含 ent 生成代码）、React 前端（`web/src`）、构建与配置文件
- 基线检查：`go vet ./...`、`go test ./...`、`go mod tidy`（零 diff）、`tsc --noEmit`、`oxlint src` 全部通过。以下均为结构、效率与一致性层面的问题，不是编译或测试错误。

---

## 一、Go 后端

### 1.1 最高影响：互斥锁跨网络调用持有

`PanService.mu` 被用作"整个 115 操作"的串行锁，而非仅保护 token 与目录字段：

| 位置                                 | 问题                                                                                    |
| ------------------------------------ | --------------------------------------------------------------------------------------- |
| `internal/service/offline.go:83-134` | `Add` 持锁贯穿 `submit`，并用 `WithoutCancel` + 2 分钟超时，浏览器关闭后仍可能锁 2 分钟 |
| `internal/service/offline.go:487`    | `Sync` 持锁翻完全部远端离线任务                                                         |
| `internal/service/pan.go:114`        | `LoginStatus` 持锁等待最长 45 秒的 token 交换                                           |
| `internal/service/library_scan.go`   | 扫描每一页（`scanPage` / `readSidecar` / `sourceInfo` / `uploadSidecar`）都持锁调 115   |

后果：文件浏览、播放启动全部串行排在扫描与离线提交之后。

建议：锁内快照 token 与 directory，锁外发请求；仅在刷新 token 时加锁（可用 singleflight）。已有 `authorizationVersion` 机制可检测并发期间的授权变更。

### 1.2 重复代码

| 优先级 | 内容                                                                                                          | 位置                                                                                                                                |
| ------ | ------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| 高     | 115 目录分页循环写了三遍（含 `total==-1` 初始化、`page.Total != total` 一致性、`offset != total` 完整性校验） | `library_scan.go:209-273`、`library_source.go:58-81`、`offline.go:255-293`                                                          |
| 高     | 115 离线任务列表翻页重复                                                                                      | `offline.go:219-237`、`offline.go:528-549`                                                                                          |
| 高     | "媒体目录"两个真相源：`PanService.directory` 内存字段 vs `loadLibrarySource()` 每次重读 SQLite                | `pan.go:49`、`library.go:68`；调用点 `library.go:82/127`、`play.go:63`、`offline.go:409/446`、`discover.go:314`                     |
| 高     | 账号校验逻辑逐字重写；`withPanToken(... client.Account ...)` 出现 6 次                                        | `offline.go:85-95` vs `library_scan.go:110-122`；`pan.go:77/145`、`pan_directory.go:40`、`offline.go:85/500`、`library_scan.go:111` |
| 中     | `javdb.getJSON` 的 `language` 参数 6 处都传 `defaultLanguage`，`transport.go:124-126` 还对空值兜底            | `movie.go:25`、`browse.go:20`、`search.go:27`、`magnet.go:21`、`tag.go:22`、`client.go:322`                                         |
| 中     | NFO 编号推导（`Normalize(doc.Code)` 为空则 `Parse(文件名)`）重复                                              | `library_scan.go:285-288`、`scrape.go:248-251`                                                                                      |
| 中     | 500 条分批循环手写 4 次，可用 `slices.Chunk`                                                                  | `offline.go:327/341`、`library_scan.go:569`、`metadata_snapshot.go:89`                                                              |
| 低     | `"magnet:?xt=urn:btih:"` 字面量重复                                                                           | `discover.go:217`、`offline.go:174`                                                                                                 |
| 低     | 错误文案"媒体目录或登录账号已变更，请重新扫描"重复                                                            | `library_scan.go:145/322`、`scrape.go:73`                                                                                           |
| 低     | "跳过 ID 为 0 的根"路径拼装重复                                                                               | `library_source.go:13-22`、`pan_directory.go:52-61`                                                                                 |
| 低     | `Cache-Control: no-store` 既有分组中间件又在 handler 内手写                                                   | `router.go:43/53/76` vs `task.go:24`、`offline.go:24/54`                                                                            |
| 低     | Zone 合法性校验三处重复，API 层 `oneof` 绑定已校验                                                            | `browse.go:56`、`search.go:64`、`tag.go:14`；`api/discover.go:31/45`                                                                |

建议：抽 `forEachPage(ctx, dir, func(page) error)`、`PanService.Source()`、`PanService.account(ctx)` 三个 helper。

### 1.3 死代码（已 grep 确认无生产调用）

- `javdb.Client.Initialize`（`client.go:103`）：仅测试调用
- `TaskService.Info`（`task.go:182`）：仅测试调用
- `javdb.SearchOptions.FilterBy`（`model.go:68`，`search.go:53-55` 分支）：生产从不设置
- `javdb.Options.Timeout/RequestsPerSecond/Burst`（`model.go:25-27`）：`main.go:51` 只传 `Proxy`，`client.go:50-64` 的负值校验仅测试可达
- `api.BadRequest`（`error.go:33`）：导出但仅包内使用
- ent `Movie.fanarts` 定义为 `[]string`，但只写一个元素（`cover.go:113`）、只读 `[0]`（`metadata_snapshot.go:129`），应改为可空 string

### 1.4 低效实现

| 内容                                                                                                                                                                               | 位置                                                                          |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| 任务 payload 双重 JSON 往返：struct→JSON→map，ent 再 map→JSON 落库；扫描每页、每目录都全量写。同时 `latestOfflineTasks` 与 `Sync` 又直接读 `Payload["hash"]` map，两套访问方式并存 | `task.go:392-414`；`offline.go:461/494`                                       |
| `Scan` 把整棵目录树装进 `observed map[string][]pan.File` 直到 reconcile，仅供比对 sidecar                                                                                          | `library_scan.go:155,229`                                                     |
| 同一页文件查两次 DB                                                                                                                                                                | `library_scan.go:81`（`identifyScanVideos`）与 `:366`（`indexScanPage`）      |
| 缺索引：`files.movie_files` 外键列（ent/SQLite 不自动建索引）被 `HasMovie/MovieIDEQ/WithFiles` 频繁过滤；`tasks.type` 几乎所有任务查询都过滤但仅有 `(status, created_at)` 索引     | `library.go:100`、`scrape.go:172`、`library_scan.go:485`；`schema/task.go:38` |
| 每次成功的 JavDB 调用都执行 `persistActiveRoute`；`DiscoverService.route` 是路由状态的第三份副本                                                                                   | `discover_cache.go:117`；`discover.go:87-88,263-269,393-416`                  |
| `LibraryService.Movies` 每页 3 次 COUNT + 1 次列表                                                                                                                                 | `library.go:92-106`                                                           |
| `metadataGroups` 对每个 scan 生成一个 `json_extract` OR 谓词，可改 `IN`                                                                                                            | `task.go:162-165`                                                             |
| `selectRoute` 用 5ms ticker 轮询取消慢探测                                                                                                                                         | `route.go:112`                                                                |

### 1.5 兜底代码

**可删或应简化：**

- `transport.go:124` language 空值兜底；`client.go:50-64` 不可达的 Options 校验；`discover.go:103-109` DeviceUUID 三级兜底（options→生成→持久化）
- `search.go:38-40` 把 `Page<=0` 置 1，而 `browse.go:27` 对同样情况报错，且 API 层已 `min=1`：不一致且多余
- `logging.go:11` 用 `slog.Level.UnmarshalText` 再解析一次级别，`config.go:126-131` 已白名单校验，二选一
- `model.go:204-215` `APIError.Error()` 四分支拼接
- `offline.go:171-217` 对"任务已存在"做 4 步深兜底：翻全部远端任务 → 查文件信息 → BFS 递归全部子目录找视频 → 删历史再提交。建议：status 完成且有 FileID 直接触发定向扫描，让扫描验证

**反向问题（fail-closed 过严）：** 整体倾向是"任一字段异常即整体失败"而非过度兜底。上游 schema 漂移会让功能整体不可用：

- 任一演员 `gender` 非 0/1 → 整个列表失败（`javdb/movie.go:163`）
- 任一推荐条目缺 `number` → 整个详情失败（`javdb/movie.go:61-67`）
- 未知 `type` → 详情失败（`javdb/movie.go:55`）
- 115 未知 `fc` → 整页列表失败（`pan/file.go:76-78`）

建议对枚举类字段降级为 `unknown` 并记日志，仅对 ID/编号缺失严格失败。

**合理防御（保留）：** `Scan` 的 `Stage=="done"` 幂等（`library_scan.go:134`）、`installFrontend` 的 SPA 回退、`errorMiddleware` 的 499 判定、`register()` 只代理 115 返回的 URL。

**codeid**（`codeid.go`）：9 个主模式 + 6 个噪声正则 + SCUTE 专用循环 + 单条目 `prefixAliases`，有 221 行测试与 fuzz。可维护但复杂度已到上限，建议不再新增个案；若要简化，先把 `numericPattern/compactDatePattern/heydougaPattern` 的"保留第二数字段"规则合并。

### 1.6 过度设计

- `worker.Pool`（`pool.go`）带 `size` 与 errgroup，但 `main.go:76-78` 固定为 1 且注释说明必须有序。退化为单协程循环即可
- `internal/api` 9 个单实现接口（`router.go:14`、`auth.go:9`、`task.go:11`、`offline.go:11`、`movie.go:11/17`、`play.go:12`、`pan.go:12`、`discover.go:12`）为可测试性设计，但该包没有任何测试。`worker` 的 `TaskStore/OfflineSyncer` 同理。要么补 handler 测试，要么直接依赖具体类型。`javdb` 的 `jsonTransport/httpClient` 有测试替身，合理
- koanf 五个模块（`go.mod:16-20`）只为 5 个键、三条来源
- `metadataSidecar` 与 `pan.File` 双结构 + `directorySnapshot` 转换（`metadata_snapshot.go:28-46`）；`OfflineSubmission.submission()` 单元素包装（`offline.go:297`）

### 1.7 最大函数

| 行数 | 位置                                         | 建议                                                                 |
| ---- | -------------------------------------------- | -------------------------------------------------------------------- |
| 194  | `library_scan.go:124` `LibraryService.Scan`  | 应拆：156-196 单文件快速路径、209-273 目录分页、276-304 NFO 兜底识别 |
| 125  | `javdb/route.go:68` `selectRoute`            | 168-191 最终选路可抽出                                               |
| 113  | `cmd/miyabi/main.go:34` `run`                | 可接受，可抽 `buildServices`                                         |
| 107  | `service/cover.go:17` `ScrapeService.Cover`  | 31-60 三路 artwork switch 抽 `resolveArtwork`                        |
| 98   | `service/offline.go:69` `OfflineService.Add` | 96-129 已有任务查询抽 `existingSubmission`                           |

---

## 二、React 前端

### 2.1 最高影响

**`keepPreviousData` 被 UI 抵消。** `api/discover.ts:159,226`、`api/library.ts:50,65` 配置了 `placeholderData: keepPreviousData`，但以下页面都用 `isPending || isPlaceholderData` 显示骨架，翻页时旧数据永远看不到：`discover/results.tsx:40`、`library/page.tsx:92`、`metadata-search-page.tsx:59`、`search/page.tsx:105`、`files-dialog.tsx:40`。`discover/page.tsx:97` 的 `key={JSON.stringify(category)}` 更是每次筛选都卸载重建。二选一：去掉 keepPreviousData，或翻页时保留列表并降透明度。

**推荐区 N+1。** `movie-detail/recommendations.tsx:66` 每张卡片各调一次 `useDiscoverMovie`，一个详情页最多 16 次完整详情请求（含演员、标签、预览图）。建议后端 `actor_movies/related_movies` 直接返回卡片所需字段。

**错误解析脆弱（唯一接近 bug 的发现）。** `api/client.ts:60` 非 2xx 直接 `response.json()`，网关返回 HTML 时抛 `SyntaxError` 而非 `ApiError`，所有 `instanceof ApiError` 分支失效。应 try/catch 后回退 `response.statusText`。

### 2.2 样式不统一

| 内容                                                                                                                                                                                                                                       | 位置                                                                                                                                                                                             |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 路由分层不一致：`routes/discover.tsx` 自己渲染 `AppPage + PageHeader` 再放 `DiscoverContent`，而 `routes/index.tsx:25`、`routes/search.tsx:31` 把整页交给 feature；`features/discover/page.tsx` 导出 `DiscoverContent`，其余导出 `XxxPage` | `routes/discover.tsx:11-17`                                                                                                                                                                      |
| 空状态表情不一致：发现页错误 `Ò︵Ó`，媒体库错误 `(･o･;)`，后者又同时用于"参数无效"和"无结果"                                                                                                                                               | `discover/results.tsx:30`、`library/page.tsx:96`                                                                                                                                                 |
| 错误 UI 五种写法：`EmptyState`+按钮（`size="sm"` vs 默认）；`text-destructive` 行内+按钮；居中段落+按钮；`variant="link" size="xs"`；`rounded-lg bg-muted` 提示块                                                                          | `results.tsx:27-38`、`library/page.tsx:81-99`、`magnets.tsx:63-83`、`discover/page.tsx:193-198`、`files-dialog.tsx:57-64`、`javdb-section.tsx:111`                                               |
| 加载 UI 四种：网格骨架、列表骨架、`LoaderCircleIcon`+文案、纯文案                                                                                                                                                                          | `access-gate.tsx:39`、`pan-directory-dialog.tsx:122`、`tasks-section.tsx:21`                                                                                                                     |
| 无 success/warning/info 语义 token，emerald/amber/sky/orange 硬编码；`**:data-[slot=progress-indicator]:bg-emerald-600 dark:...` 长串重复 3 次                                                                                             | `movie-badges.tsx:18`、`scan-progress.tsx:44`、`task-progress.tsx:38`、`pan-account.tsx:40,53`、`javdb-section.tsx:163-165`、`pan-login-dialog.tsx:116,124`                                      |
| `globals.css` 24 行零引用的 `--sidebar-*`、`--chart-*`                                                                                                                                                                                     | `globals.css:12-24, 77-84, 113-120`                                                                                                                                                              |
| `cn` 导入两套：16 个 `ui/*` 用 `from 'cn'`，8 个业务文件用 `@/lib/utils`（仅一行 re-export）；`components.json:17` 声明 `utils` 但生成组件没用                                                                                             | `lib/utils.ts`                                                                                                                                                                                   |
| 圆角漏网：主体 `rounded-2xl/4xl`，三处 `rounded-lg`                                                                                                                                                                                        | `javdb-section.tsx:111`、`previews.tsx:17`、`pan-account.tsx:40`                                                                                                                                 |
| 手写 5 个 `<hr className="border-border">`，`Separator` 已存在且在别处使用                                                                                                                                                                 | `routes/settings.tsx:27-35` vs `playback-nav-action.tsx:24`                                                                                                                                      |
| `cursor-pointer` 只在 7 处手加，其余按钮没有；应进 `ui/button.tsx:7` 基类                                                                                                                                                                  | `library/page.tsx:53`、`access-gate.tsx:50,74`、`player-status.tsx:13`、`task-toast-actions.tsx:24`、`playback-nav-action.tsx:31`、`movie-player.tsx:205`                                        |
| Button 内 `<Icon className="size-4" />` 约 30 处冗余（基类已有 `[&_svg:not([class*='size-'])]:size-4`）；`magnets.tsx:166,177` 又是裸图标；`type="button"` 时有时无                                                                        | `library/page.tsx:52`、`files-dialog.tsx:60`                                                                                                                                                     |
| `Button variant="ghost"` 再用 4 个类打掉 hover，应直接用 `<button>`                                                                                                                                                                        | `features/library/movie-card.tsx:13-16`                                                                                                                                                          |
| 命名：`movieId` vs `movieID` 混用；两个同名 `movie-card.tsx`；`DISCOVER_PAGE_SIZE as PAGE_SIZE` 别名；同 feature 内 `./` 与 `@/features/...` 混用；`components/movie/index.ts` barrel 时用时绕过                                           | `routes/discover_.$movieId.tsx`、`magnets.tsx:24`、`offline.ts:50`、`discover/page.tsx:20`、`library/page.tsx:9-15`                                                                              |
| 反馈通道混用：扫描/离线错误走 toast，其余 mutation 错误走行内文字；且 toast 在 api 层触发（api → features 依赖倒置）；`app-shell.tsx:35` 全局 `duration={6000}` 因所有 toast 自设 duration 而无效                                          | `api/library.ts:6,92`、`api/offline.ts:6,94`、`pan-directory-row.tsx:78`、`pan-section.tsx:121`、`javdb-section.tsx:115`                                                                         |
| 无意义类名：`bottom-8 … sm:bottom-8`、`gap-2 sm:gap-2`、`tracking-normal`、无人引用的 `id="movie-previews-title"`、`position === 'popper' && ''`                                                                                           | `floating-nav.tsx:27-28`、`hero.tsx:25`、`magnets.tsx:53`、`previews.tsx:7`、`recommendations.tsx:20`、`ui/select.tsx:82`                                                                        |
| JSX 属性残留空行（疑似删属性遗留）                                                                                                                                                                                                         | `search/page.tsx:67`、`pan-account.tsx:39`、`pan-directory-dialog.tsx:180,190`、`pan-directory-row.tsx:45,61`、`pan-section.tsx:39,61,75`、`ui/breadcrumb.tsx:55,66,79`、`ui/input-group.tsx:13` |

API 客户端使用一致（仅 `api/client.ts:58` 调 fetch），无需整改。

### 2.3 冗余与低效

| 内容                                                                                                                                                                                 | 位置                                                                                                                                                |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `retry: false` 11 处、`refetchOnWindowFocus: false` 9 处散落；`main.tsx:11-18` 全局 `retry: 1` 实际被全覆盖。应进 QueryClient 默认值，删 `discoverQueryDefaults`、`playQueryOptions` | `api/*.ts`、`discover.ts:139`、`play.ts:24`                                                                                                         |
| Query key 三种风格：工厂 vs 内联数组 vs 裸字符串（为躲 library.ts→pan.ts 循环依赖）；`discoverKeys.route = ['javdb','route']` 命名空间不符                                           | `auth.ts:7`、`play.ts:35,43`、`pan.ts:61-62`、`discover.ts:136`                                                                                     |
| "账号+目录是否匹配"判断 5 处                                                                                                                                                         | `api/library.ts:77-82`、`api/offline.ts:72-77,82-83`、`library/page.tsx:30-35`、`task-notifications.tsx:57-62`                                      |
| 401/400 提示逻辑两份几乎相同，文案微妙不同（"登录已失效，请前往设置" vs "授权已失效，请到设置页"）                                                                                   | `library.ts:91-105`、`offline.ts:93-106`                                                                                                            |
| `unauthorized/connected` 推导重复，应在 `usePanAccount` 的 `select` 中给出                                                                                                           | `magnets.tsx:31-32`、`pan-section.tsx:15-16`                                                                                                        |
| 页码校验三份                                                                                                                                                                         | `routes/index.tsx:8-12`、`routes/search.tsx:10-18`、`discover/metadata-search.ts:22-33`                                                             |
| 阶段/状态文案四张表                                                                                                                                                                  | `tasks/scan-status.ts:3-8`、`task-progress.tsx:6-13`、`task-toast.tsx:69`、`magnets.tsx:15-21`                                                      |
| `DiscoverResults` 三个调用点都拆 5 个 prop，而 `MovieMagnets` 直接收 `query` 对象；两种接口风格                                                                                      | `discover/page.tsx:121-131`、`metadata-search-page.tsx:57-66`、`search/page.tsx:103-112`                                                            |
| 仅内部使用却 `export` 的类型                                                                                                                                                         | `api/play.ts:7,13`、`api/discover.ts:12,27,35,42,95,123`、`api/offline.ts:23`、`discover/metadata-search.ts:4`、`ui/badge.tsx:45`、`ui/tabs.tsx:77` |
| `components/ui/textarea.tsx` 零引用（仅被同样未用的 `InputGroupTextarea` 引用）                                                                                                      | 可删                                                                                                                                                |
| `ui/sonner.tsx:36` `cn-toast` 类无定义                                                                                                                                               |                                                                                                                                                     |
| 多余 Fragment 与单子元素包裹 div                                                                                                                                                     | `library/page.tsx:107-110,122-128`                                                                                                                  |

### 2.4 包袱代码

- `api/client.ts:53-54` `?v=3` 图片缓存 buster 及注释：一次性迁移遗留，应由后端 `Cache-Control/ETag` 处理
- `routes/discover_.search.tsx:11-15` `beforeLoad` 为旧链接剥离 `zone` 参数的兼容重定向
- `routes/settings.tsx:36-40` "数据与缓存 / 即将接入" 占位 UI
- `ui/select.tsx:1` `'use client'`（RSC 残留，全仓唯一）；`ui/dialog.tsx:95` 英文 "Close"
- `/search` 与 `/discover/search` 两个"搜索"路由语义易混，可考虑 `/discover/browse`
- 干净项：stores 无迁移遗留、字段全部被读取；无 TODO、无注释代码、无 polyfill、无自写 `!important`

### 2.5 最大组件

| 文件                                         | 行数 | 建议                                                                                            |
| -------------------------------------------- | ---- | ----------------------------------------------------------------------------------------------- |
| `features/discover/page.tsx`                 | 234  | 应拆：`CategoryFilters`（90 行）+ `CategoryContent` 移到独立文件                                |
| `features/player/movie-player.tsx`           | 221  | 应拆：三处相同的 `absolute inset-0 z-20` 覆盖层抽 `PlayerOverlay`；`PlaybackQualitySelect` 独立 |
| `features/settings/pan-directory-dialog.tsx` | 219  | 可拆：`DirectoryPicker` 160 行，面包屑与分页器各成组件                                          |
| `features/movie-detail/magnets.tsx`          | 192  | 可拆：`MagnetCard` 9 个 prop，合并为一个 `offlineState` 对象                                    |
| `features/settings/javdb-section.tsx`        | 166  | 轻度：`SelectValue` 三层三元抽 `RouteSelect`                                                    |

### 2.6 Player 过度复杂

- `player/tooltip.tsx` + `player.css:86-88`：隐藏 Vidstack 自带 tooltip，再用 Radix Tooltip 把 17 个图标各包一层（每个订阅 3 个媒体状态）。用 CSS 变量给 vds tooltip 换肤可删掉 tooltip.tsx、17 个 `withPlayerTooltip` 包装与两段 CSS
- `player/controls-visibility.tsx:21-55`：改写 `controls.canIdle` 并在捕获阶段拦截 Vidstack 内部 `media-pause-controls-request` 事件，升级易碎
- `player.css:118-175`：用 `:first-child / :nth-last-child(2)` 绑定 Vidstack DOM 顺序重排控制栏，同样脆弱
- `player/icons.ts`：`DownloadButton`（:65）从未渲染；`GoogleCastButton`（:38）被 `slots.googleCastButton: null` 关闭；Captions/FontSize/Opacity/Chapters 在无字幕无章节的 HLS 源下不出现。可引用 `defaultLayoutIcons` 再覆盖需要的键
- `player/translations.ts`：用非 Partial 的 `DefaultLayoutTranslations` 被迫写满 57 键，prop 接受 `Partial<>`，约 20 个键实际不会显示

---

## 三、配置、测试数据、可精简文件

### 3.1 配置

- 5 个选项全部使用：`Listen`、`DataDir`、`LogLevel`、`Proxy`、`AccessPassword`（`cmd/miyabi/main.go:40-96`）。README 环境变量表与 `config.go` 名称、默认值完全一致
- **三条配置路径并存，两条无人知晓**：README/Dockerfile/compose 只提 `MIYABI_*` 环境变量；`config.go:18,56` 同时加载 `config.toml`；`config.go:98-102` 提供 4 个 CLI flag。`config.example.toml` 漏了唯一被 README 强调的 `access_password`；CLI 也没有 `-access-password`。建议收敛到环境变量 + 可选 `-config`，删除其余 flag 与 `set` 映射（`config.go:68-83`）；若保留 TOML，补全示例与 README
- `.env.example` 不被二进制消费，仅服务 `docker-compose.yml:12-14` 的变量插值
- `config_test.go` 只测 `access_password` 三种优先级，未覆盖 CLI override、`validate()` 分支、`-config` 指定缺失文件应报错

### 3.2 测试数据：保留

`internal/javdb/testdata/*.json` 9 个文件共约 7 KB，全部为合成数据：ID 形如 `movie-exact`，URL 用 `media.example`，magnet hash 全零，厂牌 `ExampleStudio`。不含 token、个人信息或 NSFW 内容。仅 `endpoint_test.go:44` 的 `fixtureFile` 使用。无需裁剪。

测试覆盖重复：`endpoint_test.go:152/168/181/241/258`、`wire_test.go:98/119` 共 7 个测试都在断言 "number 原样透传"，而 `movieFromWire` 只做 `TrimSpace`，真实逻辑在 `codeid_test.go`；`ResolveMovieID` 有 6 个重叠用例（`endpoint_test.go:316/375/391/413/438`、`wire_test.go:138`）。建议合并为表驱动。`signature_test.go` 只把常量哈希与自身比对。无测试的包：`api`、`worker`、`image`、`database`、`logging`。

### 3.3 文件去留

| 文件                                 | 建议                                            | 理由                                                                                                                                                                                                                              |
| ------------------------------------ | ----------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.example.toml`                | 删除，或补全 `access_password` 并在 README 说明 | 现状是半成品                                                                                                                                                                                                                      |
| `web/pnpm-workspace.yaml`            | 删除，同步删 `Dockerfile:8` 的 COPY             | 无 `packages:`，仅 `minimumReleaseAgeExclude`；仓库与全局均未设 `minimumReleaseAge`，当前零效果                                                                                                                                   |
| `Makefile`                           | 重写                                            | `install` 目标每次 `go get @latest` 会篡改 go.mod 版本且列表不全，应删；`build/test` 的 web-build→generate→go 顺序有价值（`embed.go:10` 要求 `web/dist` 存在）；`dev/web-build/lint/format-check` 是 pnpm 脚本薄包装；本机无 make |
| `.env.example`                       | 保留                                            | compose 必需配套                                                                                                                                                                                                                  |
| `docker-compose.yml`                 | 保留，README 补一节 compose 用法                | 否则与 `docker run` 纯重复                                                                                                                                                                                                        |
| `.github/cliff.toml`                 | 保留                                            | `docker-publish.yml:47-49` 引用                                                                                                                                                                                                   |
| `embed_dev.go`                       | 保留                                            | `package.json:8` 的 `go run -tags=dev` 依赖，`router.go:88` 已处理 `Frontend == nil`                                                                                                                                              |
| `web/src/components/ui/textarea.tsx` | 删除                                            | 零引用                                                                                                                                                                                                                            |

### 3.4 Dockerfile / compose / CI

- `Dockerfile:26` `COPY . .` 把 `web/src`、`pnpm-lock.yaml`、`*_test.go`、`testdata` 都复制进 Go 构建阶段；可收窄为 `COPY cmd internal embed.go embed_dev.go ./`
- `Dockerfile:34-41` `ARG/LABEL` 位于 `RUN apt-get`（:43）之前，每次 tag 变化使 apt 层失效；且 workflow 已用 `docker/metadata-action` 注入同一组 OCI label（`docker-publish.yml:173`），Dockerfile 中的重复可删或移到末尾
- `Dockerfile:43-44` `debian:bookworm-slim + curl` 仅为 HEALTHCHECK；`CGO_ENABLED=0` + modernc sqlite 是静态二进制，可用 distroless/static 并在二进制加 `-healthcheck` 子命令
- `Dockerfile:62` healthcheck 硬编码 `127.0.0.1:8080`，覆盖 `MIYABI_LISTEN` 时失效
- `.dockerignore`：`.agents/.codex/.vscode/AGENTS.md`（:3-5, :36）在仓库中不存在，陈旧；`.pnpm-store`（:17）无任何东西会生成；未排除 `**/testdata`、`**/*_test.go`；`**/data`（:13）过宽
- `.gitignore`：缺 `web/.tanstack/`（目录已存在）；冗余 `/.pnpm-store/`（:24）、`config.local.*`（:14，config.go 不加载）
- `docker-publish.yml`：所有 action（`actions/checkout@v4`、`docker/*@v3/v5/v6`、`orhun/git-cliff-action@v4`）未 SHA 固定；`version` job 仅正则校验可内联；`create-draft-release` 的 checkout（:62-63）无必要；build 矩阵预热缓存 + QEMU 组装 manifest 的思路合理，可用官方 `push-by-digest + imagetools create` 模式替代

### 3.5 依赖与前端工具链

- Go：`go mod tidy` 零 diff，155 个模块，直接依赖全部使用
- `shadcn` 依赖按用户确认保留，`globals.css` 直接引入官方 `shadcn/tailwind.css`，后续升级继续随包获取状态样式，避免维护复制的 CSS。
- `@tanstack/router-cli`（`prebuild: tsr generate`）与 `@tanstack/router-plugin` 重叠；只因 `build: "tsc && vite build"` 先跑 tsc 而 `routeTree.gen.ts` 被 gitignore。改为 `vite build && tsc` 可删 router-cli
- `tsconfig.json:18` `types: ["node"]` 无人使用 node API；`vite-env.d.ts` 与 `types: ["vite/client"]` 重复；缺 `noUncheckedIndexedAccess`（`task-progress.tsx:34` `visible[index].label` 在 index=-1 时会崩）
- 复核更正（2026-09-11）：Oxlint 内置 `react` 插件已提供 `exhaustive-deps` 和 `rules-of-hooks`，原审计关于 Hooks 完全无检查的判断不准确。现显式将两条规则设为 `error`，无需增加插件依赖。
- `components.json`：`hooks: "@/hooks"` 目录不存在；`utils` 别名与实际 import 不符
- `vite.config.ts`：已按 hls/movie-player/react 拆包，无需手动 manualChunks；proxy 硬编码 `127.0.0.1:8080`

---

## 四、建议整改顺序

1. 拆解 `PanService.mu` 跨网络持锁（1.1）
2. 统一 115 分页循环、账号校验、媒体目录访问 helper（1.2）
3. 前端 `keepPreviousData` 与骨架二选一；推荐区 N+1 改后端返回卡片字段；修 `client.ts` 错误解析（2.1）
4. 任务 payload 去掉双重 JSON 往返；`Scan` 不再全量缓存 `observed`；补 `files.movie_files`、`tasks.type` 索引（1.4）
5. 上游枚举字段降级而非整体失败（1.5）
6. 删除死代码与不可达兜底（1.3、1.5）
7. 前端：api 层去 toast、合并 5 处账号匹配与 2 处 401 处理、QueryClient 默认值、统一空状态/错误组件、补 success/warning token（2.2、2.3）
8. 配置收敛到环境变量；`worker.Pool` 退化为单循环；api 接口补测试或去掉（1.6、3.1）
9. 文件删减与 Dockerfile/CI 收紧（3.3、3.4）
10. 显式配置 Hooks lint、精简重复的路由构建工具；保留 shadcn 官方样式依赖以及播放器 Tooltip 与图标（2.6、3.5）

---

## 五、后续落实记录（2026-09-11）

本轮基于 `9d7ba47`，完成剩余查询与序列化优化，并调整媒体库卡片展示。前文保留原始审计基线的结论。

- 媒体库只统计影片总数，列表只读卡片所需的影片字段和标签。正常分页固定四次查询（挂载信息、统计、影片、标签），不再读取每部影片的文件明细。卡片显示标题；下方依次展示首个标签及剩余数量 `+N`、刮削状态、观看状态。标签没有 tooltip，尚无标题时显示番号。
- 新增持久化观看状态，以打开播放器为已观看；写入只访问当前挂载目录的本地索引，并与挂载信息读取置于同一事务。重复打开不更新记录或广播重复变更，元数据更新和重扫保留观看状态。旧数据库自动补充字段，因没有历史观看记录，已有影片初始为未观看。前端同步当前缓存并重试短暂写入失败。
- 已刮削和已入库徽章统一使用 emerald-500，未观看使用 violet-500；刮削失败使用指定的 destructive 色值，亮暗主题一致。移除未识别文件预览、对应接口及文件数量统计；保留扫描核对和下载状态所需的内部索引。
- 移除视频最小大小配置，统一固定为 100 MiB（104857600 字节）。文件名识别、离线任务身份继承、NFO 关联和目录共享判断使用相同规则；小于阈值的辅助视频不会参与影片识别或刮削，重扫会修正旧误关联。
- 离线历史按账号及目录或影片范围，在 SQLite 内选出每个磁力最新的任务。新增两个仅针对离线任务的 JSON 表达式索引，按范围筛选并直接按索引顺序分组。保留没有时间或条数上限的长期未完成任务；异常 hash 类型仍会报错。
- 扫描在同一事务中读取文件和已知影片身份，复用于关联写入；已有影片 ID 不再重复查询。NFO 补充识别后的延迟写入仍重新读取当前关联，保留账号、目录、身份和回滚校验。
- 任务 payload 改为原始 JSON，取消结构体与 map 之间的序列化往返。数据库中的 JSON 对象结构和可选字段语义保持兼容；局部进度更新保留未知字段，大整数 ID 不再经过 float64。
- 播放器继续使用项目的 shadcn Tooltip 和 Lucide 图标定制。原审计中的替换建议需结合这项界面要求评估。

同机合成基准（Windows amd64、i5-13490F、Go 1.27.1，单次 300 ms 采样）：

| 场景 | 优化前耗时 | 优化后耗时 | 优化前分配 | 优化后分配 |
| --- | ---: | ---: | ---: | ---: |
| 媒体库分页：500 部影片、5000 个文件、每页 24 部 | 2.26 ms | 1.67 ms | 202615 B/op | 67701 B/op |
| 离线历史：5000 条记录、50 个不同磁力 | 27.97 ms | 1.69 ms | 9015386 B/op | 114486 B/op |
| 封面任务 JSON 编码 | 58.34 μs | 10.53 μs | 23241 B/op | 5199 B/op |
| 封面任务 JSON 解码 | 76.54 μs | 19.15 μs | 25965 B/op | 7859 B/op |
| 重扫识别与写入：100 个已有视频 | 4.18 ms | 3.65 ms | 554015 B/op | 259379 B/op |

基准只测本地数据库和数据转换，不包含 115/JavDB 网络耗时。复现命令：

```sh
go test ./internal/service -run '^$' -bench 'Benchmark(LibraryPage|OfflineActivityHistory|TaskPayload|IdentifyAndIndexScanPage)$' -benchmem -benchtime=300ms -count=1
```

验证：Go 全量测试、`go vet ./...`、27 个前端测试、TypeScript、lint、格式检查和生产构建通过。新增回归覆盖标签与分页、账号目录隔离、观看状态的幂等写入与重扫保留、旧数据库观看字段迁移及重启保留、固定大小边界、长期下载与历史筛选、旧 JSON 重启读取、表达式索引迁移与查询计划、扫描身份复用与事务回滚。页面交互由用户手动测试。

## 六、配置与开发启动收敛（2026-09-11）

- 启动配置统一为环境变量覆盖默认值，使用 Go 标准库读取五个 `MIYABI_*` 变量。删除 TOML 加载、命令行配置参数、配置示例及 `koanf` 相关依赖。本地默认监听 `:8080`、数据目录 `./data`、日志级别 `info`。
- 保留 `healthcheck` 子命令，与应用读取相同环境变量。旧配置参数或未知命令在启动服务前明确报错并提示使用环境变量；密码按原样读取，空密码关闭访问门禁。
- 移除 `pnpm dev:all`、`concurrently` 及其独占依赖，锁文件保留其余依赖的已锁定版本。前后端分别启动：前端使用 `pnpm dev`，后端使用 GoLand 或 `go run -tags=dev ./cmd/miyabi`。README 补充本地开发方式和旧配置迁移说明。
- 验证：Go 全量测试、`go vet ./...`、开发模式编译、27 个前端测试、TypeScript、lint、格式检查、生产构建及冻结锁文件检查通过。配置回归覆盖默认值、环境变量覆盖与规范化、密码空白保留、可选空值、非法必填值、忽略旧 TOML、拒绝旧参数，以及健康检查不初始化应用数据。

## 七、样式与前端工具链收敛（2026-09-11）

- 新增统一的整页错误和行内错误组件，统一空状态表情、重试按钮尺寸及等待状态。发现页布局收回 feature，与媒体库、搜索页保持一致。设置页统一使用 `Separator`，预览图、骨架和容量条统一圆角，按钮与交互控件集中管理指针样式。
- 增加 success、warning、info 语义颜色，进度条通过 `variant` 选择颜色。已刮削和已入库仍为 emerald-500，未观看仍为 violet-500，刮削失败继续使用指定的 destructive 色值。删除未使用的 sidebar 变量和无定义的 toast 类名；chart 的五组主题映射及亮暗颜色完整保留，供后续图表使用。组件的 `cn` 导入统一为 `@/lib/utils`。
- 保留 `shadcn` 依赖及官方 `shadcn/tailwind.css` 导入，后续升级无需手动同步 CSS 副本。保留现有 shadcn/Radix 组件、播放器 Tooltip 和 Lucide 图标。
- 移除重复的 `@tanstack/router-cli` 和 `prebuild`，构建改为先由 Vite 插件生成路由并打包，再执行 TypeScript 检查。删除无实际作用的 `pnpm-workspace.yaml`，同步 Dockerfile，并忽略 `.tanstack` 缓存。路由 CLI 及其独占依赖共减少 14 个锁定包版本，其余依赖版本保持不变。
- 声明 Node.js 24 起和 pnpm 10.33.0，README 补充开发检查命令。移除重复的 Vite 类型声明和前端全局 Node 类型注入；开启 `noUncheckedIndexedAccess`，修复首个标签读取及任务阶段索引问题。未知阶段显示“等待进度同步”和不定进度，异常百分比限制在当前阶段范围内；新增四个回归测试。Hooks 检查使用 Oxlint 已有的内置 React 规则。
- 按用户要求清除 shadcn 组件之外手写的 ARIA/role 属性及相关样式选择器；播放器菜单开关通过颜色变量保留开关状态区分。第三方依赖自动生成的属性仍由各组件库管理。

验证：31 个前端测试、TypeScript、Oxlint、格式检查、Go 全量测试及 `go vet ./...` 通过；冻结锁文件检查通过，剩余 563 个依赖快照的引用全部完整。移除生成的路由文件后，Vite 能重新生成路由并完成生产构建，编译 CSS 包含所需颜色和组件状态变体。本机 Windows 沙箱下默认配置打包遇到子进程 `EPERM`，验证使用 `vite build --configLoader native` 后接 `tsc --noEmit`；仍有已有的播放器大分块提示。页面交互由用户手动测试。

## 八、观看历史与续播（2026-09-11）

- 新增 `/history` 路由及 floating-nav 入口，保留本地 jm-boom 书架页的历史管理、观看时间和封面底部细进度条交互。卡片、响应式网格与加载骨架复用媒体库组件，每页 20 条，按最近观看时间排序；支持选择本页记录、清除选中和清除全部，操作前展示确认弹窗。
- 新增 SQLite 观看历史表，以账号、挂载目录、影片为唯一记录，并建立账号/目录/时间索引。列表只读取当前目录仍可播放影片的卡片字段和进度，正常分页固定四次查询。重复打开更新历史时间和播放会话，保留已有文件与播放位置；媒体库已观看字段只在首次打开时更新。
- 播放器记录实际文件、播放位置和视频时长，每 10 秒保存一次，并在跳转进度、暂停、结束、隐藏页面和关闭播放器时补写。关闭与页面退出使用 keepalive，续播匹配原文件，已播放完成或原文件不存在时从头开始。
- 每次打开生成新的播放会话标识，每次进度请求递增版本；旧会话及乱序请求不能覆盖新进度，正常向后拖动仍可保存。进度只更新已有记录，清除后的延迟请求不会重新创建历史。清除操作核对请求中的账号和目录，保留影片文件及已观看标记。
- 历史变化使用独立的 SSE 版本号，只刷新历史相关前端缓存，不额外唤醒后台任务。观看状态更新限定到媒体库列表缓存，避免将历史分页当作影片分页修改。沿用现有 shadcn 组件、颜色变量和图片隐私设置，未增加业务 ARIA 属性。
- 旧版本只有已观看标记，没有真实观看时间及进度。迁移保留这些标记，新的历史从升级后的观看操作开始积累，不补造历史时间。

验证：35 个前端测试、Go 全量测试、`go vet ./...`、TypeScript、Oxlint、格式检查和生产构建通过。新增回归覆盖时间排序与分页、查询次数、账号/目录隔离、文件校验、重复打开续播、旧会话与版本保护、进度范围、节流和退出补写、清除后防恢复、旧库升级与重启持久化、索引使用及影片删除级联。生产构建使用本机可用的 Vite 原生配置加载模式；页面交互由用户手动测试。

## 九、封面加载与卡片统一（2026-09-11）

- 推荐卡片复用 `MovieCardSkeleton`，等待详情信息与封面加载、解码完成后一次显示整张卡片。详情未就绪时不提前加载缩略图，封面仍优先使用详情封面；保留可见区域延迟请求、共享并发限制、缓存复用和点击优先加载。
- 图片资源按 URL 与隐私模式隔离，过期加载结果不会影响新资源。空封面、图片失败和隐私模式均正常进入占位展示，详情失败保留重试入口。推荐封面在详情就绪后主动加载，避免卡片隐藏期间懒加载停滞；其他页面继续使用原有图片加载策略。
- 历史页复用 `MovieCard`、`MovieGridLayout` 和 `MovieGridSkeleton`，统一横向封面、编号位置、字号、间距与圆角。封面底部增加进度条，保留观看时间与选择清除功能，移除独立的卡片悬停阴影。
- 验证：35 个前端测试、TypeScript、Oxlint、格式检查和生产构建通过；推荐请求的延迟触发、并发限制、缓存复用、离屏取消及失败重试回归继续通过。生产构建使用 Vite 原生配置加载模式；页面交互由用户手动测试。

## 十、下载状态核对与播放器焦点（2026-09-11）

- 核对 PPPE-440 的本地任务 59、运行中接口与 115 实时任务。13:48 的上游状态仍为 `downloading`，实际进度 82.0116%，下载速度与连接数均为 0；本地记录的整数进度为 82%，toast 与上游一致。没有将尚未完成的任务强制标记为完成。
- 播放弹窗打开时聚焦播放器或弹窗容器，避免自动落到加载界面的关闭按钮；播放器首次可播放时将焦点交给播放区域。正常拖动进度或切换清晰度不会反复抢焦点，重试成功后恢复播放器焦点。
- 验证：35 个前端测试、TypeScript、Oxlint、格式检查和生产构建通过。下载状态通过实时只读查询核对；播放器键盘交互由用户手动测试。

## 十一、播放器长按三倍速与提示统一（2026-09-11）

- 右方向键确认短按后，将单组按下与松开事件交回 Vidstack，复用原生快进图标、动画和滑块处理；快进与倒退步长统一为 5 秒。按住 500 ms 后临时切换为三倍速，并在播放器顶部中央显示“3 倍速”。长按期间拦截重复按键，松开后恢复按住前的速度，不再追加快进；提示与播放器一起进入全屏。
- 窗口或播放器失焦、页面隐藏、暂停、播放结束、错误、切换片源及关闭播放器均取消长按并恢复速度。取消后继续拦截本次按键的重复与松开事件，避免恢复焦点时误快进。菜单、滑块、输入控件及其他快捷键保留各自的键盘行为。
- 按钮、时间进度和音量提示共用 shadcn Tooltip，统一背景、文字、圆角与箭头，并挂载到播放器内以保持暗色主题及全屏行为一致。滑块提示以轨道上的指示点为定位基准，随指示点移动，不再由整个点击区域的高度决定上移距离；窄屏竖向音量条的提示显示在侧边。滑块继续使用 Vidstack 的拖动、键盘、章节与数值格式化组件，移除旧数值预览的重复样式和偏移覆盖。
- 验证：44 个前端测试、TypeScript、Oxlint、格式检查和生产构建通过。新增九个回归覆盖短按转交原生反馈、连续短按及防重复跳转、长按与重复按键、原倍速恢复、延迟速度事件、失焦及播放中断、漏掉松键后的再次按下、控件键盘操作与卸载清理。播放器实际交互由用户手动测试。

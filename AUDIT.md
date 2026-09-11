# Miyabi 项目审计报告

- 审计日期：2026-09-11
- 当前分支：`master`（基线 `9ffb8e4 update README.md`，工作区含长按倍速 WIP）
- 范围：Go 后端（`internal/service`、`internal/api`、`internal/pan`、`internal/javdb`、`internal/database`、`internal/image`、`internal/worker`、`internal/nfo`、`internal/config` 等，约 1.8 万行非生成代码）+ React 前端（`features`、`api`、`components`、`stores`、`lib`，约 1 万行）
- 方法：四个并行深度审查（Go service/api 层、Go 基础设施包、前端 player/hooks/api 层、前端 features/组件/样式），全部高严重度发现均经源码逐条交叉验证
- 结论：**整体质量明显高于平均水平**——测试覆盖扎实、并发纪律好、SSE/定时器/事件清理完备、Docker/CI 规范。以下为结构、效率、一致性层面的优化点。

---

## 一、总体评价

- 强测试覆盖：Go 单元测试 + 前端 `node:test`（含长按倍速的 DOM 注入式单测，场景覆盖很全）。
- 并发正确性良好：`discover_cache` 的 singleflight + 引用计数、javdb 的 `atomic.Pointer` 路由选择、pan 客户端构造后只读，经核查无 data race。
- 批处理与预加载到位（500/页分页、`walkFilePages` 完整性校验、ent 预加载），**未发现 N+1**。
- Dockerfile 多阶段 + BuildKit 缓存 + 非 root + healthcheck；CI 预热缓存/多架构发布流水线设计讲究。
- 问题集中在四类：**重复实现、失败路径浪费、防御策略不一致、契约未兑现**。

---

## 二、🔴 高优先（建议尽快处理）

### 2.1 [Go] API 层「bind + 错误处理」样板重复 ~30 处

`internal/api/auth.go:28`、`discover.go:61,86,105,133,149,165,181,215`、`movie.go:31,53,71`、`pan.go:61,88,104`、`offline.go:41,46,63,68`、`play.go:25,43,62,91` 等，每个 handler 都是同一段 `c.ShouldBindJSON` → `c.Error(BadRequest(err))` → `c.JSON` 模板。

- `c.Error(BadRequest(err))` 共 **29 次**
- `if err != nil { c.Error(err); return }` 共 **31 次**

**改法**：抽两个公共 helper（`bind(c, target, kind) bool` 与 `respond(c, value, err)`），可删约 120 行样板，handler 平均缩到 4-6 行。全项目最划算的一次重构。

### 2.2 [Go] `internal/image/cache.go:97` —— `os.Rename` 在 Windows 上是真实竞态

```go
if _, err := os.Stat(destination); err == nil { return ... }   // TOCTOU 预检
...
if err := os.Rename(temporary.Name(), destination); err != nil { return ... }
```

`os.Rename` 在 **Windows 上目标存在会直接报错**（POSIX 是原子覆盖）。两个并发保存同一封面（扫描 + 刮削同片）、或重复抓同一 artwork 时，第二个 Rename 必然失败。dev 环境跑在 win32，问题真实存在。`saveArtwork` 还可能留下「poster 写成功、fanart 失败」的半套状态。

**改法**：Rename 失败后回查 `os.Stat(destination)`，已存在视为成功（缓存幂等），天然支持并发写同一内容。

### 2.3 [Go] `internal/pan/offline.go:35` —— 注释承诺与实现相反

```go
// 115 can return fractional percentages or numeric strings ... so one download cannot reject a page.
...
progress, err = result.Progress.Float64()
if err != nil { return fmt.Errorf("decode 115 offline progress: %w", err) }
```

进度字段一旦不是合法 JSON number（带 `%` 的字符串、对象等），`UnmarshalJSON` 报错 → **整页解码失败 → 离线同步循环中断**（连「查找重复任务」都被阻塞）。`json.Number` 根本接不住注释里说的「numeric strings」。

**改法**：失败时降级 `progress = 0` + `slog.Warn`，永不返回错误，让契约成立。

### 2.4 [Go] 空 source 契约不一致 + `fs.ErrNotExist` 污染中文错误

同一前置条件 `source == nil`，四个端点行为各异：

| 位置 | 行为 |
|---|---|
| `service/library.go:84` | 返回空列表 |
| `service/watch_history.go:123`、`movie_state.go:39` | 返回空结果 |
| `service/play.go:67-68` | **返回 400 错误** `ErrMediaDirectoryRequired` |
| `service/offline.go:439` | 返回空 |

前端无法统一处理。同时 `service/play.go:104,197,206` 用 `fmt.Errorf("...请重新扫描: %w", fs.ErrNotExist)` 包装来触发 404 映射，用户看到「…请重新扫描: **file does not exist**」，英文底层错误混进中文消息。

**改法**：统一空 source 契约（要么全空、要么全 `ErrMediaDirectoryRequired`）；引入 `StatusError` 接口（sentinel 自带 HTTP 状态码），替换 `api/error.go:56-67` 的手工 switch——新增 sentinel 时漏加会静默降级为 500（现已有 `errPanSourceChanged` 就漏在里面）。

### 2.5 [前端] `player-dialog.tsx:50` —— 播放启动被「标记已看」POST 阻塞

```tsx
historyReady={watched.variables === movieID && (watched.isSuccess || watched.isError)}
```

播放器要等到 `POST /library/movies/{id}/watched` **settle** 才显示。该 mutation 配置了 retry（5xx/断网最多 3 次，`api/library.ts:55`），意味着每次打开视频都可能卡住数秒、视频连缓冲都开始不了。设计上用一次写请求顺带取续播位置很巧妙，但把播放启动绑死在写接口上风险太大。

**改法**：`historyReady` 只依赖 `watched.variables === movieID`（请求已发起即可），播放先用缓存续播；或把取历史与标已看解耦。

### 2.6 [前端] `use-hold-speed.ts:109-110` —— 与 Vidstack 键盘控制的时序竞争（当前 WIP）

在 `player.el` 上注册**捕获** `keydown`。关键细节：**当事件目标就是 `player.el` 本身时**（代码明确在 `canPlay` 后 `player.el.focus()`），DOM 规范按**注册顺序**触发捕获/冒泡监听器——若 Vidstack 的冒泡 `keydown` 先注册（它在 `focusin` 激活时注册，时机与 `useEffect([player])` 竞态），Vidstack 的 `ArrowRight → seek(+5s)` 会先执行，`stopImmediatePropagation()` 已无法撤销。若目标是 `player.el` 的**后代**，捕获阶段则保证先赢。窄窗口但致命的时序依赖。单测 fixture 里没有 Vidstack，覆盖不到。

**改法**：把 `keydown` 绑到 `document` 捕获阶段（配 focus/`closest` 守卫），或显式关掉 Vidstack 内建 ArrowRight 快捷键，并用真实浏览器验证长短按行为。

### 2.7 [Go] `internal/service/offline.go:475-486` —— 无界历史查询

`Activity` 每次 UI 轮询都对该账号**全部历史磁力任务**（含大量带 `file_ids` 数组 payload 的已完成任务）做 group-by 并逐个 `decodeTaskPayload`。历史只增不减，查询与解码成本随使用时间线性增长。注释承认是刻意取舍，但「无 LIMIT」+「轮询触发」叠加就不该是取舍。

**改法**：加时间/条数上限，或对终态任务跳过 payload 解码。

### 2.8 [前端] `discover/results.tsx:39` —— 用「满页猜法」判分页

```tsx
hasMore={movies.length === DISCOVER_PAGE_SIZE}
```

最后一页恰好 20 条时，「下一页」仍可点，进入**空白页**（page>1 时走不到 EmptyState）。而 library 用服务端 `has_more`、history 用 `total` 反推，三条链路三种语义。

**改法**：后端列表接口补 `has_more`（与 `/api/library/movies` 对齐），或至少 `page > 1 && movies.length === 0` 时渲染 EmptyState 兜底。

### 2.9 [前端] `components/media-image.tsx:21` —— NSFW 隐私切换整页图片重建

```tsx
key={JSON.stringify([props.source, props.original, nsfwMode])}
```

`nsfwMode` 一变，页面上每张图都拿到新 key 被销毁重建（重新解码、触发 `onReady` 连锁）。隐私开关恰恰是最该秒切的操作。

**改法**：key 只用 `source`，`concealed` 只影响 `showImage` 派生值，图片节点保持常驻（可用 CSS/opacity 过渡隐藏）。

---

## 三、重复代码 / 重复实现（中低优先）

| 位置 | 问题 | 建议 |
|---|---|---|
| `internal/pan/*`（client/account/file/metadata/offline/play/upload） | `json.Unmarshal` + `result.err()` + 包装样板重复 **11 处**（已有泛型 `authRequest[T]` 可仿照） | 抽 `apiRequest[T]`，各缩为 1 行 |
| `pan/client.go:29-43`、`javdb/transport.go:40-59`、`javdb/media.go:20-29` | HTTP client 构造块重复 4 处（底层库不同：fhttp 用于 TLS 指纹、resty 常规，不可强并） | 抽统一构造工厂 |
| `service/library_source.go:13-22` vs `pan_directory.go:53-62` | 路径拼接逻辑两份（后者还有越界 bug，见 5.5） | 抽 `directoryPath()` |
| `service/library_scan.go:236-263` vs `scrape.go:256-291` | NFO 读取+解码+番号归一逻辑两份 | 抽 `readNFO()` |
| `api/router.go:43-46,57-60,81-84` | 三段一模一样的 no-store 中间件 | `noStore()` 一次定义 |
| `pan_state.go:11` vs `library_scan.go:116`、`scrape.go:72` | 「媒体目录或登录账号已变更」文案 3 份，后两处绕过 sentinel → 落到 500 | 复用 `errPanSourceChanged` |
| `config/config.go:50-55` vs `logging/logging.go:11-13` | 日志级别校验两份，易漂移 | 委托一处 |
| `features/tasks/task-progress-state.ts:5` vs `scan-status.ts:3` | 同一 scan 任务两套中文标签/阶段映射 | 合并为单一数据源 |
| `lib/watch-progress.ts` vs `features/player/watch-progress.ts` | **命名撞车**（纯函数 vs 写入器工厂）+ `formatWatchTime` 应在 `lib/format.ts` | 移动/更名 |
| `movie-player.tsx:113`、`watch-progress.ts:84`、`lib/watch-progress.ts:5`、`use-hold-speed.ts:103`、`task-progress-state.ts:21` | `clamp` 手写 5 处 | `lib/utils.ts` 加 `clamp()` 统一替换 |
| `history/history-card.tsx:40`、`library/movie-card.tsx:13`、`components/movie/movie-card.tsx:16` | 三种「整卡可点击」外壳各自实现 | 抽 `CardButton`/`CardLink` |
| `library/page.tsx:89-128`、`history/page.tsx:152-192`、`discover/results.tsx:27-44` | 「加载/错误/空/列表」四态状态机复制 3 份 | 抽 `AsyncListState` |
| `movie-grid.tsx` 写死 `DiscoverMovieCard`，library/history 各自 `map` | `MovieGrid` 无法复用 | 加 `renderItem` 入参 |

**已排除的「疑似重复」**（核查过，不是问题）：两个 `watch-progress.ts` 是互补关系；`movie-state-cache` 与 `movie-detail-cache` 算法不同、只是外形相似；`movie-cover.tsx` 是 `media-image.tsx` 的薄封装而非重复；`empty-state.tsx` 与 `error-state.tsx` 层级合理（后者组合前者）。

---

## 四、性能低效

### 后端

- **DB 无连接池**（`database/sqlite.go:32-41`）：`MaxOpenConns` 默认无上限，modernc sqlite 每连接一个文件句柄，并发下连接数线性增长、放大 `SQLITE_BUSY`。→ `SetMaxOpenConns(4)` + `SetMaxIdleConns(4)` + `SetConnMaxLifetime`。
- **路由探测 N+1 个 tls-client**（`javdb/route.go:310-315`）：每个候选 host 一个完整 tls-client；`Reselect` 的 full 模式无淘汰 ticker，半死主机可阻塞约 **60s**（`client.go:249-271`）。→ probe 复用客户端池 + full 模式也开淘汰 ticker。
- **先 `io.ReadAll` 再判状态码**（`javdb/transport.go:82-89`）：对 502/503（正是要重试的）先下载完整错误页再丢弃。→ 先判状态码，非 2xx 直接关闭 body。
- **限速双重等待**（`pan/metadata.go:78-83`）：`DownloadURL` 内部已 `limiter.Wait`，外面又等一次；而真正的大流量路径 `OpenMedia`（`play.go:84-107`）反而**不限速**，同包内限速哲学不一致。→ 统一策略。
- **`pan/upload.go:37`**：≤128KiB 时 `fileid`/`preid` 对同一段数据算两次 SHA1。→ `len<=128K` 时复用 `fileid`。
- **`worker/offline.go:14-25`**：Ticker 循环中 `Sync` 超 30s 时积压 tick 导致背靠背执行。→ 改用 `timer.Reset`。
- **`nfo/movie.go:109`** `Encode` 双重拷贝（`append` 后再整体拷贝）。→ 用 `bytes.Buffer`。
- **`image/cache.go:63`** fanart 原尺寸入库（源封面常 1080p+，可能数 MB），poster/thumbnail 都缩了唯独它没有。→ `Fit` 到合理上限。

### 前端

- **`discover.ts:176-196`** `findCachedMovieCard` 每次对全部 discover query 全表遍历 → 推荐区 8 卡 × 每次缓存事件 O(N·M)。→ 改用 `queryClient.getQueryState` 直达单条。
- **`movie-state-cache.ts:73-77`** `staleTime: Infinity` + `refetchOnMount: 'always'` 自相矛盾，滚动/反复进出详情时每张新卡片强制刷新。→ 改显式 `invalidateMovieStates`（已有 `offline.ts`/`pan.ts` 调用）驱动。
- **`history/page.tsx:35`、`discover/page.tsx:102`** 用 `key={JSON.stringify(...)}` 重置状态，副作用是整棵子树卸载重建（切页丢已滚出图片、焦点状态）。→ 改用 `useEffect` 显式 reset。
- **骨架屏数量不一致**：discover 20 格 vs library/history 8 格，加载观感跳变。→ 统一 `count={pageSize}`。
- **`components/list-pagination.tsx:24`** 默认 `scrollToTop` 埋在与路由解耦的组件里，未来局部滚动容器会滚错对象。→ 由页面决定。

---

## 五、样式不统一

- **`library/movie-card.tsx:29`**：为修暗色对比度在组件里**写死亮色 oklch 覆盖** `[--destructive:oklch(...)]`，绕过主题 token（与 `globals.css:62` 重复）。→ 应在 `ui/badge.tsx` 的 destructive 变体修暗色，删掉覆盖。
- **`library/movie-card.tsx:37`**：「未观看」写死 `bg-violet-500`，不在设计 token 体系内，同屏与 `variant="outline"` 的「已观看」割裂。
- **`javdb-section.tsx:163-166`**：延迟色阶前两档用语义 token（`text-success`/`text-warning`），第三档裸用 `text-orange-600 dark:text-orange-400`。→ 全走语义色。
- **`magnets.tsx:151`**（`ui/breadcrumb.tsx:15` 也有）：`wrap-break-word` 是**不存在的 Tailwind 类**（死类），超长磁力名可能溢出。→ 改 `break-words`/`break-all` 或补 `@utility`。
- **`player.css` / `movie-player.tsx`**：`.miyabi-player` 没设 `position: relative`，新加的「3 倍速」指示条和 loading/error 浮层的 `absolute inset-0` 实际锚定到 Dialog 而非播放器——当前恰好能用，将来复用播放器就会错位。→ 补 `position: relative`。
- **分页控件三处三种显示条件**（`results.tsx:39` / `library/page.tsx:119` / `history/page.tsx:194`）+ 两个 `PAGE_SIZE = 20` 常量。→ 统一由接口返回分页元数据。

---

## 六、冗余代码 / 简洁性

- **`icons.ts:38,65`**：`GoogleCastButton`/`DownloadButton` 图标是死配置（槽位已 `null`、layout 未开 download）。
- **未使用导出**：`ui/card.tsx` 的 `CardAction`/`CardFooter`、`ui/pagination.tsx` 的 `PaginationLink`/`PaginationEllipsis`、`ui/tabs.tsx` 的 `tabsListVariants` 全仓库零引用。
- **`offline.ts:8,10`**：同一符号先 import 再 re-export，双来源。
- **`pan/play.go:14-18`**：`PlaySource.Definition` 解码后从未消费。
- **`library_scan.go:62`**：`identifyScanVideos` 不用 receiver，应改普通函数。
- **`javdb`**：`client.go:40` 字段 `c.selectRoute` 与包级函数 `selectRoute` 同名易误改；`route.go:288-298` 包级 init 硬编码解密失败直接 panic（服务启动即崩）；`movie.go:39` vs `72/172` 的 `slog.Warn` vs `WarnContext` 混用丢失 ctx 信息；`pan/play.go:74` 基础设施层唯一中文 UI 文案。
- **`pan/file.go:47-56`**：同结构里 `CID`/`Path.ID` 用 `json.Number` 防字符串化数字，`Count`/`Size` 却用原生 int，115 一旦返回字符串就整页失败，防御策略不一致。
- **依赖卫生（`web/package.json`）**：`shadcn` CLI 放在 `dependencies`（应为 `devDependencies`）。
- **设计层面**：`Movie` schema 的 `watched` 布尔与 `watch_history` 边存在数据冗余（历史上先是 `watched` 徽章、后加 watch_history）。当前有意保留可接受，建议确认其不被反推。
- **`lib/format.ts:3`**：`formatSize` 用 1024 进制却标 KB/MB（SI 应为 KiB/MiB），仅一处使用。
- **仓库卫生**：无 `.gitattributes` + `core.autocrlf=true`，git 反复警告 LF→CRLF，跨平台提交易产生整文件 diff。→ 加 `.gitattributes` 固定 LF。

---

## 七、当前 WIP（长按倍速）专项意见

**写得好的地方**：`use-hold-speed.ts` 是纯函数 + 注入式 DOM 的测试友好设计；`hold-speed.test.mjs` 覆盖了 seek-on-release、延迟 rate 上报、8 种中断、焦点丢失、菜单排除、卸载清理等场景，质量很高。`sliders.tsx` 干净。`cancel()` 在事件层面恢复 `playbackRate` 的写法很扎实。

**要修的两点**：

1. **时序竞争**（见 2.6）——真实浏览器验证短按/长按。
2. **魔数共享**：`use-hold-speed.ts:84` 的倍速 `3`、`:103` 的 seek 步长 `+5`、`movie-player.tsx:155` 的「3 倍速」文案是三个硬编码点。→ 抽 `HOLD_SPEED_RATE` 常量，seek 步长读 Vidstack 配置而非写死。

---

## 八、值得肯定（建议不要动）

- Dockerfile 多阶段 + BuildKit 缓存 + 非 root + healthcheck；CI 预热缓存/多架构发布设计讲究。
- `discover_cache` 的 singleflight + 引用计数、javdb 的 `atomic.Pointer` 路由选择、pan 客户端构造后只读——并发正确性经核查无 race。
- 批处理加载（500/页）、`walkFilePages` 完整性校验、ent 预加载均无 N+1。
- SSE/定时器/事件监听/`AbortController` 清理完备，未发现泄漏。
- 错误大多收敛到 `ApiError`，前端状态管理（zustand + query key 分层）清晰。
- 新增的 `use-hold-speed.ts` 未重复实现任何现有工具，`AbortController` + 事件卸载 + 全事件取消覆盖（`pagehide`/`visibilitychange`/`blur`/`pause`/`error`/…）正确。

---

## 九、建议的落地顺序

1. **当天可做的小改动**（各 <30 行，风险低）：image cache Rename 幂等、pan 离线容错解析、`clamp()` + 魔数常量、noStore 中间件、watch-progress 文件更名、死图标/未用导出删除、`.gitattributes`。
2. **本周**：API 层 bind/respond helper 重构（最大重复，全项目收益）、空 source 契约统一、`has_more` 后端补全 + 前端分页对齐、播放器与「标记已看」解耦。
3. **排期**：use-hold-speed 时序硬化 + 真机验证、离线历史查询上限、DB 连接池、javdb 路由探测客户端复用。

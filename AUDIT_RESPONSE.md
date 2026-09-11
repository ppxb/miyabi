**Miyabi 审计意见复核答复**

复核日期：2026-09-11。复核对象：`AUDIT.md`。代码基线：`master / c265b0f`，复核开始时工作区干净。

我的结论是：**报告提供了有用线索，但事实准确性和优先级不足以支撑直接照单整改。应保留可复现的问题，修正错误解释，撤回会破坏现有契约的改法。**

原报告仍以 `9ffb8e4` 和“长按倍速 WIP”为基线；目前该功能已经提交，且有快捷键反馈、时间轴焦点等后续修复。因此，报告中的位置、代码片段和“当前 WIP”结论需要更新。本次逐项核查了九条高优先意见，并复核了其他部分中会影响整改决策的主要主张；没有将一般性建议当作已证实缺陷，也没有把本次复核描述为完整安全审计。

**九条高优先意见的处理决定**

| 原编号 | 复核判断 | 建议处理 |
| --- | --- | --- |
| 2.1 API bind/respond 样板 | 重复存在，属于可维护性问题；没有证据支持列为最高优先或“最划算” | 降为低优先，按具体收益决定是否抽取 |
| 2.2 Windows 图片缓存并发 | 并发保存失败已复现；“目标存在必然失败”的解释错误 | 保留为优先修复项，改正原因描述 |
| 2.3 离线进度解析 | 数字字符串支持正常；异常字段确实可能导致整页失败 | 有条件采纳容错，不能让整个任务永远解码成功 |
| 2.4 空 source 与错误契约 | 查询空结果和操作报错有合理区别；错误文案与状态映射问题成立 | 拆开处理，保留端点语义，修正文案与错误分类 |
| 2.5 播放等待已看写入 | 成立，等待链还包含缓存刷新；原建议的一行修改有续播回归风险 | 优先处理启动链，保留续播文件、位置与会话语义 |
| 2.6 长按键盘事件竞争 | 所述 DOM 规则错误，不能据此认定竞态 | 撤回该定性，真实浏览器集成验证另列待办 |
| 2.7 离线历史无界增长 | 容量风险成立，但不是每次把所有历史版本都加载解码 | 按活跃工作流和历史分开设计，不直接 LIMIT |
| 2.8 Discover 满页猜分页 | 空尾页问题成立，用户仍可返回上一页 | 中优先体验修复；可靠 has_more 需要上游事实支撑 |
| 2.9 隐私模式重建图片 | 重挂载事实成立；未测得足以列为高优先的卡顿 | 性能观察项，优化时保留隐私与图片状态行为 |

**2.2：图片缓存应修，但必须准确描述问题。**

[缓存保存](C:/Users/Administrator/Desktop/miyabi/internal/image/cache.go:79) 先检查目标是否存在，再写临时文件并重命名。同一图片并发进入时，多个调用确实可以同时通过预检。

Go 1.27.1 的 Windows 实现通过 `MoveFileEx(..., MOVEFILE_REPLACE_EXISTING)` 重命名。本次实验也确认：目标已存在时，单次 `os.Rename` 成功覆盖，结果文件内容正确。因此原报告“Windows 目标存在就直接报错、第二个必然失败”的说法不成立。

但是，在当前 Windows 工作区，对真实 `Cache.FromCover` 做 **5 组、每组 32 个同时保存相同图片**的调用，共 160 次，其中 136 次返回重命名 `Access is denied`。这足以保留“并发发布缓存失败”这一修复项，不能因为原报告解释错了就把实际问题一起撤销。这个比例来自刻意制造冲突的本地实验，不代表生产发生率，也不能据此断言具体底层文件锁来源。

建议在重命名失败后确认目标缓存已经成功生成且为正常文件，再按幂等成功处理；必要时按内容键串行化写入。不能对任意重命名错误无条件吞错。验收应包括并发调用成功、文件可解码、返回键一致、临时文件清理，以及真实 I/O 错误仍可上报。

`saveArtwork` 失败前可能已写入部分内容寻址缓存，这是残留文件问题；不能仅据此断言数据库已经发布了一套不完整 artwork，还需核查调用方提交边界。

**2.3：问题在异常字段容错，不在数字字符串支持。**

[现有测试](C:/Users/Administrator/Desktop/miyabi/internal/pan/offline_test.go:74) 已覆盖整数、小数、`"42.75"` 和 `null`，且本次执行通过。直接调用当前 `OfflineTask.UnmarshalJSON` 的实验结果如下：

| percentDone | 结果 |
| --- | --- |
| `42`、`42.75`、`"42.75"` | 成功，归一化为 42 |
| `null` | 成功，归一化为 0 |
| `"42%"`、`{}` | JSON 解码阶段失败 |
| `1e999` | Float64 转换失败 |

原报告“json.Number 接不住 numeric strings”应撤回。异常字段会使整页失败这一点成立，但本次没有来自真实 115 响应的证据证明 `%` 或对象正在发生，风险等级应考虑实际发生频率。

若要容错，应把 `percentDone` 先接成 `json.RawMessage`，单独处理缺省、数字、数字字符串与异常值；只对这个展示性字段降级并保留可观察性。**只修改 Float64 错误分支无法接住前面的 JSON 类型错误；整个 UnmarshalJSON 永不报错又会掩盖任务身份、状态等关键字段损坏。** 同步工作进程会继续下一轮，不应把一次 Sync 失败描述为进程永久停止。

**2.4：统一错误表达，不强行统一不同操作的业务结果。**

未挂载媒体目录时，列表查询返回空集合，而播放、写入观看状态返回缺少前置条件，语义合理。将全部查询改成错误会使正常空态退化；将播放操作改成“空成功”则会丢失可操作的错误提示。

[播放服务](C:/Users/Administrator/Desktop/miyabi/internal/service/play.go:104) 把 `fs.ErrNotExist` 包进中文文案，确实会出现英文后缀。[错误中间件](C:/Users/Administrator/Desktop/miyabi/internal/api/error.go:71) 已支持 `PublicMessage()`，可以沿用这条机制，在保留底层错误链的同时提供干净的展示文案。

源变更错误也确实没有完整纳入 HTTP 分类，部分同步请求会落到 500；建议统一领域错误，显式映射为合适的冲突响应，并补映射测试。没有必要为了修这两个问题，立即让所有 service 错误携带 HTTP 状态码。

**2.5：播放启动依赖链是最值得优先调整的用户体验问题之一。**

[PlayerDialog](C:/Users/Administrator/Desktop/miyabi/web/src/features/player/player-dialog.tsx:50) 等待 watched mutation 成功或失败；[MoviePlayer](C:/Users/Administrator/Desktop/miyabi/web/src/features/player/movie-player.tsx:51) 在此之前只返回加载状态，下层播放地址请求和媒体加载无法启动。这里的接口实际使用 **PUT**；重试条件为 `failures < 2`，即最多两次重试、三次总尝试。

还有报告遗漏的一层等待：[mutation 的 onSuccess](C:/Users/Administrator/Desktop/miyabi/web/src/api/library.ts:57) 会等待取消旧查询，并返回 `invalidateQueries` 的 Promise。当前 TanStack Query 在标记 mutation 成功前会等待这些回调。因此，即使观看写入已成功，播放器也可能继续等待活动媒体库列表刷新。请求封装没有统一超时，也不能把最坏等待时间简单限定为“数秒”。

但不能把 `historyReady` 直接改成“请求已发起”。[后端 MarkWatched](C:/Users/Administrator/Desktop/miyabi/internal/service/watch_history.go:71) 同时返回已存的文件 ID、续播位置和新进度会话；前端以这些数据选文件，并用 `useRef(initialPosition)` 初始化续播。先用 undefined 历史启动，可能从错误文件或 0 秒开始；相同组件后续收到历史也不会自动重设 ref。

建议先把列表刷新的等待移出播放就绪条件，保留旧查询取消和缓存更新的顺序保护；随后再明确“读取续播 / 建立会话 / 标已看”的接口职责与失败降级。应验证多文件续播、慢写入、写入失败、快速关闭重开，以及迟到响应不覆盖当前会话。不能仅以弹窗更快出现作为验收。

**2.6：目标节点上的捕获与冒泡不会简单按混合注册顺序执行。**

DOM 事件派发分别处理捕获和冒泡监听器；即使 `event.target` 就是播放器节点，捕获监听器仍先于该节点的非捕获监听器调用。注册顺序约束同一轮符合条件的监听器，不会让先注册的冒泡监听器越过后注册的捕获监听器。

当前 Vidstack 依赖中的 seek 键盘处理是非捕获监听器；另一个捕获监听器只是对原生媒体元素调用 `preventDefault`。当前 [use-hold-speed](C:/Users/Administrator/Desktop/miyabi/web/src/features/player/use-hold-speed.ts:122) 在播放器捕获 keydown，因此原报告据此推导的“Vidstack 可能先 seek”缺少成立前提。

此外，短按现在通过转发键盘事件委托 Vidstack 处理，旧版手写 `+5` 的建议已经过时。倍速数值与展示文案共享常量仍可作为小整理。

现有测试使用 Node EventTarget 和播放器替身，不能代替完整浏览器与 Vidstack 集成验证。此次真实浏览器补验未完成：创建本地测试页被自动审批服务的 503 故障拒绝。本结论依据 DOM 派发规则与当前依赖源码，不冒充浏览器实测结果。

**2.7：应保留老的活跃工作流，限制真正的历史。**

[latestOfflineTasks](C:/Users/Administrator/Desktop/miyabi/internal/service/offline.go:475) 在 SQL 内先按账号、目录或影片范围筛选，再按 hash 选最新任务；[数据库](C:/Users/Administrator/Desktop/miyabi/internal/database/indexes.go:12) 已有对应表达式索引。它会查询历史以选最新项，但应用层并没有把同一 hash 的所有旧版本都读出解码。全局 UI 也是[存在活跃任务才每 5 秒轮询](C:/Users/Administrator/Desktop/miyabi/web/src/api/offline.ts:47)，不是永远定频轮询全部历史。

随着不同 hash 数量增加，最新记录、payload 解码以及后续文件索引投影仍可能增长，这个容量风险值得测量和治理。

原报告两种快捷改法都不完整：简单时间或条数上限会隐藏老下载；直接跳过 done 任务的 payload 则会丢失等待文件定位、后续扫描、实际是否入库等信息。**下载任务 done 不等于整个入库工作流终结。** [现有回归测试](C:/Users/Administrator/Desktop/miyabi/internal/service/offline_history_test.go:12) 专门验证老下载不会被较新的完成记录淹没。

建议把活跃工作流查询与已终结历史浏览分开：活跃项无论多老都保留，终结历史分页或归档，并保留同一 hash 最新状态的投影语义。先用真实 payload 尺寸的合成数据测 1 千、1 万、10 万个不同 hash 的耗时和内存，再决定结构改造规模。

**2.8 和 2.9：可以改善，但不要扩大整改范围。**

[Discover](C:/Users/Administrator/Desktop/miyabi/web/src/features/discover/results.tsx:31) 的空态只覆盖第一页，满尾页后再翻页会看到空列表与分页控件。可以先补后续空页提示或回退。当前 JavDB Browse/Search 封装只返回列表，增加一个仍靠列表长度推算的 `has_more` 字段不会提高准确性；需要上游分页元数据，或明确额外探测和缓存成本。

[MediaImage](C:/Users/Administrator/Desktop/miyabi/web/src/components/media-image.tsx:21) 的隐私模式 key 确实触发重挂载。但移除 key 后，`showImage` 分支仍会卸载 img；因此“删 key 图片便常驻”也不成立。`original` 变化还涉及 srcSet 与加载状态，不能一并忽略。若改为 CSS 隐藏，应先确定隐私开关是否允许后台继续请求、解码封面，并验证隐藏状态和重新显示时没有闪现、旧错误状态残留。当前没有性能测量支持把它列为高优先卡顿故障。

**其他必须修正的意见**

| 报告主张 | 复核答复 |
| --- | --- |
| `staleTime: Infinity` 与 `refetchOnMount: 'always'` 自相矛盾，删除后者 | 不成立。当前还有 `initialData: emptyState`，挂载刷新承担首次加载和重新挂载校准。实验中原配置发起 1 次请求并得到 in_library；只删除后者后发起 0 次请求，停留 not_in_library。若优化，必须同时设计未加载态与失效策略。 |
| `wrap-break-word` 是不存在的 Tailwind 类 | 误报。项目使用 Tailwind 4.3.3，直接调用已安装编译器确认生成 `overflow-wrap: break-word`。 |
| 播放器缺少相对定位，absolute 锚定 Dialog | 误报。已导入的 Vidstack `theme.css:7` 的 `[data-media-player]` 规则明确设置 `position: relative`。 |
| `SelectDirectory` 的 Path 索引有越界 bug | 生产调用链前提不成立。[pan.List](C:/Users/Administrator/Desktop/miyabi/internal/pan/file.go:68) 已拒绝空路径，并校验末项 ID。可增加局部防御，但不能漏看上游保障就认定可触发 panic。 |
| `PlaySource.Definition` 没有消费者 | 误报。[PlayService](C:/Users/Administrator/Desktop/miyabi/internal/service/play.go:140) 用它标记“原画”。删除会改变清晰度标签。 |
| `MovieGrid` 无法复用 | 不完整。已经有 MovieGridLayout，library/history 都在使用。保持发现页便利封装与公共布局分层合理。 |
| GoogleCastButton / DownloadButton 都可直接删 | 必须区分。当前 DefaultLayoutIcons 中 GoogleCastButton 是必填项，直接删除会破坏 satisfies 类型契约；DownloadButton 是可选项，可按收益清理。 |
| 数据库无连接池 | 表述错误。database/sql 已有连接池，实际问题是未显式限制最大打开连接数。是否设为 4 应经负载验证；本地 SQLite 也不天然需要周期性连接寿命回收。 |
| metadata 两次限速是同一请求重复等待 | 不准确。一次获取签名下载地址，一次下载 sidecar，是两次请求。共享同一限流器是否理想可以讨论，但不能按重复等待直接删除。 |
| OpenMedia 不限速，应该统一策略 | CDN 媒体流和控制 API 具有不同流量与延迟需求。当前明确分离，以免背景扫描阻塞播放；不应把流媒体套入 2 请求/秒的 API 桶。 |
| full 路由探测应套用最快节点淘汰 ticker | 会改变完整测量语义。当前测试明确要求记录慢的已知与动态节点。可缩短独立探测超时、限制并发或渐进展示；复用客户端前也应验证 host、cookie、代理与指纹隔离。 |
| Ticker 会不断积压 tick | Go Ticker 会调整或丢弃 tick，不是无界队列。慢 Sync 后可能紧接下一轮，但不会在这个循环里并发执行 Sync；改固定延迟属于调度语义选择。 |
| 直接用 getQueryState 替代推荐缓存扫描 | 不等价。影片可能只存在某个列表查询中，还没有独立详情 query。要优化可建立 ID 索引，同时保留新鲜度、失效和列表/详情完整性边界。 |
| 用 effect 重置替代 history/discover 的 key | key 是合理的状态隔离手段；历史页会随账号、目录和页码同步重置选择与删除对话框。effect 会在新输入首次渲染之后执行，不能只为减少卸载就替换。 |

**其余建议的合理位置**

先读完整错误响应再检查状态码的问题成立：[JavDB transport](C:/Users/Administrator/Desktop/miyabi/internal/javdb/transport.go:82) 会浪费失败路径的下载、内存和时间。可以优先返回状态错误；若希望保留连接复用，只做有上限的 drain，并避免错误 body 读取失败覆盖本来的 HTTP 状态分类。

颜色 token、骨架数量、公共 no-store、少量 NFO 解码复用、CLI 依赖分类和 `.gitattributes` 都可按收益小步整理。API helper 应保留 JSON/query/URI 绑定差异、200/202/流式响应差异和统一错误中间件，不以减少行数作为唯一验收指标。路径拼接两处也并非完全相同：一处包含文件名并使用 path.Join，不能不考虑归一化行为就合并。

共享 clamp 应先明确 NaN、Infinity、上下界语义；不同场景的数值防御不一定能无损替成一个 Math.min/Math.max 封装。短标签与完整进度说明也有不同展示需求，可共享阶段定义，但不必强制使用同一文案。

≤128KiB 的 SHA1 重算、NFO 编码拷贝是真实但小的优化点；无剖析证据时低优先。fanart 保留原尺寸涉及封面质量与元数据写回，应先明确尺寸契约。`watched` 与历史不应简单去冗余：现有测试明确要求清除历史不会清除已观看徽章。`shadcn` 移到 devDependencies 属于依赖卫生，当前多阶段 Docker 最终镜像并不携带 node_modules。

未使用 UI 导出、命名调整、日志文案、单位标签等剩余细节不构成高风险结论。本次没有对每个未使用符号作独立删除验证，也没有实际测量所有性能建议的收益；不建议把这批整理混入正确性修复。

**建议的实施顺序与验收边界**

1. 优先处理已复现的图片缓存并发失败，以及播放器等待链；各自独立提交，保留明确的并发/续播回归验证。
2. 处理离线进度异常字段的隔离、Discover 空尾页、公开错误文案与源变更分类、JavDB 错误响应读取。这些改动可以小步完成。
3. 针对离线历史容量、SQLite 连接上限、路由探测做数据规模与慢上游实验，再决定结构调整。必须保留老活跃任务和完整测量等既有契约。
4. 最后处理 API 样板、主题 token、常量和仓库卫生。将纯重构与行为变更分开，避免大批改动掩盖真正的回归。

**本次验证记录与限制**

| 验证 | 结果 |
| --- | --- |
| `go test ./... -timeout 120s` | 通过；使用工作区内 Go 构建缓存 |
| 前端既有 Node 测试 | 44/44 通过，失败和跳过均为 0 |
| Oxlint | 114 个文件，0 条诊断 |
| Windows 重命名与真实图片缓存并发实验 | 已执行，结果见 2.2 |
| 当前离线进度解码实验 | 已执行，确认支持数字字符串及异常值失败边界 |
| TanStack Query 挂载刷新实验 | 已执行，证实直接删除挂载刷新会阻断首次加载 |
| Tailwind 当前版本编译实验 | 已执行，确认 wrap-break-word 有效 |
| TypeScript 完整检查 | 未通过环境验证：zustand 的声明文件读取返回 EPERM，引发无法解析模块和级联 any 错误 |
| Vite 生产构建 | 未完成：读取 rolldown 依赖文件返回 EPERM |
| 真实浏览器 / Vidstack 集成验证 | 未完成：自动审批服务 503 拒绝打开测试页 |

类型检查与构建遇到的文件均存在，本次已用独立文件读取复核 EPERM，因此不能把这批输出直接归因于业务源码，也不能宣称构建通过。环境中的 pnpm 启动器还尝试触发依赖重装并因无 TTY 中止；后续检查直接调用现有本地工具，没有执行依赖清理或重装。

没有运行 race detector、覆盖率统计、线上负载测试或真实 115/JavDB 账号操作。因此原报告“无 race”“整体质量高于平均”等绝对或比较性结论，不在本次可证明的范围内。测试通过说明已执行场景通过，不等于不存在其他缺陷。

本次业务代码和原审计报告未改动。新增本答复文档；可复现实验脚本与日志保存在已忽略的 `.tmp` 下，包括 [Go 实验](C:/Users/Administrator/Desktop/miyabi/.tmp/audit-review/probe.go)、[状态查询实验](C:/Users/Administrator/Desktop/miyabi/.tmp/audit-review/movie-state-probe.mjs)、[Go 测试日志](C:/Users/Administrator/Desktop/miyabi/.tmp/audit-go-tests.log)、[前端测试日志](C:/Users/Administrator/Desktop/miyabi/.tmp/audit-web-tests.log)。

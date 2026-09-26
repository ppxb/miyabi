# Miyabi 全项目代码审计

审计日期：2026-09-26。代码基线：`0235ad9175f300c64102a83a4d6d7adf197fb30a`。开始审计时工作区干净。

本报告以当前实现为依据，忽略 REFACTOR_PLAN。只新增审计文档，未修改应用源码、README 或依赖，未提交代码。

现有按领域划分的骨架值得保留。主要问题集中在：恢复状态与清理规则冲突、数据库事务内执行不可回滚副作用、跨来源的数据归属不一致、错误被当作成功、重复的状态投影，以及新增 UI 绕开既有复用点。比起大规模重写，先修复这些边界，再减少重复，会更有效。

## 审计范围与证据标准

| 范围 | 核查重点 |
| --- | --- |
| Go 入口、app、config、API、auth | 依赖装配、生命周期、请求校验、错误映射、权限边界 |
| database、Ent schema 与手写事务代码 | 数据归属、级联删除、事务边界、SQLite 写锁；生成代码不计作手写冗余 |
| tasks、library/scan、library/scrape | 队列恢复、扫描断点、索引清理、通知、元数据和 STRM 导出 |
| drive、pan、offline、monitor | 登录切换、源快照、上传范围、离线下载、订阅与批处理 |
| catalogue、javdb、javbus、magnet、netx | 聚合失败语义、缓存边界、重复查询、代理和下载约束 |
| subtitle、playback、image、emby | 字幕文件归属、格式、播放能力、图片处理、后台同步 |
| React API/query/store、路由和功能页面 | 状态一致性、分页、错误反馈、选择逻辑、模块引用 |
| UI primitives、卡片、设置、播放器、CSS | 复用边界、主题变量、交互一致性、表单可访问性 |
| 现有测试、前端构建、Dockerfile、CI | 测试是否覆盖真实业务边界，构建与质量检查的区别 |

这是静态调用链审计，加上现有前端检查。下文的“验证”是建议补充的回归场景，**不代表这些场景已执行**。Go 工具链在本环境不可用，因此未运行 Go test/vet/race；按项目约定未进行浏览器测试。没有用文件长度、接口数量或任意 CSS 值的数量来直接判定设计好坏。

优先级含义：P1 为可能损失数据、应优先修复的问题；P2 为明确的功能、状态、资源或边界缺陷；P3 为可按模块逐步处理的简化、复用和工程改进。这里没有将条件性风险标成无条件灾难，也没有给未经测量的性能收益编造数值。
## 分类索引

共 31 项：2 项 P1、23 项 P2、6 项 P3。按主要问题性质分为八类；一个问题只在一类展开，保留稳定编号便于复审。每条分别给出证据、影响、最小修复和验证方式；代码清理类不强行添加无意义测试。

| 分类 | ID | 级别 | 问题 |
| --- | --- | --- | --- |
| 数据安全、事务与来源一致性 | A01 | P1 | 扫描断点与全量清理协议冲突 |
| 数据安全、事务与来源一致性 | A02 | P1 | 字幕文件没有所有权边界 |
| 数据安全、事务与来源一致性 | A03 | P2 | 变更通知早于提交 |
| 数据安全、事务与来源一致性 | A12 | P2 | 字幕上传选择了任意来源的影片文件 |
| 数据安全、事务与来源一致性 | A13 | P2 | 默认字幕更新缺少关系约束 |
| 数据安全、事务与来源一致性 | A18 | P2 | 账户缓存存在跨凭据回填竞态 |
| 业务流程与前后端状态 | A04 | P2 | 批处理恢复重复累计 |
| 业务流程与前后端状态 | A05 | P2 | 演员新作创建失败后仍标记已见 |
| 业务流程与前后端状态 | A07 | P2 | 100 条分页被错误当成完整集合 |
| 业务流程与前后端状态 | A08 | P2 | 本地媒体能力没有贯穿全链路 |
| 业务流程与前后端状态 | A09 | P2 | 本地字幕绕过规范化 |
| 业务流程与前后端状态 | A10 | P2 | 字幕替换后的客户端集合不正确 |
| 业务流程与前后端状态 | A19 | P2 | STRM 导出没有统一的预期文件集合 |
| 错误处理与防御边界 | A06 | P2 | 禁用的磁力来源被当成成功来源 |
| 错误处理与防御边界 | A11 | P2 | 字幕失败被伪装成正常空结果 |
| 错误处理与防御边界 | A14 | P2 | SSRF 传输校验把本机代理当作目标拦截 |
| 错误处理与防御边界 | A15 | P2 | “直连”仍受环境变量影响 |
| 错误处理与防御边界 | A16 | P2 | 图片代理缺少目标与资源边界 |
| 错误处理与防御边界 | A17 | P2 | 应用鉴权和上游鉴权混为一类 |
| 错误处理与防御边界 | A24 | P2 | 绑定失败后继续执行本地扫描 |
| 性能与资源生命周期 | A20 | P2 | TTL 和“512”都不是实际容量上限 |
| 性能与资源生命周期 | A21 | P2 | 本地扫描的数据库事务承担慢副作用 |
| 性能与资源生命周期 | A22 | P2 | 限速批处理占住整个任务池 |
| 性能与资源生命周期 | A23 | P2 | Emby 定时同步缺少运行生命周期管理 |
| 架构职责与过度设计 | A25 | P3 | catalogue 的投影方向产生重复工作 |
| 架构职责与过度设计 | A26 | P3 | 位置敏感的 `...any` 比类型化参数更复杂 |
| 冗余代码与重复实现 | A27 | P3 | 选择逻辑和卡片交互外壳重复 |
| 冗余代码与重复实现 | A30 | P3 | 可清理项有具体范围，不宜靠关键词批量删 |
| UI 样式一致性与可访问性 | A28 | P2 | 设置行没有可访问名称关系 |
| UI 样式一致性与可访问性 | A29 | P3 | 播放器与状态颜色存在平行样式规则 |
| 测试与工程规范 | A31 | P3 | 检查通过不等于关键行为被验证 |

## 1. 数据安全、事务与来源一致性

### A01 · P1 · 扫描断点与全量清理协议冲突

证据：[walker.go:135](C:/Users/36245/Desktop/miyabi/internal/library/scan/walker.go:135)、[walker.go:206](C:/Users/36245/Desktop/miyabi/internal/library/scan/walker.go:206)、[reconcile.go:47](C:/Users/36245/Desktop/miyabi/internal/library/scan/reconcile.go:47)、[movie schema:77](C:/Users/36245/Desktop/miyabi/internal/ent/schema/movie.go:77)。

每次执行都生成新 `scanID`，但恢复时把 checkpoint 中的“剩余目录”当作完整遍历队列。reconcile 又把当前源中 `scan_id != 新 scanID` 的文件全部视为消失。已经在上次执行中扫描的目录，既不会重扫，也不会更新标记。

具体触发：目录 A 已写入 `scanID=1`；任务保存只包含 B 的剩余队列；进程重启；恢复只扫描 B，使用 `scanID=2`；reconcile 删除 A 的索引。如果 A 中影片没有其他文件关联，`RemoveUnreferencedMovies` 还会删除影片，级联删除观看历史和字幕记录。这里删除的是数据库索引及关联数据，不是云盘原视频；后续全量重扫也不能自动恢复已删除的观看历史。

最小修复：优先取消这套不完整的部分恢复，每次重试从源根目录完整重扫，再以新代际清理。如果必须恢复，则扫描代际、已观察状态、队列与清理范围必须构成同一个持久化协议，不能只保存目录列表。

验证：用两个目录和真实临时数据库，在 A 提交、B 开始前中断，运行队列恢复和完整 scanner，断言 A 的文件、影片、观看历史都保留，同时确实消失的文件仍能被清理。现有 [walker_feature_test.go:54](C:/Users/36245/Desktop/miyabi/internal/library/scan/walker_feature_test.go:54) 只验证 JSON 往返，没有执行恢复后的扫描和 reconcile。

### A02 · P1 · 字幕文件没有所有权边界

证据：[local.go:245](C:/Users/36245/Desktop/miyabi/internal/library/scan/local.go:245)、[subtitle/service.go:139](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:139)、[subtitle/service.go:473](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:473)。

本地扫描把原始 `subPath` 直接写入 `storage_path`。字幕替换按“影片、语言、版本”删除旧记录，并直接 `os.Remove(old.StoragePath)`；删除接口也先删除路径再删除数据库记录。没有区分用户原字幕与应用生成的缓存。

因此，扫描导入本地字幕后，应用同语言、同版本的在线字幕，可能删除用户原 SRT/ASS/VTT。替换中的删除发生在事务内部，即使后续插入失败、事务回滚，磁盘文件也无法恢复。查询、删除、引用计数的错误还被忽略，不能保证清理依据有效。API 删除同样存在数据库失败后文件已丢失的问题；当前前端无删除调用不消除后端接口风险。

最小修复：明确区分原文件位置与应用拥有的规范化缓存；只清理受管理目录中的缓存。必要数据库错误应返回；成功提交后再做可重试的孤儿缓存清理。仅检查 `source == local` 不够，因为“本地来源”不等于“可删除缓存”。

验证：临时目录保存原字幕，导入后替换/删除，确认原文件仍在；注入新记录保存失败，确认旧记录及其可读取内容都保留；另测共享缓存引用。

### A03 · P2 · 变更通知早于提交

证据：[persist.go:167](C:/Users/36245/Desktop/miyabi/internal/library/scan/persist.go:167)、[reconcile.go:147](C:/Users/36245/Desktop/miyabi/internal/library/scan/reconcile.go:147)、[walker.go:145](C:/Users/36245/Desktop/miyabi/internal/library/scan/walker.go:145)、[sse.go:36](C:/Users/36245/Desktop/miyabi/internal/api/sse.go:36)。

`ProcessScanPageTx` 和 `ReconcileScanTx` 在事务回调内发布通知，此时 `sess.Commit` 尚未提交。SQLite 使用 WAL，另一个连接可以读取旧快照；SSE/前端可能已经消费新 revision，却刷新到旧数据。任务结束也不保证补发 library revision：[library.go:170](C:/Users/36245/Desktop/miyabi/internal/library/library.go:170) 只返回 `ChangeOffline`。例如只有删除、或不需要后续刮削的扫描，可能留下过期卡片/徽标直到其他刷新。

最小修复：事务函数返回变更标志，调用方成功提交后统一发布；复用 [queue.go:73](C:/Users/36245/Desktop/miyabi/internal/tasks/queue.go:73) 已有的提交后通知模式。不要靠延时通知或增加轮询修补。

验证：在事务回调结束与提交之间设置可控屏障，确认该阶段不会发布新 revision；回滚不通知；提交后重新查询能得到对应变更。

### A12 · P2 · 字幕上传选择了任意来源的影片文件

证据：[subtitle/service.go:218](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:218)、[drive/session.go:129](C:/Users/36245/Desktop/miyabi/internal/drive/session.go:129)。

ApplyCandidate 使用 `File.Query().Where(MovieIDEQ(movieID)).First` 取 ParentID，没有当前账户/根目录条件，然后打开当前 drive session 上传。Session.Upload 校验会话是否仍有效，但不验证传入 dirID 是否属于该源；DirectoryEntries 的目录范围验证在上传之后才发生。

当同一影片保留多个来源的索引时，可能把当前账户字幕写到旧挂载目录；旧账户目录不可访问时则静默上传失败。会话有效与目标目录归属正确是两个独立条件。

最小修复：先取得 session，再以它的 source 限定文件查询；上传前验证目标目录属于该 source。让手动和自动字幕共用一段带明确目标的应用流程，避免两套流程继续分叉。

验证：同一影片关联两个源的文件，当前源为后者；上传只能命中后者目录。范围外目录必须在调用远端 Upload 前被拒绝。

### A13 · P2 · 默认字幕更新缺少关系约束

证据：[subtitle/service.go:460](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:460)、[api/subtitle.go:106](C:/Users/36245/Desktop/miyabi/internal/api/subtitle.go:106)。

SetDefault 先清空传入 movieID 的默认字幕，再按 subID 设置默认，没有确认该字幕属于这部影片。传入影片 A 与影片 B 的字幕 ID，可清空 A，且在 B 中留下多个 default。鉴权通过也不意味着这组关系成立。

最小修复：根据字幕记录推导影片，或在同一事务中强制 `(id, movie_id)` 匹配并检查更新结果。无需增加泛化权限框架。

验证：跨影片组合返回错误且两部影片原默认状态都不变；正常切换后每部影片至多一个默认项。

### A18 · P2 · 账户缓存存在跨凭据回填竞态

证据：[drive/account.go:54](C:/Users/36245/Desktop/miyabi/internal/drive/account.go:54)、[drive/account.go:83](C:/Users/36245/Desktop/miyabi/internal/drive/account.go:83)、[drive/account.go:264](C:/Users/36245/Desktop/miyabi/internal/drive/account.go:264)。

verifyAccount 的缓存只有账户和时间，没有 credentialVersion。旧请求可以在 fetchAccount 的版本检查通过后暂停；新登录切换凭据并清缓存；旧请求恢复，在独立 accountCacheMu 下写入旧账户。接下来使用新凭据的请求仍命中旧缓存，最长到 60 秒 TTL。

Account 后续验证的是新请求自己的凭据版本，不能发现缓存属于旧版本，还可能用旧 account.ID 调用 discardOtherAccountDirectory，错误清除新账户挂载。这里是逻辑竞态，普通字段锁或仅跑 race detector 都不足以证明正确。

最小修复：缓存条目绑定凭据代际，读取和发布都校验代际；用一致的同步协议完成检查和发布，避免检查后又失效。缩短 TTL 不能消除问题。

验证：用可控屏障安排“旧账户请求已验版本→新登录清缓存→旧请求回填”，确认新请求绝不返回旧账户，也不清掉新挂载。

## 2. 业务流程与前后端状态

### A04 · P2 · 批处理恢复重复累计

证据：[queue.go:30](C:/Users/36245/Desktop/miyabi/internal/tasks/queue.go:30)、[monitor/batch.go:102](C:/Users/36245/Desktop/miyabi/internal/monitor/batch.go:102)、[tasks/batch.go:35](C:/Users/36245/Desktop/miyabi/internal/tasks/batch.go:35)、[task schema:28](C:/Users/36245/Desktop/miyabi/internal/ent/schema/task.go:28)。

恢复保留 payload 中的 `Processed/Submitted/Failed/Waiting`，handler 却从 `IDs[0]` 重新处理并累加。已是 added 的项虽然通常不会再次提交 115，仍会再次计为 submitted；等待项则会重新查询。`Processed * 100 / Total` 可以超过 100，违反进度字段约束，保存失败只写日志，最终任务仍可能显示完成，统计却不可信。

最小修复：把已完成位置/结果作为真正的恢复检查点，从对应位置续行，持久化失败应终止或明确重试。检查点之外的远端副作用仍需维持幂等；不能仅把百分比 clamp 到 100。

验证：N 项处理 k 项后恢复，最终 processed 恰为 N，每项结果不重复累计；在检查点保存失败处中断，验证重试语义。

### A05 · P2 · 演员新作创建失败后仍标记已见

证据：[actor.go:99](C:/Users/36245/Desktop/miyabi/internal/monitor/actor.go:99)、[check.go:145](C:/Users/36245/Desktop/miyabi/internal/monitor/check.go:145)、[schedule.go:51](C:/Users/36245/Desktop/miyabi/internal/monitor/schedule.go:51)、[actor.go:66](C:/Users/36245/Desktop/miyabi/internal/monitor/actor.go:66)。

`spawnMovie` 没有返回值，只记录失败；调用方无条件 `cursor.advance(movies, today)` 并清除错误。某一新作创建发生临时数据库失败后，下一轮会被 seen 集合排除，无法依靠正常轮询重试。初次 AddActor 也是先保存包含所有影片的基线，再尝试派生订阅。

最小修复：创建结果返回给调度层；仅成功项推进游标，或保留原游标并依靠现有唯一性约束幂等重试。部分成功可以保留，但失败项必须仍可见、可重试。

验证：一页两部作品，一部创建成功、一部注入失败；下次检查补齐失败项，既不丢失也不重复生成。

### A07 · P2 · 100 条分页被错误当成完整集合

证据：[api/subscriptions.ts:47](C:/Users/36245/Desktop/miyabi/web/src/api/subscriptions.ts:47)、[api/subscription.go:26](C:/Users/36245/Desktop/miyabi/internal/api/subscription.go:26)、[subscriptions/page.tsx:38](C:/Users/36245/Desktop/miyabi/web/src/features/subscriptions/page.tsx:38)、[monitor/batch.go:75](C:/Users/36245/Desktop/miyabi/internal/monitor/batch.go:75)。

前端固定请求 `limit: 100`，没有后续分页；后端默认 page=1、上限 100。`useSubscription` 又从这份列表 `.find`，超过范围的已有订阅会看起来未订阅。演员列表/作品 feed 同样受截断影响。

页面从这 100 条计算 pendingCount，却在“全部入库”发送 `{all: true}`。后端选中数据库中全部 waiting/stale 项，所以确认文案和成功提示可能显著少于实际处理量。

最小修复：列表明确提供分页和 total/hasMore；单项订阅状态使用按身份查询或批量状态查询，不靠浏览列表证明不存在。全部操作的数量/范围由服务端返回或用明确的 ID 快照确认。不要简单把 limit 调成一个更大的常数。

验证：创建至少 101 条同类订阅，检查尾部可访问、详情订阅状态正确；全部与选中两种操作的确认数量、执行集合一致。

### A08 · P2 · 本地媒体能力没有贯穿全链路

证据：[local.go:227](C:/Users/36245/Desktop/miyabi/internal/library/scan/local.go:227)、[library.go:112](C:/Users/36245/Desktop/miyabi/internal/library/library.go:112)、[library.go:199](C:/Users/36245/Desktop/miyabi/internal/library/library.go:199)、[catalogue/state.go:28](C:/Users/36245/Desktop/miyabi/internal/catalogue/state.go:28)、[playback/files.go:22](C:/Users/36245/Desktop/miyabi/internal/playback/files.go:22)。

本地扫描导入普通视频和 STRM，并写 `account_id/root_id = local`；媒体库查询把它们纳入列表。但 playback.Files 需要当前云盘源且只查该源；观看历史也采用云盘范围。Catalogue 的状态查询在没有云盘源时直接返回未入库，甚至不调用能查本地条目的 MatchingMovies。

同一份本地媒体可能出现在媒体库，却打不开；目录页入库徽标还受是否挂载一个无关云盘源影响。[movie-card.tsx:35](C:/Users/36245/Desktop/miyabi/web/src/features/library/movie-card.tsx:35) 始终提供播放操作，没有来源/可播放能力判断。

最小修复：先明确本地扫描是“元数据恢复”还是“可播放本地库”。前者要在响应和 UI 明示能力，后者需要把源身份、定位、播放和历史范围补齐。用一个明确的来源/能力模型替代各层零散添加 `account_id == local`。

验证：无云盘源、挂载无关源、存在对应云盘文件三种情况下，本地 MP4/STRM 的列表、徽标、播放入口及历史行为均符合所选产品语义。

### A09 · P2 · 本地字幕绕过规范化

证据：[local.go:252](C:/Users/36245/Desktop/miyabi/internal/library/scan/local.go:252)、[subtitle/service.go:402](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:402)、[api/subtitle.go:53](C:/Users/36245/Desktop/miyabi/internal/api/subtitle.go:53)。

本地导入保留原格式和原文件路径，GetTrackVTT 从 StoragePath 读取后直接赋给 rawContent。字符集解码和 `ConvertToWebVTT` 仅存在于云盘 PickCode 回退分支。接口却一律返回 `text/vtt; charset=utf-8`，所以原始 SRT/ASS 或非 UTF-8 内容不能按接口承诺解析。

最小修复：读取字节后进入同一个字符集/格式规范化流程，再写受管理 VTT 缓存。把“原文件路径”与“已规范化缓存”分清，配合 A02 修复。

验证：本地 SRT、ASS、非 UTF-8 字幕与云盘同内容，均返回有效 WebVTT；偏移应用于规范化后的时间轴。

### A10 · P2 · 字幕替换后的客户端集合不正确

证据：[subtitle-menu.tsx:68](C:/Users/36245/Desktop/miyabi/web/src/features/player/subtitle-menu.tsx:68)、[subtitle/service.go:139](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:139)。

服务端删除同影片、语言、版本的旧轨道，再创建新 ID，并更新默认标志。前端只过滤 `t.id !== newTrack.id` 后追加，新 ID 通常不会命中旧轨道。后端已删除的字幕仍出现在选择列表，用户再次选择可能得到 404，旧 default 元数据也未同步。

此外，按钮仅禁用 `applyingUrl === candidate.url` 的候选项，其他候选仍可并发应用，响应乱序会进一步放大这个问题。

最小修复：mutation 成功后读取权威轨道集合，或让响应直接携带集合；若做局部更新，必须按后端实际替换身份更新并统一 default。对于同一个影片，应用操作串行或使用明确的响应版本规则。

验证：同一语言/版本连续应用两份字幕，界面只保留实际存在的轨道；快速操作两份候选并倒置返回顺序，最终 UI 与数据库一致。

### A19 · P2 · STRM 导出没有统一的预期文件集合

证据：[walker.go:79](C:/Users/36245/Desktop/miyabi/internal/library/scan/walker.go:79)、[scrape/export.go:65](C:/Users/36245/Desktop/miyabi/internal/library/scrape/export.go:65)、[scrape/export.go:99](C:/Users/36245/Desktop/miyabi/internal/library/scrape/export.go:99)。

扫描 fast path 先生成 `<code>.strm`，存在即跳过；完整导出遇到多视频又生成 `<code>-cd1.strm` 等文件，没有收敛掉原单文件入口。因此多文件影片可同时保留单入口与分片入口，Emby 看到重复或陈旧的播放项。

ExportLocalMovie 的跳过条件更直接：只要 NFO、poster 存在，`len(record.Edges.Files) > 1` 就被当成已有 STRM 的证据，即便所有 cd 文件都丢了也不修复。fanart 也未参与完整性判断。

最小修复：在 scrape 包内收拢一个类型明确的导出入口，计算当前应有的文件集合，缺失则补齐；单/多文件切换时仅清理可以确认由应用拥有的旧产物。路径、文件名和播放 URL 的构建共用，fast path 与完整路径遵守同一规则。

验证：单→多、多→单、删除一个 cd 文件、移除 fanart、修改播放文件 ID，重复导出后不存在多余入口且缺失内容可恢复。

## 3. 错误处理与防御边界

### A06 · P2 · 禁用的磁力来源被当成成功来源

证据：[catalogue/magnets.go:35](C:/Users/36245/Desktop/miyabi/internal/catalogue/magnets.go:35)、[catalogue/service.go:118](C:/Users/36245/Desktop/miyabi/internal/catalogue/service.go:118)、[magnet/aggregator.go:99](C:/Users/36245/Desktop/miyabi/internal/magnet/aggregator.go:99)。

JavBus 关闭时 `gatedSource.Find` 返回 `(nil, nil)`，但 aggregator 的“所有来源失败”判断仍以包含 JavBus 的总来源数为分母。此时 JavDB 失败，只产生一个 error，聚合结果成为成功的空数组。

影响不仅是界面显示“没有磁力”。[check.go:89](C:/Users/36245/Desktop/miyabi/internal/monitor/check.go:89) 将真实查询失败用于短期重试，却将空列表用于正常排期/过期判断，因此上游故障可能被当成无资源，并把达到期限的订阅转为 stale。

最小修复：只把启用来源计入本次聚合，或显式区分 skipped 与成功空结果。保持“部分成功可用、全部实际来源失败返回错误”的语义。

验证：JavBus 关闭且 JavDB 失败必须报错；JavDB 成功但无结果则应成功返回空数组；两源启用、一源失败且另一源成功仍可返回结果。

### A11 · P2 · 字幕失败被伪装成正常空结果

证据：[subtitle/aggregator.go:55](C:/Users/36245/Desktop/miyabi/internal/subtitle/aggregator.go:55)、[subtitle-menu.tsx:54](C:/Users/36245/Desktop/miyabi/web/src/features/player/subtitle-menu.tsx:54)、[subtitle-menu.tsx:78](C:/Users/36245/Desktop/miyabi/web/src/features/player/subtitle-menu.tsx:78)。

所有 provider 查询失败时，聚合器仍返回 `(nil, nil)`；前端即使收到异常，也 catch 后设置空数组，显示“未检索到匹配的在线字幕”。应用字幕、调整和重置偏移失败仅 console.error。用户无法区分无字幕、网络失败和保存失败，也得不到有效重试反馈。

最小修复：保留部分成功，但全部来源失败返回聚合错误；前端区分 error/empty 状态，并采用项目已有错误提示方式。自动抓取允许 best effort，但应记录原因；手动操作应有可见结果。不需要为此增加通用状态机框架。

验证：全部 provider 超时、一源失败一源有结果、成功空结果分别呈现正确状态；偏移保存失败保留旧值并提示。

### A14 · P2 · SSRF 传输校验把本机代理当作目标拦截

证据：[netx/ssrf.go:134](C:/Users/36245/Desktop/miyabi/internal/netx/ssrf.go:134)、[netx/ssrf.go:179](C:/Users/36245/Desktop/miyabi/internal/netx/ssrf.go:179)、[subtitle/aggregator.go:37](C:/Users/36245/Desktop/miyabi/internal/subtitle/aggregator.go:37)。

NewSafeTransport 的 DialContext 拒绝 loopback/private IP，同时该 transport 使用配置代理。HTTP/HTTPS 代理模式下，TCP 拨号地址是代理本身，例如界面示例 `127.0.0.1:7890`。于是用户配置的合法本机/局域网代理被拒绝。网络设置测试使用另一套客户端，可能报告连接正常，而字幕下载仍失败。

最小修复：区分用户配置的可信代理入口与不可信下载目标，分别制定连接和目标校验策略；保留目标限制，不能直接删除 SSRF 防护。代理端解析目标域名时，也要明确其信任边界，不能声称校验本机 DNS 就覆盖了代理 DNS。

验证：本机 HTTP 代理访问公开测试目标成功；不可信私网目标仍被拒绝；重定向和直连路径保持限制。

### A15 · P2 · “直连”仍受环境变量影响

证据：[netx/client.go:33](C:/Users/36245/Desktop/miyabi/internal/netx/client.go:33)、[pan/client.go:116](C:/Users/36245/Desktop/miyabi/internal/pan/client.go:116)、[netx/client_test.go:32](C:/Users/36245/Desktop/miyabi/internal/netx/client_test.go:32)。

newTransport 克隆 http.DefaultTransport，继承 ProxyFromEnvironment；NewDirectRestyClient 没有设置 `Proxy = nil`。Pan 的注释明确要求 115 始终直连，但部署环境存在 HTTP_PROXY/HTTPS_PROXY 时，API 和媒体仍可能经过代理，违背该约定。

最小修复：直连构造器显式禁用 Proxy；需要环境代理的场景另行明确。现有测试先清空环境变量，只证明无代理环境下的行为，没有检验“始终直连”。

验证：在具有非空环境代理的新进程中检查直连 transport，确认不使用代理；注意 Go 对环境代理配置的缓存，不要依赖同一进程中反复改环境变量制造不可靠测试。

### A16 · P2 · 图片代理缺少目标与资源边界

证据：[discover.go:75](C:/Users/36245/Desktop/miyabi/internal/api/discover.go:75)、[discover.go:153](C:/Users/36245/Desktop/miyabi/internal/api/discover.go:153)、[javdb/media.go:29](C:/Users/36245/Desktop/miyabi/internal/javdb/media.go:29)。

`/api/image?url=...` 将调用方 URL 交给 FetchMedia，后者只验证绝对 HTTPS 地址，不限制为实际图片 CDN 或非私网目的地。请求发生后才判断内容是否图片，无法阻止对服务端可访问的内部 HTTPS 服务发起请求；HTTPS 和证书验证不是目的地址授权。

Resty 同时完整缓冲响应，没有正文上限；XOR 分支再分配接近同等大小的内存。即便最终判定不是图片，也已付出下载和内存成本。该接口在 access gate 保护组中，风险取决于 gate 和部署可访问性，不能笼统称为始终可匿名利用。

最小修复：明确允许的图片来源策略，限制连接目标和响应字节数，再做内容判断；若复用 safe downloader，应先修 A14。图片缓存中的解码还应有合理像素上限，避免仅限制压缩字节数。

验证：受控内部 HTTPS 目标被策略拒绝；无 Content-Length 的超大响应也在上限停止；正常 CDN 图片和 XOR 图片兼容。验证用本地测试桩，不需要访问真实内网服务。

### A17 · P2 · 应用鉴权和上游鉴权混为一类

证据：[api/error.go:48](C:/Users/36245/Desktop/miyabi/internal/api/error.go:48)、[pan/errors.go:11](C:/Users/36245/Desktop/miyabi/internal/pan/errors.go:11)、[emby/service.go:199](C:/Users/36245/Desktop/miyabi/internal/emby/service.go:199)、[api/client.ts:104](C:/Users/36245/Desktop/miyabi/web/src/api/client.ts:104)。

115 凭据过期、Emby API Key 错误使用 KindUnauthorized，并映射 HTTP 401。前端对所有非 `/api/auth/` 的 401 清除本地 token、派发应用 unauthorized 事件。测试一个错误的 Emby Key 就能影响仍然有效的 Miyabi 登录状态。

浏览器有效 cookie 可能让随后重新验证恢复，因此不是必然永久退出；但清 token、触发访问门重新校验和相关界面扰动本身就是错误的跨领域副作用。

最小修复：区分应用会话失效与上游账户失效，返回明确业务错误码；前端仅针对本应用鉴权失效执行全局清理。避免依赖提示文本判断。

验证：Emby 错 Key、115 过期、Miyabi 过期分别走各自恢复流程；前两者不清除有效应用会话。

### A24 · P2 · 绑定失败后继续执行本地扫描

证据：[api/movie.go:92](C:/Users/36245/Desktop/miyabi/internal/api/movie.go:92)、[api/helpers.go:11](C:/Users/36245/Desktop/miyabi/internal/api/helpers.go:11)、[api/error.go:79](C:/Users/36245/Desktop/miyabi/internal/api/error.go:79)。

handler 使用 `req, _ := bindJSON` 丢弃成功标志。非法 JSON 会产生绑定错误，但零值请求继续回退到 defaultDir 并执行扫描；成功写出响应后，错误中间件因 Writer.Written 而跳过原错误。错误请求不仅没有被拒绝，还产生导入副作用。

最小修复：与其他 handler 一样检查 ok 后返回。若允许空 body 表示默认路径，专门定义该语义；语法错误不能等同于空配置。

验证：损坏 JSON 返回 400，ScanLocal 测试桩调用次数为零；合法空对象是否走默认目录按接口约定验证。

## 4. 性能与资源生命周期

### A20 · P2 · TTL 和“512”都不是实际容量上限

证据：[scrape/scrape.go:319](C:/Users/36245/Desktop/miyabi/internal/library/scrape/scrape.go:319)、[javbus/cache.go:45](C:/Users/36245/Desktop/miyabi/internal/javbus/cache.go:45)。

目录缓存命中时检查 TTL，但过期条目不被清除；未再次访问的目录和整份 `[]pan.File` 持续驻留。InvalidateDirCache 当前仅测试调用，生产路径没有利用它清理。随着不同目录被刮削，驻留量会累积。

JavBus 缓存在 `len >= 512` 时只删过期项，如果 512 条都新鲜，仍继续插入，所谓容量上限并不存在。每次超限插入还扫描整个 map。它还保存生产代码没有读取的 HTML，进一步增加成本。

最小修复：为各自缓存设置可证明的容量/字节预算及淘汰规则，去掉未读取 HTML；视业务需要让写入后失效真正接入。现有 catalogue 响应缓存语义不同，无需把所有缓存强行合成一个复杂通用框架。

验证：持续读取大量不同目录和 513 个以上新鲜详情键，断言条目/字节预算受到约束；过期淘汰不误删刚更新的数据。

### A21 · P2 · 本地扫描的数据库事务承担慢副作用

证据：[local.go:154](C:/Users/36245/Desktop/miyabi/internal/library/scan/local.go:154)、[local.go:169](C:/Users/36245/Desktop/miyabi/internal/library/scan/local.go:169)、[local.go:182](C:/Users/36245/Desktop/miyabi/internal/library/scan/local.go:182)、[database/sqlite.go:105](C:/Users/36245/Desktop/miyabi/internal/database/sqlite.go:105)。

SQLite 使用 immediate 写事务；ingestMedia 在事务内读取 NFO、原图、STRM，进行图片解码/变换/缓存落盘，还发布外部更新。一个目录的多段视频会重复走这段流程。慢磁盘和大图处理期间，占用的是数据库唯一写入机会，会阻塞播放进度等其他写请求。

同时 SaveMovieMetadata、Movie 更新、Subtitle 创建等关键失败被忽略，方法仍可能成功返回并累计“导入成功”统计。允许缺失可选 sidecar 与忽略数据库失败，应当有明确区别。

最小修复：在短事务前按目录准备并复用 NFO/图片结果；事务中只做必要读写，持久化失败返回；统计在提交后确认，外部通知在提交后执行。不要通过增大 busy_timeout 掩盖写锁持有时间。

验证：注入慢图片处理时，其他短写事务不被这段处理持锁阻塞；数据库保存失败不能返回成功计数；同目录多段视频不重复解码同一封面。

### A22 · P2 · 限速批处理占住整个任务池

证据：[config/runtime.go:9](C:/Users/36245/Desktop/miyabi/internal/config/runtime.go:9)、[monitor/batch.go:96](C:/Users/36245/Desktop/miyabi/internal/monitor/batch.go:96)、[tasks/pool.go:65](C:/Users/36245/Desktop/miyabi/internal/tasks/pool.go:65)。

所有任务共享一个 worker。订阅批处理在 handler 内遍历全部 ID，每项间 sleep 1.5–3 秒，还等待查询和提交。100 项仅间隔时间就约 148.5–297 秒；期间后续扫描、刮削、封面任务无法由该 worker 执行。后台下载已完成也可能等待入库。

最小修复：让批处理按小批次/下一执行时间推进并释放 worker，或给订阅提交提供独立的串行通道。保持扫描与写 sidecar 的有序约束，不能简单把全局 worker 数改大。

验证：长批处理运行时，后加入的扫描能在有界时间内开始；115 的提交间隔仍满足原约束；结合 A04 验证分段恢复。

### A23 · P2 · Emby 定时同步缺少运行生命周期管理

证据：[emby/service.go:88](C:/Users/36245/Desktop/miyabi/internal/emby/service.go:88)、[emby/service.go:119](C:/Users/36245/Desktop/miyabi/internal/emby/service.go:119)、[emby/service.go:463](C:/Users/36245/Desktop/miyabi/internal/emby/service.go:463)。

ScheduleActorSync 使用 time.AfterFunc；Stop 不能停止已经开始的回调。新通知/配置变更再次调度时，旧 SyncActorAvatars 可能仍运行，新一轮可以重叠查询和上传。Close 取消 context，但 wg 只登记通知 worker，没有登记这些回调，因此不等待它们实际退出。

最小修复：让头像同步进入单一、可合并触发、可取消且可等待的 worker 生命周期。可以在同一个 emby 包内分离 HTTP client、通知队列和头像任务，不必新增服务层级或抽象调度框架。

验证：第一次同步被测试桩阻塞时重复触发，最多一个同步在执行；Close 取消并等待退出后不再出现新的调用。

## 5. 架构职责与过度设计

### A25 · P3 · catalogue 的投影方向产生重复工作

证据：[catalogue/service.go:184](C:/Users/36245/Desktop/miyabi/internal/catalogue/service.go:184)、[catalogue/service.go:239](C:/Users/36245/Desktop/miyabi/internal/catalogue/service.go:239)、[catalogue/service.go:254](C:/Users/36245/Desktop/miyabi/internal/catalogue/service.go:254)、[catalogue/magnets.go:79](C:/Users/36245/Desktop/miyabi/internal/catalogue/magnets.go:79)、[movie-state-cache.ts:71](C:/Users/36245/Desktop/miyabi/web/src/api/movie-state-cache.ts:71)。

Search/Browse/MovieDetail 都调用 projectMovies，查询本地状态。MovieSummary 再把它丢掉；BrowseMovies 先构造 API Movie，再拆回 domain.Movie；CatalogueMagnets 先构造 URI 再剥离。前端又明确不采用 catalogue 返回的本地状态，而单独请求状态缓存。核心数据与展示投影的依赖方向倒置，造成可避免的分配、数据库查询和失败耦合。

最小修复：domain 数据读取作为基础，API 投影是终点；后台使用基础结果。前端已有独立本地状态协议，应收敛重复字段，若保留 API 兼容则先在内部绕开无用查询。保留前端批量状态请求和防止旧 catalogue 覆盖新状态的设计。

验证：统计同一浏览/订阅路径的数据库调用，后台只需要元数据时不查询库状态；用户可见发布状态、日期规范化等投影行为仍保持正确。

### A26 · P3 · 位置敏感的 `...any` 比类型化参数更复杂

证据：[scan/reconcile.go:20](C:/Users/36245/Desktop/miyabi/internal/library/scan/reconcile.go:20)。

ReconcileScan/ReconcileScanTx 对 embyOpts 做运行时 type switch，再按“第几个 string”解释 embyDir/publicURL/token。字符串次序写错仍能编译，不支持的类型被静默忽略，函数体增加与 reconciliation 无关的兼容解析。

最小修复：一个小的类型化配置值，或由已持有这些依赖的 Scanner 显式传递；避免无必要的 variadic compatibility。结合 A03/A19 使 reconciliation 聚焦数据库状态，导出与通知由调用方安排。没有必要再为每个字段拆接口/包。

## 6. 冗余代码与重复实现

### A27 · P3 · 选择逻辑和卡片交互外壳重复

证据：[use-history-selection.ts:3](C:/Users/36245/Desktop/miyabi/web/src/features/history/use-history-selection.ts:3)、[subscriptions/page.tsx:34](C:/Users/36245/Desktop/miyabi/web/src/features/subscriptions/page.tsx:34)、[history-card.tsx:35](C:/Users/36245/Desktop/miyabi/web/src/features/history/history-card.tsx:35)、[subscription-card.tsx:123](C:/Users/36245/Desktop/miyabi/web/src/features/subscriptions/subscription-card.tsx:123)。

历史页已有泛型 ID 集合选择 hook；订阅页重新实现 selected Set、toggle、退出清空、全选判断和筛选有效 ID。两个卡片还复制选中边框/按钮类和右上 Checkbox 的整套样式。

建议：把现有 hook 改为用途准确的集合选择 hook，让调用方传入 eligible items；提取小型选中卡片外壳或 selection control，复用边框、焦点和勾选样式。保留历史播放与订阅导航/按钮等差异，不把所有 MovieCard 合成几十个 boolean 参数的万能卡片。

验收由用户在浏览器检查：退出选择、切换页签、列表刷新和删除后的选中集合一致；键盘焦点、点击区域和禁用状态一致。

### A30 · P3 · 可清理项有具体范围，不宜靠关键词批量删

下表按生产调用检索与前端入口引用检查整理。无生产调用不等于可无条件删除公开接口；测试专用 API、后端 HTTP 兼容性需分别判断。

| 位置 | 当前证据 | 建议 |
| --- | --- | --- |
| [ui/popover.tsx](C:/Users/36245/Desktop/miyabi/web/src/components/ui/popover.tsx) | 当前前端入口引用图不可达，未发现业务 import | 无近期用途可删除此闲置 wrapper；不要批量删除其他正在使用的 UI primitives |
| [api/auth.ts:45](C:/Users/36245/Desktop/miyabi/web/src/api/auth.ts:45) | useAccessGateLogout 无生产调用 | 移除未接入 hook，或在确有登出需求时接入 |
| [api/play.ts:107](C:/Users/36245/Desktop/miyabi/web/src/api/play.ts:107) | setDefaultSubtitle、deleteSubtitle 无前端调用 | 清理前端闲置封装；这不证明后端端点没有外部使用者 |
| [lib/source.ts:5](C:/Users/36245/Desktop/miyabi/web/src/lib/source.ts:5) | sameSource 无调用，sourceKey 在使用 | 统一确实相同的来源比较，否则移除无用函数，不整文件删除 |
| [catalogue/service.go:345](C:/Users/36245/Desktop/miyabi/internal/catalogue/service.go:345) | Facets/关联类型仅测试调用，当前 API 未接入 | 核实没有契约后移除闲置 facade 与仅验证它的测试 |
| [subtitle/service.go:77](C:/Users/36245/Desktop/miyabi/internal/subtitle/service.go:77) | ListByMovie 仅测试调用，播放路径另查 Subtitle | 优先收敛实际查询入口；不要保留只在测试中存在的复用承诺 |
| [javbus/cache.go:9](C:/Users/36245/Desktop/miyabi/internal/javbus/cache.go:9) | html 存入但无生产读取 | 删除字段及相应传参，配合 A20 |
| [task-notification-diff.ts:4](C:/Users/36245/Desktop/miyabi/web/src/features/tasks/task-notification-diff.ts:4) 与 [api/tasks.ts:70](C:/Users/36245/Desktop/miyabi/web/src/api/tasks.ts:70) | isScanTask、isBatchTask、isTaskActive 重复定义 | 在现有纯模型边界共用；若为测试隔离，移动纯类型/谓词即可，不逐函数拆文件 |

这些清理优先级低于功能缺陷。建议在对应模块修复时顺手收敛，避免一笔巨大“删除冗余代码”改动遮蔽行为变化。

## 7. UI 样式一致性与可访问性

### A28 · P2 · 设置行没有可访问名称关系

证据：[settings/shared.tsx:25](C:/Users/36245/Desktop/miyabi/web/src/features/settings/shared.tsx:25)、[network-section.tsx:106](C:/Users/36245/Desktop/miyabi/web/src/features/settings/network-section.tsx:106)、[emby-section.tsx:119](C:/Users/36245/Desktop/miyabi/web/src/features/settings/emby-section.tsx:119)。

SettingRow 的标题、说明都是普通 div，与右侧 children 没有 htmlFor、aria-labelledby 或 aria-describedby 关系。网络开关和多个 Emby 输入直接放入，调用处也未补充名称。视觉上清楚的“代理服务”等标题，不会自动成为控件的可访问名称；placeholder 也不是可靠的持久标签。

建议：在 SettingRow 这一现有复用点生成/接收稳定 ID，并由调用方把名称、说明关联到实际 Switch/Input/SelectTrigger。一个 row 含多个控件时分别提供标签，避免自动 clone 任意 children 的魔法封装。

验收由用户在浏览器完成：键盘访问各控件，辅助技术能读出名称、说明和状态；点击可点击标签时关联正确。

### A29 · P3 · 播放器与状态颜色存在平行样式规则

证据：[player/player.css:35](C:/Users/36245/Desktop/miyabi/web/src/features/player/player.css:35)、[subtitle-menu.tsx:157](C:/Users/36245/Desktop/miyabi/web/src/features/player/subtitle-menu.tsx:157)、[subtitle-menu.tsx:212](C:/Users/36245/Desktop/miyabi/web/src/features/player/subtitle-menu.tsx:212)、[library/movie-status.tsx:10](C:/Users/36245/Desktop/miyabi/web/src/features/library/movie-status.tsx:10)。

播放器已有基于 popover/foreground/accent 的 `--media-menu-*` 变量。字幕菜单另写 zinc950、white 多组透明度、9/10/11px 字号和独立行样式；四个偏移按钮复制同一个长 class。LibraryMovieStatus 还把 destructive 变量强制覆盖为固定色值，绕开暗色主题对应值。

建议：字幕菜单复用现有播放器语义变量和小型菜单行/偏移按钮变体；状态 Badge 交给全局语义 token。播放器保持固定暗色本身合理，不应为统一主题而强迫它跟随页面变亮。尺寸特例可以保留，但应能说明其布局用途。

验收由用户对比字幕、画质/设置菜单在全屏、窗口、窄屏中的背景、字号、行高、hover/focus、分隔线；检查明暗主题下失败 Badge。一致性在此是静态样式规则结论，未声称已看过浏览器像素效果。

## 8. 测试与工程规范

### A31 · P3 · 检查通过不等于关键行为被验证

证据：[docker-publish.yml:3](C:/Users/36245/Desktop/miyabi/.github/workflows/docker-publish.yml:3)、[Dockerfile:13](C:/Users/36245/Desktop/miyabi/Dockerfile:13)、[walker_feature_test.go:54](C:/Users/36245/Desktop/miyabi/internal/library/scan/walker_feature_test.go:54)。

当前 workflow 主要是 tag 发布/镜像构建，未发现 PR 级别的前后端 test/lint/format 门禁。Docker 编译不能覆盖断点恢复、事务回滚、源切换和前端集合一致性。现有 checkpoint 测试只证明 JSON 能往返；名字不能证明业务恢复正确。

建议：增加与项目规模匹配的自动检查，优先补 A01/A02/A04/A18 这类有真实失败模式的回归测试。保留既有 pipeline 测试，不用大量“函数调用后等于函数内部常量”的测试换取覆盖率数字。格式检查应进入同一个基础检查流程。

## 结构收敛建议

| 区域 | 合理的职责边界 | 应减少的复杂度 |
| --- | --- | --- |
| scan | 遍历/观察 → 数据库 reconciliation → 提交后调度与通知 | 不完整断点协议、事务内文件副作用、`...any` 参数 |
| scrape / Emby export | 一个入口计算并写出预期产物；可重复执行、可修复缺失 | fast/full 多写者、重复 URL/路径构建、用记录数量猜文件是否存在 |
| subtitle | 来源读取 → 统一转换 → 缓存 → 带目标范围的上传 → 数据库关联 | 手动/自动流程分叉、原文件与缓存混用、吞错和无来源查询 |
| catalogue | domain 元数据/磁力结果在底层，展示投影只在需要处执行 | 投影后剥离、后台流程依赖无用本地状态查询 |
| monitor / tasks | 明确的每项结果、检查点和调度间隔 | 重新从头跑却保留计数、失败仍前移游标、长 sleep 占住唯一 worker |
| emby | 包内 HTTP 访问、通知批处理、头像同步生命周期清楚 | AfterFunc 形成未登记工作、重复调度重叠 |
| frontend state | 一个业务事实由一套权威 query/响应维护 | 从截断列表推导不存在、局部拼接服务端已替换集合 |
| frontend UI | 基础 primitives + 小型语义复用 + 页面自己的编排 | 多套选择集合逻辑、卡片外壳粘贴、局部主题变量覆盖 |

这些边界大多可在现有包/目录内完成，不要求新增仓储层、事件总线、全局状态机、微服务或配置驱动表单。判断拆分是否值得，应看它是否消除真实重复或隔离独立生命周期，而不是追求单文件行数阈值。

UI 复用的具体落点：

| 复用对象 | 放置/演进方向 | 保留差异 |
| --- | --- | --- |
| MovieCard 的媒体视觉骨架 | 继续使用现有 components/movie | 历史进度、订阅操作、库内 hover details 留在各 feature |
| 集合选择 hook | 从 useHistorySelection 演进为 ID 集合选择 | 哪些项可选、何时清空由页面决定 |
| 可选卡片外壳 | 统一边框、checkbox、focus、disabled | Button/Link 的导航语义必须正确 |
| SettingRow | 补足 label/description/control 关联 | 不同表单控件和校验保留在设置 section |
| 设置操作区 | 网络/Emby 重复布局足够稳定时共用小布局 | 测试与保存的请求、pending 和错误策略各自保留 |
| 播放器菜单 | 使用现有主题 token 和局部行/按钮样式 | 播放器专用暗色及全屏 portal 容器保留 |

## 不应以“洁癖”为由删除的设计

- app.New 是组合根，集中装配依赖属于它的职责。它连接很多服务不等于上帝对象；不要把装配分散到隐式全局初始化。
- Ent 生成的 CRUD、TanStack 生成的路由文件、正在使用的 shadcn/Radix wrappers，不应按手写冗余来评判。真正闲置的 wrapper 可单独清理。
- drive 的来源快照、凭据代际、提交锁，以及播放进度的会话/版本约束，有账户切换和迟到请求的具体失败模式；A18 需要补齐缓存代际，不能通过删除这些约束来“简化”。
- 分页不完整时拒绝危险的全量 reconcile、下载大小限制、连接时校验、文件原子写入和 Windows 兼容回退，都有明确保护对象。应修正错误作用域，不应整体撤掉防护。
- catalogue 响应缓存中的请求合并、取消和引用生命周期，以及前端独立本地状态的失效机制，有实际用途；需要纠正重复投影，不能用回填旧 catalogue 状态替代。
- provider/driver/任务 handler 接口有测试替身或多个实现支撑。当前没有依据建议全面取消接口，也没有依据再给每个函数增加一层接口。
- 大型字幕菜单同时承载查询、应用、偏移与样式，确实值得按数据操作和菜单展示收敛；但把每个小按钮、每个 if 都移成独立文件只会增加跳转成本。

## 已执行检查与限制

| 检查 | 结果 |
| --- | --- |
| `npm.cmd test`，web 目录 | 102 项通过 |
| `npm.cmd run lint` | 通过，退出码 0 |
| `npm.cmd run build` | 通过，包含 Vite 构建和 `tsc --noEmit` |
| `npm.cmd run fmt:check` | 未通过，4 个文件存在格式差异，见下方 |
| Go test / vet / race | 未执行：Go 不在 PATH，检查的常见安装位置也无可执行文件；未安装依赖/工具链 |
| 浏览器、视觉与辅助技术测试 | 未执行，按项目约定由用户完成 |
| 外部平台/真实网络故障复现 | 未执行；相关结论来自实现和调用链，不声称已在真实服务上复现 |

格式差异文件：[dropdown-menu.tsx](C:/Users/36245/Desktop/miyabi/web/src/components/ui/dropdown-menu.tsx)、[access-gate.tsx](C:/Users/36245/Desktop/miyabi/web/src/features/access-gate/access-gate.tsx)、[data-source-section.tsx](C:/Users/36245/Desktop/miyabi/web/src/features/settings/data-source-section.tsx)、[network-section.tsx](C:/Users/36245/Desktop/miyabi/web/src/features/settings/network-section.tsx)。本次未顺手格式化应用源码。

初次直接运行 TypeScript 检查时，缺少构建过程生成的路由文件；执行项目正式 build 后检查通过，所以不将初始错误列为源码缺陷。构建产生忽略的 dist 和生成路由文件。HLS 懒加载 chunk 大小警告单独不足以认定性能问题，需要真实加载与使用数据。

## 建议修复顺序与复审方式

1. **先保护数据。** 修 A01、A02，并补中断恢复和事务失败的回归测试。修复前不要把“支持 checkpoint”当成安全恢复保证。
2. **再统一状态边界。** 处理 A03–A13、A17–A18、A24；先确定本地媒体能力，再贯通后端范围与前端行为。
3. **处理资源与后台流程。** A14–A16、A19–A23；统一导出、明确缓存上限、缩短事务、管理后台任务生命周期。
4. **按功能域收敛重复。** A25–A30 配合对应功能修复，小批次提交；保留 UI 和接口行为的可审查性。
5. **补工程门禁并由用户验证 UI。** 针对实际失败模式补测试，前端构建/lint/format 保持可执行，再验收播放器、订阅选择和设置控件。

交给第二位审计者时，建议要求其逐项确认“基线和行号 → 完整调用链 → 是否有遗漏的保护条件 → 最小触发场景 → 最小修复”，重点反证 A01/A02/A03/A18，而不是只复述标题。若代码在此之后变化，应重新定位证据；没有发现的问题不代表已被证明不存在。

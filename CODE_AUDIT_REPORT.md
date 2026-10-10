# Miyabi 代码审计报告

审计日期：2026-10-10  
审计基线：`f3f3a79a3750534e3b2fc3c1baabe1bced5ab66f`

## 1. 审计结论与范围

本次记录 **29 项发现：P1 2 项、P2 17 项、P3 10 项**。其中前端 9 项、后端 12 项、测试 6 项、工程配置 2 项。统计包含维护性改进，不表示存在 29 个独立的运行时故障。

最先处理的是图片代理的目标地址边界，以及演员订阅失败后仍推进游标的问题。两者均已通过隔离的本地验证确认。另已验证 Debug SQL 日志暴露设置值、图片响应缺少字节限制、115 并发限制提前释放，并检查了演员新作查询的 SQLite 执行计划。

项目已经具备统一 UI 基础组件、严格 TypeScript 配置、任务快照共享、请求合并、扫描检查点、事务提交后通知和较多回归测试。建议针对下列问题做小步修正，不进行整库重写或引入新的通用框架。

| 优先级 | 含义                                           | 数量 |
| ------ | ---------------------------------------------- | ---: |
| P1     | 应优先修复的访问边界或业务数据遗漏问题         |    2 |
| P2     | 有明确触发条件的可靠性、资源、性能或可用性问题 |   17 |
| P3     | 冗余、组织、样式或测试维护性问题               |   10 |

审查覆盖前端样式、重复实现、职责划分、状态管理、错误处理、性能，以及后端 API、订阅、任务、扫描、元数据、图片、导出、数据库和工程配置；同时加入多余、不必要、重复及与实现过度绑定的测试审查。

本报告遵循审计过程中补充的项目约定：**除 shadcn 原始组件外，所有组件不使用 aria 无障碍。** 业务组件缺少 aria 不列为问题，也不建议增加 aria；现有手写 aria 用法按约定一致性检查。shadcn 原始组件内部实现保留。

盘点范围包括 `cmd`、`internal`、`web/src`、`web/tests`、嵌入入口、构建与 CI 配置。`cmd/internal` 中盘点到 196 个手写 Go 源文件和 194 个 Go 测试文件；前端盘点到 142 个源码文件、30 个测试文件及 2 个测试辅助文件。Ent 自动生成代码和路由生成文件不作为“手写冗余代码”评价对象，保留对 Ent schema 和手写事务代码的审查。

方法为全仓静态检索、关键调用链审阅、已有检查执行及隔离复现。没有安装依赖，没有操作浏览器，没有连接真实网盘或为审计访问外部站点，也没有读取生产数据库内容。未修改 README、业务代码或正式测试，未提交 Git。构建生成了被 Git 忽略的前端产物；临时审计验证源码已清理。

本文的文件行号对应上述基线。“静态确认”表示可以从当前实现确认行为或结构；性能影响若未测量实际耗时，会明确写出适用规模或后续验证方法。浏览器布局、交互和运行时性能由用户验收。

未单列本地验证记录的项目，依据静态代码、调用关系或构建产物判断；整改后的验证建议不表示本次已经执行了相应整改。

## 2. 前端发现

### F01 · P2 · 状态徽标的前景与背景对比度不足

**位置：** [badge.tsx:13](E:/ppxb/miyabi/web/src/components/ui/badge.tsx:13)、[globals.css:17](E:/ppxb/miyabi/web/src/styles/globals.css:17)、[movie-badges.tsx:31](E:/ppxb/miyabi/web/src/components/movie/movie-badges.tsx:31)。

`success` 使用 emerald-500 配白字，`downloading` 使用 cyan-400 配白字；徽标文字为 `text-xs`。按当前 Tailwind OKLCH 色值换算 sRGB 后，白字对比度分别约为 **2.46:1、1.81:1**，浅色背景上的小字辨识度偏低。下载状态还直接指定调色板颜色，与全局语义色的管理方式不一致。

**影响：** “已浏览”“下载中”等状态在亮色背景上不易辨认；调整全局主题时，这部分颜色容易遗漏。

**建议与验收：** 为状态色配套定义前景色，使用更深的背景或深色文字，并通过同一语义变量管理。核对明暗主题下的实际颜色；本次只进行了色值计算，没有进行浏览器视觉验收。

### F02 · P3 · 三处业务组件手写 aria 与最新项目约定不一致

**位置：** [task-toast-actions.tsx:28](E:/ppxb/miyabi/web/src/features/tasks/task-toast-actions.tsx:28)、[task-toast-actions.tsx:51](E:/ppxb/miyabi/web/src/features/tasks/task-toast-actions.tsx:51)、[magnets.tsx:184](E:/ppxb/miyabi/web/src/features/movie-detail/magnets.tsx:184)。

排除 shadcn 基础组件目录后，发现取消任务、关闭通知、尝试下一条磁力三个业务按钮手写了 `aria-label`。三处同时已有同内容的 `title`。

**影响：** 业务层的实现约定不统一；此项是根据本次新增约定识别的整理项，不作为功能故障。

**建议与验收：** 后续整理时移除这三处业务层手写 aria，保留现有 title、点击行为和禁用状态，不改动 shadcn 原始组件内部实现。通过源码检索核对即可，不新增以 aria 属性为目标的业务组件测试。

### F03 · P2 · 每张订阅卡片重复建立全量订阅 Map

**位置：** [subscriptions.ts:55](E:/ppxb/miyabi/web/src/api/subscriptions.ts:55)、[subscriptions.ts:90](E:/ppxb/miyabi/web/src/api/subscriptions.ts:90)、[movie-subscribe-button.tsx:10](E:/ppxb/miyabi/web/src/components/movie/movie-subscribe-button.tsx:10)、[monitor/service.go:165](E:/ppxb/miyabi/internal/monitor/service.go:165)。

`useSubscriptionTargets` 的内联 select 每次遍历完整 targets 数组并创建 Map。`useSubscription` 在每个使用订阅按钮的卡片中创建独立 observer，且每次渲染产生新的 select 函数。Query 缓存可以合并 HTTP 请求，但不会把这些独立的 Map 构建自动合并成一次。

**影响：** N 条订阅、C 个同时显示的消费者，在首次处理和选择器重算时产生 O(N×C) 遍历及多份 Map；订阅较多时，状态刷新会放大分配和渲染成本。服务端 targets 又是全量返回，成本随历史订阅增加。本次未测量浏览器帧率。

**建议与验收：** 按 targets 数组引用共享一次索引构建，单卡片仅选择自己的结果；页级统计只在数组变化时计算。若数据规模继续增长，再考虑类似 movie-states 的按可见 ID 批量查询。用合成大订阅集检查索引构建次数，不把 Map.get 的 O(1) 当作整个处理流程的复杂度。

### F04 · P2 · 状态 Hook 丢弃错误信息，失败被表现为“未订阅”或无状态

**位置：** [subscriptions.ts:90](E:/ppxb/miyabi/web/src/api/subscriptions.ts:90)、[movie-states.ts:96](E:/ppxb/miyabi/web/src/api/movie-states.ts:96)、[subscription-action.tsx:8](E:/ppxb/miyabi/web/src/features/movie-detail/subscription-action.tsx:8)、[movie-subscribe-button.tsx:11](E:/ppxb/miyabi/web/src/components/movie/movie-subscribe-button.tsx:11)。

`useSubscription` 只返回 subscription 和 isPending，`useMovieState` 只返回 data 和 isPlaceholderData。初次查询失败时 subscription 为 undefined、isPending 已为 false，按钮会恢复为可订阅状态；影片状态失败则只是不显示徽标。调用方无法区分“已确认不存在”和“尚未读到状态”。

**影响：** 用户获得错误的操作提示，无法重试状态查询，也无法判断状态是否可信。后端已有幂等保护，因此这里不直接断言会产生重复数据库记录。

**建议与验收：** 保留错误、加载和重试信息；在状态未知时展示简短错误或禁用依赖该状态的操作。验证首次失败、已有缓存后刷新失败及恢复成功三种情况。

### F05 · P2 · SSE 重连探测请求没有完整的清理与超时生命周期

**位置：** [task-events.tsx:59](E:/ppxb/miyabi/web/src/features/tasks/task-events.tsx:59)、[task-events.tsx:134](E:/ppxb/miyabi/web/src/features/tasks/task-events.tsx:134)。

重连探测的 AbortController 和五秒计时器只存在于 `restartConnection` 内，effect cleanup 无法取消该请求。fetch 返回响应头后立即清除计时器，再等待 401 响应的 JSON；如果响应体迟迟未结束，五秒期限已经失效，后续重试不会安排。请求抛错时，计时器也没有通过 finally 统一清理。

**影响：** 手动重连或组件卸载后仍可能保留旧探测请求；慢 401 响应体可以卡住该次自动重连流程。过期探测还可能触发额外的全局认证刷新。

**建议与验收：** 让 effect 管理探测 controller，在 cleanup 中 abort；计时器覆盖读取响应体的全过程并在 finally 清理，异步返回后检查 disposed。用模拟 fetch 验证“先返回 401 响应头、延迟响应体”和“探测期间卸载”，无需浏览器或真实服务。

### F06 · P3 · 同规格按钮重复指定图标间距，产生局部样式漂移

**位置：** [button.tsx:26](E:/ppxb/miyabi/web/src/components/ui/button.tsx:26)、[network-section.tsx:135](E:/ppxb/miyabi/web/src/features/settings/network-section.tsx:135)、[emby-section.tsx:262](E:/ppxb/miyabi/web/src/features/settings/emby-section.tsx:262)、[data-section.tsx:92](E:/ppxb/miyabi/web/src/features/settings/data-section.tsx:92)。

`Button size="sm"` 已提供 gap-1。网络和 Emby 的保存按钮又给加载图标增加 `mr-1.5`，使图标与文字间距叠加为 10px；其他同规格按钮通常只使用 4px gap。前两者还使用 14px 图标，其他按钮沿用默认 16px。

**影响：** 相同操作栏的加载态间距、图标尺寸不一致；调整统一 Button 样式时仍需追踪局部覆盖。

**建议与验收：** 删除无必要的额外 margin，同规格按钮沿用组件默认尺寸。只在确有不同语义或密度需求时使用局部覆盖。此项属于样式整理，适合由用户视觉验收，不必新增锁定 className 的测试。

### F07 · P3 · 通用影片卡片同时承担展示和发现页业务绑定

**位置：** [movie-card.tsx:3](E:/ppxb/miyabi/web/src/components/movie/movie-card.tsx:3)、[movie-card.tsx:12](E:/ppxb/miyabi/web/src/components/movie/movie-card.tsx:12)、[movie-card.tsx:34](E:/ppxb/miyabi/web/src/components/movie/movie-card.tsx:34)、[subscriptions/page.tsx:29](E:/ppxb/miyabi/web/src/features/subscriptions/page.tsx:29)。

同一个 components 模块既定义纯展示的 `MovieCard`，又定义绑定详情弹窗、订阅按钮和状态查询的 `DiscoverMovieCard`，形成基础展示层对 feature 的反向依赖。订阅页面还从 `subscription-card.tsx` 导入纯状态判断函数 `isPendingSubscription`。

**影响：** 修改展示组件需要同时考虑发现页业务；纯业务规则的使用者和测试也被带入 UI 模块依赖，职责边界不清晰。

**建议与验收：** 将发现页的绑定组件放到对应 feature，保留现有 MovieCard 的 props 组合方式；把纯状态判断放到订阅 feature 的普通 ts 模块。移动后核对导入图和构建，不新增卡片工厂、通用 controller 或多层转发。

### F08 · P3 · Popover 组件没有调用者

**位置：** [popover.tsx:1](E:/ppxb/miyabi/web/src/components/ui/popover.tsx:1)。

从 main.tsx 开始检查静态及字面量动态导入图，并用全仓引用检索复核，未发现该组件的使用者。

**影响：** 增加无效维护面；其样式字符串还可能被 Tailwind 源码扫描纳入样式候选。它未被应用导入，不能据此声称其 JavaScript 已进入首屏包。

**建议与验收：** 如没有明确的近期使用计划，可删除该单独文件。不要因为它未使用就删除仍被其他组件使用的 Radix 依赖，也不要把所有 UI 组件的备用导出一概认定为冗余。

### F09 · P2 · 列表后台刷新失败时隐藏仍可使用的缓存内容

**位置：** [discover/results.tsx:27](E:/ppxb/miyabi/web/src/features/discover/results.tsx:27)、[library/page.tsx:62](E:/ppxb/miyabi/web/src/features/library/page.tsx:62)、[movie-detail/content.tsx:23](E:/ppxb/miyabi/web/src/features/movie-detail/content.tsx:23)。

发现页和媒体库只要 isError 为 true 就切换到整页错误状态。TanStack Query 后台刷新失败时可能仍保留上一份 data，因此列表和分页会被一起隐藏。详情页已经使用“有缓存则展示内容并附加 InlineError”的处理，二者不一致。

**影响：** 返回已浏览页面或任务触发刷新时，一次临时失败就会中断浏览；用户丢失当前可用的页面内容。

**建议与验收：** 只有首次加载且没有可用数据时显示整页错误；刷新失败时保留内容并提供重试提示。核对缓存页恢复、换页失败和详情页行为，避免把不同查询页的数据错误地当作当前页结果。

## 3. 后端发现

### B01 · P1 · 图片代理绕过已有的安全下载边界

**位置：** [api/router.go:134](E:/ppxb/miyabi/internal/api/router.go:134)、[api/discover.go:166](E:/ppxb/miyabi/internal/api/discover.go:166)、[javdb/media.go:16](E:/ppxb/miyabi/internal/javdb/media.go:16)、[netx/client.go:25](E:/ppxb/miyabi/internal/netx/client.go:25)。

`/api/image` 接收调用方提供的 URL，经 Catalogue.Media 直接进入 FetchMedia。FetchMedia 只检查 https 和非空 Host，随后使用普通 Resty transport；没有限定来源域名，也没有使用项目现有的私有地址及 DNS/IP 检查。

**验证：** 在独立测试进程中，将本地 httptest HTTPS 服务的证书加入该进程信任池，使用真实 javdb.Client.FetchMedia 请求其 loopback 地址，服务端收到请求且图片响应被返回。没有关闭 TLS 验证，也没有探测任何真实内网服务。

**影响：** 拥有图片接口访问权限的调用方，可以使服务端请求本来不应作为图片来源的 HTTPS 目标。开启访问密码时需要应用会话；未开启时接口无需会话。目标仍需满足正常 TLS 验证，本项不是 TLS 绕过。

**建议与验收：** 按实际合法图片来源限制目标，复用已有安全下载边界，覆盖 URL、直连 DNS/IP 和重定向检查，保留现有代理能力及 XOR 图片解码。验证合法 CDN 图片仍可加载，私有地址及指向私有地址的域名在发送请求前被拒绝。

### B02 · P1 · 演员子订阅写入失败后，游标仍标记为已见

**位置：** [monitor/actor.go:73](E:/ppxb/miyabi/internal/monitor/actor.go:73)、[monitor/actor.go:108](E:/ppxb/miyabi/internal/monitor/actor.go:108)、[monitor/check.go:159](E:/ppxb/miyabi/internal/monitor/check.go:159)、[monitor/schedule.go:52](E:/ppxb/miyabi/internal/monitor/schedule.go:52)。

`spawnMovie` 写入失败只记录日志，不返回错误。首次订阅演员时，父记录已经保存了包含所有当前作品的游标；日常检查也会在调用 spawnMovie 后，无条件把整批作品传给 cursor.advance。

**验证：** 临时数据库通过 Ent hook 仅让一次影片订阅创建失败。AddActor 仍成功；恢复正常写入、将父订阅设为到期并再执行 Check，结果仍为 **0 条影片订阅**，检查也没有返回错误。

**影响：** 一次临时写入失败即可漏订新作，后续检查又因作品在已见集合中而跳过，用户只看到演员订阅正常。

**建议与验收：** 让子订阅写入结果参与游标推进。可以把同批数据库变更放入事务，或仅确认成功/已存在的作品并保留失败项重试；继续处理其他作品不应意味着把失败项标记为已完成。保留已有幂等语义，验证首次订阅和日常检查的局部失败均能补回。

### B03 · P2 · Debug SQL 日志会记录明文凭据

**位置：** [database/sqlite.go:44](E:/ppxb/miyabi/internal/database/sqlite.go:44)、[database/setting.go:31](E:/ppxb/miyabi/internal/database/setting.go:31)、[drive/account.go:153](E:/ppxb/miyabi/internal/drive/account.go:153)、[drive/token.go:50](E:/ppxb/miyabi/internal/drive/token.go:50)。

开启 Debug 后，Ent 日志通过 fmt.Sprint 原样写入 slog，包含 SQL 参数。设置表保存的 JSON 含有 115 access_token、refresh_token，以及 Emby、代理等配置中的敏感值。HTTP 日志的清洗逻辑无法覆盖这条数据库日志路径。

**验证：** 在临时数据库中保存人工构造的设置值，捕获 Debug logger，确认日志包含该明文标记。验证未使用或输出真实凭据。

**影响：** 部署者为排查问题开启 Debug 并导出日志时，可能一并暴露登录凭据。默认非 Debug 模式不触发这条 SQL 日志路径。

**建议与验收：** SQL 调试日志只保留语句、耗时或参数类型等必要信息，敏感值不输出。验证登录保存、令牌刷新和 Emby 设置更新的日志不包含测试凭据，同时保留定位失败请求所需的信息。

### B04 · P2 · 图片响应无限缓冲，且前端图片代理没有并发容量限制

**位置：** [javdb/media.go:26](E:/ppxb/miyabi/internal/javdb/media.go:26)、[javdb/media.go:53](E:/ppxb/miyabi/internal/javdb/media.go:53)、[netx/client.go:25](E:/ppxb/miyabi/internal/netx/client.go:25)。

图片通过 Resty 完整读取到内存，没有响应体字节上限。XOR 图片还会分配第二个接近原响应大小的缓冲。浏览图片的路径也没有使用 metadata.Service 的容量限制；请求超时不能代替字节和并发上限。

**验证：** 本地 HTTPS 服务返回带 PNG 文件头的 **17 MiB（17,825,792 字节）**响应，FetchMedia 完整接受并返回。该验证确认读取缺少边界，没有执行内存耗尽攻击，也不代表测得了生产峰值内存。

**影响：** 异常大响应和并发图片请求会占用大量内存；吞吐量较高时，即使未超过请求时限也可能耗尽资源。

**建议与验收：** 在读取阶段限制实际响应字节数，并给图片传输设置独立的合理并发容量；保留取消能力。根据真实图片尺寸选择上限，覆盖有/无 Content-Length、XOR 与普通图片等情况。B01 的地址校验和本项资源限制需要分别成立。

### B05 · P2 · 115 并发名额在响应头返回时就被释放

**位置：** [pan/client.go:57](E:/ppxb/miyabi/internal/pan/client.go:57)、[pan/client.go:95](E:/ppxb/miyabi/internal/pan/client.go:95)、[pan/client.go:117](E:/ppxb/miyabi/internal/pan/client.go:117)、[pan/client_test.go:75](E:/ppxb/miyabi/internal/pan/client_test.go:75)。

`panTransport.RoundTrip` 用 defer 释放 inFlight。HTTP RoundTrip 返回时，响应体通常还没有读取完，因此 maxInFlight=2 实际只约束到返回响应头，无法约束完整请求生命周期。

**验证：** 使用 Go overlay 将临时验证注入原包，保持生产 transport 不变；模拟立即返回响应头、阻塞响应体的上游，排除节奏限速干扰后，观察到 **并发设置为 2，却同时存在 6 个未完成响应体**。

**影响：** 慢响应场景会突破声明的并发限制，增加连接和内存占用，也可能影响 115 风控。现有测试把延迟放在响应头之前，所以未覆盖这个问题。

**建议与验收：** 把名额释放绑定到响应体读完或 Close，并确保错误路径释放、重复 Close 不重复释放。扩展现有并发测试为响应头前与响应体内两类延迟，保留取消、超时和 Retry-After 测试。

### B06 · P2 · 扫描检查点反复序列化整个剩余目录队列

**位置：** [scan/walker.go:203](E:/ppxb/miyabi/internal/library/scan/walker.go:203)、[scan/walker.go:223](E:/ppxb/miyabi/internal/library/scan/walker.go:223)、[scan/persist.go:154](E:/ppxb/miyabi/internal/library/scan/persist.go:154)、[domain/scan.go:16](E:/ppxb/miyabi/internal/domain/scan.go:16)。

每进入一个目录，都把 directories[next:] 整段 Marshal 成字符串，再写入任务 JSON；一个目录内每批文件落库时，又携带该检查点保存。对于一个根目录下挂载 D 个子目录的情况，仅目录切换就累计序列化约 D(D+1)/2 个目录条目。

**影响：** 目录数达到 10,000 时，累计处理约 5,000 万个目录条目，带来二次增长的序列化和数据库写入成本。此为代码路径的复杂度推导，不是实际吞吐量测量。

**建议与验收：** 保留断点恢复语义，把目录队列改为增量持久化或分批队列加稳定游标，避免每次写回完整尾部。增加宽目录树的针对性基准，并继续验证中断恢复、目录发现、计数恢复和事务失败回滚。现有 BenchmarkScanPayload 只设置目录计数，没有构造这种大检查点。

### B07 · P2 · 单演员新作查询缺少匹配过滤和排序的索引

**位置：** [monitor/service.go:207](E:/ppxb/miyabi/internal/monitor/service.go:207)、[schema/subscription.go:63](E:/ppxb/miyabi/internal/ent/schema/subscription.go:63)。

单演员 feed 使用 kind、origin_id 过滤，再按 release_date、id 倒序分页。现有索引是 (kind, target_id) 和 (status, next_check_at)，没有覆盖该查询。

**验证：** 在当前 schema 创建的临时数据库，对同形 SQL 执行 EXPLAIN QUERY PLAN，得到：

```text
SEARCH subscriptions USING INDEX subscription_kind_target_id (kind=?)
USE TEMP B-TREE FOR ORDER BY
```

**影响：** 索引只能先按 kind 缩小范围，仍需过滤大量影片订阅并建立临时排序结构。订阅增长后，查看单个演员也会承担与该演员无关的数据处理成本。

**建议与验收：** 评估增加 (kind, origin_id, release_date, id) 组合索引，先针对该实际查询优化。使用代表性数据核对查询计划、分页结果和写入成本，再决定是否另行优化全部演员视图。本次只验证计划，未进行生产数据压测。

### B08 · P2 · 媒体附件直接覆盖写入，失败可能留下截断文件

**位置：** [scrape/export.go:105](E:/ppxb/miyabi/internal/library/scrape/export.go:105)、[export/export.go:81](E:/ppxb/miyabi/internal/export/export.go:81)、[subtitle/service.go:148](E:/ppxb/miyabi/internal/subtitle/service.go:148)。

NFO、STRM、导出图片及字幕使用 os.WriteFile 直接写最终文件。已存在的文件会先被截断；写入中断、磁盘错误或进程退出时，旧内容不能保留。Emby 等独立进程也不受应用内部互斥锁保护，可能在覆盖期间读到半份内容。

**影响：** 重建、地址重写等操作可能短暂或持续产生不可读附件。任务重试有机会修复，但不能替代发布时的完整性保护。

**建议与验收：** 同目录临时文件写完并关闭后，再使用适配目标平台的替换方式发布；保留图片/NFO 先于 STRM 的顺序和重复导出不改写相同文件的行为。已有 image.Cache 的临时写入实现可作为一致性参考。验证覆盖失败时旧文件仍完整，以及 Windows/Linux 上的替换行为。

### B09 · P2 · JSON 请求体没有字节上限和读取期限

**位置：** [api/helpers.go:11](E:/ppxb/miyabi/internal/api/helpers.go:11)、[api/auth.go:23](E:/ppxb/miyabi/internal/api/auth.go:23)、[api/auth.go:65](E:/ppxb/miyabi/internal/api/auth.go:65)、[app/app.go:205](E:/ppxb/miyabi/internal/app/app.go:205)。

统一 bindJSON 直接解码 Request.Body，没有 MaxBytesReader。字段的长度、数组数量校验在解码之后执行，不能限制解码阶段分配。HTTP Server 只配置 ReadHeaderTimeout 和 IdleTimeout，没有限制已经收完请求头的 JSON 请求体读取时间。

**影响：** 异常大 JSON 可以在被校验拒绝前大量占用内存；慢速请求体会持续占用处理资源。登录接口虽有限流，单个获准请求仍没有该边界。

**建议与验收：** 按实际最大批量输入给 JSON 路由设置合理字节和读取期限，超限返回可识别错误。将限制限定在 JSON 请求体，保留 SSE 和媒体流的长连接行为；验证正常批量请求、超大输入和慢读取取消。

### B10 · P3 · 导出职责分散，旧转发接口继续维持不必要的依赖

**位置：** [scrape/export.go:20](E:/ppxb/miyabi/internal/library/scrape/export.go:20)、[scrape/export.go:87](E:/ppxb/miyabi/internal/library/scrape/export.go:87)、[scrape/scrape.go:122](E:/ppxb/miyabi/internal/library/scrape/scrape.go:122)、[scan/local_ingest.go:75](E:/ppxb/miyabi/internal/library/scan/local_ingest.go:75)。

项目已有 internal/export，scrape 却继续重导出其配置、管理器、常量和函数，并保留无生产调用者的 RewriteSTRM 包装和仅被测试使用的 SetEmbyExport。实际的媒体附件写出又在 scrape/export.go。扫描代码仍通过 scrape 间接调用原本属于 export 的 STRM 解析等能力。

**影响：** 功能归属需要跨两个包判断，重构后的兼容壳增加维护面；部分测试仅为这些转发层重复运行，详见 T01。

**建议与验收：** 调用方直接使用实际归属包，逐步把文件输出职责集中到 export；刮削保留业务编排与检查点。测试改用已有构造参数注入导出配置，随后删除确认无生产用途的旧包装。保留当前小函数组合，不再加一层 exporter 管理框架。

### B11 · P2 · 持久化元数据缓存过期后没有回收机制

**位置：** [metadata/service.go:290](E:/ppxb/miyabi/internal/metadata/service.go:290)、[metadata/service.go:320](E:/ppxb/miyabi/internal/metadata/service.go:320)、[schema/metadatacache.go:12](E:/ppxb/miyabi/internal/ent/schema/metadatacache.go:12)、[maintenance/service.go:56](E:/ppxb/miyabi/internal/maintenance/service.go:56)。

expires_at 只参与是否命中的判断，写入则按 provider/code upsert。全仓生产调用检索未发现 MetadataCache 的过期清理路径；现有“清理缓存”只处理图片。

**影响：** 对持续出现的新番号，正缓存和负缓存行会不断累积；过期且不再查询的 JSON 结果仍长期占据数据库。此项是长期容量风险，不表示已测得当前数据库过大。

**建议与验收：** 为该缓存制定有界保留策略，按批回收过期行，并根据清理查询评估 expires_at 索引。范围限定为可重取的来源缓存，保留媒体库已经保存的资料快照和有业务用途的历史记录。验证缓存回收后仍能正确重新获取，且不会删除正式影片资料。

### B12 · P2 · 纠正番号时按扩展名清理，无法区分自定义附件

**位置：** [scrape/rename_export.go:14](E:/ppxb/miyabi/internal/library/scrape/rename_export.go:14)、[scrape/rename_export.go:36](E:/ppxb/miyabi/internal/library/scrape/rename_export.go:36)、[rename_export_test.go:27](E:/ppxb/miyabi/internal/library/scrape/rename_export_test.go:27)。

removePreviousExport 承诺保留用户文件，但会删除旧目录内全部 .strm、.nfo，以及固定名称的海报和背景图。它没有验证附件是否对应旧影片的生成文件名或本项目的 STRM 内容。

**影响：** 如果用户在旧导出目录放入 custom.nfo、bonus.strm 等自定义文件，纠正影片番号时也会被删除。当前测试只验证 original.mp4 和 notes.txt 被保留，没有覆盖同扩展名的非生成文件。本项由删除条件静态确认，未对真实导出目录执行清理。

**建议与验收：** 按旧影片明确的生成文件集合及可确认的归属清理，无法确认归属的文件保留；避免仅以扩展名判定所有权。在临时目录加入自定义 NFO/STRM，验证正常旧附件可清理、自定义附件保留，以及操作重放仍幂等。

## 4. 多余、重复和不必要测试

### T01 · P3 · STRM 重写与解析测试跨包重复

**位置：** [export/export_test.go:15](E:/ppxb/miyabi/internal/export/export_test.go:15)、[export/export_test.go:87](E:/ppxb/miyabi/internal/export/export_test.go:87)、[scrape/export_test.go:209](E:/ppxb/miyabi/internal/library/scrape/export_test.go:209)、[scrape/export_test.go:273](E:/ppxb/miyabi/internal/library/scrape/export_test.go:273)。

两个包中的 TestRewriteSTRM 使用相同目录、文件内容、目标地址、幂等和令牌轮换场景；scrape 的函数只转调 export。TestParseSTRMFileID 的六组输入和断言相同，scrape 版本调用的也是函数别名。export 版本还额外覆盖取消，测试信息更多。

**处理建议：合并并删除重复函数。** 以 internal/export 中的测试为准；清理旧包装后，删除 scrape 中这两个重复测试函数。保留同文件中的媒体附件输出顺序、分片命名、旧 STRM 清理和增量导出测试，这些验证了不同的业务行为。

**收益与边界：** 减少两套几乎相同用例的维护成本；不建议删除整个 scrape/export_test.go，也不把不同业务层的集成测试全部视为重复。

### T02 · P3 · 实验性解析器在测试文件内自成一套实现

**位置：** [codeid/prototype_test.go:13](E:/ppxb/miyabi/internal/codeid/prototype_test.go:13)、[codeid/prototype_test.go:58](E:/ppxb/miyabi/internal/codeid/prototype_test.go:58)、[codeid/prototype_test.go:122](E:/ppxb/miyabi/internal/codeid/prototype_test.go:122)、[codeid/random_corpus_test.go:73](E:/ppxb/miyabi/internal/codeid/random_corpus_test.go:73)。

测试文件实现了 prototypeQueries、prototypeMatch、prototypeResolve 等独立的查询与匹配流程。多个大语料测试调用该原型，而不是生产 metadata.Resolve 或实际来源适配器；部分场景让每个查询返回整个候选池。文件注释也明确其为实验。

**处理建议：迁移有价值场景，再移出默认回归集。** 把分层匹配、歧义和查询失败等独有场景迁到实际生产路径的测试；纯原型实验可归档或移出日常测试集。生产逻辑没有对应需求时，不为维持该实验而增加生产接口。

**收益与边界：** 避免测试原型正确被误当成生产流程正确。保留真实调用 Normalize、Parse 的语料测试和有价值的 fixture；它们覆盖真实输入差异，不应随原型一起删掉。

### T03 · P2 · 订阅索引测试复制实现，只验证自己构造的 Map

**位置：** [subscriptions.test.ts:50](E:/ppxb/miyabi/web/tests/subscriptions.test.ts:50)、[subscriptions.test.ts:61](E:/ppxb/miyabi/web/tests/subscriptions.test.ts:61)、[api/subscriptions.ts:55](E:/ppxb/miyabi/web/src/api/subscriptions.ts:55)。

“超过 100 条且 O(1) 查询”的测试自己构造 150 条数据，自己写 queryFn、staleTime 和 select，再断言自己创建的 Map。它没有调用生产 useSubscriptionTargets 的配置，也没有经过实际 API；即使生产 select 出错或请求重新出现数量截断，该测试仍可能通过。Map.get 的结果断言也不能证明整条路径的复杂度，F03 就未被覆盖。

**处理建议：重写，不保留复制实现。** 让生产 Hook 与测试共享实际 query options/选择函数，或采用项目现有的捕获真实 Hook 配置方式。替换测试自写的 select，验证真实数据来源、超过 100 条的结果和多消费者处理成本。

**收益与边界：** 删除的是测试中的第二份实现。subscriptionKeys 的层级契约仍有验证价值；无需重复测试 TanStack Query 本身的通用算法。两个查询测试文件中重复的 observe/settled 辅助代码，也可在确有复用时合并成小型测试工具。

### T04 · P3 · 健康检查的成功与 503 用例重复验证同一个函数

**位置：** [app/app_test.go:56](E:/ppxb/miyabi/internal/app/app_test.go:56)、[cmd/healthcheck_test.go:14](E:/ppxb/miyabi/cmd/miyabi/healthcheck_test.go:14)、[cmd/healthcheck_test.go:57](E:/ppxb/miyabi/cmd/miyabi/healthcheck_test.go:57)、[cmd/healthcheck.go:7](E:/ppxb/miyabi/cmd/miyabi/healthcheck.go:7)。

cmd.checkHealth 仅转调 app.CheckHealth；两边分别建立 HTTP 服务验证正常状态和 503。该部分没有增加独立的边界覆盖。

**处理建议：集中函数用例，保留命令入口测试。** 将 HTTP 探测行为集中在实际函数所属层；cmd 重点保留 healthcheck 子命令读取配置、使用 loopback、不启动应用和不创建数据目录等入口契约。

**收益与边界：** 无效地址、IPv6、重定向和超时各有独立价值，应保留或迁移。不要因测试耗时就删除唯一的超时验证。

### T05 · P3 · 部分组件测试锁定内部结构，并手动模拟 React 生命周期

**位置：** [subscription-card.test.tsx:49](E:/ppxb/miyabi/web/tests/subscription-card.test.tsx:49)、[subscription-card.test.tsx:68](E:/ppxb/miyabi/web/tests/subscription-card.test.tsx:68)、[subscription-card.test.tsx:95](E:/ppxb/miyabi/web/tests/subscription-card.test.tsx:95)、[task-events.test.tsx:22](E:/ppxb/miyabi/web/tests/task-events.test.tsx:22)、[task-events.test.tsx:114](E:/ppxb/miyabi/web/tests/task-events.test.tsx:114)。

组件测试直接调用组件函数，按 children 索引取节点，并断言按钮必须为 icon-sm、某个 children 必须为空。SSE 测试通过全局 mock useEffect，再取倒数第二个调用手动 setup/cleanup；这依赖内部 effect 数量和顺序。

**影响：** 无行为变化的包裹层、插槽或 effect 整理可能导致测试失败；手动执行 effect 又不能验证真实 React 的重渲染调度。它们仍能验证部分纯逻辑，不应整体判为无效。

**处理建议：精简装饰性断言和脆弱定位。** 删除与行为目标无关的 size/空 children 断言，减少固定索引访问；网络事件与取消逻辑使用明确的测试入口。保留点击隔离、选择行为、旧响应不能覆盖新快照等有效断言。不要为测试另造 React 运行模型，也不要按此项要求新增业务 aria。

### T06 · P3 · 架构测试的包名单没有覆盖新增元数据模块

**位置：** [deps_test.go:16](E:/ppxb/miyabi/internal/app/deps_test.go:16)、[deps_test.go:61](E:/ppxb/miyabi/internal/app/deps_test.go:61)、[metadata/service.go:1](E:/ppxb/miyabi/internal/metadata/service.go:1)。

TestBusinessPackagesDoNotImportEachOther 使用手写 business/shared 包名单，并只读取每个目录的直接 Go 文件。metadata 及其 providers 未被纳入检查对象或相应禁用规则；这些包的依赖发生变化时，现有测试可能始终通过。

**处理建议：更新覆盖边界，不删除测试。** 将当前真实业务包纳入名单，明确来源适配器的允许依赖。保留已经有意允许的包内关系，不根据目录名称建立一套过宽的禁止规则。

**收益与边界：** 让架构测试约束当前项目结构，而不是只维持重构前的部分目录。该测试成本较低且规则有实际价值，问题是规则陈旧，不是“存在架构测试”本身。

## 5. 工程配置

### C01 · P2 · 仓库 CI 没有把现有质量检查接入变更流程

**位置：** [docker-publish.yml:3](E:/ppxb/miyabi/.github/workflows/docker-publish.yml:3)、[Dockerfile:13](E:/ppxb/miyabi/Dockerfile:13)、[Dockerfile:32](E:/ppxb/miyabi/Dockerfile:32)、[package.json:6](E:/ppxb/miyabi/web/package.json:6)。

仓库内工作流只在版本 tag 推送时执行发布流程。镜像构建会进行前端构建、TypeScript 检查和 Go 编译，但没有运行 Go 测试、Vitest、Lint 和格式检查，也没有发现面向 PR/普通提交的质量检查工作流。

**影响：** 已有回归测试的保护依赖人工执行；能成功编译的行为回归仍可能进入发布。

**建议与验收：** 为普通变更加入已有检查命令，发布依赖其成功结果；按前后端变更范围控制执行成本。本项只评价仓库内配置，不推断仓库外是否另有 CI 或分支保护。本次没有创建或触发外部工作流。

### C02 · P2 · 运行镜像创建了专用用户，但最终仍默认使用 root

**位置：** [Dockerfile:34](E:/ppxb/miyabi/Dockerfile:34)、[Dockerfile:39](E:/ppxb/miyabi/Dockerfile:39)、[Dockerfile:56](E:/ppxb/miyabi/Dockerfile:56)。

runtime 阶段创建 miyabi 用户并设置目录所有权，却没有 USER 指令。Debian 基础镜像的默认身份因此仍被继承；COPY --chown 只改变文件所有权，不改变运行用户。

**影响：** 专用用户配置没有达到预期，进程拥有不必要的权限。

**建议与验收：** 在确认数据卷和 Emby 输出目录的权限后使用专用用户运行。核对新建数据目录、已有挂载目录及健康检查，避免仅添加 USER 就造成历史部署无法写入。本次未构建或启动容器。

## 6. 本次验证记录

以下检查使用工作区已有工具和依赖完成。Go 命令设置了 GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off，且使用 -mod=readonly，避免自动获取依赖或修改模块声明。

| 检查         | 执行方式                                               | 结果                                          |
| ------------ | ------------------------------------------------------ | --------------------------------------------- |
| Go 测试      | 项目根目录执行 `go test -mod=readonly ./...`           | 通过；部分包复用 Go 测试缓存                  |
| Go 静态检查  | `go vet -mod=readonly ./...`                           | 通过                                          |
| TypeScript   | web 目录已有 `tsc.cmd --noEmit`                        | 通过                                          |
| 前端 Lint    | 已有 `oxlint.cmd src tests`                            | 通过                                          |
| 前端格式     | 已有 `oxfmt.cmd src tests vitest.config.ts --check`    | 通过                                          |
| 前端测试     | 已有 `vitest.cmd run`，Node 环境                       | 30 个文件、185 项测试通过                     |
| 生产前端构建 | 已有 `vite.cmd build`                                  | 通过                                          |
| 本地缺陷验证 | 临时数据库、httptest HTTPS、模拟 transport、Go overlay | 确认 B01/B02/B03/B04/B05，并取得 B07 查询计划 |

隔离验证的关键输出如下。验证程序断言的是当前缺陷行为可被观察到；这些验证通过不代表缺陷已经修复。

```text
loopback_https_reached=true
body_bytes_accepted=17825792
content_type=image/png

child_write_failed=true
add_actor_error=<nil>
recheck_error=<nil>
movie_subscriptions_after_recheck=0

plaintext_setting_value_in_debug_log=true

configured_concurrency=2
simultaneous_response_bodies=6

SEARCH subscriptions USING INDEX subscription_kind_target_id (kind=?)
USE TEMP B-TREE FOR ORDER BY
```

临时验证没有使用真实凭据、生产数据或外部站点。Go overlay 只让测试进程看到临时文件，没有将验证写入正式源码包。相关临时源码和 overlay 文件已删除。

构建产物盘点到 103 个 WOFF2 文件，总计 4,583,636 字节，CSS 为 193,340 字节。这是产物体积，不等同于首次访问全部下载；本报告没有据此判定首屏性能不合格。

未执行浏览器测试、真实上游联调、容器运行、生产负载压测及完整 race 检测；现有测试通过也不等同于上述静态问题不存在。

## 7. 建议整改顺序与回归边界

| 顺序 | 工作范围                                        | 最小验证重点                                                                              |
| ---- | ----------------------------------------------- | ----------------------------------------------------------------------------------------- |
| 1    | B01、B02：图片访问边界和漏订问题                | 合法图片仍可获取；错误目标被阻止；失败的子订阅可重试且不重复创建                          |
| 2    | B03/B04/B05/B08/B09/B12：日志、资源和文件完整性 | 敏感值不进入日志；慢响应不突破并发；超限/取消及时释放；覆盖失败保留旧文件；保留自定义附件 |
| 3    | F03/F04/F05/F09、B06/B07/B11：状态与规模问题    | 真实查询配置、异步取消、缓存保留、宽目录树、查询计划和缓存容量                            |
| 4    | F01/F02/F06/F07/F08、B10、测试整理及 CI         | 遵守业务层无 aria 约定，保留 shadcn 原始实现；导入与构建通过；删除重复测试前迁移独有覆盖  |

测试精简应按以下边界执行：

- **可直接列入合并候选：** T01 的两个重复函数、T04 重复的成功/503 探测场景、T05 与业务目标无关的装饰性断言。
- **应先迁移或重写：** T02 的独有原型场景、T03 的复制实现、T05 脆弱的 effect 定位。不能先删除，再假设其他测试会覆盖。
- **应保留：** 请求乱序、取消、令牌刷新、来源切换、任务恢复、事务回滚、幂等导出、编号身份等不同故障窗口的测试。
- **应修正规则：** T06 的架构名单和 B05 对应的慢响应体并发场景。

尤其应保留 [query-refresh.test.ts](E:/ppxb/miyabi/web/tests/query-refresh.test.ts)、[task-events.test.tsx](E:/ppxb/miyabi/web/tests/task-events.test.tsx)、[token_test.go](E:/ppxb/miyabi/internal/drive/token_test.go)、[concurrency_test.go](E:/ppxb/miyabi/internal/offline/concurrency_test.go)、[scan_regression_test.go](E:/ppxb/miyabi/internal/library/scan_regression_test.go)、[retry_test.go](E:/ppxb/miyabi/internal/tasks/retry_test.go) 中针对具体故障场景的覆盖。它们有相似的准备代码，不等于验证的是同一件事。

不建议把已经有明确用途的响应缓存、SSE 快照协调、任务重试或来源保护锁整体删掉，也不建议为消除少量重复增加通用仓储、工作流引擎或组件工厂。

需要用户进行的浏览器验收集中在：

- 明暗主题下状态徽标、加载图标间距，以及小屏操作栏布局。
- 状态读取失败与后台刷新失败时，已有内容和重试提示是否合理。
- 订阅数量较多时的页面刷新、卡片渲染和网络请求情况。
- SSE 断开、重连、页面离开后，通知和任务状态是否符合预期。

本次交付仅为审计报告，未实施上述业务整改。

建议英文 commit message：

```text
docs: add comprehensive project code audit report
```

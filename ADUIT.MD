# Miyabi 审计与复查清单

> 复核日期：2026-10-02。基线：`v0.2.0 / e79cb1e`。
> 本轮逐项复核原清单 111 项，保留原编号；补充用户反馈第 245 项，共 112 项。已解决项移出待办，仅保留简短结论及代码位置。
> 首次复核只更新审计报告；后续已完成第 135、138、140、142、146、147、148、149、153、156、157、158、159、161、166、168、173、184、189、192、193、194、206、222、224、235 项处理，并同步结论。“确认存在”表示代码路径仍在，不代表已经在真实大库或浏览器中测得故障。可选重构不能计作已确认 bug。
> 第 151 项已完成手动重连优化；首屏业务数据校正按本轮范围保留，是否进一步合并仍需实测，因此不计作整项完成。
> 第 152 项已完成四类设置查询的缓存复用；任务、影片状态、离线活动及订阅列表的挂载刷新保留，不计作整项完成。

## 复核结果

| 状态 | 数量 | 含义 |
| --- | ---: | --- |
| 确认仍存在，待处理 | 1 | 有明确代码依据的边界、状态反馈或行为问题 |
| 可选优化与整理 | 49 | 重复、风格或潜在成本仍在，但收益和方案需验证 |
| 需要用户实测 | 3 | 195、227 为视觉推断；245 为用户反馈，尚未定位 |
| 按现有约定暂缓 | 10 | 用户明确暂不处理的事项，或不恢复已取消的 UI、ARIA 等要求 |
| 撤回原问题结论 | 11 | 原结论不成立、风险描述过度，或把设计选择当成 bug |
| 已解决或随功能删除失效 | 38 | 移出实施队列，保留回归能力 |

优先处理顺序：

1. **237**：可信代理配置及 HTTPS 转发头。

### 当前约定

- 只支持当前数据结构，不迁移旧数据库、旧刮削数据、旧检查点或旧任务。当前数据的重启恢复、失败重试、暂停恢复、标准 NFO 导入及索引必须保留。
- 已入库使用紫色 badge；Emby 关闭时隐藏更多设置，并沿用与代理一致的保存方式。
- 设置页不再展示已挂载媒体目录的路径输入框，保留挂载、更换和取消挂载操作；目录选择器仍提供目录浏览与当前挂载位置识别。
- 不恢复 toast 和媒体库同步/重建按钮的 tooltip，不新增 shadcn 原始组件之外的自定义 ARIA 描述。相关无障碍缺口是暂缓项，不写成“已经修复”。
- 固定圆角、手势按帧更新保留；独立预览缩略图已经回退。保留模糊背景，未经性能记录不把它认定为卡顿根因。
- 本地详情继续复用完整的详情骨架，包括磁力和推荐占位；用户计划后续可能将非 JavDB 来源聚合为统一详情数据，因此暂不按本地来源裁减骨架区域。此约定不代表数据聚合已经实现。
- 不增加番号特例黑名单、不因文件短或只有一个调用方而强制合并、不机械抽取通用组件。

### 本轮验证与限制

- 第 159 项整理后通过：`go test ./internal/api ./internal/app`、同范围 `go vet`。扩充现有 Cookie 鉴权用例，验证缺失、伪造和过期 Cookie 返回原有文案及 UNAUTHORIZED，拒绝后受保护处理器不执行；有效 Cookie、忽略 Bearer/查询参数令牌、关闭密码保护及 PAN_UNAUTHORIZED 等既有回归通过。未执行浏览器测试。
- 第 158 项整理后通过：`go test ./internal/api ./internal/app ./internal/strm ./internal/export`、同范围 `go vet`。回归验证普通 4xx/5xx 只记录一条请求日志、完整错误链与多个错误均保留且不重复写 cause、请求 ID/耗时/分类及公共响应保持正确、URL 凭据与查询参数不泄露。覆盖已写成功/失败响应后的错误、仅状态码的失败、断链、panic 及已写响应后的 panic；panic 仍保留专用 ERROR 堆栈日志和同 ID 的访问摘要。499 访问日志继续过滤，DEBUG 取消诊断保留。未执行浏览器测试或性能基准，不将日志条数减少换算成接口提速。
- 第 156 项整理后通过：`go test ./internal/drive ./internal/offline ./internal/library/... ./internal/strm ./internal/app`、同范围 `go vet`。新增五种来源读取的回归覆盖正常结果、过期会话不刷新令牌、读取中换目录/断开后丢弃结果、上游错误优先且不泄露部分结果、请求前/请求中取消、正常令牌刷新及刷新中来源变更后不发起读取。既有离线提交成功后保留原来源记录、账号级任务同步和播放地址回归通过。未进行真实 115 或浏览器实测，不声称减少网络请求或实际耗时。
- 第 157 项整理后通过：`go test ./internal/javbus ./internal/catalogue ./internal/magnet ./internal/app`、同范围 `go vet`。新增离线回归覆盖详情与磁力接口的 404/重定向差异、未找到内容与挑战页的判断顺序、来源可用状态、响应体关闭、2 MiB 读取上限和错误上下文；虚拟时间验证共用限流及等待期间取消，既有取消/超时和新鲜详情参数回归保留，并加强两类请求头检查。未进行真实站点的详情/磁力抓取或浏览器实测，不声称减少正常获取磁力所需的两次请求。
- 第 153 项修复后通过：前端 `npm run test`（29 个文件 / 176 项）、`npm run lint`、`npm run typecheck`、`npm run build` 及修改文件的 oxfmt 格式化。新增 22 项 Node 回归，使用真实 QueryClient/QueryObserver、操作回调及模拟 SSE，覆盖同批失效合并、读取期间多次变化只排一次后续校正、无缓存旧请求丢弃、快照取消当前和排队任务读取、不同客户端隔离、失败后恢复、SSE/操作响应两种顺序及在途交错、账号切换和取消/换源失败后的状态校正。未进行浏览器测试，不宣称每次操作固定只有一次 HTTP 请求；读取开始后发生的变化仍保留必要的后续校正。
- 第 152 项设置查询优化后通过：前端 `npm run test -- tests/settings-cache.test.tsx tests/emby-settings.test.ts tests/query-errors.test.ts`（3 个文件 / 12 项）、`npm run lint`、`npm run build`（包含 TypeScript 检查）及修改文件的 oxfmt 格式化。新增 4 项 Node 回归，将实际查询选项交给 QueryObserver，验证四类查询首次读取、14999ms 内重新挂载复用缓存、15000ms 边界重新挂载刷新，以及主动失效立即重读；保留既有 Emby 保存回写、部分失败校正和旧响应竞争回归。未执行浏览器测试，不将缓存窗口视为每 15 秒轮询。
- 第 151 项手动重连优化后通过：前端 `npm run test`（27 个文件 / 150 项）、`npm run lint`、`npm run build`（包含 TypeScript 检查）及修改文件的 oxfmt 格式化。新增 6 项 Node 回归，通过模拟 EventSource、驱动实际连接 effect 和 QueryObserver，验证成功重连不追加 HTTP 任务查询、15 秒连接等待超时及失败时恢复兜底、同一断线期间只请求一次、旧 HTTP 响应不能覆盖新 SSE 快照、首屏/重连版本校正保留及卸载时清理定时器。未执行浏览器测试，不据此推算真实首屏请求减少量。
- 第 146 项优化后通过：`go test ./internal/javdb ./internal/catalogue ./internal/app`、同范围 `go vet`。新增虚拟时间回归覆盖按实际探测开始时间取消、更快结果到达后提前更新截止时间、已取消探测延迟收尾时不忙等，以及相同延迟和零延迟的优先顺序；保留动态线路发现、排除客户端创建耗时、全量测速和调用方取消等既有回归。未进行真实线路测速或 CPU 基准，不据此声称实际请求提速比例。
- 第 147 项优化后通过：`go test ./internal/domain/subtitle ./internal/subtitle ./internal/library/...`、同范围 `go vet`。新增混合 ASCII、汉字和 emoji 的取样边界回归，验证第 15000 个字符计入、第 15001 个起忽略，保留提示优先、简繁统计及缺省结果。局部基准 `BenchmarkDetectLanguage`（Windows/amd64、i5-12600K、200ms，重复汉字合成输入）中，15000 字符由约 0.476ms / 65536 B 降至 0.353ms / 0 B；100 万字符由约 5.424ms / 4005901 B 降至 0.361ms / 0 B，每次堆分配均从 1 次降至 0 次。仅测语言检测函数，不包含字幕下载、解码或入库，不据此推算整个流程提速。
- 第 140 项优化后通过：`go test ./...`、`go vet ./internal/strm ./internal/drive ./internal/api ./internal/app`。新增离线回归验证缓存命中跳过文件索引与地址申请、文件/UA/账号/授权版本/挂载隔离、12 个并发请求合并、失败不驻留、签名到期前刷新、未知有效期跳过缓存、容量淘汰、单个调用取消、共享超时和关闭等待，以及 Range 请求仍返回原样签名重定向。未进行真实 115 播放或浏览器测试，不宣称实际起播提速比例；未运行 race 检测（当前 Go 环境 CGO_ENABLED=0）。
- 第 235 项修复后通过：`go test ./internal/api ./internal/app ./internal/strm ./internal/export`、同范围 `go vet`。新增离线 HTTP 回归覆盖 GET/HEAD 播放鉴权、签名重定向及图片源 URL 原样传递、查询参数/Referer 不落访问日志、错误消息/cause/已写响应错误的地址脱敏、编码与嵌套 URL、panic 堆栈保留及 Gin 原始输出关闭、断链处理和 499 日志过滤。只使用测试凭据，未读取生产令牌、清理历史日志或修改 STRM 文件。
- 第 235 项局部微基准：`go test ./internal/api -run '^$' -bench '^BenchmarkRequestLogFields$' -benchmem -benchtime=200ms`。Windows/amd64、i5-13490F，128 字节/64 KiB 查询参数的过滤后记录分别约 653/654 ns，每条 160 B、1 次分配；未过滤对照约 734 ns/61.4 μs。该基准仅测合成访问日志字段处理与 JSON 编码，输出到 io.Discard，不含中间件其余工作、错误脱敏、网络或实际日志写入，不据此声称全链路提速或零开销。
- 第 224 项按用户要求移除路径展示后，通过前端 `npm run typecheck`、`npm run lint` 及修改文件的 oxfmt 格式检查。仅调整设置行展示，未新增测试或执行浏览器测试，未操作实际挂载目录。
- 第 222 项修复后通过：前端 `npm run typecheck`、`npm run lint` 及修改文件的 oxfmt 格式检查。仅拆分按钮禁用条件，未新增测试或执行浏览器测试；未实际执行缓存清理。
- 第 206 项修复后通过：前端 `npm run test -- tests/query-errors.test.ts tests/client.test.ts tests/emby-settings.test.ts`（3 个文件 / 23 项既有回归）、`npm run typecheck`、`npm run lint` 及三个修改文件的 oxfmt 格式检查。现有用例覆盖错误解析、全局 toast 触发范围和 Emby 保存状态；本次仅让操作错误复用现有文案转换，未新增测试或执行浏览器测试。
- 第 194 项修复后通过：前端 `npm run typecheck`、`npm run lint` 及修改文件的 oxfmt 格式检查。仅调整说明文字的 CSS 换行规则，未新增测试或执行浏览器测试；窄屏中英文与长路径的实际换行效果由用户确认。
- 第 193 项修复后通过：前端 `npm run test -- tests/category-filters.test.ts tests/error-state.test.tsx`（2 个文件 / 6 项既有回归）、`npm run typecheck`、`npm run lint` 及修改文件的 oxfmt 格式检查。现有用例覆盖分类参数和错误组件重试禁用状态；未新增测试或执行浏览器测试。
- 第 187 项仅统一重连按钮文案，通过前端 `npm run typecheck`、`npm run lint` 及两个修改文件的 oxfmt 格式检查。未新增测试或执行浏览器测试；加载展示与设置重试入口按用户决定暂缓。
- 第 189 项修复后通过：前端 `npm run test -- tests/subscriptions.test.ts tests/subscription-card.test.tsx tests/movie-detail-content.test.tsx`（3 个文件 / 11 项既有回归）、`npm run typecheck`、`npm run lint` 及三个修改文件的 oxfmt 格式检查。本次调整状态展示与动作文案，未新增测试或执行浏览器测试；图标与提示的实际视觉效果由用户确认。
- 第 192 项修复后通过：前端 `npm run test -- tests/movie-detail-content.test.tsx tests/movie-detail.test.ts`（2 个文件 / 21 项既有回归）、`npm run typecheck`、`npm run lint` 及两个修改文件的 oxfmt 格式检查。未新增测试或执行浏览器测试；实际加载切换效果由用户确认，未宣称消除动态标题、资料数量等导致的所有布局变化。

- 通过：`go test ./internal/api ./internal/app ./internal/database ./internal/tasks ./internal/library/... ./internal/emby ./internal/netx ./internal/monitor`。
- 通过：前端 `npm run test`，25 个文件 / 141 项测试。相关用例验证任务共享快照、索引、事务、来源隔离、重启恢复、Emby 通知和详情交互逻辑。
- 额外核对依赖实现：slog-gin v1.21.1 会构造 RawQuery/Referer 字段，第 235 项现已在日志输出前按字段省略；Gin v1.12.0 的代理解析会在错误时保留已解析前缀。没有读取或输出生产令牌。
- 没有浏览器测试、真实上游/Emby 联调、race 检测或全链路性能基准；第 235、147 项仅进行了上述局部日志与语言检测微基准。旧报告中的毫秒数、倍数、对比度和固定积压耗时不再作为本轮结论。
- 第 135 项修复后通过：`go test ./internal/library/... ./internal/app`、`go vet ./internal/library/... ./internal/app`；新增覆盖跨目录过期回收（命中、未命中及请求失败）、目录数/文件总数上限、淘汰后重新读取、超大目录完整读取且不驻留、账号隔离和返回列表修改隔离。保留现有视频位置核验与发布回放回归。限额约束缓存记录数，不代表内存字节硬上限或真实大库性能结论。
- 第 138 项修复后通过：`go test ./internal/pan ./internal/drive ./internal/library/... ./internal/app`、同范围 `go vet`。新增离线回归覆盖仅 downurl 使用 API 限流、CDN 请求 UA/凭据隔离、文件大小边界与响应体关闭、瞬时故障重试、Retry-After、请求头/正文超时及退避期间取消。未测量真实网盘扫描提速比例。
- 第 142 项修复后通过：`go test ./internal/pan ./internal/drive ./internal/library/... ./internal/app ./internal/api ./internal/monitor`、同范围 `go vet`。新增回归覆盖并发账号校验的成功/失败共享、TTL 到期重查、等待者取消、共享请求超时与关闭流程、明确拒绝后的凭据持久化清除与重启、临时故障后重试、正在进行的扫码登录及新旧凭据交错；保留原有刷新合并、来源切换与登录回归。
- 第 148 项修复后通过：`go test ./internal/monitor ./internal/tasks ./internal/app ./internal/api`、同范围 `go vet`。新增虚拟时间回归覆盖超过单页的影片/演员到期积压、相同到期时间的翻页、保存失败后继续处理、跨页节流、未到期/暂停/过期订阅排除及取消后续查；验证无磁力等待状态通知、批量入库的订阅/配置读取次数和原有节流。未进行真实大量订阅或上游性能测试。
- 第 149 项修复后通过：`go generate ./internal/ent`、`go test ./internal/database ./internal/library/... ./internal/app ./internal/api`、同测试范围 `go vet`。捕获实际列表 SQL 并用 EXPLAIN QUERY PLAN 验证排序索引生效且没有临时排序；覆盖本地/挂载来源筛选、相同创建时间的分页顺序和多文件去重。本地扫描回归覆盖跨根目录同名文件、等价相对/绝对根路径、文件信息刷新、手动番号保护及 STRM 网盘身份保留。未新增真实大库性能结论。
- 第 161 项修复后通过：`go test ./internal/api ./internal/app`、同范围 `go vet`。并发 HTTP 回归验证 16 个同 IP 请求仅 5 个进入验证，其他请求返回 429；覆盖成功与在途请求交错、格式错误释放名额且不清除失败历史、容量上限、未访问记录回收、封禁/窗口到期边界及动态时长文案。保留密码关闭、Cookie 登录和伪造代理 IP 等现有回归；未运行 race 检测或浏览器测试。
- 第 166 项修复后通过：`go test ./internal/api ./internal/app ./cmd/miyabi`、同范围 `go vet`。新增 HTTP 回归覆盖目录拒绝访问、入口缺失时不列目录、SPA 页面回退、哈希/非哈希资源缓存策略、缺失资源与 API 请求不回退 HTML、HEAD、304、Range、方法限制及原请求路径保留；浏览器缓存和发布更新体验未做实测。
- 第 168 项修复后通过：`go test ./internal/gfriends ./internal/emby ./internal/app`、同范围 `go vet`。新增离线回归覆盖网络/缓存恰好达到上限、声明超限、缺失或低报长度的超限流、镜像回退与响应体关闭、只持久化有效索引、超大缓存重新下载，以及首次加载失败/旧索引保留后的退避和恢复；同时验证头像超限不再返回截断数据。2026-10-02 查询 GitHub 文件元数据，Filetree.json 为 6,502,018 字节（约 6.2 MiB），据此设置 32 MiB 上限；未进行真实 Emby 头像同步或浏览器测试。上限约束输入文件体积，不是 JSON 解析后的内存硬上限。
- 第 173 项修复后通过：前端 `npm run test -- tests/access-gate.test.tsx tests/emby-settings.test.ts`（2 个文件 / 6 项既有登录与 Emby 保存回归）、`npm run typecheck`、`npm run lint` 及三个修改文件的 oxfmt 格式检查。未新增 DOM 测试或执行浏览器测试；实际自动聚焦体验由用户确认，不将现有用例通过当作焦点行为已实测。
- 第 184 项修复后通过：`go test ./internal/config ./internal/export ./internal/library/... ./internal/emby ./internal/netx ./internal/app`、同范围 `go vet`。新增回归覆盖从 config.Load 到实际 STRM 导出的自定义数据目录/9090 端口、显式目录/HTTPS 地址/路径前缀和 token 编码；空目录、空地址、纯空白及规范化后为空的地址均在文件写入前拒绝，直接或 Manager 批量重写保留已有 STRM，本地资料导出保留已有 NFO/图片/STRM。保留增量导出、配置切换和并发重写的既有回归。未进行真实 Emby 播放或浏览器测试。
- **独立工具链问题仍存在**：本地执行 `web/node_modules/.bin/shadcn --version` 仍报 `ERR_PACKAGE_PATH_NOT_EXPORTED`：MCP SDK 依赖访问 zod 的 `./v3` 导出失败。未安装或调整依赖；这不是应用构建失败，也不意味着需要恢复旧数据兼容。

## 确认仍存在，待处理

- [ ] **237.** setAuthCookie 仍直接相信 X-Forwarded-Proto，未关联可信代理；config.Load 未校验 TrustedProxies，NewRouter 忽略 SetTrustedProxies 错误。已核对 Gin v1.12.0：解析失败返回已解析的 CIDR 前缀并写入，可能静默使用部分列表。应启动时拒绝无效配置，并按可信代理决定 HTTPS 转发头。
  - 定位：`internal/api/auth.go:139`；`internal/api/router.go`；`internal/config/config.go`。

## 可选优化与整理

- **136.** Activity 仍加载当前来源每个 hash 的最新下载记录，没有历史上限；前端有活动任务时每 5 秒轮询。存在随历史增长的查询成本，但不能直接截断长时间下载、待定位任务或失败记录；先测量，再拆分活动与历史读取。
  - 定位：`internal/offline/projection.go`；`web/src/api/offline.ts`。
- **137.** 批量订阅每项完成后编码并写入包含全部 IDs、FailedIDs 的 payload。全量 JSON 写放大仍存在，但 UI 已共享快照，不再是每个 SSE 连接各查一遍。优化应分离不变输入与检查点，不能简单降低持久化频率而丢失恢复进度。
  - 定位：`internal/monitor/batch.go`；`internal/monitor/batch_tasks.go`。
- **145.** ResolveMovieID 仍多次遍历累积候选，matchCandidates 重复 Normalize/IsFormatEquivalent。可缓存规范化结果；需保留层级优先、同 ID 去重和歧义拒绝，尚无当前负载收益数据。
  - 定位：`internal/javdb/movie.go`。
- **151.** 手动重连已优先接收新 SSE 的任务快照，不再立即失效 tasks；同时重置本次尝试的兜底标记，连接失败或等待超时时仍能通过 HTTP 校正任务状态。首个 changes 事件仍按三类 revision 刷新相关业务查询，以补齐读取与订阅建立之间的变化；这部分按本轮范围保留，实际首屏重复请求次数及进一步合并收益需浏览器确认。
  - 定位：`web/src/features/tasks/task-events.tsx`；`web/tests/task-events.test.tsx`。
- **152.** 网络、Emby、订阅设置及缓存统计已移除 refetchOnMount:'always'，重新挂载时复用 15 秒内的新鲜数据，过期或主动失效后仍刷新；保存/清理成功回写和失败校正保留。任务、影片状态、离线活动及订阅列表仍保留强制挂载刷新，后续需结合 SSE 连接状态与漏事件恢复逐项评估，不统一删除。
  - 定位：`web/src/api/network.ts`；`web/src/api/emby.ts`；`web/src/api/subscription-settings.ts`；`web/src/api/data.ts`；`web/tests/settings-cache.test.tsx`；`web/src/api/tasks.ts`；`web/src/api/movie-states.ts`；`web/src/api/offline.ts`；`web/src/api/subscriptions.ts`。
- **154.** 媒体库和关键词搜索查询仍未提供 keepPreviousData，翻页会进入无缓存加载态；发现页查询已经使用。路由 intent 预加载仍未接查询 loader。是否预取及保留旧页应按请求量和交互实测决定。
  - 定位：`web/src/api/library.ts`；`web/src/api/discover.ts`；`web/src/router.tsx`。
- **160.** Emby/网络测试接口仍先加载已保存配置再用请求体替换；订阅批量接口仍单独写 202，feed 复用含 kind 的查询结构。router 的 STRM 方法注册、gin 全局模式及命名整理均是可选重构；代理错误另归 237，不强制新增泛型 handler。
  - 定位：`internal/api/emby.go`；`internal/api/network.go`；`internal/api/subscription.go`；`internal/api/router.go`。
- **162.** Claim 仍先 SELECT 后条件 UPDATE，但已有队列锁、状态条件与资源顺序约束，不能据此判定重复领取。批次列表的近期+活动合并及 registry 检查仍存在；更改单条 SQL 前须证明收益并保留重试/暂停语义。
  - 定位：`internal/tasks/queue.go`；`internal/tasks/pool.go`；`internal/monitor/batch_tasks.go`。
- **163.** Bus、Drive 与 ProxyManager 的订阅/退订 API 风格仍不同；ProxyManager.Unsubscribe 仍遍历通道 map。可在相关模块修改时统一生命周期接口，不作为独立功能故障。
  - 定位：`internal/tasks/bus.go`；`internal/drive/events.go`；`internal/netx/proxy.go`。
- **164.** 部分 goroutine 仍使用 Add/Done，Kind 字符串转换风格不一。ContextLock 未持锁 Unlock 会阻塞，但本轮未发现生产误用路径；不将它写成已发生死锁。wg.Go 和锁误用诊断属于可选整理。
  - 定位：`internal/app/app.go`；`internal/magnet/aggregator.go`；`internal/syncx/lock.go`。
- **165.** 浏览记录裁剪仍是 Count、读取最旧 ID、Delete；可评估单条有界删除，但须保留 viewed_at/id 同序规则与单调时间。当前已有限额，未证明此处为性能瓶颈。
  - 定位：`internal/library/viewed.go`。
- **169.** apiPost/apiPatch/apiPut 的 JSON 请求初始化仍重复，imageURL 也仍位于 client.ts。可以小范围整理，但改参数形状或新增转发层不一定改善维护性，不以调用次数要求全面重写。
  - 定位：`web/src/api/client.ts`。
- **170.** 查询键仍分散：discoverKeys 位于 movie-detail-cache，pan.ts 使用 library/offline 字面量键，subscriptionSettingsKey 命名不同。优先消除实际重复键，不为集中管理引入循环依赖。
  - 定位：`web/src/api/movie-detail-cache.ts`；`web/src/api/pan.ts`；`web/src/api/subscription-settings.ts`。
- **172.** build 仍先 vite build 后 tsc --noEmit；类型错误会较晚暴露，但脚本仍失败，不能说会把类型错误当成功发布。可调整执行顺序。
  - 定位：`web/package.json`。
- **174.** 相对路径、@ 别名、barrel 与常量别名仍混用；MovieGrid 直接导入 DiscoverMovieCard，barrel 同名导出未被它使用。只在相关改动中清理，不把书写风格当运行缺陷。
  - 定位：`web/src/features/settings/tasks-section.tsx`；`web/src/components/movie/index.ts`；`web/src/components/movie/movie-grid.tsx`；`web/src/features/discover/category-content.tsx`。
- **175.** api 的 data/movie 文件名与 maintenance/library 职责仍不完全一致。可按资源整理，短文件和独立接口本身不构成拆分问题。
  - 定位：`internal/api/data.go`；`internal/api/movie.go`；`internal/api/router.go`。
- **176.** HEAD 编排仍经过 api → Relay.Probe → Drive.OpenMedia → pan。可考虑由 Relay 封装 HEAD 行为，但窄接口转发也有隔离价值，不自动删除层。
  - 定位：`internal/api/strm.go`；`internal/strm/relay.go`；`internal/drive/drive.go`。
- **177.** healthcheck 子命令仍先执行完整 config.Load，默认 PublicURL 推导会调用 OutboundIP；checkHealth 仍转发 App.CheckHealth。可把 CLI 所需的监听地址读取与探活实现收拢，避免无关初始化。
  - 定位：`cmd/miyabi/main.go`；`cmd/miyabi/healthcheck.go`；`internal/config/config.go`；`internal/netx/ip.go`。
- **179.** BrowseResults、SearchResults、MetadataSearchPage 仍重复把 query 字段传给 DiscoverResults。是否改成接收 query 对象需权衡组件与 React Query 的耦合，不强制删壳。
  - 定位：`web/src/features/discover/browse-results.tsx`；`web/src/features/discover/metadata-search-page.tsx`；`web/src/features/search/page.tsx`。
- **180.** ActorSubscriptions 仍透传 displayItems/displayQuery 与选择状态到 MovieSubscriptions。可减少 props 穿透；须保持未选演员时展示全部演员新作的 feed。
  - 定位：`web/src/features/subscriptions/actor-subscriptions.tsx`；`web/src/features/subscriptions/page.tsx`。
- **182.** catalogue.MovieDetail 仍在带 release_status 的 Movie 外声明 zone/推荐列表，直接嵌入 domain.MovieDetail 需保留额外字段；domain/library.go、source.go 命名仍宽泛。ScanPayload.Checkpoint 仍为嵌套 JSON 字符串，可在当前任务结构内简化，不引入旧任务兼容。
  - 定位：`internal/catalogue/types.go`；`internal/domain/library.go`；`internal/domain/source.go`；`internal/domain/scan.go`。
- **183.** app.New 的失败分支仍重复释放已创建资源；多源客户端加入后分支变多。当前关闭路径及对应测试存在，可评估局部清理栈，但没有证据支持“当前仍泄漏 worker”。
  - 定位：`internal/app/app.go`；`internal/app/deps_test.go`。
- **188.** scan-status 与 task-progress-state 仍维护两套阶段文案。当前扫描 toast 已显式传 scanStatus，所以原先“同一 toast 用两套文案”不成立；可统一剩余默认标签，保留暂停和自动重试优先级。
  - 定位：`web/src/features/tasks/scan-status.ts`；`web/src/features/tasks/task-progress-state.ts`；`web/src/features/tasks/task-toast.tsx`。
- **190.** success/info 仍直接使用固定色阶，失败 badge 覆盖 destructive，订阅选中使用 success。可以整理语义 token；已入库紫色是明确产品选择，不能当错误改回其他颜色。删除已移除的数据源警示颜色引用，不沿用未经本轮测量的对比度数字。
  - 定位：`web/src/styles/globals.css`；`web/src/components/ui/badge.tsx`；`web/src/features/library/movie-status.tsx`；`web/src/features/subscriptions/subscription-card.tsx`。
- **191.** 筛选 Select 使用 popper，订阅偏好仍使用 item-aligned 默认值，Select Viewport 还存在 position === 'popper' && '' 空表达式。可先清理空表达式，弹出方式由交互验收决定。
  - 定位：`web/src/components/ui/select.tsx`；`web/src/features/settings/subscription-section.tsx`；`web/src/features/discover/category-filters.tsx`。
- **196.** 部分 Button 图标仍重复声明默认 size，Emby/代理保存 spinner 仍有 mr-1.5 与按钮 gap 叠加。仅属样式整理；原数据源页静态 Loader 图标已随页面删除。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/src/components/ui/button.tsx`。
- **198.** 普通元数据链接仍仅在 hover 变灰，与相邻纯文本区别较弱；标签与 EntityRow 行高不同，部分 tracking-normal/换行类风格不一。属于视觉一致性候选，不能把合法 Tailwind 类名差别当 bug。
  - 定位：`web/src/features/movie-detail/metadata-link.tsx`；`web/src/features/movie-detail/metadata.tsx`；`web/src/features/movie-detail/magnets.tsx`。
- **199.** 任务时间、订阅时间和缓存数量仍在各处格式化。三者展示精度/类型不同；只抽取相同格式的 formatter，不要求日期和数字共用同一逻辑。
  - 定位：`web/src/features/settings/tasks-section.tsx`；`web/src/features/subscriptions/subscription-card.tsx`；`web/src/features/settings/data-section.tsx`；`web/src/lib/format.ts`。
- **200.** 网盘头像与演员头像的尺寸 API 不同；容量足够时剩余空间在进度条内与图例各显示一次，演员条骨架与按钮尺寸也不同。是否合并显示需视觉验收，未确认布局故障。
  - 定位：`web/src/features/settings/pan-account.tsx`；`web/src/features/subscriptions/actor-chip.tsx`；`web/src/features/subscriptions/actor-subscriptions.tsx`。
- **202.** 订阅卡片错误仍用原生 title，设置任务进度使用 Tooltip/OverflowTooltip，并可能重复展示同一错误。用户取消的是 toast 和媒体库两个按钮的 tooltip，这里不能误判为回归，也不自动扩展无障碍改动。
  - 定位：`web/src/features/subscriptions/subscription-card.tsx`；`web/src/features/tasks/scan-progress.tsx`。
- **204.** SettingsSection 仍由调用方传图标尺寸，设置路由手写分隔线且关闭返回顶部。图标重复和页面文件位置属于可选整理；“数据源与缓存都用 DatabaseIcon”已失效，刮削设置使用 ScanSearchIcon。
  - 定位：`web/src/features/settings/shared.tsx`；`web/src/routes/settings.tsx`；`web/src/features/settings/scraping-section.tsx`。
- **207.** Emby 与代理的测试连接/保存操作栏仍重复。若后续一起调整可以提取小组件，不要求为两个相似块新增复杂通用接口。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`。
- **208.** Emby 仍逐字段比较 form，代理分开保存 enabled/url 草稿；代理手写 Enter 保存，Emby 没有 form 提交。可以优先统一键盘提交；草稿抽取须保留保存后等待 query 同步的防闪回逻辑。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/tests/emby-settings.test.ts`。
- **209.** 清理缓存与批量入库确认弹窗仍重复 pending/关闭/错误/操作栏结构。可复用相同部分，但保留各操作的失败展示与关闭约束，不引入额外确认流程。
  - 定位：`web/src/features/settings/data-section.tsx`；`web/src/features/subscriptions/page.tsx`。
- **210.** 订阅偏好的三个 Select 仍分别书写。可用轻量配置减少重复；设置请求和失败语义保持原样。
  - 定位：`web/src/features/settings/subscription-section.tsx`。
- **211.** MovieCard 与骨架仍重复卡片外框及比例样式，多个详情触发器仍重复 class。实际卡片已共享 MovieCard，hover 外框已不存在；只保留骨架/外框同步候选，不新增多层 CardShell 包装。
  - 定位：`web/src/components/movie/movie-card.tsx`；`web/src/components/movie/movie-grid-skeleton.tsx`；`web/src/features/library/movie-card.tsx`。
- **213.** CategoryContent 与 CategoryFilters 仍重复派生有效分类；分区选择与 main 分类常量也有重复。可先抽纯计算函数，是否增加 hook/ZoneSelect 按调用复杂度决定。
  - 定位：`web/src/features/discover/category-content.tsx`；`web/src/features/discover/category-filters.tsx`；`web/src/features/discover/metadata-search-page.tsx`。
- **214.** 详情 section 标题仍重复少量 class，预览标题 id 没有引用。可清理未用 id；是否抽 DetailSection 属于可选整理，不能只因相同 HTML 就增加组件。
  - 定位：`web/src/features/movie-detail/previews.tsx`；`web/src/features/movie-detail/recommendations.tsx`；`web/src/features/movie-detail/magnets.tsx`。
- **215.** task-notification-diff 仍本地重写 isScanTask/isBatchTask/isTaskActive/isOfflineTaskActive。可共用纯状态判断，需考虑该模块独立测试与运行时依赖，避免为消除几行重复引入 API 层耦合。
  - 定位：`web/src/features/tasks/task-notification-diff.ts`；`web/src/api/tasks.ts`；`web/src/api/offline.ts`。
- **216.** 三种任务 toast 仍分别构造加载图标和断线文案，但扫描/离线现在额外考虑暂停，已不是完全相同的表达式。可统一文案，抽取时不能让已暂停任务继续转圈。
  - 定位：`web/src/features/tasks/task-toast.tsx`。
- **217.** 目录弹窗仍手写上一页/下一页，ListPagination 已支持无总页数模式。若复用应保留目录统计、正在请求时禁用和不滚动页面；不重新要求自定义 ARIA。
  - 定位：`web/src/features/settings/pan-directory-dialog.tsx`；`web/src/components/list-pagination.tsx`。
- **219.** app、library、offline、drive、strm 仍有多份 115 fixture，部分桩结构重复；不沿用旧的 1500 行统计。评估抽共享 transport/窄接口，保留各包的事务与来源边界断言。
  - 定位：`internal/app/drive_fixture_test.go`；`internal/library/fixture_test.go`；`internal/offline/fixture_test.go`；`internal/drive/drive_fixture_test.go`；`internal/strm/fixture_test.go`。
- **221.** 批量入库成功仍由 page 发“已创建任务”toast，全局任务通知随后也可能出现。两条反馈路径已确认，是否在当前时序重复显示需浏览器验收；失败只在弹窗内展示的旧修复保留。
  - 定位：`web/src/features/subscriptions/page.tsx`；`web/src/features/tasks/task-toast.tsx`。
- **225.** copiedHash 仍保留至下一次复制或组件卸载，无自动恢复；复制失败提示位于 section 底部。是否改短暂反馈、移近按钮属于交互优化，未影响剪贴板实际复制。
  - 定位：`web/src/features/movie-detail/magnets.tsx`。
- **228.** MediaImage 的 className 只作用于前景图，外层尺寸另由父容器约束。可改名 imageClassName 提高清晰度，但属性命名本身不是图片局部显示的已知原因。
  - 定位：`web/src/components/media-image.tsx`。
- **229.** 固定 rounded-2xl 已替代 FRAME_RADIUS 动态计算，缩放已按动画帧更新。仍可整理 React 类型导入、卡片固定预留宽度；不要把跨浏览器 scrollbar CSS 全部当重复代码删除。
  - 定位：`web/src/features/movie-detail/preview-viewer.tsx`；`web/src/features/movie-detail/preview-frame.ts`；`web/src/components/movie/movie-card.tsx`；`web/src/styles/globals.css`。
- **236.** Emby 设置 GET 和更新响应仍返回完整 API Key，页面 password Input 仅遮住显示。该接口位于受保护设置路由，不是无条件匿名泄漏；可评估只返回已配置标记及留空保留旧值的写入协议，属于密钥暴露面收缩。
  - 定位：`internal/api/emby.go`；`internal/emby/service.go`；`web/src/api/emby.ts`；`web/src/features/settings/emby-section.tsx`。
- **238.** prototype_test 仍复制解析/匹配流程，prototypeQueries 没有生产 Queries 的 FC2- 查询变体；测试副本通过不能证明生产解析正确。可迁到真实 JavDB fixture transport；两个语料加载器仍重复，原始语料作为独立预期应保留。
  - 定位：`internal/codeid/prototype_test.go:22`；`internal/codeid/codeid.go`；`internal/codeid/corpus_test.go`；`internal/codeid/random_corpus_test.go`。
- **240.** 继续按真实覆盖收益复核简单 helper/容器与跨包集成测试；不把健康检查、失败预算、恢复或事务测试因篇幅直接删掉。当前尚未逐个证明这些用例可移除，不能写成已确认测试冗余。
  - 定位：`internal/tasks/tasks_test.go`；`cmd/miyabi/healthcheck_test.go`；`internal/app/pipeline_e2e_test.go`；`web/tests/pan-login-poll.test.ts`。

## 需要用户实测

- **195.** 115 登录弹窗关闭立即卸载 LoginContent，目录选择器保留内容直到退出动画结束。代码差异存在，但“动画塌陷”尚未在当前浏览器验证，不作为已复现故障。
  - 定位：`web/src/features/settings/pan-login-dialog.tsx`；`web/src/features/settings/pan-directory-dialog.tsx`。
- **227.** index.html 未提供 favicon、theme-color 或预先设置主题的脚本；暗色首屏闪白需浏览器冷启动确认。第 166 项修复后，缺失 favicon 请求返回 404，不再回退 index.html；主题首屏效果仍待实测，不得写成已复现。
  - 定位：`web/index.html`；`web/src/main.tsx`；`internal/api/router.go`。
- **245.** 补充用户反馈：部分卡片封面只显示一部分且不恢复，尚无具体影片与浏览器复现定位。应核对返回图片完整性、decode/onLoad 状态及容器尺寸；不能声称独立预览缩略图回退已修复它，也不能直接归因于模糊背景。
  - 定位：`web/src/components/media-image.tsx:69`；`web/src/components/movie/movie-card.tsx`。

## 按现有约定暂缓

- **155.** 卡片模糊背景仍存在；每卡片 matchMedia 的旧 hover hook 已删除。按用户选择保留模糊效果，尚无浏览器性能证据证明它是滚动卡顿原因，不据此要求移除。独立预览缩略图已按用户要求回退，不能重新列为必做优化。
  - 定位：`web/src/components/media-image.tsx`；`web/src/features/movie-detail/previews.tsx`。
- **167.** 用户明确当前无法限制上游地址，图片代理的地址与体积约束整项暂不处理。/api/image 经 Catalogue.Media 调用 FetchMedia，仅检查 https 和非空 host，未限制目标主机/IP，也未设置响应体大小上限；图片 MIME 检查发生在下载之后。当前客户端已禁止重定向，但仍可请求服务器可达的任意 HTTPS 目标并产生无上限的响应体读取。访问密码启用时需会话，未启用时可直接访问。此项仅暂缓，未解决或撤回风险结论。
  - 定位：`internal/javdb/media.go:26`；`internal/catalogue/service.go`；`internal/api/router.go`；`internal/netx/client.go`。
- **186.** Emby 已按用户要求与代理一致：开启后展开并保存，关闭立即提交；本地外观、即时订阅设置与文本表单职责不同。不再强制统一所有设置为即时保存，也不恢复已删除的 JavDB 线路设置。
  - 定位：`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/src/features/settings/subscription-section.tsx`。
- **187.** 已将设置任务区域及共享进度组件的重连按钮统一为“重新连接”，与媒体库一致，重连行为不变。其余两项按用户决定暂缓：订阅设置首次加载时仍显示禁用的默认选项，网络/Emby/订阅设置读取失败仍仅显示错误文字，不增加加载占位或就地重试入口，用户接受刷新页面恢复。此项为部分完成，不标记整项已解决；Emby 关闭时详情设置已隐藏。
  - 定位：`web/src/features/settings/tasks-section.tsx`；`web/src/features/tasks/scan-progress.tsx`；`web/src/features/library/page.tsx`；`web/src/features/settings/subscription-section.tsx`；`web/src/features/settings/network-section.tsx`；`web/src/features/settings/emby-section.tsx`。
- **205.** 撤回强制引入同时附加 aria-label 和 Tooltip 的 IconButton。用户已明确移除 shadcn 原始组件之外的 ARIA 描述，并取消 toast/媒体库动作 tooltip；不能以复用为由重新添加。其他现存 Tooltip 是否抽取单独评估。
  - 定位：`web/src/features/library/scan-button.tsx`；`web/src/features/tasks/task-toast-actions.tsx`。
- **230.** 图标控件缺少自定义可访问名称的事实仍在，但属于用户已决定不补充的自定义 ARIA 范围；不标记“已修复”，也不重新添加标签或 tooltip。旧数据源页和已删除的控件引用移除。
  - 定位：`web/src/components/floating-nav.tsx`；`web/src/features/movie-detail/preview-viewer.tsx`。
- **231.** MediaImage 仍没有 alt，预览按钮没有自定义 ARIA 名称。按当前无障碍范围约定暂不扩展此工作；标准 shadcn 内部行为保留，未把这个选择称为无障碍已达标。
  - 定位：`web/src/components/media-image.tsx`；`web/src/features/movie-detail/previews.tsx`。
- **233.** SettingRow 标题仍是 div，部分输入只提供 placeholder，缺少程序化名称；按用户移除自定义无障碍描述的要求，本轮只记录现状，不建议自动补 label/ARIA。
  - 定位：`web/src/features/settings/shared.tsx`；`web/src/features/search/page.tsx`。
- **234.** 用户明确暂不调整底部导航列表结构及页码元素，保留当前样式与行为。FloatingNav 的 ul 仍直接渲染 Link；数字页码使用 href="#" 并阻止默认跳转，前后页使用 button，未将其描述为普通点击必然跳到页首。PreviewViewer 缺少 DialogTitle 的部分继续按无障碍范围约定暂缓，开发态提示尚未浏览器确认。此项整项暂缓，未标记已解决。
  - 定位：`web/src/components/floating-nav.tsx`；`web/src/components/list-pagination.tsx`；`web/src/features/movie-detail/preview-viewer.tsx`。
- **244.** 非番号文件名可能符合通用番号模式的问题继续暂缓。用户已拒绝相机前缀、时间戳与分辨率硬编码黑名单；原示例 IMG-XXXX 没有数字，不能据原报告当作已复现样本。后续只在真实样本与通用规则基础上评估。
  - 定位：`internal/codeid/codeid.go:38`。

## 撤回原问题结论

- **141.** 撤回“ValidateSafeURL 只做语法检查”的直接建议。请求前、重定向、直连拨号各有安全职责；走代理时拨号目标是代理，拨号检查不能替代目标 URL 检查。也不能将每个请求固定描述成三次 DNS 查询。
  - 定位：`internal/netx/ssrf.go`。
- **171.** 撤回将回滚后 invalidate 或不同乐观更新策略直接列为问题。Emby 已明确可能先保存再在后续操作报错；重新读取用于校准最终服务端状态。网络设置也有事务/持久化边界，尚无证据证明这些校准多余。
  - 定位：`web/src/api/emby.ts`；`web/src/api/network.ts`；`internal/network/service.go`。
- **178.** 仅因 Source 接口单独放在 source.go 而要求合并不成立；它仍是磁力聚合器的有效依赖边界。旧 netx/probe.go 与 api/javbus.go 已不存在。
  - 定位：`internal/magnet/source.go`。
- **181.** movie-status 和 metadata-link 都有明确业务展示职责；单调用方或文件短不是问题。movie-states 已承担批量状态加载，本轮不建议为减少文件数而合并。
  - 定位：`web/src/features/library/movie-status.tsx`；`web/src/features/movie-detail/metadata-link.tsx`；`web/src/api/movie-states.ts`。
- **197.** 独立详情页与 hover 详情已删除，旧 max-w-7xl 与悬浮详情字号问题失效。剩余空状态高度、嵌套容器 gap/space-y 不能仅凭 class 判为重复间距，需有实际视觉差异再立项。
  - 定位：`web/src/features/library/page.tsx`；`web/src/features/discover/results.tsx`；`web/src/features/movie-detail/dialog.tsx`。
- **201.** cursor-pointer 单独一行或写在基类中运行等价，不是问题。将其统一到全局 CSS 可能改变其他控件行为，无需为了书写位置重构 shadcn 组件。
  - 定位：`web/src/components/ui/tabs.tsx`；`web/src/components/ui/select.tsx`；`web/src/components/ui/switch.tsx`。
- **203.** 访问密码页用 bg-muted、内部页面用 bg-background 是独立页面的设计差异，未发现功能或一致性约束要求两者相同。
  - 定位：`web/src/features/access-gate/access-gate.tsx`。
- **218.** 本地与远端 NFO 均调用 nfo.Decode，已非两套 XML 解码。剩余规范化/缺号回退分别依赖本地媒体番号和远端 sidecar 文件名，差异有业务语义，不强制合并。
  - 定位：`internal/library/scan/local_ingest.go`；`internal/library/scan/nfo.go`；`internal/nfo/movie.go`。
- **220.** 保留事实：清理未使用缓存需确认，退出/取消挂载/取消订阅多为直接执行。但“所有操作统一 ConfirmDialog”是产品决策，不是已证实正确性问题，也与避免多余确认的偏好不符，不据此增加弹窗。
  - 定位：`web/src/features/settings/data-section.tsx`；`web/src/features/settings/pan-section.tsx`；`web/src/features/subscriptions/actor-chip.tsx`。
- **226.** 搜索无关键词时不展示结果、返回按钮无历史时去 /discover 都是有效产品选择。没有证据表明 px-3 构成布局缺陷，不保留为强制待办。
  - 定位：`web/src/features/search/page.tsx`；`web/src/components/page-back-button.tsx`。
- **232.** 撤回旧的原生 <a><button> 嵌套和整卡 Button 包 Checkbox 描述：现在是 div 型 MovieDetailTrigger，内部按钮阻止冒泡，选择 Checkbox 是兄弟节点。卡片与 Checkbox 双 Tab 停靠点仍存在，属于键盘交互选择，不能继续沿用原生嵌套 bug。
  - 定位：`web/src/features/movie-detail/detail-trigger.tsx`；`web/src/components/movie/movie-card.tsx`；`web/src/features/subscriptions/subscription-card.tsx`。

## 已解决或随功能删除失效

- [x] **126.** 工作流 JSON 表达式索引与按通知版本共享的任务快照已实现；索引现在随当前数据库初始化创建，不再描述为旧库迁移。
  - 定位：`internal/database/task_indexes.go`；`internal/app/task_views.go`；`internal/app/task_snapshot_test.go`。
- [x] **127.** UI 通知与工作线程唤醒已分离。修正旧记录：当前 Finish 会 WakePool，这是结束资源占用、让后续任务继续执行所需；不能恢复旧的“Finish 不唤醒”要求。
  - 定位：`internal/tasks/bus.go`；`internal/tasks/queue.go`；`internal/tasks/pool_test.go`。
- [x] **128.** 当前 schema 保留 type/status、重试时间、资源键与历史 type 索引；领取与历史查询的索引测试通过。旧索引迁移已取消。
  - 定位：`internal/ent/schema/task.go`；`internal/database/task_indexes_test.go`。
- [x] **129.** Emby 增量通知只调用 /Library/Media/Updated，没有每批再请求 /Library/Refresh。
  - 定位：`internal/emby/service.go`；`internal/emby/client.go`。
- [x] **130.** MatchMovies 已使用 canonical_code 等值查询与分组匹配；schema hook 维护派生键，当前写入和重启测试通过。
  - 定位：`internal/library/scan/movies.go`；`internal/ent/schema/movie.go`；`internal/database/movie_keys_test.go`。
- [x] **131.** 本地导入已按 32 项分批，文件读取与图片准备在数据库写事务外，资料、文件、字幕及通知在批内提交。
  - 定位：`internal/library/scan/local_ingest.go`；`internal/library/scan/local_batch_test.go`。
- [x] **132.** 收尾按 100 部分页准备导出，再核对版本、批量入队并原子提交；导出已直接读取完整 metadata，不再需要 actors/tags 关联预加载。
  - 定位：`internal/library/scan/reconcile.go`；`internal/library/scan/reconcile_test.go`。
- [x] **133.** 独立 cover 任务、scrape_task_id 查询及专用索引已经删除。当前 scrape 自身保存 metadata_ready、artwork、completed 检查点，本项随架构替换失效。
  - 定位：`internal/tasks/kind.go`；`internal/library/scrape/scrape.go`。
- [x] **134.** 图片保护锁仅覆盖图片生成和检查点保存；下载及发布在锁外，缓存清理保留未完成任务引用。当前扫描与刮削已分池，旧报告的“单个媒体库 worker”描述失效。
  - 定位：`internal/library/scrape/artwork.go`；`internal/maintenance/service.go`；`internal/app/task_workers.go`；`internal/app/cover_lock_test.go`。
- [x] **135.** 目录缓存已限制为最多 128 个目录、合计 20,000 条文件记录，超限淘汰最早写入的列表；45 秒 TTL 到期项在下一次缓存访问或写入时回收，空闲时仍受容量约束。单个超过限额的目录完整读取但不缓存。网络请求仍在锁外，账号隔离、返回列表副本、索引变化后的缓存清空与发布前视频位置核验保留。
  - 定位：`internal/library/scrape/scrape.go`；`internal/library/scrape/dir_cache_test.go`；`internal/library/scrape/publish.go`。
- [x] **138.** ReadMetadata 已复用 OpenMedia 下载 CDN 文件，删除手动限流与 Client 中多余的 limiter 引用；无重试时两次 HTTP 请求仅 downurl 消耗一次 API 限流额度。保持相同 UA、无 OAuth 凭据的 CDN 请求及大小上限；单次下载用独立 35 秒上下文约束请求头和正文，不改变视频流超时。瞬时错误按 domain.RetryDelay 最多重试 3 次，遵守 Retry-After 和调用方取消，响应体在重试前关闭；CDN 退避不再影响其他 API 请求。
  - 定位：`internal/pan/metadata.go`；`internal/pan/client.go`；`internal/pan/metadata_test.go`。
- [x] **139.** 115 sidecar 上传实现已删除，已无逐文件申请 OSS STS token 的生产路径；不再增加 token 缓存。
  - 定位：`internal/pan/client.go`。
- [x] **140.** STRM 地址按文件、UA、账号、挂载目录和授权版本合并请求，最多缓存 256 条、60 秒，并在可识别签名到期前 30 秒停止复用；无法识别有效期或临近到期的地址不驻留，失败不缓存。命中后跳过文件索引和 115 地址申请，仍校验当前授权；登录、断开和换目录会隔离旧结果。共享查询有独立 45 秒超时并纳入关闭等待，调用方取消不影响其他等待者。原画/转码回退、GET/Range 重定向和 HEAD 探测保持原有行为，不缓存 CDN 响应头。
  - 定位：`internal/strm/relay.go`；`internal/strm/cache.go`；`internal/strm/cache_test.go`；`internal/drive/session.go`；`internal/api/strm_test.go`。
- [x] **142.** verifyAccount 按 credentialVersion 合并并发校验，复用 60 秒成功缓存；共享请求使用独立 45 秒超时并纳入关闭等待，单个等待者取消不影响其他调用，失败不缓存。RefreshToken 明确返回 ErrUnauthorized 时，在提交锁内重新核对凭据和 token 版本，再清除数据库与内存中的对应凭据并失效账号缓存；后续请求及重启不再重复刷新已拒绝的凭据。挂载目录和正在进行的扫码登录保留，旧响应不能清除新登录；网络超时、限流、服务端故障和响应解析错误不清除凭据。
  - 定位：`internal/drive/account.go`；`internal/drive/token.go`；`internal/drive/account_test.go`；`internal/drive/token_test.go`；`internal/drive/login_test.go`。
- [x] **146.** selectRoute 的故障恢复选线已用可重设的单次定时器替代 5ms ticker，按实际探测开始时间和当前最佳耗时安排下一次取消；只有严格超过最佳耗时才取消，保留同耗时优先顺序。已取消探测退出计时但继续等待收尾，无待计时探测时停止定时器。动态线路尚未发现时不提前取消慢引导线路，客户端创建耗时不计入探测延迟，全量测速行为保留。
  - 定位：`internal/javdb/route.go`；`internal/javdb/route_test.go`。
- [x] **147.** DetectLanguage 直接遍历字符串的前 15000 个 Unicode 字符，到达上限即停止，不再为全文构造 []rune。显式语言提示、简繁标记统计和同分时默认简体的规则保留；小型 marker 列表继续使用现有线性查找，不扩展未经测量的替换。
  - 定位：`internal/domain/subtitle/label.go`；`internal/domain/subtitle/label_test.go`。
- [x] **148.** Check 现在按本轮开始时间筛选到期订阅，以 next_check_at/id 游标连续翻页；10 部影片和 5 个演员是交替处理的页大小，不再是整轮上限，跨页保留 2 秒请求间隔。单条查询或保存失败仍继续后续条目，游标避免失败记录在本轮反复命中；取消后未处理条目仍保持到期，供后续检查。5 分钟周期和新增订阅唤醒机制保留。无磁力且继续等待时也发送订阅更新通知。BatchHandler 每项只读取一次订阅，每批复用一次配置与磁力选择器，保留随机节流、失败统计和检查点恢复。
  - 定位：`internal/monitor/check.go`；`internal/monitor/batch.go`；`internal/monitor/check_backlog_test.go`；`internal/monitor/batch_test.go`；`internal/app/app.go`。
- [x] **149.** Movie schema 已增加 created_at/id 组合索引，实际媒体库列表查询可利用该索引完成倒序分页。本地扫描先规范化绝对根路径，再以媒体绝对路径生成本地 fileID，区分不同根目录的同名文件；STRM 中识别出的网盘 fileID 继续保留。已有本地文件的名称、大小和相对路径仅在变化时更新，即使影片被手动纠正也刷新文件信息，同时保护手动资料及网盘文件属性。未添加旧本地 fileID 迁移或兼容分支。
  - 定位：`internal/ent/schema/movie.go`；`internal/ent/migrate/schema.go`；`internal/library/scan/local.go`；`internal/library/scan/local_ingest.go`；`internal/library/listing_index_test.go`；`internal/library/scan/local_identity_test.go`。
- [x] **153.** 扫描、重新刮削、离线提交/取消/换源、任务控制和 SSE 共用按 QueryClient/查询前缀隔离的刷新队列；开始前合并重复失效，读取期间的变化只排一次后续校正，避免相互取消重发。首轮丢弃变化前的旧请求，完整任务快照取消当前及排队的任务读取，后续新变化仍可刷新。操作响应只在缓存仍与提交前一致且没有读取进行时回写，避免晚到响应回退已更新的状态；原有账号、目录、影片筛选及 enabled 限制保留，重新刮削回写也核对来源。首屏校正、手动重连兜底和失败后的重新读取保留。
  - 定位：`web/src/api/query-refresh.ts`；`web/src/api/offline.ts`；`web/src/api/library.ts`；`web/src/api/tasks.ts`；`web/src/api/movie-states.ts`；`web/src/features/tasks/task-events.tsx`；`web/tests/query-refresh.test.ts`；`web/tests/task-events.test.tsx`。
- [x] **156.** sourceSession 的 List、Info、Read、PlayURL、DownloadURL 已共用私有 readSource 泛型函数，集中请求前来源检查、现有令牌处理和成功后来源检查；失败或来源改变时返回对应类型零值，保留上游错误优先顺序。AddOffline 继续保留已提交结果，RemoveOffline 继续检查完成后的来源，OfflineTasks 和 CommitAccount 维持账号范围；未统一读写语义、增加策略开关或延长锁的持有范围。
  - 定位：`internal/drive/session.go`；`internal/drive/session_read_test.go`。
- [x] **157.** MoviePage 与 fetchMagnets 已共用限流、GET 请求与公共请求头、网络错误处理、限量响应读取，以及挑战页/验证页/状态码校验。详情页独有的未找到空结果规则、两类重定向处理、Accept/Referer 等请求头和各自解析保留在调用方；取消不禁用来源，响应体在成功与提前返回时均关闭。ensureDetailParams 继续只委托详情读取与参数解析，健康探测保持独立，不增加通用策略开关或缓存。
  - 定位：`internal/javbus/client.go`；`internal/javbus/client_test.go`；`internal/javbus/response_test.go`。
- [x] **158.** 普通业务错误由访问日志统一输出一条记录：错误中间件保留响应映射，恢复中间件外侧统一收集最终错误，附加 kind 和经脱敏的完整错误链；固定日志消息，移除重复的 cause 字段。已写响应及断链恢复后的错误也可记录，实际响应状态与日志级别规则保持原有行为。panic 单独保留 ERROR 堆栈诊断，并增加请求 ID 与访问摘要关联；499 访问过滤和 DEBUG 取消诊断保留，未修改领域错误字符串或公共响应。
  - 定位：`internal/api/error.go`；`internal/api/logging.go`；`internal/api/router.go`；`internal/api/error_test.go`；`internal/api/logging_test.go`。
- [x] **159.** authMiddleware 的令牌缺失和无效/过期分支已共用私有 abortAccessUnauthorized 响应函数，保留 HTTP 401、UNAUTHORIZED、两种提示文案及中止后续处理的行为。仅整理本站 Cookie 鉴权响应，不改变 PAN_UNAUTHORIZED、密码错误、播放令牌错误或通用错误中间件。
  - 定位：`internal/api/auth.go`；`internal/api/cookie_auth_test.go`。
- [x] **161.** 登录改为锁内预占与统一结算，失败数和在途数共同限制准入；格式错误只释放名额，成功清除失败历史但保留其他在途请求。IP 记录最多 4096 条，登录访问触发定期过期回收；满额时暂缓新 IP，不淘汰有效失败记录、封禁或在途记录。空闲时不主动清理，但仍受容量约束。保留默认 5 分钟内失败 5 次封禁 24 小时的策略，修正单次失败阈值及到期边界；错误 cause 使用通用限流文案，封禁提示按真实剩余时间生成。
  - 定位：`internal/api/rate_limiter.go`；`internal/api/auth.go`；`internal/api/rate_limiter_test.go`；`internal/api/auth_rate_limit_test.go`。
- [x] **166.** 前端目录请求返回 404，首页与 SPA 回退直接提供 index.html，入口缺失也不列目录。仅已存在且符合当前 Vite 八位哈希命名的 assets 资源使用 `public, max-age=31536000, immutable`；入口和普通静态文件使用 `no-cache`。缺失资源、无效文件路径和 API 路径不回退 HTML；前端只接受 GET/HEAD，并保留静态文件条件请求与 Range 支持。SPA 回退不再改写原请求路径。
  - 定位：`internal/api/router.go`；`internal/api/frontend_test.go`；`web/vite.config.ts`。
- [x] **168.** GFriends 索引下载与本地缓存读取统一限制为 32 MiB，先检查已知大小，再限量读取至上限加 1 字节以识别超限，不接受截断数据。超限镜像尝试下一来源；全部失败时沿用有效旧索引及 5 分钟重试间隔，首次无索引时返回错误，超大磁盘缓存不阻止重新下载。7 天缓存有效期保留。共享下载函数同时严格执行头像原有的 10 MiB 限额，超限进入镜像回退；第 167 项图片代理仍暂缓。
  - 定位：`internal/gfriends/client.go`；`internal/gfriends/limits_test.go`；`internal/gfriends/refresh_test.go`。
- [x] **173.** 代理与 Emby 开启后的地址聚焦改为条件 autoFocus，由用户开启时随输入框挂载执行，移除 50ms 定时器；读取已开启的配置不自动聚焦，代理空地址校验的主动聚焦保留。AccessGate 的登录失效监听仅依赖 refetch，保留解绑清理，避免每次渲染重复绑定。原监听已有清理，并不存在持续累积监听或每次输入都请求后端的问题。
  - 定位：`web/src/features/settings/network-section.tsx`；`web/src/features/settings/emby-section.tsx`；`web/src/features/access-gate/access-gate.tsx`。
- [x] **184.** 已移除 export 内的 ./data/emby 和 8080 默认推导，默认目录/播放地址统一由 config.Load 按 DataDir/Listen 解析。实际导出与批量重写共用 export.Config.Validate，缺失目的目录或播放地址时在写文件前报错，Manager 不再静默跳过空配置重写；纯路径/内容构造函数使用调用方提供的已解析配置。OutboundIP 注释已准确描述出口路由优先、失败后遍历网卡优先私网、最终回环的顺序，选址行为保留。前端补协议的少量重复不单独认定为缺陷，Emby 端口拆分与代理输入继续保留各自职责。
  - 定位：`internal/export/config.go`；`internal/export/export.go`；`internal/library/scrape/export.go`；`internal/netx/ip.go`；`internal/library/scrape/export_config_test.go`；`internal/export/export_test.go`。
- [x] **189.** 影片卡片已区分 waiting 的铃铛图标和 added 的勾选图标，提示分别为持续检查符合偏好的磁力、已加入 115 离线下载；已有长期无源记录的可操作入口显示“重新订阅”。卡片与详情的等待提示不再无条件承诺自动加入 115，手动入库设置同样适用。详情保留 waiting 的取消操作及 added/stale 的重新订阅，演员 active/paused 的取消按钮改用 BellOff 和“取消订阅”。卡片防重复提交与详情重新启动检查的行为保留；状态按钮与操作按钮的样式差异不再认定为缺陷。
  - 定位：`web/src/components/movie/movie-subscribe-button.tsx`；`web/src/features/movie-detail/subscription-action.tsx`；`web/src/features/discover/metadata-search-page.tsx`。
- [x] **192.** 继续使用 shadcn Skeleton，将番号与标题归组，元资料改为标签/内容两列占位并对齐行距；统计区域改为带分隔线的一整条三栏容器，内边距与实际内容一致。预览占位统一为 rounded-xl。整页加载和磁力单独加载共用 MovieMagnetsSkeleton，使用与实际列表一致的 Card、space-y-4 间距及桌面/移动端内容与按钮布局。按用户要求，本地详情继续使用完整共享骨架，保留磁力和推荐占位；未修改真实详情布局或请求逻辑。原磁力骨架重复是代码重复，不是已证实的重复渲染或性能故障。
  - 定位：`web/src/features/movie-detail/skeleton.tsx`；`web/src/features/movie-detail/magnets.tsx`；`web/src/features/movie-detail/content.tsx`。
- [x] **193.** 分类错误组件已传入 taxonomy.isFetching，错误提示仍可见时，重试期间按钮禁用并显示旋转图标。首次无数据的重试仍切回原有骨架，保持原请求与分支逻辑。复核确认分类页和元资料筛选页骨架均为 rounded-full，撤回原圆角差异描述；isLoading/isPending 的差别不作为缺陷或强制统一项。
  - 定位：`web/src/features/discover/category-filters.tsx`；`web/src/components/error-state.tsx`；`web/src/features/discover/metadata-search-page.tsx`。
- [x] **194.** SettingRow 说明文字已从 break-all 改为 wrap-break-word，优先按正常规则换行，仅在长单词或路径无法容纳时允许断开。保留 min-w-0 及现有 Input/Select 宽度，不强制不同用途的控件同宽。
  - 定位：`web/src/features/settings/shared.tsx:45`。
- [x] **206.** 网络代理、Emby 的保存/关闭/测试连接及订阅设置保存共七处操作错误，统一使用 describeApiError：普通网络异常显示中文重试提示，115 登录失效显示重新登录提示，其他 ApiError 保留服务端消息。继续使用单个本地 toast；这些 mutation 未设置全局 errorTitle，没有新增重复 toast。页面 InlineError 仍用于配置读取失败，与操作失败是不同请求；保存回滚、重新读取及现有校验提示保留，不增加第 187 项已暂缓的重试入口。
  - 定位：`web/src/features/settings/network-section.tsx`；`web/src/features/settings/emby-section.tsx`；`web/src/features/settings/subscription-section.tsx`；`web/src/api/client.ts`；`web/src/api/query-client.ts`。
- [x] **212.** movie-hover-details 已删除，详情统计栏与悬浮卡片之间的重复不存在。
  - 定位：`web/src/features/movie-detail/hero.tsx`。
- [x] **222.** 刷新统计和清理缓存已分别控制禁用状态：刷新按钮在统计请求或清理期间禁用；清理按钮仅在清理中、读取失败、无统计或无可清理缓存时禁用，已有有效统计的后台刷新不再阻止清理入口。确认弹窗、后端引用检查和清理成功后取消旧统计请求的逻辑保留。按用户认可的范围，刷新按钮继续位于数据目录行，不将位置选择视为缺陷。
  - 定位：`web/src/features/settings/data-section.tsx`；`web/src/api/data.ts`。
- [x] **224.** 用户决定不再展示已挂载媒体目录路径，已删除禁用输入框及其宽度样式，设置行统一为文字与操作按钮同行。挂载/更换目录、取消挂载、请求中禁用及错误提示保留，目录选择器仍接收当前目录。原只读/复制改造随路径展示删除失效，不再添加只读输入框。
  - 定位：`web/src/features/settings/pan-directory-row.tsx`；`web/src/features/settings/pan-directory-dialog.tsx`。
- [x] **235.** API 访问日志在输出前省略 request.query 与 request.referer，成功请求只按字段名过滤，保留方法、路径、状态码、耗时和请求 ID；不逐个解析请求 URL。错误诊断（包含完整原因链）和 panic 信息中的 HTTP(S)/SOCKS5 地址移除凭据及片段、遮蔽全部查询参数；独立编码或不可解析的 URL 整段遮蔽。第 158 项整理后，普通请求使用固定日志消息，完整错误链仅记录在 error 字段。Gin 恢复的原始错误/请求输出已关闭，普通 panic 改为结构化脱敏日志并保留堆栈。原请求、响应、STRM 文件及播放鉴权不变，499 访问日志过滤保留。此处理作用于 Miyabi API 日志，不清理既有日志，也不覆盖外部反向代理或 Emby 自身的日志配置。
  - 定位：`internal/api/logging.go`；`internal/api/logging_test.go`；`internal/api/router.go`；`internal/api/error.go`。
- [x] **239.** 旧 115 字幕、旧封面 payload 和历史迁移测试已清理；dir_cache、snapshot、检查点、来源隔离与事务测试仍验证当前功能，不按旧报告要求删除。剩余跨包测试归属统一放入 219/240。
  - 定位：`internal/library/scrape/dir_cache_test.go`；`internal/library/scrape/snapshot_test.go`；`internal/library/scrape/cover_checkpoint_test.go`；`internal/database/sqlite_test.go`。

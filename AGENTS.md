# Miyabi

接入 115 网盘的 JAV 媒体库管理工具。单二进制部署，Go 后端嵌入 React 前端。

## 核心原则

- 单用户自用工具，不做多租户、不做权限系统。
- 代码简洁优先：不写防御性兜底，错误直接向上返回，由 API 层统一转成响应。
- 成熟库能用就用，不自己造轮子。
- 网盘目前只有 115，包名直接叫 `pan`，不带 115 前缀。
- 115 是唯一媒体文件来源；JavDB 是唯一发现、元数据和磁力来源，不接入 DMM、JavBus 或其他刮削源。
- JavDB 接入参考 `javdb-cli` 的实现与测试，但 Miyabi 本地实现所需协议，不直接依赖其 SDK，也不以子进程调用 CLI。
- JavDB 的剧情字段覆盖率和准确性不足，项目不采集、不存储、不展示 `plot`，也不把标题或其他字段伪装成剧情。
- 每层职责单一，禁止跨层调用（handler 不碰 ent；`javdb`、`pan` 不碰数据库）。

## 技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| HTTP | `github.com/gin-gonic/gin` | 参数绑定、校验、JSON 响应、文件上传、SSE 开箱即用 |
| ORM | `entgo.io/ent` | schema 即代码，查询类型安全，多对多关系清晰 |
| 数据库 | SQLite (`modernc.org/sqlite`) | 纯 Go 无 CGO，开 WAL 模式 |
| 任务队列 | 自维护 goroutine 池 + ent 任务表 | 单机足够，重启可恢复 |
| 实时推送 | SSE | 单向进度推送，比 WebSocket 简单 |
| 通用 HTTP 客户端 | `github.com/go-resty/resty/v2` | 115、图片下载等普通 HTTP 请求 |
| JavDB Transport | `github.com/bogdanfinn/tls-client` | App API 已验证使用 Chrome TLS 指纹；只在 `internal/javdb` 内使用 |
| 图片处理 | `github.com/disintegration/imaging` | 海报裁剪、缩略图 |
| 限速 | `golang.org/x/time/rate` | 115 与 JavDB 分别限速 |
| 配置 | `github.com/knadh/koanf/v2` | 启动参数：监听地址、数据目录、日志级别、代理。文件 + 环境变量覆盖。服务端运行时设置存 Setting 表；NSFW 由 Zustand persist 存 localStorage，主题由 next-themes 管理 |
| 日志 | `log/slog` | 标准库 |
| 前端 | Vite 8 + React 19 + TypeScript | |
| 前端状态 | TanStack Query + Zustand | 服务端状态与 UI 状态分离 |
| 前端 UI | shadcn/ui + Tailwind CSS 4 | 组件可拷贝可改 |
| 播放器 | Vidstack React + hls.js | 共用播放浮层，支持原文件与 115 HLS 转码 |
| 前端路由 | TanStack Router | 类型安全 |
| 前端 lint/格式化 | oxlint + oxfmt | 不用 ESLint/Prettier |

## 目录结构

```
miyabi/
├── cmd/miyabi/main.go          入口：加载配置、初始化依赖、启动 HTTP
├── internal/
│   ├── api/                    gin 路由与 handler，只做参数绑定与响应
│   │   ├── router.go
│   │   ├── movie.go
│   │   ├── discover.go
│   │   ├── task.go
│   │   ├── pan.go
│   │   ├── play.go
│   │   ├── setting.go
│   │   └── sse.go
│   ├── service/                业务编排，唯一允许同时调用 ent / pan / javdb 的层
│   │   ├── library.go          扫描入库、文件与影片关联
│   │   ├── discover.go         JavDB 发现、搜索、分类与本地状态投影
│   │   ├── scrape.go           JavDB 元数据入库、封面下载与 NFO 写入
│   │   ├── offline.go          JavDB 磁力提交、115 离线任务轮询
│   │   └── play.go             获取播放地址
│   ├── ent/                    ent 生成代码（schema/ 手写，generate.go 触发生成）
│   │   ├── generate.go
│   │   └── schema/
│   │       ├── mixin.go        created_at / updated_at
│   │       ├── movie.go
│   │       ├── actor.go
│   │       ├── tag.go
│   │       ├── file.go
│   │       ├── task.go
│   │       └── setting.go
│   ├── pan/                    115 客户端，对外只暴露 Client
│   │   ├── client.go           鉴权、限速、通用请求
│   │   ├── auth.go             开放平台 OAuth / 设备码
│   │   ├── file.go             列目录、搜索、移动、重命名、上传
│   │   ├── offline.go          离线下载增删查
│   │   └── play.go             直链与 m3u8
│   ├── javdb/                  JavDB App API，本地最小协议实现
│   │   ├── client.go           公共参数、响应 envelope、错误处理与限速
│   │   ├── transport.go        tls-client、超时、代理与 context
│   │   ├── signature.go        jdsignature
│   │   ├── route.go            startup、动态域名解密、选线与故障重选
│   │   ├── wire.go             私有 API JSON wire 类型
│   │   ├── model.go            对 service 暴露的强类型模型
│   │   ├── movie.go            搜索、发现、影片详情
│   │   ├── entity.go           演员、系列、厂牌、导演
│   │   ├── tag.go              标签 taxonomy
│   │   └── magnet.go           磁力列表与排序
│   ├── nfo/                    Kodi 格式 .nfo 读写
│   ├── codeid/                 番号识别与规范化，纯函数，表驱动测试
│   ├── image/                  封面存储路径、裁剪、缩略图
│   ├── worker/                 任务池：取任务、执行、写进度、广播事件
│   └── config/
├── web/                        React 项目
│   ├── src/
│   │   ├── routes/             TanStack Router 文件路由
│   │   ├── api/                fetch 封装 + TanStack Query hooks
│   │   ├── components/
│   │   └── stores/
│   └── dist/                   构建产物，由 Go embed
├── embed.go                    //go:embed web/dist
├── Makefile
└── AGENTS.md
```

## 数据持久化原则

115 是唯一媒体文件事实来源，JavDB 是外部目录与元数据来源。本地只有索引和缓存，媒体库可从 115 中的文件和 NFO 完整重建。

- **SQLite**：索引。影片、演员、文件关系、任务状态。
- **本地图片目录**：缓存。封面、海报、缩略图，用于列表页快速渲染。直接从 115 读图需要换取时效直链且受限速，不适合高频小图请求。
- **115 影片目录**：元数据完成后写入 `<code>.nfo`、`poster.jpg`、`fanart.jpg`，与视频同目录。挂载根目录、多影片共用目录或已有不同内容图片时，图片使用 `<code>-poster.jpg` / `<code>-fanart.jpg`，由 NFO 引用，避免覆盖。NFO 不写剧情。目录结构兼容 Emby/Jellyfin。
- **JavDB 缓存**：不做全量镜像。taxonomy、发现列表和详情只做有 TTL 的按需缓存；缓存失效不影响已有媒体库。
- **发现与媒体库分离**：发现结果不写入 Movie。只有 115 中实际出现视频文件并完成扫描后，才创建或关联 Movie。
- **重建**：扫描时若目录已有 `.nfo`，直接解析入库并下载图片到缓存，不再请求 JavDB。换机器或删库后重新扫描即可恢复。

## 数据模型

- **Movie**：`code`（唯一，规范化番号）、`javdb_id`（可空唯一）、`title`、`release_date`、`duration`（分钟）、`director_id/name`、`maker_id/name`、`series_id/name`、`rating`、`cover`、`poster`、`fanarts`（JSON）、`scrape_status`（pending/done/failed）。没有 `plot` 字段。
- **Actor**：`javdb_id` 唯一、`name`、`name_zht`、`gender`、`avatar`，与 Movie 多对多。JavDB 的演员数组包含男性演员，因此不使用 Actress 模型；UI 可默认只展示女性演员。年龄由生日计算，不持久化。
- **Tag**：`javdb_id` 唯一、`name`、`name_zht`、`category_id`，与 Movie 多对多。标签名称不作为唯一键。
- **File**：`file_id`（唯一）、`pick_code`、`sha1`、`name`、`size`、`parent_id`、`account_id`、`root_id`、`path`、`scan_id`，可选关联 Movie。账号和根目录用于区分索引来源；每次扫描执行使用新的 `scan_id`。
- **Task**：`type`、`status`（queued/running/done/failed）、`payload`（JSON）、`progress`、`error`、`created_at`、`updated_at`。
- **Setting**：`key` 唯一、`value` JSON。

## JavDB 协议约定

JavDB 当前没有官方公开 API。Miyabi 使用经 `javdb-cli` 验证的 Android App 1.9.28 私有 JSON API，但只实现产品需要的匿名只读子集。私有协议没有稳定性保证，wire 类型、签名、设备参数和线路细节必须封装在 `internal/javdb`，不能泄漏到 handler、ent schema 或前端；`javdb_id` 等业务需要的稳定来源标识除外。

### 已验证接口

- 搜索：`GET /api/v2/search`
- 发现与分类浏览：`GET /api/v1/movies/tags`
- 影片详情：`GET /api/v4/movies/{id}`
- 磁力：`GET /api/v1/movies/{id}/magnets`
- 标签 taxonomy：`GET /api/v2/tags`
- 实体详情：`GET /api/v1/actors/{id}`、`series/{id}`、`makers/{id}`、`directors/{id}`
- 实体作品仍通过 `/api/v1/movies/tags` 的实体 filter mask 查询。
- 2026-09-07 实网对比：厂牌、演员、导演样本切换四个分区后，返回影片 ID 和顺序不变；系列样本在有码与 FC2 两区也相同。实体作品的页面 URL 和 API 参数不携带 `zone`，缓存不再按分区拆分；上游 mask 使用 `:a:{id}`、`:s:{id}`、`:m:{id}`、`:d:{id}`。四类实体均验证空分区槽与带区号的结果相同，必须保留前导冒号，省略整个槽位会被解析成普通发现列表。
- 关键词搜索的 `movie_type`、发现列表及标签浏览的分区均经实网对比有效。全局搜索仅使用关键词和分页，不携带分区、分类或排序条件；分区选择只放在分类浏览及标签关联作品页。不跨分区请求后拼接列表。
- 最新与即将发行使用空分区槽（`:t:m::::` / `:t:::::`），保留各自的排序和上游原始分页。实测最新返回跨分区结果，即将发行前两页与有码列表相同；不得因此把空分区槽解释为固定有码。不能省略整个 `filter_by` 参数，上游会返回参数错误。
- 当前需求全部可匿名访问，不实现 JavDB 登录、看过、想看、评论、TOP250、用户合集和以图搜番。

### 已验证字段边界

- 列表：`id`、`number`、`title`、`origin_title`、`release_date`、`duration`、`thumb_url`、`cover_url`、`preview_images`、`magnets_count`、字幕/预览标志。
- 详情：列表字段以及 `score`、演员、标签、系列、厂牌、导演、预览图、预览视频。
- 演员详情还可返回头像、繁中名、生日、出生地、身高、三围、罩杯、血型和社交账号；第一阶段只持久化媒体库和列表需要的字段，其余按需请求。
- taxonomy 实测有码区为 11 个分类、355 个标签；只请求繁中数据，保留标签 ID。
- 磁力：`hash`、`name`、`size`、`cnsub`、`hd`、`files_count`、`created_at`。提交 115 时自行从 hash 构造标准 magnet URI，不依赖第三方跳转 URL。
- `duration` 在 26 部跨年份、跨分区样本中覆盖 26/26，可作为稳定字段，单位为分钟。
- `summary` 在 26 部样本中繁中仅 8/26 非空，近期有码、无码和 FC2 样本均为 0，且出现与影片演员不一致的错误内容。项目明确丢弃该字段，不寻找其他剧情替代字段。

### 请求与语言

- 请求携带 App 版本、Android 设备参数、稳定 `device_uuid`、`jdsignature` 和 `Dart/3.4 (dart:io)` User-Agent。
- 签名常量、App 版本和设备 profile 集中定义，使用 golden vector 测试，不散落在业务代码。
- 影片详情默认使用 `zh-TW`。实测 `en` 的剧情均为空，`ja` 与 `zh-TW` 完全相同，视为回退而非独立日文数据，不实现 `ja` 模式。
- taxonomy 只获取 `zh-TW`，使用单一 `name` 字段展示分类和选项，不请求英文数据或合并别名。
- JavDB 是 Resty 约定的唯一例外：使用 `tls-client` 的 Chrome profile。不要自己实现 TLS 指纹，也不要为了形式统一强行套入 Resty。

### 自动线路

- 服务重启后直接恢复已保存的 host、自动/手动选择模式和最近测速延迟，业务请求复用该线路，不重复执行全线路测速。缓存线路发生连接失败时仍按下述快速重选规则处理。
- 没有缓存线路时，首次请求对固定 bootstrap 和发现的动态候选完整测速，等待各线路成功或超时后汇总，使用延迟最低者；不因已有较快线路而提前取消其他探测。
- 手动重新测速同样覆盖全部已知线路，以及各合法 startup 响应中解密得到的 `backup_domains_data.apiDomains`，按 host 去重，每条线路只探测一次，最后自动选择延迟最低者。完整测速与连接失败时的快速重选分别合并并发请求，互不替代。
- 缓存稳定 `device_uuid` 和最终 host；选线探测不携带 token、不开业务重试。
- Miyabi 是长驻服务，不能照搬 CLI 的“一次命令一次选线”。当前线路发生连接失败、DNS、TLS、超时或 502/503/504 时，用 `singleflight` 触发一次重选，原 GET 请求最多重放一次，并原子替换共享 Client。
- 4xx、JavDB `success: 0` 业务错误、JSON 解码错误和字段不兼容不触发换线，应直接暴露错误以便发现协议变化。
- 设置页沿用 jm-boom 的线路下拉框和测速按钮，展示当前线路、候选状态与最近延迟，支持自动优选和手动切换。手动选择也由后端先验证再启用；连接失败仍按自动重选规则处理。测速按钮重新执行自动优选，不做固定周期的全线路探测。
- 后端不可用时，设置页显示启动服务和重试提示，不直接展示底层异常；NSFW 等本地显示偏好不受影响。

## 115 授权与媒体目录

- 设置页“115 网盘”使用“账号登录”行，右侧 Lucide 二维码按钮打开 shadcn Dialog；扫码确认后关闭弹窗并更新账号状态。已登录时隐藏二维码按钮，显示账号和退出入口；授权失效后恢复扫码入口。
- 已登录账号左侧使用 shadcn Avatar 展示头像，头像右侧上方为用户名称、下方为 115 返回的等级名称，退出按钮位于最右侧。账号头像和登录二维码均不受 NSFW 影响；NSFW 只隐藏媒体内容图片。
- 媒体目录下方使用 shadcn Progress 展示容量：右上角为“已使用 / 总容量”，加粗的容量条区分已用和剩余空间，剩余区域宽度足够时在条内显示数值，下方保留图例和剩余容量。数据复用 `/open/user/info`，不单独请求容量，也不虚构文件分类占比。
- 使用 JavdBviewed 的 OpenList APP ID 设备码授权方式，直接请求 115，不依赖 OpenList 服务。PKCE 使用随机 verifier、`base64(SHA256(verifier))` 和 `code_challenge_method=sha256`。
- 二维码通过 115 的 `/api/1.0/web/1.0/qrcode?uid=...` 图片接口取得，后端用 Resty 获取，前端以普通 `<img>` 展示。登录二维码不经过 `MediaImage`，不受 NSFW 影响，也不引入二维码生成库或第三方二维码服务。
- 授权参数留在 `pan` 包，service 保管一个临时登录会话；前端用 TanStack Query 轮询，关闭弹窗时停止。过期、取消或出错后由用户重新获取二维码。
- `access_token`、`refresh_token` 和过期时间一起存入 Setting 的 `pan.credentials`，不返回前端、不写入日志或 localStorage。按请求需要刷新令牌，串行处理刷新与凭据写入；令牌交换或刷新开始后，即使浏览器取消请求也完成持久化。
- 退出登录删除 Miyabi 保存的凭据，不调用 115 撤销应用授权接口。
- 登录后可在“媒体目录”行通过 Dialog 浏览 115 文件夹并挂载当前目录，支持路径导航和分页；文件仅用于浏览，不能作为媒体目录选择。
- “媒体目录”描述保持固定，绑定路径显示在右侧操作按钮之前的禁用 shadcn Input 中，挂载/更换按钮使用 shadcn Tooltip。目录弹窗的路径导航使用 shadcn Breadcrumb，长名称保持单行省略。
- 挂载目录由后端向 115 读取确认，目录 ID、名称、路径及所属账号一起存入 Setting 的 `pan.library_directory`。同一账号重新登录保留选择，换号后不沿用旧账号目录；取消挂载只删除本地配置。
- 详情页磁力在未登录时只显示复制，登录后增加“一键加入 115”。必须先挂载当前账号的媒体目录；未挂载时提示前往设置，保留复制功能。提交期间禁用重复点击，只有 115 单条磁力结果成功才显示“已加入”。
- 磁力按钮通过本地 Task 查询恢复状态，按账号、影片和 hash 读取最新一次任务；下载中、入库处理中禁用提交，只有当前目录的关联 File 仍存在时显示“已入库”。有视频但未识别出对应影片时显示“已下载”。完成的下载历史不代表文件仍存在；扫描确认删除后恢复普通“一键加入 115”，没有“重新加入”状态。存在下载或入库流程时每 5 秒刷新本地状态，SSE 同步更新影片标签和按钮。
- 115 返回重复任务（10008）时，复用同目录的进行中任务；已完成且视频仍在媒体目录内时复用结果并扫描；只有确认视频不存在才以 `del_source_file=0` 删除该条历史任务并提交。网络、鉴权和协议错误不当作文件不存在，不删除源文件，不接管媒体目录之外的资源。
- 影片状态 `saving` 的界面文案为“下载中”，表示离线任务进行中，不表示正在刮削。只有扫描创建或关联 Movie 后才能显示“已入库”；已下载仍需后续扫描入库。
- 离线提交由 service 核对磁力属于当前 JavDB 影片，从 hash 构造 URI，再由 pan 使用开放平台 `/open/offline/add_task_urls` 提交到已挂载目录；前端不接触令牌，也不提交任意来源的 URL。
- 已接受的离线任务写入 Task（type=offline），保存规范化 code、javdb_id、hash、info_hash、account_id 和 directory_id。同账号同 hash 的进行中任务复用已有记录；后台每 30 秒同步当前账号的远端任务状态，无进行中任务时不请求 115。
- 115 接受任务后只记录下载状态。下载完成时保存 `file_id` 并在同一事务内创建独立的定向扫描任务，支持文件或文件夹，不能合并到可能已越过该目录的运行中扫描。扫描后才创建 Movie，并继续刮削、缓存封面和写回 NFO；切换账号或挂载目录后不向旧任务的来源写入数据。

## 关键流程

### 当前扫描实现

- 媒体库手动创建 `scan` 任务，后台递归读取已挂载目录的全部分页。视频按文件名识别番号，分集关联同一 Movie，未识别视频仍保存 File，前端提供文件列表查看。
- 扫描分页批量 upsert File/Movie，与本页任务进度一起提交事务；已有元数据、演员标签关系和刮削状态保持不变，新影片为 `pending`。
- 封面写回成功时，在 cover 任务中保存已处理视频与 NFO、图片的快照。重扫复用目录分页结果比较快照，仅对新增、失败、内容变化或本地图片缓存缺失的影片重新入队；没有快照的旧记录完成一次处理后建立基线。
- 媒体库查询和发现页的“已入库”投影均要求影片关联当前挂载账号与根目录的 File，不因旧目录索引或孤立元数据记录显示已入库；浏览本地媒体库不请求 115。
- 完整扫描成功后才按账号和挂载根目录清理未出现的 File，并删除因此失去全部文件的 Movie。扫描失败或取消不执行未读取文件的清理；只更新本地索引，不删除 115 文件。
- 任务池注册 `scan`、`scrape`、`cover`，并发为 1；重启恢复排队任务。扫描的索引收尾与刮削入队、元数据写库与封面入队均在同一事务完成；已完成衔接的阶段不会在重启时重复入队。`offline` 保留远端轮询流程，不由本地任务池重新执行。
- `/api/tasks/events` 提供 SSE，每次连接发送最新任务快照，进度更新写入 TanStack Query；未知总量阶段显示活动进度条及实际计数。媒体库和设置页展示扫描状态，不新增顶级任务菜单。
- SSE 分别推送任务快照与媒体库、离线状态的数据版本；单纯进度变化不刷新影片查询。子任务进度在 SQLite 内分组汇总，不把完整 NFO payload 逐条读回再解析。
- 完整扫描只清理当前挂载根目录；离线下载触发的定向扫描只清理目标文件或目标子树，不能清理整库。目录内单一 NFO 可为未识别的视频提供番号，避免重扫时丢失已有影片关系。
- 刮削优先从匹配的 NFO 恢复元数据和同目录图片；没有 NFO 时复用完整的本地元数据，或按规范化番号精确匹配 JavDB。封面裁剪和缩略图存入本地内容寻址缓存，API 以不可变 URL 提供图片。图片仍经过 MediaImage 的 NSFW 控制。
- 写回按图片、NFO 的顺序进行，NFO 最后写入作为完成标记；重启重试复用相同内容的图片，保留已有 NFO 及不同内容文件，不覆盖用户编辑。存在不完整 NFO 时直接报告错误，不绕过它请求 JavDB。
- SSE 将刮削和封面子任务汇总进对应扫描的紧凑进度区；扫描阶段不伪造总量，元数据阶段展示已处理部数。扫描完成不等于刮削完成；封面缓存与 115 写回都完成后才标记 `scrape_status=done`。

### 完整流程目标

**扫描入库**：`pan.Client.List` 递归目录 → 目录内有 `.nfo` 则解析入库并入队封面缓存任务 → 否则逐文件 `codeid.Parse` → upsert File，命中番号则 upsert Movie 并关联 → 新影片入队刮削任务。

**发现**：`javdb.Client.Browse` 或 `Search` → 转换为发现 DTO → 按规范化番号查询本地 Movie 与进行中的 Task → 返回 `not_in_library`、`saving`、`in_library` 状态。发现列表允许包含未入库影片，不依赖 115 中已有内容。

**最新与即将发行**：发现列表保留 JavDB 返回的原始分页内容，不按发行日期做本地过滤，避免每页条目减少。`release_date > today` 的影片在卡片信息区显示“即将发行”，放在“有磁力”旁边。有磁力与发行日期状态独立；`magnets_count > 0` 只显示“有磁力”，不显示数量；`magnets_count == 0` 不展示保存按钮。

**精确匹配**：JavDB 搜索是模糊搜索，同一查询会返回相似番号。扫描刮削必须用 `codeid.Normalize` 后完整相等匹配，禁止直接取搜索第一项。

**刮削**：规范化番号 → 精确解析 JavDB ID → 获取详情 → 将强类型字段写库 → 入队封面任务。封面任务下载图片到本地缓存并裁剪，同时把不含剧情的 `.nfo`、`poster.jpg`、`fanart.jpg` 上传到 115 影片目录。不存在多源遍历、字段 merge 或换源功能。

**播放**：媒体库卡片与详情页共用 Vidstack 播放浮层；详情页仅在影片已入库时于预览图下方显示播放按钮。按番号读取当前媒体目录的全部关联视频，支持切换文件、原文件/115 转码及可用清晰度；切换同一文件的来源或清晰度保留播放位置，切换文件从头开始。关闭浮层立即卸载播放器、取消请求并释放播放会话。播放由用户主动发起，不受媒体图片的 NSFW 隐藏影响。

- 请求时调用 `pan.Client.PlayURL` 获取直链或 HLS；播放前校验当前账号、挂载目录与 115 文件实际路径，不使用其他来源的索引。地址仅保存在有时限的内存会话，不落库、不返回上游签名 URL。
- `/api/play/:id/stream/:resource` 转发会话内已登记资源，获取地址与流请求使用相同 User-Agent；支持 Range、If-Range、HEAD 与 206/416，视频流不整段缓冲、不持有令牌锁、不套用 API 的总超时。登录或目录变更后旧会话不可继续请求。
- HLS 播放列表中的分段、子列表、密钥和初始化片段地址都改写到同源代理；相对地址按 CDN 最终响应 URL 解析，不依赖 `.m3u8` 扩展名。hls.js 从本地依赖按需加载，不从 CDN 注入脚本。

**离线下载**：用户在影片详情选择 JavDB 返回的磁力 → service 核对来源并从 hash 构造 magnet URI → `pan.Client.AddOffline` → 写 Task（type 为 offline，payload.code 在写入时规范化，同时保留 javdb_id、hash）→ 轮询下载完成 → 同一事务创建定向扫描 → 创建或关联 Movie → 刮削、缓存图片并写回 NFO。核心流程不要求用户手动粘贴磁力或上传 torrent。

## 约定

### ent
- schema 手写在 `internal/ent/schema/`，生成代码提交进仓库，改 schema 后 `go generate ./...`。
- 开启 `sql/upsert` 特性，入库一律用 `OnConflict` 做 upsert，不先查后写。
- 迁移用 `client.Schema.Create` 自动迁移，不引入 Atlas。
- 多对多用 edge，不自建关联表。时间字段用 mixin 统一。
- 事务用 ent 官方 `WithTx` 助手，仅在跨表写入时使用。

### 错误与响应
- 成功响应直接返回数据，不套 `{code, msg, data}` 外壳，前端以 HTTP 状态码判断。
- 失败统一返回 `{"error": "message"}`。
- service 层用 `fmt.Errorf("...: %w", err)` 包上下文后返回；handler 只 `c.Error(err)`，不各自写响应。
- 一个 Gin 中间件统一映射：`ent.IsNotFound` → 404，绑定/校验错误 → 400，`pan.ErrUnauthorized` → 401，其余 500 并记日志。
- 只有错误与当前 HTTP 请求上下文都为 `context.Canceled` 时，才按客户端取消处理：标记 499、不写错误响应体、仅记 Debug。HTTP 请求仍有效时的上游取消和超时继续作为错误处理。
- 不定义业务错误码体系。
- 请求参数一律用 gin 绑定标签（`json`/`form`/`binding`）校验，不手写校验逻辑。

### 日志
- 标准库 `slog`，JSON 输出到 stdout，级别由配置控制。
- Gin 日志用 `github.com/samber/slog-gin` 中间件替换，保持单一格式。
- 普通请求错误由 slog-gin 统一记录，错误响应中间件不重复打印 Error；客户端取消的 499 不再打印一份访问错误日志。
- 循环内（扫描逐文件、轮询）只打 Debug，Info 只记任务级事件。
- 日志性能不是本项目瓶颈，不为此引入额外库。

### 其他
- 115 与图片请求走 Resty；JavDB 按上文使用隔离的 tls-client transport。115 与 JavDB 各自一个 `rate.Limiter`。
- ent schema 改动后执行 `go generate ./...`。
- 前端所有服务端数据通过 TanStack Query，不放 Zustand。
- NSFW 等普通显示偏好由 Zustand persist 持久化到当前浏览器 localStorage，不存 SQLite、不调用设置 API。服务端不可用时本地偏好仍可使用。
- 媒体图片的 NSFW 行为沿用 jm-boom：开启隐私模式时不渲染 `<img>`；隐藏、缺图和加载失败统一显示灰底 `ImageIcon` 占位，图片地址变化后重置失败状态。
- 媒体卡片不添加悬停阴影、缩放或背景变色。
- 设置页最上方为“外观”分组，主题切换沿用 jm-boom `feat/docker` 的 shadcn Tabs 图标按钮，支持跟随系统、浅色、深色。主题由 next-themes 持久化到 `miyabi-theme`，全局 ThemeProvider 负责应用到页面。
- 所有项目依赖的安装、升级和移除均由用户执行，包括 Go module、前端 npm/pnpm 包及 shadcn/ui 组件。助手先告知所需依赖、用途和具体命令，不自行运行依赖变更命令；可使用已安装的依赖进行构建、测试、lint 和格式化。依赖未就绪时继续完成不受影响的工作，并明确说明尚未完成的检查。
- 前端提交前跑 `oxlint` 与 `oxfmt`，Go 用 `gofmt` 与 `go vet`。
- 业务界面使用 `components/ui` 中的 shadcn 组件，不直接使用 Radix 原语拼装同类控件；缺少组件时先告知安装命令，由用户安装。
- 不使用 HTML 原生 `title` 悬停提示或图片 `alt` 提示文案；需要悬停说明的控件使用 shadcn Tooltip。
- 不额外编写无障碍属性、屏幕阅读器隐藏文案或专用关联代码；shadcn 组件内部的行为沿用库实现。
- 设置项标题与描述统一使用 `SettingRow` 的块级布局，标题不代理触发右侧控件。
- 根目录一份 `.gitignore`，不在 `web/` 单独放。`web/dist` 不入库，`make build` 先构建前端再编译 Go。
- API 路径前缀 `/api`，JSON 字段 snake_case。
- 测试重点放在 `codeid`、JavDB 签名 golden vector、startup 动态域名解密、线路选择、wire JSON 解码与字段映射。使用脱敏固定 JSON fixture，不提交真实 token、磁力 hash 或完整线上响应。
- JavDB 实网测试独立为手动或显式环境变量开启的 E2E，不进入默认单元测试；默认测试不得依赖外网。
- 不写 handler 集成测试。
- 不要写无用的兜底代码、防御性代码。

## 前端信息架构

- 顶级菜单为：**媒体库、发现、搜索、设置**，均通过 floating-nav 进入。
- 媒体库只展示 115 已存在并完成扫描的影片。
- 发现页由 JavDB 驱动，保留最新、即将发行、分类浏览三个 tab，各自管理页码；分类浏览独立保存分区和标签条件，不影响另外两个 tab。卡片同时显示本地/下载状态。
- 搜索为独立页面，参考 jm-boom 的搜索页布局，URL 只保存 `q` 与必要的 `page`，默认搜索全部分区。搜索状态不放入发现页的 store，也不继承分类、分区或排序。TOP250 需要 JavDB 登录，当前阶段不接入。
- 磁力选择与保存到 115 是发现页和详情页的动作，不单独增加顶级“下载”菜单。
- 任务进度通过全局入口或设置页中的任务面板展示，不增加顶级“任务中心”菜单。
- 设置页负责 115 登录与目录、JavDB 当前线路/重选、代理、限速和数据目录。

## 实施阶段

### Stage 1 骨架
- Go module、目录结构、gin 路由、koanf 配置、slog。
- ent schema 全部定义并生成，SQLite WAL 初始化。
- React 脚手架，embed 打通，`make dev` 前后端并行。
- 验收：启动后能打开空白首页并请求 `/api/health`。

### Stage 2 番号识别
- `codeid.Parse` 与 `Normalize`，覆盖前缀噪声、分集、字幕后缀、FC2、无横线格式。
- 支持欧美 `Site.YY.MM.DD` / `Site.YYYY.MM.DD` 编号，统一大小写但保留站点、完整日期段及点号，不截断成 `Site-YY`。没有站点前缀的日期不作为番号。
- 表驱动测试。
- 验收：测试通过。

### Stage 3 JavDB 协议与发现
- `internal/javdb`：签名 transport、强类型 envelope、稳定 device UUID、自动选线与线路缓存。
- 实现 Search、Browse、MovieDetail、Tags；用固定 JSON fixture 覆盖字段解码。
- 前端：发现页（最新、即将发行、分类浏览）、独立搜索页和空媒体库状态。
- 验收：不登录 115 也能浏览 JavDB；相似番号搜索只命中完整相等项；线路失败可手动重选。

### Stage 4 115 接入与扫描
- `pan.Client`：开放平台登录、令牌刷新、限速、列目录。
- `service/library` 扫描入库。
- 前端：设置页登录与网盘目录选择、触发扫描。
- 验收：指定目录扫描后影片库出现条目。

### Stage 5 任务系统
- `worker` 池与 Task 表，重启恢复 queued/running 任务。
- SSE 推送进度。
- 前端任务面板。
- 验收：扫描作为任务执行，前端实时看到进度。

### Stage 6 元数据、NFO 与封面
- JavDB 精确番号解析、详情字段映射、演员/标签关系、封面下载与裁剪。
- `nfo` 包读写，封面任务同时上传 `.nfo` 与图片到 115 影片目录。
- 扫描识别已有 `.nfo` 直接入库。
- 前端影片详情页与手动重新获取 JavDB 元数据；没有换源功能。
- 验收：扫描后影片自动获得除剧情外的元数据与海报，115 目录出现 NFO 与图片；删库重扫后无需访问 JavDB 即恢复。

### Stage 7 播放
- `pan/play.go`、`/api/play` 与流代理。
- 前端 Vidstack 共用浮层，原文件与 115 转码、多文件与清晰度选择。
- 验收：媒体库与已入库影片的详情页均可播放，关闭即停止并释放请求。

### Stage 8 JavDB 磁力与 115 离线下载
- `javdb/magnet.go`、`pan/offline.go`、`service/offline` 与轮询任务。
- 前端磁力选择：字幕、高清、大小、文件数筛选，直接保存到 115。
- 验收：在发现页选择磁力后自动完成 115 离线下载、扫描、元数据和封面全链路。

### Stage 9 完善
- 影片库筛选（演员、标签、系列、状态）、虚拟滚动。
- 演员页、标签页。
- JavDB 线路健康状态、缓存刷新与协议 E2E 检查。

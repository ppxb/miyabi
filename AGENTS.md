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
| 配置 | `github.com/knadh/koanf/v2` | 启动参数：监听地址、数据目录、日志级别、代理。文件 + 环境变量覆盖。运行时设置存 Setting 表 |
| 日志 | `log/slog` | 标准库 |
| 前端 | Vite 8 + React 19 + TypeScript | |
| 前端状态 | TanStack Query + Zustand | 服务端状态与 UI 状态分离 |
| 前端 UI | shadcn/ui + Tailwind CSS 4 | 组件可拷贝可改 |
| 播放器 | Artplayer + hls.js | m3u8 与直链都支持 |
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
- **115 影片目录**：元数据完成后写入 `<code>.nfo`、`poster.jpg`、`fanart.jpg`，与视频同目录。NFO 不写剧情。目录结构兼容 Emby/Jellyfin。
- **JavDB 缓存**：不做全量镜像。taxonomy、发现列表和详情只做有 TTL 的按需缓存；缓存失效不影响已有媒体库。
- **发现与媒体库分离**：发现结果不写入 Movie。只有 115 中实际出现视频文件并完成扫描后，才创建或关联 Movie。
- **重建**：扫描时若目录已有 `.nfo`，直接解析入库并下载图片到缓存，不再请求 JavDB。换机器或删库后重新扫描即可恢复。

## 数据模型

- **Movie**：`code`（唯一，规范化番号）、`javdb_id`（可空唯一）、`title`、`release_date`、`duration`（分钟）、`director_id/name`、`maker_id/name`、`series_id/name`、`rating`、`cover`、`poster`、`fanarts`（JSON）、`scrape_status`（pending/done/failed）。没有 `plot` 字段。
- **Actor**：`javdb_id` 唯一、`name`、`name_zht`、`gender`、`avatar`，与 Movie 多对多。JavDB 的演员数组包含男性演员，因此不使用 Actress 模型；UI 可默认只展示女性演员。年龄由生日计算，不持久化。
- **Tag**：`javdb_id` 唯一、`name`、`name_zht`、`category_id`，与 Movie 多对多。标签名称不作为唯一键。
- **File**：`file_id`（唯一）、`pick_code`、`sha1`、`name`、`size`、`parent_id`，可选关联 Movie。
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
- 当前需求全部可匿名访问，不实现 JavDB 登录、看过、想看、评论、TOP250、用户合集和以图搜番。

### 已验证字段边界

- 列表：`id`、`number`、`title`、`origin_title`、`release_date`、`duration`、`thumb_url`、`cover_url`、`preview_images`、`magnets_count`、字幕/预览标志。
- 详情：列表字段以及 `score`、演员、标签、系列、厂牌、导演、预览图、预览视频。
- 演员详情还可返回头像、繁中名、生日、出生地、身高、三围、罩杯、血型和社交账号；第一阶段只持久化媒体库和列表需要的字段，其余按需请求。
- taxonomy 实测有码区为 11 个分类、355 个标签；英文与繁中数据以标签 ID 对齐。
- 磁力：`hash`、`name`、`size`、`cnsub`、`hd`、`files_count`、`created_at`。提交 115 时自行从 hash 构造标准 magnet URI，不依赖第三方跳转 URL。
- `duration` 在 26 部跨年份、跨分区样本中覆盖 26/26，可作为稳定字段，单位为分钟。
- `summary` 在 26 部样本中繁中仅 8/26 非空，近期有码、无码和 FC2 样本均为 0，且出现与影片演员不一致的错误内容。项目明确丢弃该字段，不寻找其他剧情替代字段。

### 请求与语言

- 请求携带 App 版本、Android 设备参数、稳定 `device_uuid`、`jdsignature` 和 `Dart/3.4 (dart:io)` User-Agent。
- 签名常量、App 版本和设备 profile 集中定义，使用 golden vector 测试，不散落在业务代码。
- 影片详情默认使用 `zh-TW`。实测 `en` 的剧情均为空，`ja` 与 `zh-TW` 完全相同，视为回退而非独立日文数据，不实现 `ja` 模式。
- taxonomy 可分别获取 `en` 与 `zh-TW`，按 ID 合并，以支持中文展示和英文别名搜索。
- JavDB 是 Resty 约定的唯一例外：使用 `tls-client` 的 Chrome profile。不要自己实现 TLS 指纹，也不要为了形式统一强行套入 Resty。

### 自动线路

- 启动或首次请求时，优先用 `/api/v1/startup` 验证上次缓存 host；成功立即复用。
- 缓存 host 失败后并发探测固定 bootstrap，从合法 startup 响应中解密 `backup_domains_data.apiDomains`，再并发测速动态候选，选择成功请求中延迟最低者。
- 缓存稳定 `device_uuid` 和最终 host；选线探测不携带 token、不开业务重试。
- Miyabi 是长驻服务，不能照搬 CLI 的“一次命令一次选线”。当前线路发生连接失败、DNS、TLS、超时或 502/503/504 时，用 `singleflight` 触发一次重选，原 GET 请求最多重放一次，并原子替换共享 Client。
- 4xx、JavDB `success: 0` 业务错误、JSON 解码错误和字段不兼容不触发换线，应直接暴露错误以便发现协议变化。
- 设置页展示当前线路与最近延迟，并提供手动重新选线；不做固定周期的全线路探测。

## 关键流程

**扫描入库**：`pan.Client.List` 递归目录 → 目录内有 `.nfo` 则解析入库并入队封面缓存任务 → 否则逐文件 `codeid.Parse` → upsert File，命中番号则 upsert Movie 并关联 → 新影片入队刮削任务。

**发现**：`javdb.Client.Browse` 或 `Search` → 转换为发现 DTO → 按规范化番号查询本地 Movie 与进行中的 Task → 返回 `not_in_library`、`saving`、`in_library` 状态。发现列表允许包含未入库影片，不依赖 115 中已有内容。

**最新与即将发行**：JavDB 按 `release desc` 会返回未来日期。`release_date <= today` 才进入“最新已发行”，未来日期单独标记为“即将发行”；`magnets_count == 0` 不展示保存按钮。

**精确匹配**：JavDB 搜索是模糊搜索，同一查询会返回相似番号。扫描刮削必须用 `codeid.Normalize` 后完整相等匹配，禁止直接取搜索第一项。

**刮削**：规范化番号 → 精确解析 JavDB ID → 获取详情 → 将强类型字段写库 → 入队封面任务。封面任务下载图片到本地缓存并裁剪，同时把不含剧情的 `.nfo`、`poster.jpg`、`fanart.jpg` 上传到 115 影片目录。不存在多源遍历、字段 merge 或换源功能。

**播放**：请求时实时调用 `pan.Client.PlayURL` 取直链或 m3u8，不落库。直链需带 115 指定 User-Agent，由 `/api/play/:id/stream` 反向代理解决。

**离线下载**：用户在发现页或影片详情选择 JavDB 返回的磁力 → service 从 hash 构造 magnet URI → `pan.Client.AddOffline` → 写 Task（保留 code、javdb_id、hash）→ worker 定时轮询 → 完成后触发目录扫描与刮削。核心流程不要求用户手动粘贴磁力或上传 torrent。

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
- 不定义业务错误码体系。
- 请求参数一律用 gin 绑定标签（`json`/`form`/`binding`）校验，不手写校验逻辑。

### 日志
- 标准库 `slog`，JSON 输出到 stdout，级别由配置控制。
- Gin 日志用 `github.com/samber/slog-gin` 中间件替换，保持单一格式。
- 循环内（扫描逐文件、轮询）只打 Debug，Info 只记任务级事件。
- 日志性能不是本项目瓶颈，不为此引入额外库。

### 其他
- 115 与图片请求走 Resty；JavDB 按上文使用隔离的 tls-client transport。115 与 JavDB 各自一个 `rate.Limiter`。
- ent schema 改动后执行 `go generate ./...`。
- 前端所有服务端数据通过 TanStack Query，不放 Zustand。
- 需要新增 shadcn/ui 组件时，先告知用户所需组件及安装命令，由用户执行安装；不自行安装。
- 前端提交前跑 `oxlint` 与 `oxfmt`，Go 用 `gofmt` 与 `go vet`。
- 根目录一份 `.gitignore`，不在 `web/` 单独放。`web/dist` 不入库，`make build` 先构建前端再编译 Go。
- API 路径前缀 `/api`，JSON 字段 snake_case。
- 测试重点放在 `codeid`、JavDB 签名 golden vector、startup 动态域名解密、线路选择、wire JSON 解码与字段映射。使用脱敏固定 JSON fixture，不提交真实 token、磁力 hash 或完整线上响应。
- JavDB 实网测试独立为手动或显式环境变量开启的 E2E，不进入默认单元测试；默认测试不得依赖外网。
- 不写 handler 集成测试。
- 不要写无用的兜底代码、防御性代码。

## 前端信息架构

- 顶级菜单只保留：**媒体库、发现、设置**。
- 媒体库只展示 115 已存在并完成扫描的影片。
- 发现页由 JavDB 驱动，包含最新已发行、即将发行、分类筛选和搜索；卡片同时显示本地/下载状态。
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
- 表驱动测试。
- 验收：测试通过。

### Stage 3 JavDB 协议与发现
- `internal/javdb`：签名 transport、强类型 envelope、稳定 device UUID、自动选线与线路缓存。
- 实现 Search、Browse、MovieDetail、Tags；用固定 JSON fixture 覆盖字段解码。
- 前端发现页：最新已发行、即将发行、搜索、分类和空媒体库状态。
- 验收：不登录 115 也能浏览 JavDB；相似番号搜索只命中完整相等项；线路失败可手动重选。

### Stage 4 115 接入与扫描
- `pan.Client`：开放平台登录、令牌刷新、限速、列目录。
- `service/library` 扫描入库。
- 前端：设置页登录、网盘浏览页、触发扫描。
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
- 前端 Artplayer 集成。
- 验收：详情页可直接播放。

### Stage 8 JavDB 磁力与 115 离线下载
- `javdb/magnet.go`、`pan/offline.go`、`service/offline` 与轮询任务。
- 前端磁力选择：字幕、高清、大小、文件数筛选，直接保存到 115。
- 验收：在发现页选择磁力后自动完成 115 离线下载、扫描、元数据和封面全链路。

### Stage 9 完善
- 影片库筛选（演员、标签、系列、状态）、虚拟滚动。
- 演员页、标签页。
- JavDB 线路健康状态、缓存刷新与协议 E2E 检查。

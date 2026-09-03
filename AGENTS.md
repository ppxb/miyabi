# Miyabi

接入 115 网盘的 JAV 媒体库管理工具。单二进制部署，Go 后端嵌入 React 前端。

## 核心原则

- 单用户自用工具，不做多租户、不做权限系统。
- 代码简洁优先：不写防御性兜底，错误直接向上返回，由 API 层统一转成响应。
- 成熟库能用就用，不自己造轮子。
- 网盘目前只有 115，包名直接叫 `pan`，不带 115 前缀。
- 每层职责单一，禁止跨层调用（handler 不碰 ent，scraper 不碰数据库）。
- 项目依赖一律使用当前最新稳定版本，Go 依赖与前端依赖都如此。新增依赖前先查最新版，不凭记忆写版本号。

## 技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| HTTP | `github.com/gin-gonic/gin` | 参数绑定、校验、JSON 响应、文件上传、SSE 开箱即用 |
| ORM | `entgo.io/ent` | schema 即代码，查询类型安全，多对多关系清晰 |
| 数据库 | SQLite (`modernc.org/sqlite`) | 纯 Go 无 CGO，开 WAL 模式 |
| 任务队列 | 自维护 goroutine 池 + ent 任务表 | 单机足够，重启可恢复 |
| 实时推送 | SSE | 单向进度推送，比 WebSocket 简单 |
| HTTP 客户端 | `github.com/go-resty/resty/v2` | 重试、限速钩子、代理配置齐全 |
| HTML 解析 | `github.com/PuerkitoBio/goquery` | 刮削页面 |
| 图片处理 | `github.com/disintegration/imaging` | 海报裁剪、缩略图 |
| 限速 | `golang.org/x/time/rate` | 115 与刮削源都要限速 |
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
│   │   ├── task.go
│   │   ├── pan.go
│   │   ├── play.go
│   │   ├── setting.go
│   │   └── sse.go
│   ├── service/                业务编排，唯一允许同时调用 ent / pan / scraper 的层
│   │   ├── library.go          扫描入库、文件与影片关联
│   │   ├── scrape.go           刮削编排、多源合并、封面下载
│   │   ├── offline.go          磁力/种子提交、离线任务轮询
│   │   └── play.go             获取播放地址
│   ├── ent/                    ent 生成代码（schema/ 手写，generate.go 触发生成）
│   │   ├── generate.go
│   │   └── schema/
│   │       ├── mixin.go        created_at / updated_at
│   │       ├── movie.go
│   │       ├── actress.go
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
│   ├── nfo/                    Kodi 格式 .nfo 读写
│   ├── scraper/
│   │   ├── scraper.go          Scraper 接口与 Metadata 结构
│   │   ├── merge.go            多源字段合并
│   │   ├── javbus/
│   │   ├── javdb/
│   │   └── dmm/
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

115 是唯一数据源，本地只有索引和缓存，两者都可从 115 完整重建。

- **SQLite**：索引。影片、演员、文件关系、任务状态。
- **本地图片目录**：缓存。封面、海报、缩略图，用于列表页快速渲染。直接从 115 读图需要换取时效直链且受限速，不适合高频小图请求。
- **115 影片目录**：刮削完成后写入 `<code>.nfo`、`poster.jpg`、`fanart.jpg`，与视频同目录。目录结构兼容 Emby/Jellyfin。
- **重建**：扫描时若目录已有 `.nfo`，直接解析入库并下载图片到缓存，不再刮削。换机器或删库后重新扫描即可恢复。

## 数据模型

- **Movie**：`code`（唯一，规范化番号）、`title`、`release_date`、`duration`、`director`、`studio`、`series`、`rating`、`plot`、`cover`、`poster`、`fanarts`（JSON）、`scrape_status`（pending/done/failed）、`source`（各字段来源，JSON）。
- **Actress**、**Tag**：`name` 唯一，与 Movie 多对多。
- **File**：`file_id`（唯一）、`pick_code`、`sha1`、`name`、`size`、`parent_id`，可选关联 Movie。
- **Task**：`type`、`status`（queued/running/done/failed）、`payload`（JSON）、`progress`、`error`、`created_at`、`updated_at`。
- **Setting**：`key` 唯一、`value` JSON。

## 关键流程

**扫描入库**：`pan.Client.List` 递归目录 → 目录内有 `.nfo` 则解析入库并入队封面缓存任务 → 否则逐文件 `codeid.Parse` → upsert File，命中番号则 upsert Movie 并关联 → 新影片入队刮削任务。

**刮削**：按配置顺序遍历 Scraper，`Search` 命中即 `Fetch`；`merge` 按字段优先级合并；先写库，再入队封面任务。封面任务下载图片到本地缓存并裁剪，同时把 `.nfo`、`poster.jpg`、`fanart.jpg` 上传到 115 影片目录。

**播放**：请求时实时调用 `pan.Client.PlayURL` 取直链或 m3u8，不落库。直链需带 115 指定 User-Agent，由 `/api/play/:id/stream` 反向代理解决。

**离线下载**：接收磁力或 torrent → `pan.Client.AddOffline` → 写 Task → worker 定时轮询状态 → 完成后触发目录扫描任务。

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
- 外部请求统一走 `resty`，每个外部源一个 `rate.Limiter`。
- ent schema 改动后执行 `go generate ./...`。
- 前端所有服务端数据通过 TanStack Query，不放 Zustand。
- 前端提交前跑 `oxlint` 与 `oxfmt`，Go 用 `gofmt` 与 `go vet`。
- 根目录一份 `.gitignore`，不在 `web/` 单独放。`web/dist` 不入库，`make build` 先构建前端再编译 Go。
- API 路径前缀 `/api`，JSON 字段 snake_case。
- 测试重点放在 `codeid`、`scraper/merge`、各 scraper 的 HTML 解析（用固定 fixture），不写 handler 集成测试。
- 不要写无用的兜底代码、防御性代码。

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

### Stage 3 115 接入与扫描
- `pan.Client`：开放平台登录、令牌刷新、限速、列目录。
- `service/library` 扫描入库。
- 前端：设置页登录、网盘浏览页、触发扫描。
- 验收：指定目录扫描后影片库出现条目。

### Stage 4 任务系统
- `worker` 池与 Task 表，重启恢复 queued/running 任务。
- SSE 推送进度。
- 前端任务中心。
- 验收：扫描作为任务执行，前端实时看到进度。

### Stage 5 刮削与封面
- Scraper 接口、javbus 实现、merge、封面下载与裁剪。
- `nfo` 包读写，封面任务同时上传 `.nfo` 与图片到 115 影片目录。
- 扫描识别已有 `.nfo` 直接入库。
- 前端影片详情页、手动重刮、换源。
- 验收：扫描后影片自动获得元数据与海报，115 目录出现 nfo 与图片；删库重扫后无需刮削即恢复。

### Stage 6 播放
- `pan/play.go`、`/api/play` 与流代理。
- 前端 Artplayer 集成。
- 验收：详情页可直接播放。

### Stage 7 离线下载
- `pan/offline.go`、`service/offline`、轮询任务。
- 前端下载中心：批量磁力粘贴、torrent 上传、状态列表。
- 验收：粘贴磁力后自动下载、扫描、刮削全链路完成。

### Stage 8 完善
- javdb、dmm 刮削源。
- 影片库筛选（演员、标签、系列、状态）、虚拟滚动。
- 演员页、标签页。

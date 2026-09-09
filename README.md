# Miyabi

接入 115 网盘的媒体库管理工具，使用 JavDB 发现和获取元数据，支持扫描入库、离线下载和 HLS 播放。Go 后端嵌入 React 前端，以单个二进制或 Docker 容器运行。

## Docker 部署

镜像发布地址为 `ghcr.io/ppxb/miyabi`，支持 `linux/amd64` 和 `linux/arm64`。首次版本发布后可使用仓库中的 Compose 部署：

```bash
cp .env.example .env
# 编辑 .env，填写 MIYABI_ACCESS_PASSWORD 后再启动
docker compose pull
docker compose up -d
```

访问 <http://localhost:8080>。SQLite 索引、115 凭据和图片缓存保存在 `miyabi-data` 命名卷中，升级或重建容器会保留这些数据；视频文件仍保存在 115。

查看日志与升级：

```bash
docker compose logs -f miyabi
docker compose pull
docker compose up -d
```

从本地源码构建：

```bash
docker build -t ghcr.io/ppxb/miyabi:latest .
docker compose up -d
```

容器以 UID/GID `10001:10001` 运行；如将命名卷改为主机目录挂载，需要让该用户能够写入目录。健康检查使用 `/api/health`。

## 访问门禁与启动配置

门禁沿用 jm-boom 的轻量实现：通过 `MIYABI_ACCESS_PASSWORD` 配置访问密码，验证后使用 `sessionStorage` 在当前标签页内放行，刷新页面无需再次输入。浏览器只保存放行标记，不保存密码；登录后继续打开原先访问的页面。

这套门禁只控制 Web 入口，业务 API 仍可直接访问。未配置密码或将密码设为空时关闭门禁；Compose 部署要求先在 `.env` 中填写密码。

| 环境变量 | 用途 | 默认值 |
|---|---|---|
| `MIYABI_LISTEN` | HTTP 监听地址 | `:8080` |
| `MIYABI_DATA_DIR` | SQLite 与图片缓存目录 | 二进制为 `./data`，容器为 `/app/data` |
| `MIYABI_LOG_LEVEL` | 日志级别 | `info` |
| `MIYABI_PROXY` | 访问上游服务的 HTTP 代理 | 空 |
| `MIYABI_ACCESS_PASSWORD` | Web 入口密码 | 空，关闭门禁 |

也可在本地 `config.toml` 中设置 `listen`、`data_dir`、`log_level`、`proxy` 和 `access_password`。环境变量覆盖配置文件；监听地址、数据目录、日志级别和代理还支持命令行覆盖。密码只通过配置文件或环境变量提供。

## 开发与二进制构建

使用 Go 1.27、Node.js 24 和 pnpm 10。由开发者安装依赖后运行：

```bash
pnpm --dir web install --frozen-lockfile
go mod download
make dev
```

开发页面位于 <http://localhost:5173>，Vite 将 `/api` 转发到 `8080` 端口。`make build` 构建前端并生成包含前端资源的 `bin/miyabi`。

两个 embed 文件按构建标签互斥使用：`embed.go` 在发布构建中嵌入 `web/dist`，`embed_dev.go` 在 `-tags=dev` 时关闭静态前端托管，因此开发启动不依赖已有的前端构建产物。

## Docker 版本发布

`.github/workflows/docker-publish.yml` 沿用 jm-boom 的发布流程。推送 `v1.2.3` 形式的 Git tag 后，工作流会：

1. 校验版本并通过 git-cliff 生成变更日志，创建 GitHub Release 草稿。
2. 在 AMD64、ARM64 runner 上构建镜像并写入各自的构建缓存。
3. 发布双架构镜像到 GHCR，生成版本、主次版本、主版本、`latest` 和 `sha-*` 标签。
4. 镜像发布成功后公开 GitHub Release。

工作流使用仓库的 `GITHUB_TOKEN`，发布镜像的 job 需要 `packages: write`，创建和公开 Release 的 job 需要 `contents: write`。首次发布后可在 GitHub Packages 中设置镜像可见性。

## NSFW 警告

本软件可能存在裸露、暴力、色情或冒犯等不适宜公众场合的内容，请勿在公共场合使用本软件，避免不必要的纷争。

## 特别鸣谢

感谢 [莫愁](https://github.com/zk020106) 的 token，感谢 [老罗](https://github.com/luoqiz) 的115账号。

## 致谢

本项目参考了以下项目的部分实现，在此表示衷心的感谢！

- [javdb-cli](https://github.com/FlanChanXwO/javdb-cli)
- [jm-boom](https://github.com/ppxb/jm-boom)

同时感谢社区 [LinuxDO](https://linux.do) 的帮助。

## 免责声明

本项目仅供学习、研究和技术交流使用。项目作者与任何第三方服务、原始应用或内容提供方无关。
使用者应自行遵守当地法律法规以及相关服务条款。因使用本项目产生的任何法律、版权、账号、数据或财务风险均由使用者自行承担。

## License

遵循 [MIT](./LICENSE) 协议。

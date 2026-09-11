# Miyabi

接入 115 网盘和 JavDB 的媒体库管理工具。

## Docker 部署

公开镜像：

```text
ghcr.io/ppxb/miyabi
```

直接使用 Docker 运行（请替换访问密码）：

```bash
docker run -d \
  --name miyabi \
  --restart unless-stopped \
  --security-opt no-new-privileges:true \
  -p 8080:8080 \
  -v miyabi-data:/app/data \
  -e MIYABI_ACCESS_PASSWORD='change-this-password' \
  ghcr.io/ppxb/miyabi:latest
```

启动后访问 `http://<服务器IP>:8080`。

查看日志：

```bash
docker logs -f miyabi
```

升级时先拉取新镜像，再停止并移除旧容器：

```bash
docker pull ghcr.io/ppxb/miyabi:latest
docker stop miyabi
docker rm miyabi
```

然后重新执行上方的 `docker run` 命令，沿用原访问密码和 `miyabi-data` 数据卷。

容器以 UID/GID `10001:10001` 运行；如将命名卷改为主机目录挂载，需要让该用户能够写入目录。

健康检查通过 `/app/miyabi healthcheck` 访问容器内的 `/api/health`，与应用读取相同的 `MIYABI_LISTEN` 环境变量，不受宿主机端口映射影响。修改对外访问端口只需调整 `-p` 的左侧端口。

## 启动配置

启动配置统一使用环境变量，未设置时采用默认值。监听地址、数据目录和日志级别显式设为空时会报错。

通过 `MIYABI_ACCESS_PASSWORD` 配置访问密码。未配置密码或将密码设为空时关闭门禁。**强烈建议您开启门禁**。

| 环境变量                 | 用途                     | 默认值                                |
| ------------------------ | ------------------------ | ------------------------------------- |
| `MIYABI_LISTEN`          | HTTP 监听地址            | `:8080`                               |
| `MIYABI_DATA_DIR`        | SQLite 与图片缓存目录    | 二进制为 `./data`，容器为 `/app/data` |
| `MIYABI_LOG_LEVEL`       | 日志级别                 | `info`                                |
| `MIYABI_PROXY`           | 访问上游服务的 HTTP 代理 | 空                                    |
| `MIYABI_ACCESS_PASSWORD` | Web 入口密码             | 空，关闭门禁                          |

旧的 `config.toml` 和应用启动参数配置需要迁移到对应环境变量。程序保留 `healthcheck` 子命令，不再提供配置参数或读取 TOML 文件。

挂载或更换媒体目录后，系统会自动扫描。小于 100 MiB（104857600 字节）的视频视为辅助文件，不参与影片识别和自动刮削，也不会因目录内存在 NFO 而关联到影片。该阈值固定，无需配置；重新扫描可清理已有索引中的误关联。

## 本地开发

前端使用 Node.js 24 或更新版本、pnpm 10.33.0，后端使用 Go 1.27。

前后端分别启动。在项目根目录运行后端，也可以在 GoLand 中运行，工作目录设为项目根目录：

```bash
go run -tags=dev ./cmd/miyabi
```

另开终端，在 `web` 目录安装依赖并启动前端：

```bash
pnpm install --frozen-lockfile
pnpm dev
```

默认访问 `http://127.0.0.1:5173`，前端将 API 请求转发到 `127.0.0.1:8080`。后端数据保存在项目根目录的 `data` 中；修改 Go 代码后重新运行后端。

在 `web` 目录运行 `pnpm build` 时，Vite 会自动生成路由并构建页面，随后执行 TypeScript 检查。`pnpm lint` 检查代码及 Hooks 用法，`pnpm test` 执行前端回归测试，`pnpm fmt:check` 检查代码格式。

## NSFW 警告

本软件可能存在裸露、暴力、色情或冒犯等不适宜公众场合的内容，请勿在公共场合使用本软件，避免不必要的纷争。

## 特别鸣谢

感谢 [莫愁](https://github.com/zk020106) 的 token，感谢 [老罗](https://github.com/luoqiz) 的115账号。

## 致谢

本项目参考了以下项目的部分实现，在此表示衷心的感谢！

- [javdb-cli](https://github.com/FlanChanXwO/javdb-cli)

同时感谢社区 [LinuxDO](https://linux.do) 的帮助。

## 免责声明

本项目仅供学习、研究和技术交流使用。项目作者与任何第三方服务、原始应用或内容提供方无关。
使用者应自行遵守当地法律法规以及相关服务条款。因使用本项目产生的任何法律、版权、账号、数据或财务风险均由使用者自行承担。

## License

遵循 [MIT](./LICENSE) 协议。

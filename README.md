# Miyabi

接入 115 网盘和 JavDB 的媒体库管理工具。

## Docker 部署

镜像发布地址为 `ghcr.io/ppxb/miyabi`，支持 `linux/amd64` 和 `linux/arm64`。

```bash
cp .env.example .env
# 编辑 .env，填写 MIYABI_ACCESS_PASSWORD 后再启动
docker compose pull
docker compose up -d
```

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

通过 `MIYABI_ACCESS_PASSWORD` 配置访问密码，验证后使用 `sessionStorage` 在当前标签页内放行，刷新页面无需再次输入。浏览器只保存放行标记，不保存密码；登录后继续打开原先访问的页面。

未配置密码或将密码设为空时关闭门禁；Compose 部署要求先在 `.env` 中填写密码。

| 环境变量                 | 用途                     | 默认值                                |
| ------------------------ | ------------------------ | ------------------------------------- |
| `MIYABI_LISTEN`          | HTTP 监听地址            | `:8080`                               |
| `MIYABI_DATA_DIR`        | SQLite 与图片缓存目录    | 二进制为 `./data`，容器为 `/app/data` |
| `MIYABI_LOG_LEVEL`       | 日志级别                 | `info`                                |
| `MIYABI_PROXY`           | 访问上游服务的 HTTP 代理 | 空                                    |
| `MIYABI_ACCESS_PASSWORD` | Web 入口密码             | 空，关闭门禁                          |

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

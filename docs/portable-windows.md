# Windows 便携版

便携版把 Miyabi 打包成一个自带的 `miyabi.exe`：不需要 Docker、不需要 Node、不需要安装任何运行库，解压后双击即可使用，数据保存在 exe 同级目录，整个目录可以拷贝到 U 盘或另一台电脑。

## 快速开始

1. 解压 `miyabi-<版本>-windows-<架构>.zip` 到一个**可写**目录，例如 `D:\miyabi`。
   - 不要放在 `C:\Program Files` 等受保护目录：那里无法写入数据库与日志。
2. 双击 `miyabi.exe`。程序在后台启动，并自动用默认浏览器打开 `http://127.0.0.1:8080`。
   - 首次运行 Windows 会弹出防火墙授权提示。选择「允许访问」后，局域网内的电视、手机、Emby 才能访问；只在自己电脑上用可以选择「取消」。
3. 在「设置」页登录 115 账号、挂载媒体目录，或添加本地媒体目录后开始扫描。

解压后目录里有两个程序，功能完全相同，只是控制台窗口的有无：

| 文件 | 用途 |
| --- | --- |
| `miyabi.exe` | 双击启动，不显示控制台窗口；日志写入 `data\miyabi.log` |
| `miyabi-console.exe` | 保留控制台窗口，方便查看启动报错；按 `Ctrl+C` 退出 |

## 目录结构

首次运行后，exe 同级目录会变成：

```
D:\miyabi\
├── miyabi.exe           # 主程序（无窗口）
├── miyabi-console.exe   # 带控制台的诊断版本
├── miyabi.env           # 首次运行自动生成的配置，可编辑
└── data\                # 数据目录，可整体备份或迁移
    ├── miyabi.db        # SQLite：媒体库索引、设置、任务
    ├── miyabi.log       # 日志（超过 8MB 时轮转为 miyabi.log.1）
    ├── images\          # 封面与缩略图缓存
    └── emby\            # 导出的 nfo / 海报 / .strm
```

`data` 目录默认跟随 `miyabi.exe`：整个文件夹复制到别处后，路径不需要任何修改。

## 配置文件 `miyabi.env`

首次运行时会自动生成，可用记事本直接编辑：

- 优先级：**环境变量 > `miyabi.env` > 内置默认值**；留空或删除某行表示使用默认值。
- 修改后需要重新启动 `miyabi.exe`。
- 常用项：`MIYABI_LISTEN`（监听地址）、`MIYABI_DATA_DIR`（数据目录）、`MIYABI_ACCESS_PASSWORD`（访问密码）、`MIYABI_PUBLIC_URL`（对外地址）、`MIYABI_EMBY_*`（Emby 集成）。
- 文件里已列出全部可配置项和说明，与容器版的环境变量同名。

`miyabi.env` 存在时，程序固定按便携模式启动；如果它不存在且进程带有任何 `MIYABI_*` 环境变量，则按环境变量方式启动（数据目录为当前工作目录下的 `./data`）。删除 `miyabi.env` 即可回到环境变量方式。

## 局域网访问

便携版默认监听 `:8080`，即所有网卡，局域网内其它设备可以直接打开 `http://<本机IP>:8080`。

- 首次运行时请在 Windows 防火墙提示中允许访问，否则只有本机能连。
- 暴露到局域网时建议在 `miyabi.env` 中设置 `MIYABI_ACCESS_PASSWORD`（Web 访问密码）与 `MIYABI_STRM_TOKEN`（播放器免登录令牌）。
- 只想本机使用：把 `MIYABI_LISTEN` 改为 `127.0.0.1:8080`。

### Emby / 播放器

- Miyabi 导出 `.strm` 时使用 `MIYABI_PUBLIC_URL`，留空则自动使用本机局域网 IP:端口。
- Emby 或播放器与 Miyabi 在**同一台电脑**上时，建议把 `MIYABI_PUBLIC_URL` 设为 `http://127.0.0.1:8080`。
- 在**其它设备**上播放时保持自动的局域网地址，并确保防火墙已放行、`MIYABI_STRM_TOKEN` 已设置。

## 日志与排查

- 日志：`data\miyabi.log`（JSON，每行一条），超过 8MB 轮转为 `miyabi.log.1`。
- 启动失败、端口被占用等问题都会写在这里；需要实时观察时改用 `miyabi-console.exe`。
- 双击 `miyabi.exe` 时如果程序已经在运行，新进程会直接打开浏览器，不会启动第二个实例。

## 退出与卸载

- 退出：在任务管理器中结束 `miyabi.exe`（无窗口版本没有其它入口）；使用 `miyabi-console.exe` 时按 `Ctrl+C` 退出。
- 卸载：删除整个目录即可，不写注册表、不安装服务。

## 迁移与备份

- 备份：直接复制 `data` 目录（或整个 exe 目录）。
- 迁移：把整个目录拷到新电脑/新盘符，再双击 `miyabi.exe`。若 `miyabi.env` 中手工填写过绝对路径（例如 `MIYABI_DATA_DIR`），需要一并修改。
- 115 登录状态保存在 `data\miyabi.db`，迁移后通常无需重新登录。

## 从源码构建

### 脚本

```powershell
./scripts/build-portable.ps1 -Version 0.2.0 -Arch amd64   # 也可以 -Arch arm64
```

脚本会先构建前端（需要 Node 与 pnpm），再输出 `dist/miyabi-<版本>-windows-<架构>.zip`。前端已经构建过时可以加 `-SkipFrontend`。

### 手动命令

```powershell
cd web; pnpm install --frozen-lockfile; pnpm build; cd ..
$env:CGO_ENABLED = "0"; $env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w -H=windowsgui" -o miyabi.exe ./cmd/miyabi
```

`-H=windowsgui` 生成无控制台窗口的程序；去掉它即可得到带控制台的版本。

### 发布

推送 `v*.*.*` 标签时，GitHub Actions 会自动构建 `windows/amd64` 与 `windows/arm64` 两个 zip 并附加到对应的 Release 草稿上。

## 与 Docker 部署的差异

| | 便携版 | Docker |
| --- | --- | --- |
| 配置来源 | `miyabi.env`（也可用环境变量） | 环境变量 / compose |
| 数据目录 | exe 同级 `data\` | 容器内 `/app/data`（挂载卷） |
| 默认监听 | `:8080` | `:8080` |
| 健康检查 | `miyabi.exe healthcheck` | 镜像内置 HEALTHCHECK |
| 启动方式 | 双击，浏览器自动打开 | `docker run` / compose |

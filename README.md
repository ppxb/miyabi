# Miyabi

Miyabi 是一个接入 115 网盘的单用户 JAV 媒体库管理工具。Go 后端嵌入 React 构建产物，可以单二进制文件部署。

## 开发

Windows PowerShell：

```powershell
.\scripts\bootstrap.ps1
.\scripts\dev.ps1
```

安装 GNU Make 的环境也可以使用：

```shell
make install
make generate
make dev
```

开发服务默认为：

- React：`http://127.0.0.1:5173`
- Go API：`http://127.0.0.1:8080/api/health`

## 配置

默认读取根目录的 `config.toml`，可复制 `config.example.toml` 作为起点。覆盖顺序为：默认值、TOML 文件、`MIYABI_` 环境变量、命令行参数。

```text
--config
--listen
--data-dir
--log-level
--proxy
```

## 构建与检查

```powershell
.\scripts\build.ps1
.\scripts\check.ps1
```

package config

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// portableFileName is the optional configuration file a Windows build reads
// next to its executable.
const portableFileName = "miyabi.env"

// Portable reports whether the process should start the way a double-clicked
// Windows build does: the configuration file next to the executable already
// exists, or the process inherits no MIYABI_* variable at all. An explicit
// environment always wins, and other systems keep the documented defaults.
func Portable(goos, exeDir string, environ []string) bool {
	if goos != "windows" || exeDir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(exeDir, portableFileName)); err == nil {
		return true
	}
	for _, entry := range environ {
		if strings.HasPrefix(entry, "MIYABI_") {
			return false
		}
	}
	return true
}

// ExecutableDir returns the directory holding the running executable, where a
// portable build keeps its configuration and its data.
func ExecutableDir() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	// Windows short paths and launch-from-share spellings must resolve to one
	// real directory, because the data and configuration paths derive from it.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Dir(path), nil
}

// LoadPortable resolves the configuration of a double-clicked Windows build:
// environment variables first, then <exeDir>/miyabi.env, then portable defaults.
// The file is generated on first run so it can be edited instead of the
// environment.
func LoadPortable(exeDir string) (Config, error) {
	file := portableValues(exeDir)
	return resolve(file, defaults{Listen: defaultListen, DataDir: filepath.Join(exeDir, "data")})
}

// portableValues reads the portable configuration file, generating it on first
// run. A file that cannot be created is not fatal: the portable defaults in code
// keep the build usable, so only a hint is logged.
func portableValues(exeDir string) map[string]string {
	path := filepath.Join(exeDir, portableFileName)
	body, err := os.ReadFile(path)
	if err == nil {
		return parseEnvFile(body)
	}
	if !os.IsNotExist(err) {
		slog.Warn("portable configuration is unreadable; using built-in defaults", "path", path, "error", err)
		return nil
	}
	body = []byte(portableFile(exeDir))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		slog.Warn("portable configuration cannot be created; using built-in defaults", "path", path, "error", err)
		return nil
	}
	slog.Info("created portable configuration", "path", path)
	return parseEnvFile(body)
}

// parseEnvFile reads KEY=VALUE lines, ignoring blanks, comments and empty
// values. Windows paths survive verbatim: no escape sequences are interpreted.
func parseEnvFile(body []byte) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(normalizeEnvFile(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) > 1 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if key == "" || value == "" {
			continue
		}
		values[key] = value
	}
	return values
}

// normalizeEnvFile decodes what Windows editors write: UTF-8 with a byte order
// mark, or UTF-16 in either byte order. Without this, a file saved by Notepad
// would be parsed as one broken key and every setting in it silently ignored.
func normalizeEnvFile(body []byte) string {
	switch {
	case bytes.HasPrefix(body, utf8BOM):
		return string(body[len(utf8BOM):])
	case bytes.HasPrefix(body, []byte{0xFF, 0xFE}):
		return decodeUTF16(body[2:], binary.LittleEndian)
	case bytes.HasPrefix(body, []byte{0xFE, 0xFF}):
		return decodeUTF16(body[2:], binary.BigEndian)
	default:
		return string(body)
	}
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func decodeUTF16(body []byte, order binary.ByteOrder) string {
	units := make([]uint16, 0, len(body)/2)
	for index := 0; index+1 < len(body); index += 2 {
		units = append(units, order.Uint16(body[index:]))
	}
	return string(utf16.Decode(units))
}

// portableFile renders the first-run configuration file. Paths that follow the
// executable stay commented out, so the whole directory can be moved.
func portableFile(exeDir string) string {
	dataDir := filepath.Join(exeDir, "data")
	return fmt.Sprintf(`# Miyabi 便携版配置（首次运行自动生成，可直接编辑）
#
# 优先级：环境变量 > 本文件 > 内置默认值；留空或删除某行表示使用默认值。
# 修改后需要重新启动 miyabi.exe。
#
# 监听地址。默认监听所有网卡，局域网内其他设备可以访问；
# 首次运行 Windows 会询问防火墙权限。只允许本机访问可改为 127.0.0.1:8080。
MIYABI_LISTEN=:8080
# 日志级别：debug / info / warn / error。日志同时写入数据目录下的 miyabi.log。
MIYABI_LOG_LEVEL=info
#
# 数据目录（SQLite、图片缓存与日志）。默认跟随 miyabi.exe 移动，当前为：
#   %[1]s
# 注意：放在 C:\Program Files 等受保护目录时可能没有写入权限。
# MIYABI_DATA_DIR=%[1]s
#
# 对外服务地址，用于生成 .strm 播放文件与图片链接；默认自动使用本机局域网 IP:端口。
# 播放器与 Miyabi 在同一台电脑时填 http://127.0.0.1:8080。
# MIYABI_PUBLIC_URL=
#
# STRM 播放令牌，供 Emby 等播放器免登录访问 /api/strm/play。暴露到局域网时建议设置。
# MIYABI_STRM_TOKEN=
#
# Web 访问密码；留空表示不需要登录。暴露到局域网时强烈建议设置。
# MIYABI_ACCESS_PASSWORD=
#
# 登录令牌的签名密钥；留空时由访问密码派生（修改密码会使已登录会话失效）。
# MIYABI_JWT_SECRET=
#
# Emby 集成：填写服务器地址与 API Key 后会自动启用。
# MIYABI_EMBY_ENABLED=
# MIYABI_EMBY_SERVER_URL=http://192.168.1.100:8096
# MIYABI_EMBY_API_KEY=
# MIYABI_EMBY_MEDIA_PATH=/media
# MIYABI_EMBY_SYNC_ACTORS=true
#
# 反向代理信任列表，逗号分隔；只有前面部署了反向代理时才需要设置。
# MIYABI_TRUSTED_PROXIES=
`, dataDir)
}

# Miyabi - MPV playback bridge uninstaller (removes the miyabi-mpv:// protocol)
$ErrorActionPreference = 'SilentlyContinue'

$base = 'HKCU:\Software\Classes\miyabi-mpv'
if (Test-Path $base) { Remove-Item -Path $base -Recurse -Force }

$wrapperPath = Join-Path (Join-Path $env:LOCALAPPDATA 'Miyabi') 'mpv-open.ps1'
if (Test-Path $wrapperPath) { Remove-Item -Path $wrapperPath -Force }

Write-Host 'Miyabi MPV bridge removed.' -ForegroundColor Green

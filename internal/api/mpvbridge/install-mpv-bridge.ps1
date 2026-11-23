# Miyabi - MPV playback bridge installer (Windows, per-user, reversible)
# Registers the "miyabi-mpv://" URL protocol so the web UI's
# "Play with MPV" can launch the LOCAL mpv and attach subtitles automatically.
$ErrorActionPreference = 'Stop'

function Find-Mpv {
  $cmd = Get-Command mpv -ErrorAction SilentlyContinue
  if ($cmd -and $cmd.Source) { return $cmd.Source }
  foreach ($key in @(
    'HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\mpv.exe',
    'HKLM:\Software\Microsoft\Windows\CurrentVersion\App Paths\mpv.exe'
  )) {
    if (Test-Path $key) {
      $p = (Get-ItemProperty -Path $key -ErrorAction SilentlyContinue).'(default)'
      if ($p) { $p = $p.Trim('"'); if (Test-Path $p) { return $p } }
    }
  }
  foreach ($p in @(
    "$env:ProgramFiles\mpv\mpv.exe",
    "${env:ProgramFiles(x86)}\mpv\mpv.exe",
    "$env:LOCALAPPDATA\Programs\mpv\mpv.exe",
    "$env:USERPROFILE\scoop\shims\mpv.exe"
  )) { if (Test-Path $p) { return $p } }
  return $null
}

$mpv = Find-Mpv
if (-not $mpv) {
  $mpv = Read-Host 'mpv.exe not found. Enter full path (e.g. C:\Program Files\mpv\mpv.exe)'
  $mpv = $mpv.Trim('"')
}
if (-not (Test-Path $mpv)) {
  Write-Host "mpv.exe not found: $mpv" -ForegroundColor Red
  exit 1
}

$targetDir = Join-Path $env:LOCALAPPDATA 'Miyabi'
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
$wrapperPath = Join-Path $targetDir 'mpv-open.ps1'

$wrapper = @'
param([string]$Uri)
$ua = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36'
if ($Uri -notmatch '^miyabi-mpv://') { exit }
$qidx = $Uri.IndexOf('?')
if ($qidx -lt 0) { exit }
$query = $Uri.Substring($qidx + 1)
$p = @{}
foreach ($pair in $query.Split('&')) {
  $kv = $pair.Split('=', 2)
  if ($kv[0]) { $p[$kv[0]] = [Uri]::UnescapeDataString($kv[1]) }
}
if ($p['u'] -notmatch '^https?://') { exit }
$argList = '"' + $p['u'] + '" --user-agent="' + $ua + '"'
if ($p['s'] -match '^https?://') { $argList += ' --sub-file="' + $p['s'] + '"' }
Start-Process -FilePath '__MPV__' -ArgumentList $argList
'@
$wrapper = $wrapper.Replace('__MPV__', $mpv)
Set-Content -Path $wrapperPath -Value $wrapper -Encoding UTF8

$base = 'HKCU:\Software\Classes\miyabi-mpv'
New-Item -Path $base -Force | Out-Null
Set-ItemProperty -Path $base -Name '(default)' -Value 'URL:miyabi-mpv'
New-ItemProperty -Path $base -Name 'URL Protocol' -Value '' -PropertyType String -Force | Out-Null
$commandKey = Join-Path $base 'shell\open\command'
New-Item -Path $commandKey -Force | Out-Null
$command = 'powershell -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "' + $wrapperPath + '" "%1"'
Set-ItemProperty -Path $commandKey -Name '(default)' -Value $command

Write-Host ''
Write-Host 'Miyabi MPV bridge installed.' -ForegroundColor Green
Write-Host "  mpv:     $mpv"
Write-Host "  wrapper: $wrapperPath"
Write-Host '  scheme:  miyabi-mpv://'
Write-Host 'Now use "Play with MPV" from the movie card context menu.'

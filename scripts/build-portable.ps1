# Builds the Windows portable package: a window-less miyabi.exe for double-click
# use plus a console twin for diagnostics, packaged with the licence and guide.
[CmdletBinding()]
param(
    [string] $Version = 'dev',
    [ValidateSet('amd64', 'arm64')] [string] $Arch = 'amd64',
    [switch] $SkipFrontend
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$package = "miyabi-$Version-windows-$Arch"
$stage = Join-Path $root "dist/$package"
$zip = Join-Path $root "dist/$package.zip"

Push-Location $root
try {
    if (-not $SkipFrontend) {
        Write-Host 'Building the web frontend...'
        Push-Location web
        try {
            pnpm install --frozen-lockfile
            pnpm build
        } finally {
            Pop-Location
        }
    }
    if (-not (Test-Path 'web/dist/index.html')) {
        throw 'web/dist is missing; build the frontend first or drop -SkipFrontend.'
    }

    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Force -Path $stage | Out-Null

    # modernc.org/sqlite is pure Go, so the portable build needs no C toolchain
    # and no runtime redistributables.
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = $Arch

    Write-Host "Building $package..."
    # The window-less build is what a double-click launches; without a console
    # the logs land in <data>/miyabi.log.
    go build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $stage 'miyabi.exe') ./cmd/miyabi
    go build -trimpath -ldflags '-s -w' -o (Join-Path $stage 'miyabi-console.exe') ./cmd/miyabi

    Copy-Item LICENSE $stage
    Copy-Item docs/portable-windows.md (Join-Path $stage '使用说明.md')

    if (Test-Path $zip) { Remove-Item -Force $zip }
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
    Write-Host "Created $zip"
} finally {
    Pop-Location
}

<#
.SYNOPSIS
    Builds kisaf for Windows.

.DESCRIPTION
    The -H=windowsgui linker flag stops a black console window from flashing
    up at startup. Because of that, diagnostics go to
    %APPDATA%\kisaf\kisaf.log rather than to the screen.

.PARAMETER Version
    Version label. Shown by "kisaf.exe --version" and in the interface.

.PARAMETER Console
    Build a variant that keeps its console window (handy when debugging).

.EXAMPLE
    .\scripts\build.ps1
    .\scripts\build.ps1 -Version 1.0.0
    .\scripts\build.ps1 -Console
#>

[CmdletBinding()]
param(
    [string]$Version = 'dev',
    [switch]$Console
)

$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')

try {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host 'Go is not installed: https://go.dev/dl/' -ForegroundColor Red
        exit 1
    }

    # -s -w strips debug symbols and cuts the file size by roughly a third.
    $ldflags = "-s -w -X main.version=$Version"
    $output  = 'kisaf.exe'

    if ($Console) {
        $output = 'kisaf-console.exe'
        Write-Host "Building the console variant ($Version)..." -ForegroundColor Cyan
    } else {
        $ldflags = "$ldflags -H=windowsgui"
        Write-Host "Building ($Version)..." -ForegroundColor Cyan
    }

    $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags $ldflags -o $output .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $size = [math]::Round((Get-Item $output).Length / 1MB, 1)
    Write-Host "Ready: $output ($size MB)" -ForegroundColor Green
    Write-Host "To install: .\scripts\install-windows.ps1"
} finally {
    Pop-Location
}

<#
.SYNOPSIS
    Installs kisaf on this computer.

.DESCRIPTION
    What it does:
      1. copies kisaf.exe into %LOCALAPPDATA%\kisaf
      2. creates Desktop and Start Menu shortcuts
      3. (optional) starts kisaf when Windows starts
      4. (optional, needs admin) adds the name "kisaf" to the hosts file
      5. (optional, needs admin) opens the port in the firewall

    None of this is required: you can simply double-click kisaf.exe instead.

.PARAMETER Port
    Port for the server. Defaults to 80, which is what lets you type just
    "kisaf" in the address bar with no port suffix.

.PARAMETER NoAutoStart
    Skip starting kisaf automatically when Windows starts.

.PARAMETER AllowNetwork
    Open the port in the firewall so phones and other machines on the LAN can
    reach it. Requires running as administrator.

.PARAMETER Uninstall
    Undo the installation. Your project list (%APPDATA%\kisaf) is kept.

.EXAMPLE
    .\install-windows.ps1
    .\install-windows.ps1 -Port 7777 -NoAutoStart
    .\install-windows.ps1 -Uninstall
#>

[CmdletBinding()]
param(
    [int]$Port = 80,
    [switch]$NoAutoStart,
    [switch]$AllowNetwork,
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'

$AppName    = 'kisaf'
$InstallDir = Join-Path $env:LOCALAPPDATA $AppName
$ExePath    = Join-Path $InstallDir 'kisaf.exe'
$DataDir    = Join-Path $env:APPDATA $AppName
$StartupDir = [Environment]::GetFolderPath('Startup')
$DesktopDir = [Environment]::GetFolderPath('Desktop')
$StartMenu  = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Programs'

function Write-Step($message) { Write-Host "  $message" -ForegroundColor Cyan }
function Write-Ok($message)   { Write-Host "  [ok] $message" -ForegroundColor Green }
function Write-Skip($message) { Write-Host "  [skipped] $message" -ForegroundColor DarkYellow }

function Test-Admin {
    $identity  = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function New-Shortcut {
    param([string]$Path, [string]$Target, [string]$Description)

    $shell = New-Object -ComObject WScript.Shell
    $link  = $shell.CreateShortcut($Path)
    $link.TargetPath       = $Target
    $link.WorkingDirectory = Split-Path $Target -Parent
    $link.IconLocation     = "$Target,0"
    $link.Description      = $Description
    $link.Save()
}

# ---------------------------------------------------------------- uninstall

if ($Uninstall) {
    Write-Host "`nUninstalling kisaf...`n" -ForegroundColor White

    Get-Process -Name 'kisaf' -ErrorAction SilentlyContinue | ForEach-Object {
        $_.Kill(); Write-Ok 'stopped the running copy'
    }

    foreach ($shortcut in @(
        (Join-Path $StartupDir "$AppName.lnk"),
        (Join-Path $DesktopDir 'kisaf.lnk'),
        (Join-Path $StartMenu  'kisaf.lnk')
    )) {
        if (Test-Path $shortcut) { Remove-Item $shortcut -Force; Write-Ok "removed shortcut: $shortcut" }
    }

    if (Test-Path $InstallDir) { Remove-Item $InstallDir -Recurse -Force; Write-Ok "removed program: $InstallDir" }

    if (Test-Admin) {
        $hosts   = "$env:SystemRoot\System32\drivers\etc\hosts"
        $content = Get-Content $hosts
        $cleaned = $content | Where-Object { $_ -notmatch '#\s*kisaf$' }
        if ($cleaned.Count -ne $content.Count) {
            Set-Content -Path $hosts -Value $cleaned -Encoding ASCII
            Write-Ok 'removed the hosts entry'
        }
        Get-NetFirewallRule -DisplayName 'kisaf project manager' -ErrorAction SilentlyContinue |
            Remove-NetFirewallRule
    }

    Write-Host "`nUninstalled. Your project list is still here: $DataDir" -ForegroundColor White
    Write-Host "Delete that folder by hand if you want it gone too.`n"
    return
}

# ------------------------------------------------------------------ install

Write-Host "`nInstalling kisaf...`n" -ForegroundColor White

# 1. Copy the program ------------------------------------------------------
$source = Join-Path $PSScriptRoot '..\kisaf.exe' | Resolve-Path -ErrorAction SilentlyContinue
if (-not $source) { $source = Join-Path $PSScriptRoot 'kisaf.exe' | Resolve-Path -ErrorAction SilentlyContinue }
if (-not $source) {
    Write-Host "  kisaf.exe not found." -ForegroundColor Red
    Write-Host "  Put this script next to kisaf.exe, or build it first:" -ForegroundColor Red
    Write-Host "      go build -o kisaf.exe ." -ForegroundColor DarkGray
    exit 1
}

Write-Step "copying the program -> $InstallDir"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

Get-Process -Name 'kisaf' -ErrorAction SilentlyContinue | ForEach-Object {
    $_.Kill(); Start-Sleep -Milliseconds 400
}
Copy-Item $source $ExePath -Force
Write-Ok 'kisaf.exe installed'

# 2. Set the port ----------------------------------------------------------
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$configPath = Join-Path $DataDir 'config.json'

if (Test-Path $configPath) {
    $config = Get-Content $configPath -Raw | ConvertFrom-Json
    $config.port = $Port
} else {
    $config = [pscustomobject]@{
        host          = 'kisaf'
        port          = $Port
        fallbackPorts = @(7777, 8777, 8080, 0)
        bind          = '0.0.0.0'
        token         = ''
        enableMDNS    = $true
        enableTray    = $true
        openBrowser   = $true
        allowedHosts  = @()
    }
}
$config | ConvertTo-Json -Depth 5 | Set-Content $configPath -Encoding UTF8
Write-Ok "port set to $Port"

# 3. Shortcuts -------------------------------------------------------------
New-Shortcut -Path (Join-Path $DesktopDir 'kisaf.lnk') -Target $ExePath -Description 'kisaf project manager'
New-Shortcut -Path (Join-Path $StartMenu  'kisaf.lnk') -Target $ExePath -Description 'kisaf project manager'
Write-Ok 'created Desktop and Start Menu shortcuts'

if ($NoAutoStart) {
    $startupLink = Join-Path $StartupDir "$AppName.lnk"
    if (Test-Path $startupLink) { Remove-Item $startupLink -Force }
    Write-Skip 'auto-start (not requested)'
} else {
    New-Shortcut -Path (Join-Path $StartupDir "$AppName.lnk") -Target $ExePath -Description 'kisaf project manager'
    Write-Ok 'will start with Windows'
}

# 4. hosts entry -----------------------------------------------------------
# mDNS/LLMNR already resolve "kisaf" and "kisaf.local"; this entry is only a
# safety net for setups where those are disabled.
if (Test-Admin) {
    $hosts = "$env:SystemRoot\System32\drivers\etc\hosts"
    $line  = "127.0.0.1`tkisaf`t# kisaf"
    if ((Get-Content $hosts -Raw) -notmatch '#\s*kisaf') {
        Add-Content -Path $hosts -Value $line -Encoding ASCII
        Write-Ok 'added "kisaf" to the hosts file'
    } else {
        Write-Ok 'hosts entry already present'
    }
} else {
    Write-Skip 'hosts entry (needs administrator — not required)'
}

# 5. Firewall --------------------------------------------------------------
if ($AllowNetwork) {
    if (Test-Admin) {
        Get-NetFirewallRule -DisplayName 'kisaf project manager' -ErrorAction SilentlyContinue |
            Remove-NetFirewallRule
        New-NetFirewallRule -DisplayName 'kisaf project manager' `
            -Direction Inbound -Action Allow -Protocol TCP -LocalPort $Port `
            -Profile Private -Program $ExePath | Out-Null
        Write-Ok "opened port $Port in the firewall (private networks only)"
        Write-Host "  NOTE: also set the 'token' field in config.json before exposing this." -ForegroundColor Yellow
    } else {
        Write-Skip 'firewall rule (needs administrator)'
    }
} else {
    Write-Skip 'firewall rule (pass -AllowNetwork to enable)'
}

# --------------------------------------------------------------------- done

$suffix = if ($Port -eq 80) { '' } else { ":$Port" }

Write-Host "`nDone.`n" -ForegroundColor Green
Write-Host "  Addresses:"
Write-Host "    http://localhost$suffix"
Write-Host "    http://kisaf$suffix"
Write-Host "    http://kisaf.local$suffix   (phone / another computer)"
Write-Host "`n  Data folder : $DataDir"
Write-Host "  Program     : $ExePath`n"

Write-Host "Starting kisaf..." -ForegroundColor White
Start-Process $ExePath

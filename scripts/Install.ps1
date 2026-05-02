# Journal Companion installer
#
# Usage:
#   iwr https://journal.erros.gg/companion/install.ps1 | iex
#
# Or, if the script is downloaded:
#   powershell -ExecutionPolicy Bypass -File install.ps1
#
# What this does:
#   1. Downloads the latest journal-companion.exe from GitHub releases
#   2. Verifies the SHA-256 hash against the published checksum
#   3. Installs to %LOCALAPPDATA%\JournalCompanion\
#   4. Creates a Start Menu shortcut
#   5. Optionally registers the app to start with Windows
#   6. Launches the companion
#
# This script does not require admin rights. Everything is per-user.
# To uninstall, run: install.ps1 -Uninstall

[CmdletBinding()]
param(
    [switch]$Uninstall,
    [switch]$NoLaunch,
    [switch]$NoAutostart,
    [string]$Version = "latest"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"  # Speeds up Invoke-WebRequest considerably

# ─── Constants ──────────────────────────────────────────────────────────────

$AppName       = "Journal Companion"
$BinaryName    = "journal-companion.exe"
$InstallDir    = Join-Path $env:LOCALAPPDATA "JournalCompanion"
$BinaryPath    = Join-Path $InstallDir $BinaryName
$ShortcutDir   = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
$ShortcutPath  = Join-Path $ShortcutDir "$AppName.lnk"
$AutostartKey  = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$AutostartName = "JournalCompanion"
$Repo          = "erros-gg/journal-companion"

# ─── Helpers ────────────────────────────────────────────────────────────────

function Write-Step { param([string]$Message) Write-Host "  $Message" -ForegroundColor Cyan }
function Write-Done { param([string]$Message) Write-Host "  $Message" -ForegroundColor Green }
function Write-Warn { param([string]$Message) Write-Host "  $Message" -ForegroundColor Yellow }
function Write-Fail { param([string]$Message) Write-Host "  $Message" -ForegroundColor Red }

function Stop-RunningCompanion {
    $running = Get-Process -Name "journal-companion" -ErrorAction SilentlyContinue
    if ($running) {
        Write-Step "Stopping running companion..."
        $running | Stop-Process -Force
        Start-Sleep -Milliseconds 500
    }
}

# ─── Uninstall path ─────────────────────────────────────────────────────────

if ($Uninstall) {
    Write-Host ""
    Write-Host "Uninstalling $AppName" -ForegroundColor White
    Write-Host ""

    Stop-RunningCompanion

    if (Test-Path $AutostartKey) {
        $existing = Get-ItemProperty -Path $AutostartKey -Name $AutostartName -ErrorAction SilentlyContinue
        if ($existing) {
            Remove-ItemProperty -Path $AutostartKey -Name $AutostartName -ErrorAction SilentlyContinue
            Write-Done "Removed autostart entry"
        }
    }

    if (Test-Path $ShortcutPath) {
        Remove-Item $ShortcutPath -Force
        Write-Done "Removed Start Menu shortcut"
    }

    if (Test-Path $InstallDir) {
        Remove-Item $InstallDir -Recurse -Force
        Write-Done "Removed $InstallDir"
    }

    # Note: we deliberately leave config and database alone in %APPDATA%\Journal Companion.
    # The user's auth tokens, watch config, and upload history live there. If they're
    # uninstalling for good they can delete that folder manually; if they're reinstalling
    # they'll want it preserved.
    $configDir = Join-Path $env:APPDATA "Journal Companion"
    if (Test-Path $configDir) {
        Write-Host ""
        Write-Warn "Your config and history remain at:"
        Write-Warn "  $configDir"
        Write-Warn "Delete that folder if you want a fully clean uninstall."
    }

    Write-Host ""
    Write-Host "Uninstall complete." -ForegroundColor Green
    Write-Host ""
    return
}

# ─── Install path ───────────────────────────────────────────────────────────

Write-Host ""
Write-Host "Installing $AppName" -ForegroundColor White
Write-Host ""

# Resolve the release we're installing.
Write-Step "Resolving release..."

$releaseUrl = if ($Version -eq "latest") {
    "https://api.github.com/repos/$Repo/releases/latest"
} else {
    "https://api.github.com/repos/$Repo/releases/tags/$Version"
}

try {
    $release = Invoke-RestMethod -Uri $releaseUrl -Headers @{ "User-Agent" = "journal-companion-installer" }
} catch {
    Write-Fail "Could not reach GitHub. Check your internet connection."
    Write-Fail $_.Exception.Message
    exit 1
}

$tag = $release.tag_name
Write-Done "Found release $tag"

# Locate the binary asset and its checksum file.
$binaryAsset   = $release.assets | Where-Object { $_.name -eq $BinaryName }
$checksumAsset = $release.assets | Where-Object { $_.name -eq "$BinaryName.sha256" }

if (-not $binaryAsset) {
    Write-Fail "Release $tag does not contain $BinaryName."
    exit 1
}
if (-not $checksumAsset) {
    Write-Fail "Release $tag does not contain $BinaryName.sha256. Refusing to install unverified binary."
    exit 1
}

# Download to a temp location first so a failed download never replaces a working install.
$tempDir      = Join-Path $env:TEMP "journal-companion-install-$([Guid]::NewGuid().ToString('N'))"
$tempBinary   = Join-Path $tempDir $BinaryName
$tempChecksum = Join-Path $tempDir "$BinaryName.sha256"
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

try {
    Write-Step "Downloading $BinaryName ($([math]::Round($binaryAsset.size / 1MB, 1)) MB)..."
    Invoke-WebRequest -Uri $binaryAsset.browser_download_url   -OutFile $tempBinary
    Invoke-WebRequest -Uri $checksumAsset.browser_download_url -OutFile $tempChecksum

    Write-Step "Verifying checksum..."
    # The .sha256 file convention: "<hash>  <filename>" (two spaces, like sha256sum output)
    $expectedLine = (Get-Content $tempChecksum -First 1).Trim()
    $expectedHash = ($expectedLine -split '\s+')[0].ToLower()
    $actualHash   = (Get-FileHash -Path $tempBinary -Algorithm SHA256).Hash.ToLower()

    if ($expectedHash -ne $actualHash) {
        Write-Fail "Checksum mismatch."
        Write-Fail "  Expected: $expectedHash"
        Write-Fail "  Actual:   $actualHash"
        Write-Fail "Refusing to install. The download may be corrupted or tampered with."
        exit 1
    }
    Write-Done "Checksum verified"

    Stop-RunningCompanion

    Write-Step "Installing to $InstallDir..."
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    Move-Item -Path $tempBinary -Destination $BinaryPath -Force
    Write-Done "Binary installed"

    # Mark the binary as downloaded from the internet but trusted by the user (avoids the
    # second SmartScreen prompt on every launch). PowerShell does not have a clean stdlib
    # way to do this; the simplest approach is to remove the Zone.Identifier ADS that
    # Windows attached during download.
    Unblock-File -Path $BinaryPath -ErrorAction SilentlyContinue

} catch {
    Write-Fail "Install failed: $($_.Exception.Message)"
    exit 1
} finally {
    Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}

# ─── Start Menu shortcut ────────────────────────────────────────────────────

Write-Step "Creating Start Menu shortcut..."
try {
    $shell    = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($ShortcutPath)
    $shortcut.TargetPath       = $BinaryPath
    $shortcut.WorkingDirectory = $InstallDir
    $shortcut.Description      = "Watch and upload Journal data files"
    $shortcut.Save()
    Write-Done "Shortcut created"
} catch {
    Write-Warn "Could not create Start Menu shortcut: $($_.Exception.Message)"
    Write-Warn "(Install will continue. You can launch the binary directly from $BinaryPath.)"
}

# ─── Autostart ──────────────────────────────────────────────────────────────

if (-not $NoAutostart) {
    Write-Step "Registering autostart..."
    try {
        Set-ItemProperty -Path $AutostartKey -Name $AutostartName -Value "`"$BinaryPath`""
        Write-Done "Will start with Windows. (Toggle from the tray menu later if you want.)"
    } catch {
        Write-Warn "Could not register autostart: $($_.Exception.Message)"
        Write-Warn "(You can enable this from the tray menu after launching.)"
    }
}

# ─── Launch ─────────────────────────────────────────────────────────────────

Write-Host ""
Write-Host "Installed $AppName $tag" -ForegroundColor Green
Write-Host ""

if (-not $NoLaunch) {
    Write-Step "Launching..."
    Start-Process -FilePath $BinaryPath
    Start-Sleep -Seconds 1
    Write-Done "Look for the leaf icon in your system tray."
    Write-Host ""
    Write-Host "  Next: right-click the tray icon and choose 'Sign in to Journal'." -ForegroundColor White
    Write-Host ""
}
# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# Radius CLI Installer (PowerShell - Cross-Platform)
#
# Usage:
#   # Windows (PowerShell)
#   iwr -useb https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.ps1 | iex
#   ./install.ps1 -Version 0.40.0
#   ./install.ps1 -InstallDir "$HOME/.local/bin"
#   ./install.ps1 -IncludeRC
#
#   # macOS/Linux (pwsh)
#   pwsh -c "& { iwr -useb https://...install.ps1 | iex }"
#
# Parameters:
#   -Version       Version to install (e.g., 0.40.0, v0.40.0, edge)
#   -InstallDir    Installation directory (default: auto-detected)
#   -IncludeRC     Include release candidates when determining latest
#   -Help          Show this help message
#
# Environment Variables:
#   INSTALL_DIR    Override installation directory
#   INCLUDE_RC     Set to "true" to include release candidates
# ============================================================================

[CmdletBinding()]
param (
    [Alias("v")]
    [string]$Version,

    [Alias("d", "RadiusRoot")]
    [string]$InstallDir,

    [Alias("rc")]
    [switch]$IncludeRC,

    [Alias("h")]
    [switch]$Help
)

$ErrorActionPreference = 'Stop'

# Constants
$GitHubOrg = "radius-project"
$GitHubRepo = "radius"
$GitHubReleaseJsonUrl = "https://api.github.com/repos/$GitHubOrg/$GitHubRepo/releases"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Show-Usage {
    $helpText = @"
Radius CLI Installer (PowerShell)

Usage: install.ps1 [OPTIONS]

Options:
  -Version <VERSION>       Version to install (e.g., 0.40.0, edge)
  -InstallDir <DIR>        Installation directory (default: auto-detected)
  -IncludeRC               Include release candidates in latest version
  -Help                    Show this help message

Environment Variables:
  INSTALL_DIR              Override installation directory
  INCLUDE_RC               Set to "true" to include release candidates

Install Directory Detection:
  Windows: `$env:LOCALAPPDATA\radius
  macOS/Linux: `$HOME/.local/bin
Examples:
  ./install.ps1
  ./install.ps1 -Version 0.40.0
  ./install.ps1 -InstallDir "`$HOME/bin"
  ./install.ps1 -Version edge
"@
    Write-Output $helpText
    exit 0
}

function Get-SystemInfo {
    $detectedOS = if ($IsWindows -or $env:OS -eq "Windows_NT") {
        "windows"
    }
    elseif ($IsMacOS) {
        "darwin"
    }
    else {
        "linux"
    }

    # Use PROCESSOR_ARCHITECTURE on Windows (always available, including PS 5.1).
    # Fall back to RuntimeInformation for non-Windows platforms (pwsh on Linux/macOS).
    if ($detectedOS -eq "windows") {
        $rawArch = ($env:PROCESSOR_ARCHITECTURE).ToLower()
    }
    else {
        $rawArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLower()
    }

    $detectedArch = switch ($rawArch) {
        "x64" { "amd64" }
        "amd64" { "amd64" }
        "arm64" { "arm64" }
        "arm" { "arm" }
        default { $rawArch }
    }

    return @{ OS = $detectedOS; Arch = $detectedArch }
}

function Get-DefaultInstallDir {
    param ([string]$DetectedOS)

    if ($env:INSTALL_DIR) {
        return $env:INSTALL_DIR
    }

    if ($DetectedOS -eq "windows") {
        return Join-Path $env:LOCALAPPDATA "radius"
    }

    return Join-Path $HOME ".local" "bin"
}

function Get-CliFileName {
    param ([string]$DetectedOS)
    if ($DetectedOS -eq "windows") { return "rad.exe" }
    return "rad"
}

function Get-GithubHeaders {
    $headers = @{}
    if ($env:GITHUB_USER -and $env:GITHUB_TOKEN) {
        $pair = "$($env:GITHUB_USER):$($env:GITHUB_TOKEN)"
        $bytes = [System.Text.Encoding]::ASCII.GetBytes($pair)
        $base64 = [System.Convert]::ToBase64String($bytes)
        $headers["Authorization"] = "Basic $base64"
    }
    return $headers
}

function Get-LatestRelease {
    param (
        [bool]$ShouldIncludeRC,
        [hashtable]$Headers
    )

    $releases = Invoke-RestMethod -Headers $Headers -Uri $GitHubReleaseJsonUrl -Method Get
    if ($releases.Count -eq 0) {
        throw "No releases found at github.com/$GitHubOrg/$GitHubRepo"
    }

    if ($ShouldIncludeRC) {
        $release = $releases | Select-Object -First 1
    }
    else {
        $release = $releases | Where-Object { $_.tag_name -notlike "*rc*" } | Select-Object -First 1
    }

    if (-not $release) {
        throw "Could not determine latest release"
    }

    return $release
}

function Get-ReleaseAsset {
    param (
        $Release,
        [string]$ArtifactName
    )

    $asset = $Release | Select-Object -ExpandProperty assets | Where-Object { $_.name -eq $ArtifactName }
    if (-not $asset) {
        throw "Cannot find asset '$ArtifactName' in release $($Release.tag_name)"
    }

    return @{
        Url  = $asset.url
        Name = $asset.name
    }
}

function Install-RadEdge {
    param (
        [string]$TargetDir,
        [string]$DetectedOS,
        [string]$DetectedArch,
        [string]$CliFileName
    )

    $orasCmd = Get-Command oras -ErrorAction SilentlyContinue
    if (-not $orasCmd) {
        Write-Output "Error: oras CLI is not installed or not found in PATH."
        Write-Output "Please visit https://edge.docs.radapp.io/installation for edge CLI installation instructions."
        exit 1
    }

    $downloadURL = "ghcr.io/$GitHubOrg/rad/$DetectedOS-$($DetectedArch):latest"
    Write-Output "Downloading edge CLI from $downloadURL..."

    & oras pull $downloadURL -o $TargetDir
    if ($LASTEXITCODE -ne 0) {
        Write-Output "Failed to download edge rad CLI."
        Write-Output "If this was an authentication issue, please run 'docker logout ghcr.io' to clear any expired credentials."
        Write-Output "Visit https://edge.docs.radapp.io/installation for edge CLI installation instructions."
        exit 1
    }

    $downloadedFile = Join-Path $TargetDir "rad"
    $cliFilePath = Join-Path $TargetDir $CliFileName
    if (($downloadedFile -ne $cliFilePath) -and (Test-Path $downloadedFile -PathType Leaf)) {
        Move-Item -Path $downloadedFile -Destination $cliFilePath -Force
    }

    # Set executable permission on non-Windows
    if ($DetectedOS -ne "windows") {
        & chmod +x $cliFilePath
    }
}

function Install-RadRelease {
    param (
        [string]$ReleaseTag,
        [string]$TargetDir,
        [string]$DetectedOS,
        [string]$DetectedArch,
        [string]$CliFileName,
        [hashtable]$Headers
    )

    $releases = Invoke-RestMethod -Headers $Headers -Uri $GitHubReleaseJsonUrl -Method Get
    if ($releases.Count -eq 0) {
        throw "No releases found at github.com/$GitHubOrg/$GitHubRepo"
    }

    $release = $releases | Where-Object { $_.tag_name -eq $ReleaseTag } | Select-Object -First 1
    if (-not $release) {
        throw "Cannot find release $ReleaseTag"
    }

    $artifactSuffix = if ($DetectedOS -eq "windows") { ".exe" } else { "" }
    $artifactName = "rad_$($DetectedOS)_$($DetectedArch)$artifactSuffix"

    $asset = Get-ReleaseAsset -Release $release -ArtifactName $artifactName
    $tmpFile = Join-Path ([System.IO.Path]::GetTempPath()) $asset.Name

    try {
        Write-Output "Downloading $($asset.Url)..."
        $downloadHeaders = @{}
        foreach ($key in $Headers.Keys) {
            $downloadHeaders[$key] = $Headers[$key]
        }
        $downloadHeaders["Accept"] = "application/octet-stream"
        $oldProgressPreference = $ProgressPreference
        $ProgressPreference = "SilentlyContinue"
        Invoke-WebRequest -Headers $downloadHeaders -Uri $asset.Url -OutFile $tmpFile
    }
    catch {
        throw "Failed to download rad CLI: $_"
    }
    finally {
        $ProgressPreference = $oldProgressPreference
    }

    if (-not (Test-Path $tmpFile -PathType Leaf)) {
        throw "Failed to download rad CLI binary"
    }

    $cliFilePath = Join-Path $TargetDir $CliFileName
    if (Test-Path $cliFilePath -PathType Leaf) {
        Remove-Item -Force $cliFilePath
    }
    Move-Item -Path $tmpFile -Destination $cliFilePath -Force

    # Set executable permission on non-Windows
    if ($DetectedOS -ne "windows") {
        & chmod +x $cliFilePath
    }
}

function Show-ExistingRadiusWarning {
    param (
        [string]$TargetDir,
        [string]$CliFileName
    )

    $resolvedInstall = (Resolve-Path -Path $TargetDir -ErrorAction SilentlyContinue).Path
    if (-not $resolvedInstall) {
        $resolvedInstall = $TargetDir
    }

    $separator = if ($IsWindows -or $env:OS -eq "Windows_NT") { ';' } else { ':' }
    $stalePaths = @()

    foreach ($dir in ($env:PATH -split [regex]::Escape($separator))) {
        if (-not $dir -or -not (Test-Path $dir -PathType Container)) {
            continue
        }

        $candidate = Join-Path $dir $CliFileName
        if (Test-Path $candidate -PathType Leaf) {
            # On non-Windows, skip non-executable files (matches bash installer's -x check)
            if (-not ($IsWindows -or $env:OS -eq "Windows_NT")) {
                if (-not (& test -x $candidate)) { continue }
            }
            $resolvedDir = (Resolve-Path -Path $dir).Path
            if ($resolvedDir -ne $resolvedInstall) {
                $stalePaths += $candidate
            }
        }
    }

    if ($stalePaths.Count -eq 0) {
        return
    }

    Write-Output "============================================================================"
    Write-Output "WARNING: Existing Radius CLI installation(s) found in different location(s):"
    foreach ($p in $stalePaths) {
        Write-Output "  $p"
    }
    Write-Output ""
    Write-Output "The new installation will be placed in:"
    Write-Output "  $(Join-Path $TargetDir $CliFileName)"
    Write-Output ""
    Write-Output "Remove the old binary(ies) before continuing to avoid using the wrong version:"
    foreach ($p in $stalePaths) {
        if ($IsWindows -or $env:OS -eq "Windows_NT") {
            Write-Output "  Remove-Item -LiteralPath `"$p`""
        }
        else {
            Write-Output "  rm `"$p`""
        }
    }
    Write-Output "============================================================================"
}

function Add-ToPath {
    param (
        [string]$TargetDir,
        [string]$DetectedOS
    )

    if ($DetectedOS -eq "windows") {
        Add-ToPathWindows -TargetDir $TargetDir
    }
    else {
        Add-ToPathUnix -TargetDir $TargetDir
    }
}

function Add-ToPathWindows {
    param ([string]$TargetDir)

    $userPath = (Get-Item -Path HKCU:\Environment).GetValue(
        'PATH',
        $null,
        'DoNotExpandEnvironmentNames'
    )

    if (-not $userPath) {
        $userPath = ""
    }

    # Remove legacy c:\radius from User Path if it exists
    if ($userPath -like '*c:\radius*') {
        $userPath = $userPath -replace 'c:\\radius;?', ''
        $userPath = $userPath.TrimEnd(';')
        Set-ItemProperty HKCU:\Environment "PATH" "$userPath" -Type ExpandString
    }

    # Remove old rad CLI at legacy location
    if (Test-Path "C:\radius\rad.exe" -PathType Leaf) {
        Remove-Item -Recurse -Force "C:\radius"
    }

    $pathEntries = $userPath -split ';' | Where-Object { $_ -ne '' }
    if (-not ($pathEntries -contains $TargetDir)) {
        Write-Output "Adding $TargetDir to User Path..."
        $newPath = if ($userPath) { "$userPath;$TargetDir" } else { $TargetDir }
        Set-ItemProperty HKCU:\Environment "PATH" "$newPath" -Type ExpandString
        $env:PATH += ";$TargetDir"
    }
}

function Add-ToPathUnix {
    param ([string]$TargetDir)

    $pathDirs = $env:PATH -split ':'
    if ($pathDirs -contains $TargetDir) {
        return
    }

    Write-Output ""
    Write-Output "============================================================================"
    Write-Output "NOTE: $TargetDir is not in your PATH."
    Write-Output ""
    Write-Output "Add it by running one of the following:"
    Write-Output ""

    $shellName = if ($env:SHELL) { Split-Path $env:SHELL -Leaf } else { "bash" }
    switch ($shellName) {
        "zsh" {
            Write-Output "  echo 'export PATH=""$TargetDir"":$PATH' >> ~/.zshrc"
            Write-Output "  source ~/.zshrc"
        }
        "fish" {
            Write-Output "  fish_add_path $TargetDir"
        }
        default {
            Write-Output "  echo 'export PATH=""$TargetDir"":$PATH' >> ~/.bashrc"
            Write-Output "  source ~/.bashrc"
        }
    }
    Write-Output "============================================================================"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

if ($Help) {
    Show-Usage
}

# Respect INCLUDE_RC environment variable
$shouldIncludeRC = [bool]$IncludeRC
if ($env:INCLUDE_RC -eq "true") {
    $shouldIncludeRC = $true
}

# Detect system
$sysInfo = Get-SystemInfo
$detectedOS = $sysInfo.OS
$detectedArch = $sysInfo.Arch
$cliFileName = Get-CliFileName -DetectedOS $detectedOS

Write-Output ""
Write-Output "Your system is $($detectedOS)_$($detectedArch)"

# Determine install directory
if ($InstallDir) {
    $resolvedInstallDir = $InstallDir
}
else {
    $resolvedInstallDir = Get-DefaultInstallDir -DetectedOS $detectedOS
}

# Support RADIUS_INSTALL_DIR as backward-compatible alias
if ($env:RADIUS_INSTALL_DIR -and -not $InstallDir -and -not $env:INSTALL_DIR) {
    $resolvedInstallDir = $env:RADIUS_INSTALL_DIR
}

# Ensure install directory exists
if (-not (Test-Path $resolvedInstallDir -PathType Container)) {
    Write-Output "Creating $resolvedInstallDir directory..."
    New-Item -Path $resolvedInstallDir -ItemType Directory -Force | Out-Null
    if (-not (Test-Path $resolvedInstallDir -PathType Container)) {
        throw "Cannot create $resolvedInstallDir"
    }
}

$cliFilePath = Join-Path $resolvedInstallDir $cliFileName

# Check for existing installation
if (Test-Path $cliFilePath -PathType Leaf) {
    Write-Output ""
    try {
        $currentVer = & $cliFilePath version --cli 2>$null
        Write-Output "Radius CLI is detected. Current version: $currentVer"
    }
    catch {
        Write-Output "Previous installation detected (version unknown)"
    }
    Write-Output ""
    Write-Output "Reinstalling Radius CLI - $cliFilePath..."
}
else {
    Write-Output "Installing Radius CLI..."
}

# Warn if rad exists elsewhere in PATH
Show-ExistingRadiusWarning -TargetDir $resolvedInstallDir -CliFileName $cliFileName

# Ensure TLS 1.2 on Windows PowerShell (version < 6)
if ($PSVersionTable.PSVersion.Major -lt 6) {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}

$githubHeaders = Get-GithubHeaders

# Determine version to install
if (-not $Version) {
    Write-Output "Getting the latest Radius CLI..."
    if ($shouldIncludeRC) {
        Write-Output "Including release candidates in version selection..."
    }
    $release = Get-LatestRelease -ShouldIncludeRC $shouldIncludeRC -Headers $githubHeaders
    $releaseTag = $release.tag_name
}
elseif ($Version -eq "edge") {
    $releaseTag = "edge"
}
else {
    # Strip leading "v" if present, then normalize to v-prefixed format
    $releaseTag = "v$($Version.TrimStart('v'))"
}

Write-Output ""
Write-Output "Installing $releaseTag Radius CLI..."
Write-Output "Install directory: $resolvedInstallDir"

# Download and install
if ($releaseTag -eq "edge") {
    Install-RadEdge -TargetDir $resolvedInstallDir -DetectedOS $detectedOS -DetectedArch $detectedArch -CliFileName $cliFileName
}
else {
    Install-RadRelease -ReleaseTag $releaseTag -TargetDir $resolvedInstallDir -DetectedOS $detectedOS -DetectedArch $detectedArch -CliFileName $cliFileName -Headers $githubHeaders
}

if (-not (Test-Path $cliFilePath -PathType Leaf)) {
    throw "Failed to install rad CLI binary"
}

Write-Output "$cliFileName installed into $resolvedInstallDir successfully"

# Install bicep
Write-Output ""
Write-Output "Installing bicep..."
& $cliFilePath bicep download
if ($LASTEXITCODE -ne 0) {
    Write-Warning "Failed to install bicep"
}
else {
    Write-Output "bicep installed successfully"
}

# Update PATH
Add-ToPath -TargetDir $resolvedInstallDir -DetectedOS $detectedOS

Write-Output ""
Write-Output "To get started with Radius, please visit https://docs.radapp.io/getting-started/"

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

param (
    [string]$Version,
    [string]$RadiusRoot = "$env:LOCALAPPDATA\radius"
)

Write-Output ""
$ErrorActionPreference = 'stop'

# Constants
$RadiusCliFileName = "rad.exe"
$RadiusCliFilePath = "${RadiusRoot}\${RadiusCliFileName}"
$OS = "windows"
$Arch = "amd64"
$GitHubOrg = "radius-project"
$GitHubRepo = "radius"
$GitHubReleaseJsonUrl = "https://api.github.com/repos/${GitHubOrg}/${GitHubRepo}/releases"

function GetVersionInfo {
    param (
        [string]$Version,
        $Releases
    )
    # Filter windows binary and download archive
    if (!$Version) {
        $release = $Releases | Where-Object { $_.tag_name -notlike "*rc*" } | Select-Object -First 1
    }
    else {
        $release = $Releases | Where-Object { $_.tag_name -eq "v$Version" } | Select-Object -First 1
    }

    return $release
}

function GetWindowsAsset {
    param (
        $Release
    )
    $windowsAsset = $Release | Select-Object -ExpandProperty assets | Where-Object { $_.name -Like "*${OS}_${Arch}.exe" }
    if (!$windowsAsset) {
        throw "Cannot find the Windows rad CLI binary"
    }
    [hashtable]$return = @{}
    $return.url = $windowsAsset.url
    $return.name = $windowsAsset.name
    return $return
}

# Set Github request authentication for basic authentication.
if ($Env:GITHUB_USER) {
    $basicAuth = [System.Convert]::ToBase64String([System.Text.Encoding]::ASCII.GetBytes($Env:GITHUB_USER + ":" + $Env:GITHUB_TOKEN));
    $githubHeader = @{"Authorization" = "Basic $basicAuth" }
}
else {
    $githubHeader = @{}
}

if ((Get-ExecutionPolicy) -gt 'RemoteSigned' -or (Get-ExecutionPolicy) -eq 'ByPass') {
    Write-Output "PowerShell requires an execution policy of 'RemoteSigned'."
    Write-Output "To make this change please run:"
    Write-Output "'Set-ExecutionPolicy RemoteSigned -scope CurrentUser'"
    break
}

# Change security protocol to support TLS 1.2 / 1.1 / 1.0 - old powershell uses TLS 1.0 as a default protocol
[Net.ServicePointManager]::SecurityProtocol = "tls12, tls11, tls"

# Remove old rad CLI at legacy location, C:\radius
if (Test-Path "C:\radius\rad.exe" -PathType Leaf) {
    Remove-Item -Recurse -Force "C:\radius"
}

# Check if Radius CLI is installed.
if (Test-Path $RadiusCliFilePath -PathType Leaf) {
    Write-Output "Previous rad CLI detected: $RadiusCliFilePath"
    try {
        $CurrentVersion = &$RadiusCliFilePath version -o json | ConvertFrom-JSON | Select-Object -ExpandProperty version
        Write-Output "Previous version: $CurrentVersion`r`n"
    }
    catch {
        Write-Output "Previous installation corrupted`r`n"
    }
    Write-Output "Reinstalling rad CLI..."
}
else {
    Write-Output "Installing rad CLI..."
}

# Create Radius Directory
if (-Not (Test-Path $RadiusRoot -PathType Container)) {
    Write-Output "Creating $RadiusRoot directory..."
    New-Item -ErrorAction Ignore -Path $RadiusRoot -ItemType "directory" | Out-Null
    if (!(Test-Path $RadiusRoot -PathType Container)) {
        throw "Cannot create $RadiusRoot"
    }
}

if ($Version -eq "edge") {
    # Check if oras CLI is installed
    $orasExists = Get-Command oras -ErrorAction SilentlyContinue
    if (-Not $orasExists) {
        Write-Output "Error: oras CLI is not installed or not found in PATH."
        Write-Output "Please visit https://edge.docs.radapp.io/installation for edge build installation instructions."
        Exit 1
    }

    $downloadURL = "ghcr.io/${GitHubOrg}/rad/${OS}-${Arch}:latest"
    Write-Output "Downloading edge CLI from ${downloadURL}..."
    oras pull $downloadURL -o $RadiusRoot

    # Check if the oras pull command was successful
    if ($LASTEXITCODE -ne 0) {
        Write-Output "Failed to download edge rad CLI."
        Write-Output "If this was an authentication issue, please run 'docker logout ghcr.io' to clear any expired credentials."
        Write-Output "Visit https://edge.docs.radapp.io/installation for edge build installation instructions."
        Exit 1
    }
}
else {
    # Get the list of releases from GitHub
    $releases = Invoke-RestMethod -Headers $githubHeader -Uri $GitHubReleaseJsonUrl -Method Get
    if ($releases.Count -eq 0) {
        throw "No releases from github.com/${GitHubOrg}/${GitHubRepo}"
    }

    $release = GetVersionInfo -Version $Version -Releases $releases
    if (!$release) {
        throw "Cannot find the specified rad CLI binary version"
    }
    $asset = GetWindowsAsset -Release $release
    $assetName = $asset.name
    $exeFileUrl = $asset.url
    $exeFilePath = $RadiusRoot + "\" + $assetName

    # Download rad CLI
    try {
        Write-Output "Downloading $exeFileUrl..."
        $githubHeader.Accept = "application/octet-stream"
        $oldProgressPreference = $ProgressPreference
        $ProgressPreference = "SilentlyContinue" # Do not show progress bar
        Invoke-WebRequest -Headers $githubHeader -Uri $exeFileUrl -OutFile $exeFilePath
    }
    catch [Net.WebException] {
        throw "ERROR: The specified release version: $Version does not exist."
    }
    finally {
        $ProgressPreference = $oldProgressPreference;
    }

    if (!(Test-Path $exeFilePath -PathType Leaf)) {
      throw "Failed to download rad CLI binary - $exeFilePath"
    }
    
    # Remove old rad CLI if exists
    if (Test-Path $RadiusCliFilePath -PathType Leaf) {
        Remove-Item -Recurse -Force $RadiusCliFilePath
    }
    
    # Rename the downloaded rad CLI binary
    Rename-Item -Path $exeFilePath -NewName $RadiusCliFileName -Force
}

if (!(Test-Path $RadiusCliFilePath -PathType Leaf)) {
  throw "Failed to download rad CLI binary - $exeFilePath"
}

# Print the version string of the installed CLI
Write-Output "rad CLI version: $(&$RadiusCliFilePath version -o json | ConvertFrom-JSON | Select-Object -ExpandProperty version)"

# Add RadiusRoot directory to User Path environment variable
$UserPathEnvironmentVar = (Get-Item -Path HKCU:\Environment).GetValue(
    'PATH', # the registry-value name
    $null, # the default value to return if no such value exists.
    'DoNotExpandEnvironmentNames' # the option that suppresses expansion
)

# Remove legacy c:\radius from User Path if it exists
if ($UserPathEnvironmentVar -like '*c:\radius*') {
    $UserPathEnvironmentVar = $UserPathEnvironmentVar -replace 'c:\\radius;', ''
    Set-ItemProperty HKCU:\Environment "PATH" "$UserPathEnvironmentVar" -Type ExpandString
}

if (-Not ($UserPathEnvironmentVar -like '*radius*')) {
    Write-Output "Adding $RadiusRoot to User Path..."
    # [Environment]::SetEnvironmentVariable sets the value kind as REG_SZ, use the function below to set a value of kind REG_EXPAND_SZ
    Set-ItemProperty HKCU:\Environment "PATH" "$UserPathEnvironmentVar;$RadiusRoot" -Type ExpandString
    # Also add the path to the current session
    $env:PATH += ";$RadiusRoot"
}

Write-Output "rad CLI has been successfully installed"

Write-Output "`r`nInstalling Bicep..."
$cmd = (Start-Process -NoNewWindow -FilePath $RadiusCliFilePath -ArgumentList "bicep download" -PassThru -Wait)
if ($cmd.ExitCode -ne 0) {
    Write-Warning "`r`nFailed to install rad-bicep"
}
else {
    Write-Output "Bicep has been successfully installed"
}

Write-Output "`r`nTo get started with Radius, please visit https://docs.radapp.io/getting-started/"

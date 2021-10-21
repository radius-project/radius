# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

param (
    [string]$Version,
    [string]$RadiusRoot = "c:\radius"
)

Write-Output ""
$ErrorActionPreference = 'stop'

#Escape space of RadiusRoot path
$RadiusRoot = $RadiusRoot -replace ' ', '` '

# Constants
$RadiusCliFileName = "rad.exe"
$RadiusCliFilePath = "${RadiusRoot}\${RadiusCliFileName}"
$OsArch = "windows-x64"
$BaseDownloadUrl = "https://get.radapp.dev/tools/rad"
$StableVersionUrl = "https://get.radapp.dev/version/stable.txt"

if ((Get-ExecutionPolicy) -gt 'RemoteSigned' -or (Get-ExecutionPolicy) -eq 'ByPass') {
    Write-Output "PowerShell requires an execution policy of 'RemoteSigned'."
    Write-Output "To make this change please run:"
    Write-Output "'Set-ExecutionPolicy RemoteSigned -scope CurrentUser'"
    break
}

# Change security protocol to support TLS 1.2 / 1.1 / 1.0 - old powershell uses TLS 1.0 as a default protocol
[Net.ServicePointManager]::SecurityProtocol = "tls12, tls11, tls"

# Check if Radius CLI is installed.
if (Test-Path $RadiusCliFilePath -PathType Leaf) {
    Write-Warning "Radius is detected - $RadiusCliFilePath"
    #TODO Invoke-Expression "$RadiusCliFilePath --version"
    Write-Output "Reinstalling Radius..."
}
else {
    Write-Output "Installing Radius..."
}

# Create Radius Directory
Write-Output "Creating $RadiusRoot directory"
New-Item -ErrorAction Ignore -Path $RadiusRoot -ItemType "directory"
if (!(Test-Path $RadiusRoot -PathType Container)) {
    throw "Cannot create $RadiusRoot"
}

if($Version -eq "")
{
    $Version = Invoke-WebRequest $StableVersionUrl -UseBasicParsing
    $Version = $Version.Trim()
}
$urlParts = @(
    $BaseDownloadUrl,
    $Version,
    $OsArch,
    $RadiusCliFileName
)
$binaryUrl =  $urlParts -join "/"

$binaryFilePath = $RadiusRoot + "\" + $RadiusCliFileName
Write-Output "Downloading $binaryUrl ..."

$uri = [uri]$binaryUrl
try
{
    $ProgressPreference = "SilentlyContinue" # Do not show progress bar
    Invoke-WebRequest -Uri $binaryUrl -OutFile $binaryFilePath -UseBasicParsing
    if (!(Test-Path $binaryFilePath -PathType Leaf)) {
        throw "Failed to download Radius Cli binary - $binaryFilePath"
    }
}
catch [Net.WebException]
{
    throw "ERROR: The specified release version: $Version does not exist."
}

# TODO Check the Radius CLI version: Invoke-Expression "$RadiusCliFilePath --version"

# Add RadiusRoot directory to User Path environment variable
Write-Output "Try to add $RadiusRoot to User Path Environment variable..."
$UserPathEnvironmentVar = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPathEnvironmentVar -like '*radius*') {
    Write-Output "Skipping to add $RadiusRoot to User Path - $UserPathEnvironmentVar"
}
else {
    [System.Environment]::SetEnvironmentVariable("PATH", $UserPathEnvironmentVar + ";$RadiusRoot", "User")
    $UserPathEnvironmentVar = [Environment]::GetEnvironmentVariable("PATH", "User")
    Write-Output "Added $RadiusRoot to User Path - $UserPathEnvironmentVar"
}

Write-Output "`r`nRadius CLI is installed successfully."

Write-Output "Installing rad-bicep (""rad bicep download"")..."
$cmd = (Start-Process -NoNewWindow -FilePath $RadiusCliFilePath -ArgumentList "bicep download" -PassThru -Wait)
if ($cmd.ExitCode -ne 0) {
    Write-Warning "`r`nFailed to install rad-cli"
} else {
    Write-Output "`r`nrad-bicep installed successfully"
}

Write-Output "To get started with Radius, please visit https://docs.radapp.dev/getting-started/."

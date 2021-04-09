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
$BaseDownloadUrl = "https://radiuspublic.blob.core.windows.net/tools/rad"

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

# TODO Get the list of release from GitHub and get the latest version
if($Version -eq "")
{
    $Version = "edge"
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

$githubHeader.Accept = "application/octet-stream"
$uri = [uri]$binaryUrl
if (Test-Connection $uri.IdnHost -quiet) {
    Invoke-WebRequest -Headers $githubHeader -Uri $binaryUrl -OutFile $binaryFilePath
    if (!(Test-Path $binaryFilePath -PathType Leaf)) {
        throw "Failed to download Radius Cli binary - $binaryFilePath"
    }
}
else {
    throw "The specified release version: $Version does not exist."
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
Write-Output "To get started with Radius, please visit https://github.com/Azure/radius."
Write-Output "Ensure that Docker Desktop is set to Linux containers mode when you run Radius in self hosted mode."

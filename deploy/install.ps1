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
    Write-Output "Current version:"
    Write-Output $(Invoke-Expression "$RadiusCliFilePath version -o json | ConvertFrom-JSON | Format-List")
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

if ($Version -eq "") {
    $Version = Invoke-WebRequest $StableVersionUrl -UseBasicParsing
    $Version = $Version.Trim()
}
$urlParts = @(
    $BaseDownloadUrl,
    $Version,
    $OsArch,
    $RadiusCliFileName
)
$binaryUrl = $urlParts -join "/"

$binaryFilePath = $RadiusRoot + "\" + $RadiusCliFileName
Write-Output "Downloading $binaryUrl ..."

try {
    $ProgressPreference = "SilentlyContinue" # Do not show progress bar
    Invoke-WebRequest -Uri $binaryUrl -OutFile $binaryFilePath -UseBasicParsing
    if (!(Test-Path $binaryFilePath -PathType Leaf)) {
        throw "Failed to download Radius Cli binary - $binaryFilePath"
    }
}
catch [Net.WebException] {
    throw "ERROR: The specified release version: $Version does not exist."
}

# Print the version string of the installed CLI
Write-Output "Radius version:"
Write-Output $(Invoke-Expression "$RadiusCliFilePath version -o json | ConvertFrom-JSON | Format-List")

# Add RadiusRoot directory to User Path environment variable
Write-Output "Try to add $RadiusRoot to User Path Environment variable..."
$UserPathEnvironmentVar = (Get-Item -Path HKCU:\Environment).GetValue(
    'PATH', # the registry-value name
    $null, # the default value to return if no such value exists.
    'DoNotExpandEnvironmentNames' # the option that suppresses expansion
)
  
if ($UserPathEnvironmentVar -like '*radius*') {
    Write-Output "Skipping to add $RadiusRoot to User Path - $UserPathEnvironmentVar"
}
else {
    Write-Host "Adding $InstallFolder to PATH"
    # [Environment]::SetEnvironmentVariable sets the value kind as REG_SZ, use the function below to set a value of kind REG_EXPAND_SZ
    Set-ItemProperty HKCU:\Environment "PATH" "$UserPathEnvironmentVar$RadiusRoot" -Type ExpandString
    # Also add the path to the current session
    $env:PATH += ";$RadiusRoot"
}
Write-Output "Added $RadiusRoot to User Path - $env:PATH"
Write-Output "`r`nRadius CLI is installed successfully."

Write-Output "Installing rad-bicep (""rad bicep download"")..."
$cmd = (Start-Process -NoNewWindow -FilePath $RadiusCliFilePath -ArgumentList "bicep download" -PassThru -Wait)
if ($cmd.ExitCode -ne 0) {
    Write-Warning "`r`nFailed to install rad-bicep"
}
else {
    Write-Output "`r`nrad-bicep installed successfully"
}

Write-Output "To get started with Radius, please visit https://docs.radapp.dev/getting-started/."

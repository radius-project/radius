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
    Write-Output "Previous rad CLI detected: $RadiusCliFilePath"
    try {
        $CurrentVersion = Invoke-Expression "$RadiusCliFilePath version -o json | ConvertFrom-JSON | Select-Object -ExpandProperty version"
        Write-Output "Previous version: $CurrentVersion`r`n"
    }
    catch {
        Write-Output "Current installation corrupted`r`n"
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
Write-Output "Downloading $binaryUrl..."

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
Write-Output "rad CLI version: $(Invoke-Expression "$RadiusCliFilePath version -o json | ConvertFrom-JSON | Select-Object -ExpandProperty version")"

# Add RadiusRoot directory to User Path environment variable
$UserPathEnvironmentVar = (Get-Item -Path HKCU:\Environment).GetValue(
    'PATH', # the registry-value name
    $null, # the default value to return if no such value exists.
    'DoNotExpandEnvironmentNames' # the option that suppresses expansion
)

Write-Output "Adding $RadiusRoot to User Path..."  
if (-Not ($UserPathEnvironmentVar -like '*radius*')) {
    # [Environment]::SetEnvironmentVariable sets the value kind as REG_SZ, use the function below to set a value of kind REG_EXPAND_SZ
    Set-ItemProperty HKCU:\Environment "PATH" "$UserPathEnvironmentVar$RadiusRoot" -Type ExpandString
    # Also add the path to the current session
    $env:PATH += ";$RadiusRoot"
}
Write-Output "✅ rad CLI has been successfully installed"

Write-Output "`r`nInstalling Bicep..."
$cmd = (Start-Process -NoNewWindow -FilePath $RadiusCliFilePath -ArgumentList "bicep download" -PassThru -Wait)
if ($cmd.ExitCode -ne 0) {
    Write-Warning "`r`nFailed to install rad-bicep"
}
else {
    Write-Output "✅ Bicep has been successfully installed"
}

Write-Output "`r`nTo get started with Project Radius, please visit https://docs.radapp.dev/getting-started/"

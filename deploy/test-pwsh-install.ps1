
param(
    [string] $InstallFolder = "$($env:USERPROFILE)\rad-install-test"
)

## Fetch PATH variable values
$regKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $false)
$originalPath = $regKey.GetValue( `
        'PATH', `
        '', `
        [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames `
)
$originalPathType = $regKey.GetValueKind('PATH')

## Run rad installer
& $PSScriptRoot/install.ps1 `
    -RadiusRoot $InstallFolder `
    -Verbose

if ($LASTEXITCODE) {
    Write-Error "Install failed. Last exit code: $LASTEXITCODE"
    exit $LASTEXITCODE
}

$currentPath = $regKey.GetValue( `
        'PATH', `
        '', `
        [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames `
)

## Verify that the rad installation directory was successfully added to PATH
$expectedPathEntry = $InstallFolder
if (!$currentPath.Contains($expectedPathEntry)) {
    Write-Error "Could not find path entry. Expected substring: $expectedPathEntry, Actual: $path"
    exit 1
}

## Verify that the installation didn't change the REG_KEY Kind
$afterInstallPathType = $regKey.GetValueKind('PATH')
if ($originalPathType -ne $afterInstallPathType) {
    Write-Error "Path registry key type does not match. Expected: $originalPathType,  Actual: $afterInstallPathType"
    exit 1
}

## Verify the original path is not overridden
if (!$currentPath.StartsWith($originalPath)) {
    Write-Error "Path is not using original path as a prefix after installation. Expected: $originalPath, Actual: $currentPath"
    exit 1
}

## Verify you can run rad using an absolute path
& $InstallFolder/rad version

if ($LASTEXITCODE) {
    Write-Error "Could not execute '$InstallFolder/rad version'"
    exit 1
}

## Verify you rad is resolved from PATH
& rad version

if ($LASTEXITCODE) {
    Write-Error "Could not execute 'rad version'"
    exit 1
}

## ---------------------------------------------------------------
## Test: stale rad elsewhere in PATH emits a warning banner
## ---------------------------------------------------------------

# Create a temporary directory with a dummy rad binary to simulate a stale install
$staleDir = Join-Path $env:TEMP "rad-stale-test-$(Get-Random)"
New-Item -Path $staleDir -ItemType Directory -Force | Out-Null

$staleCliName = if ($env:OS -eq "Windows_NT" -or $IsWindows) { "rad.exe" } else { "rad" }
$staleBinary = Join-Path $staleDir $staleCliName

# Create a minimal dummy executable
if ($env:OS -eq "Windows_NT" -or $IsWindows) {
    # Copy the real binary so it's a valid executable
    Copy-Item -Path (Join-Path $InstallFolder $staleCliName) -Destination $staleBinary -Force
}
else {
    Set-Content -Path $staleBinary -Value '#!/bin/sh' -NoNewline
    & chmod +x $staleBinary
}

try {
    # Prepend the stale directory to PATH so the installer detects it
    $savedPath = $env:PATH
    $separator = if ($env:OS -eq "Windows_NT" -or $IsWindows) { ';' } else { ':' }
    $env:PATH = "$staleDir$separator$env:PATH"

    # Run the installer again and capture output
    $output = & $PSScriptRoot/install.ps1 `
        -RadiusRoot $InstallFolder 2>&1 | Out-String

    # Assert the warning banner was emitted
    if ($output -notmatch "WARNING: Existing Radius CLI installation\(s\) found in different location\(s\)") {
        Write-Error "Expected stale-install warning banner, but it was not found in output:`n$output"
        exit 1
    }

    if ($output -notmatch [regex]::Escape($staleBinary)) {
        Write-Error "Expected stale path '$staleBinary' in warning output, but it was not found:`n$output"
        exit 1
    }

    Write-Output "PASS: stale rad warning banner was correctly emitted"
}
finally {
    $env:PATH = $savedPath
    Remove-Item -Recurse -Force $staleDir -ErrorAction SilentlyContinue
}

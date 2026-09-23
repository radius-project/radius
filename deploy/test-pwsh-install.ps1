
param(
    [string] $InstallFolder = "$($env:USERPROFILE)\rad-install-test",
    [switch] $EdgeOnly
)

function Test-EdgeInstallation {
    $ErrorActionPreference = 'Stop'
    $tokens = $null
    $parseErrors = $null
    $installer = [System.Management.Automation.Language.Parser]::ParseFile(
        (Join-Path $PSScriptRoot 'install.ps1'), [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -gt 0) {
        throw "Installer parse errors: $parseErrors"
    }
    $edgeFunction = $installer.Find({
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq 'Install-RadEdge'
    }, $true)
    if ($null -eq $edgeFunction) {
        throw 'Install-RadEdge was not found in the installer'
    }
    . ([scriptblock]::Create($edgeFunction.Extent.Text))

    $GitHubOrg = 'radius-project'
    $invocation = @{ Arguments = @() }
    function oras {
        $invocation.Arguments = @($args)
        if ($args.Count -ne 4 -or $args[0] -ne 'pull' -or $args[2] -ne '-o') {
            throw "Unexpected oras arguments: $args"
        }
        [System.IO.File]::WriteAllText((Join-Path $args[3] 'rad'), 'edge-test-payload')
        Set-Variable -Name LASTEXITCODE -Value 0 -Scope 1
    }

    $testRoot = Join-Path ([System.IO.Path]::GetTempPath()) "rad-edge-test-$([guid]::NewGuid())"
    New-Item -Path $testRoot -ItemType Directory | Out-Null
    try {
        foreach ($architecture in @('amd64', 'arm64')) {
            $target = Join-Path $testRoot $architecture
            New-Item -Path $target -ItemType Directory | Out-Null
            $invocation.Arguments = @()
            Install-RadEdge -TargetDir $target -DetectedOS windows `
                -DetectedArch $architecture -CliFileName 'rad.exe'

            $expectedReference = "ghcr.io/radius-project/rad/windows-${architecture}:edge"
            if ($invocation.Arguments.Count -ne 4 -or
                $invocation.Arguments[1] -cne $expectedReference -or
                $invocation.Arguments[3] -cne $target) {
                throw "Incorrect edge OCI reference or output directory: $($invocation.Arguments)"
            }
            $installedFile = Join-Path $target 'rad.exe'
            if (-not (Test-Path $installedFile -PathType Leaf) -or
                (Get-Content -Raw $installedFile) -cne 'edge-test-payload' -or
                (Test-Path (Join-Path $target 'rad'))) {
                throw 'The edge payload was not installed as rad.exe'
            }
            Write-Output "PASS: Windows $architecture edge installer uses :edge and installs rad.exe"
        }
    }
    finally {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}

Test-EdgeInstallation
if ($EdgeOnly) {
    return
}

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

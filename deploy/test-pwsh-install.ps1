
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

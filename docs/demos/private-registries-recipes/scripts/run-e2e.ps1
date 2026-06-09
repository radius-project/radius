#Requires -Version 7.0

<#
.SYNOPSIS
    E2E runner for the "Private Registries & Repositories" demo (cross-platform
    PowerShell / pwsh).

.DESCRIPTION
    Automates the manual walkthrough in ..\README.md: creates the Radius group
    and namespaces, optionally publishes the sample Bicep recipe, deploys the
    environment + app for the selected scenario, and verifies the result.

    Configuration is supplied through environment variables (see .PARAMETER notes
    and ..\README.md) so secrets stay off the command line.

.PARAMETER Scenario
    Scenario to run: bicep, terraform, combined, or all. Default: all.

.PARAMETER SkipPublish
    Skip 'rad bicep publish' (recipe already pushed).

.PARAMETER Cleanup
    Delete all demo resources and exit.

.EXAMPLE
    pwsh ./run-e2e.ps1 -Scenario bicep

.EXAMPLE
    pwsh ./run-e2e.ps1 -Scenario terraform -SkipPublish

.EXAMPLE
    pwsh ./run-e2e.ps1 -Cleanup

.NOTES
    Environment variables:
      Scenario 1 (bicep / combined):
        BICEP_REGISTRY, BICEP_RECIPE,
        BICEP_REGISTRY_USERNAME, BICEP_REGISTRY_PASSWORD
      Scenario 2 (terraform / combined):
        TF_REGISTRY_HOST, TF_RECIPE_LOCATION, TF_REGISTRY_TOKEN
#>

[CmdletBinding()]
param(
    [ValidateSet('bicep', 'terraform', 'combined', 'all')]
    [string]$Scenario = 'all',

    [switch]$SkipPublish,

    [switch]$Cleanup
)

$ErrorActionPreference = 'Stop'

$DemoDir = Split-Path -Parent $PSScriptRoot
$BicepDir = Join-Path $DemoDir 'bicep'
$RecipesDir = Join-Path $DemoDir 'recipes'

$RadGroup = 'demo-private-registries'
$BicepNamespace = 'private-bicep-demo'
$TfNamespace = 'private-tf-demo'
$CombinedNamespace = 'private-combined-demo'

function Test-Tools {
    foreach ($tool in @('rad', 'kubectl')) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
            throw "Required tool '$tool' not found on PATH"
        }
    }
}

function Assert-Vars {
    param([string[]]$Names)
    $missing = @()
    foreach ($name in $Names) {
        $value = [Environment]::GetEnvironmentVariable($name)
        if ([string]::IsNullOrEmpty($value)) {
            $missing += $name
        }
    }
    if ($missing.Count -gt 0) {
        throw "Missing required variables: $($missing -join ', ')"
    }
}

function Get-Var {
    param([string]$Name)
    return [Environment]::GetEnvironmentVariable($Name)
}

function Confirm-Namespace {
    param([string]$Namespace)
    kubectl get namespace $Namespace 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Creating namespace $Namespace"
        kubectl create namespace $Namespace
    }
}

function Initialize-Group {
    # Group may already exist; allow that but let 'rad group switch' validate.
    rad group create $RadGroup | Out-Null
    rad group switch $RadGroup
}

function Invoke-Bicep {
    Assert-Vars @('BICEP_REGISTRY', 'BICEP_RECIPE',
        'BICEP_REGISTRY_USERNAME', 'BICEP_REGISTRY_PASSWORD')
    Confirm-Namespace $BicepNamespace

    if (-not $SkipPublish) {
        Write-Host "Publishing Bicep recipe to $(Get-Var 'BICEP_RECIPE')"
        rad bicep publish `
            --file (Join-Path $RecipesDir 'redis-recipe.bicep') `
            --target "br:$(Get-Var 'BICEP_RECIPE')"
    }

    Write-Host 'Deploying Scenario 1 (private Bicep registry)'
    rad deploy (Join-Path $BicepDir 'bicep-private-registry.bicep') `
        --parameters registryHostname="$(Get-Var 'BICEP_REGISTRY')" `
        --parameters recipeLocation="$(Get-Var 'BICEP_RECIPE')" `
        --parameters registryUsername="$(Get-Var 'BICEP_REGISTRY_USERNAME')" `
        --parameters registryPassword="$(Get-Var 'BICEP_REGISTRY_PASSWORD')"

    Write-Host 'Verifying Scenario 1'
    rad resource list Applications.Core/extenders
    kubectl get pods -n $BicepNamespace
}

function Invoke-Terraform {
    Assert-Vars @('TF_REGISTRY_HOST', 'TF_RECIPE_LOCATION', 'TF_REGISTRY_TOKEN')
    Confirm-Namespace $TfNamespace

    Write-Host 'Deploying Scenario 2 (private Terraform registry)'
    rad deploy (Join-Path $BicepDir 'terraform-private-registry.bicep') `
        --parameters terraformRegistryHostname="$(Get-Var 'TF_REGISTRY_HOST')" `
        --parameters recipeLocation="$(Get-Var 'TF_RECIPE_LOCATION')" `
        --parameters terraformRegistryToken="$(Get-Var 'TF_REGISTRY_TOKEN')"

    Write-Host 'Verifying Scenario 2'
    rad resource list Applications.Core/extenders
    kubectl get pods -n $TfNamespace
}

function Invoke-Combined {
    Assert-Vars @('BICEP_REGISTRY', 'BICEP_RECIPE',
        'BICEP_REGISTRY_USERNAME', 'BICEP_REGISTRY_PASSWORD',
        'TF_REGISTRY_HOST', 'TF_RECIPE_LOCATION', 'TF_REGISTRY_TOKEN')
    Confirm-Namespace $CombinedNamespace

    if (-not $SkipPublish) {
        Write-Host "Publishing Bicep recipe to $(Get-Var 'BICEP_RECIPE')"
        rad bicep publish `
            --file (Join-Path $RecipesDir 'redis-recipe.bicep') `
            --target "br:$(Get-Var 'BICEP_RECIPE')"
    }

    Write-Host 'Deploying Scenario 3 (combined)'
    rad deploy (Join-Path $BicepDir 'combined.bicep') `
        --parameters terraformRegistryHostname="$(Get-Var 'TF_REGISTRY_HOST')" `
        --parameters terraformRecipeLocation="$(Get-Var 'TF_RECIPE_LOCATION')" `
        --parameters terraformRegistryToken="$(Get-Var 'TF_REGISTRY_TOKEN')" `
        --parameters bicepRegistryHostname="$(Get-Var 'BICEP_REGISTRY')" `
        --parameters bicepRegistryUsername="$(Get-Var 'BICEP_REGISTRY_USERNAME')" `
        --parameters bicepRegistryPassword="$(Get-Var 'BICEP_REGISTRY_PASSWORD')"

    Write-Host 'Verifying Scenario 3'
    rad resource show Radius.Core/environments combined-env
    rad resource list Radius.Core/terraformConfigs
    rad resource list Radius.Core/bicepConfigs
}

function Remove-Demo {
    Write-Host 'Cleaning up demo resources'
    rad group switch $RadGroup 2>$null
    rad app delete $BicepNamespace --yes 2>$null
    rad app delete $TfNamespace --yes 2>$null
    rad app delete $CombinedNamespace --yes 2>$null
    rad group switch default 2>$null
    rad group delete $RadGroup --yes 2>$null
    kubectl delete namespace `
        $BicepNamespace $TfNamespace $CombinedNamespace --ignore-not-found
    Write-Host 'Cleanup complete'
}

function Main {
    Test-Tools

    if ($Cleanup) {
        Remove-Demo
        return
    }

    Write-Host '============================================================'
    Write-Host "Private registries E2E demo - scenario: $Scenario"
    Write-Host '============================================================'

    Initialize-Group

    switch ($Scenario) {
        'bicep' { Invoke-Bicep }
        'terraform' { Invoke-Terraform }
        'combined' { Invoke-Combined }
        'all' {
            Invoke-Bicep
            Invoke-Terraform
            Invoke-Combined
        }
    }

    Write-Host '============================================================'
    Write-Host "Done. Run 'pwsh ./run-e2e.ps1 -Cleanup' to remove demo resources."
    Write-Host '============================================================'
}

Main

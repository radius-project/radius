param(
    [Parameter(Mandatory)][string]$Name,
    [ValidateSet('show', 'logs')][string]$Action = 'show'
)
$ErrorActionPreference = 'Stop'
if ($Name -notmatch '^delegated-deebec8c-(?:[a-f0-9]{10}|b-[a-z0-9]{10})$') { throw 'Refusing a non-owned ACA app name.' }
$subscription = '2b04a171-9c7d-495f-9b99-96cefa291b05'
$group = 'rg-radius-aca-proto-28500d6a'
if ($Action -eq 'logs') {
    $result = az containerapp logs show --subscription $subscription --resource-group $group --name $Name --type console --tail 50 --format json --only-show-errors
    if ($LASTEXITCODE -ne 0) { throw 'Owned ACA console-log read failed.' }
    @($result) | ConvertTo-Json -AsArray
    exit
}
$raw = az account get-access-token --tenant a2068dc5-c592-4577-84ce-26877cfd264e --resource https://management.azure.com/ -o json --only-show-errors
if ($LASTEXITCODE -ne 0) { throw 'Target-tenant ARM authentication required.' }
$token = ($raw -join "`n") | ConvertFrom-Json
$url = "https://management.azure.com/subscriptions/$subscription/resourceGroups/$group/providers/Microsoft.App/containerApps/$Name" + '?api-version=2025-07-01'
try {
    $result = Invoke-RestMethod -Headers @{Authorization = "Bearer $($token.accessToken)"} -Uri $url
    @{
        id = $result.id; name = $result.name; systemData = $result.systemData
        provisioningState = $result.properties.provisioningState
        latestRevisionName = $result.properties.latestRevisionName
        latestReadyRevisionName = $result.properties.latestReadyRevisionName
        ingress = $result.properties.configuration.ingress
        containers = @($result.properties.template.containers | ForEach-Object {
            @{ name = $_.name; image = $_.image }
        })
    } | ConvertTo-Json -Depth 15
} catch {
    if ($_.Exception.Response.StatusCode -eq 404) {
        @{name = $Name; absent = $true} | ConvertTo-Json
    } else {
        throw 'Owned ACA resource read failed.'
    }
}

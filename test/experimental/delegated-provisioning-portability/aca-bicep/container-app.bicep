@description('Radius-supplied target properties and desired delegate snapshots.')
param context object

@description('Existing, operator-supplied ACA managed environment ID.')
param managedEnvironmentId string

@description('Region of the supplied ACA environment.')
param location string = 'eastus2'

var containers = context.resource.properties.containers
var incoming = context.resource.incomingReferences
var invalidRoutes = filter(incoming, route => route.type != 'Radius.Compute/routes' || route.properties.kind != 'TCP' || length(route.properties.rules) != 1 || !empty(route.properties.?hostnames ?? []) || !empty(route.properties.rules[0].matches) || !contains(containers, route.properties.rules[0].destinationContainer.containerName) || !contains(map(items(containers[route.properties.rules[0].destinationContainer.containerName].?ports ?? {}), port => port.value.containerPort), route.properties.rules[0].destinationContainer.containerPort))
var validRoutes = length(incoming) <= 1 && empty(invalidRoutes)
var appName = validRoutes
  ? 'delegated-deebec8c-b-${take(uniqueString(toLower(context.resource.id)), 10)}'
  : fail('Bicep ACA demo requires one unconditional internal TCP route to a declared container port.')
var port = empty(incoming) ? 80 : incoming[0].properties.rules[0].destinationContainer.containerPort

resource app 'Microsoft.App/containerApps@2025-07-01' = {
  name: appName
  location: location
  tags: {
    experiment: 'delegated-provisioning-deebec8c-bicep'
  }
  properties: {
    environmentId: managedEnvironmentId
    managedEnvironmentId: managedEnvironmentId
    configuration: {
      activeRevisionsMode: 'Single'
      ingress: empty(incoming) ? null : {
        external: false
        transport: 'tcp'
        targetPort: port
        exposedPort: port
        traffic: [{
          latestRevision: true
          weight: 100
        }]
      }
    }
    template: {
      scale: {
        minReplicas: 1
        maxReplicas: 1
      }
      containers: map(items(containers), container => {
        name: container.key
        image: container.value.image
        command: container.value.?command ?? []
        args: container.value.?args ?? []
        env: map(items(container.value.?env ?? {}), entry => {
          name: entry.key
          value: entry.value.value
        })
        resources: {
          cpu: json('0.25')
          memory: '0.5Gi'
        }
      })
    }
  }
}

// Radius inventories the declared app automatically; do not duplicate its ID.
output result object = {
  values: {}
  delegatedOutputs: map(incoming, route => {
    id: route.id
    revision: route.revision
    values: {
      listener: {
        hostname: app.name
        port: route.properties.rules[0].destinationContainer.containerPort
        protocol: 'TCP'
      }
    }
  })
}

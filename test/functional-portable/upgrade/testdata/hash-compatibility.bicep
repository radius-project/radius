extension radius

@description('The test-owned HTTP server hosting the Terraform fixture.')
param moduleServer string

@description('The image for the secret-consuming workload.')
param workloadImage string

@description('Create an additional resource while the control plane is upgrading.')
@allowed([
  'false'
  'true'
])
param createDuring string = 'false'

@description('A value that can be changed without replacing the Terraform resource.')
param message string = 'before'

var appName = 'hash-upgrade-app'
var envName = 'hash-upgrade-env'

// Exercise the retained API used by installations that created SHA-1 identities.
resource environment 'Applications.Core/environments@2023-10-01-preview' = {
  name: envName
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: envName
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/identity.zip//modules'
        }
      }
    }
  }
}

resource application 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  properties: {
    environment: environment.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: appName
      }
    ]
  }
}

resource consumer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'hash-upgrade-consumer'
  properties: {
    application: application.id
    environment: environment.id
    container: {
      image: workloadImage
      imagePullPolicy: 'IfNotPresent'
      command: [
        '/bin/sh'
        '-c'
      ]
      args: [
        'test "$CONNECTION_STATE_SENTINEL" = "hash-upgrade-fixture" && while true; do sleep 3600; done'
      ]
    }
    connections: {
      state: {
        source: stateful.id
      }
    }
  }
}

resource stateful 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'hash-upgrade-state'
  properties: {
    application: application.id
    environment: environment.id
    recipe: {
      name: 'default'
      parameters: {
        message: message
      }
    }

  }
}

resource during 'Applications.Core/extenders@2023-10-01-preview' = if (createDuring == 'true') {
  name: 'hash-upgrade-during'
  properties: {
    application: application.id
    environment: environment.id
    recipe: {
      name: 'default'
      parameters: {
        message: 'created during upgrade'
      }
    }
  }
}

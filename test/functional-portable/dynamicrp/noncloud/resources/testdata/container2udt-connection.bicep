extension radius
extension testresources

param  environment string

resource udtapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dynamicrp-cntr2udt'
  location: 'global'
  properties: {
    environment: environment
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'dynamicrp-postgres-existing-app'
      }
    ]
  }
}


resource udtcntr 'Applications.Core/containers@2023-10-01-preview' = {
    name: 'udtcntr'
    properties: {
      application: udtapp.id
      container: {
        image: 'ghcr.io/radius-project/samples/demo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }

      connections: {
        postgres: {
          source: udtpgexisting.id
        }
      }
  
    }
  
    }
}

resource udtpgexisting 'Test.Resources/postgres@2023-10-01-preview' existing= {
  name: 'existing-postgres'
}

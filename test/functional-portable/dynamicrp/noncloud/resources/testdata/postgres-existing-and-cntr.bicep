extension radius
extension testresources

param  environment string

resource udtapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dynamicrp-postgres-existing'
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
    name: 'postgres-cntr'
    properties: {
      application: udtapp.id
      container: {
        image: 'ghcr.io/radius-project/samples/demo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
  
        env: {
          CONNECTION_POSTGRES_HOST: {
            value: udtpgexisting.properties.host
          }
          CONNECTION_POSTGRES_PORT: {
            value: string(udtpgexisting.properties.port)
          }
          CONNECTION_POSTGRES_USERNAME: {
            value: udtpgexisting.properties.username
          }
          CONNECTION_POSTGRES_DATABASE: {
            value: udtpgexisting.properties.database
          }
          CONNECTION_POSTGRES_PASSWORD: {
            value: udtpgexisting.properties.password
          }
        }
    }
  
    }
}

resource udtpgexisting 'Test.Resources/postgres@2023-10-01-preview' existing= {
  name: 'existing-postgres'
}

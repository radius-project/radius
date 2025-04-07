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
        image: 'ghcr.io/nithyatsu/todo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
  
        env: {
          CONNECTION_POSTGRES_HOST: {
            value: udtpgexisting.properties.status.binding.host
          }
          CONNECTION_POSTGRES_PORT: {
            value: string(udtpgexisting.properties.status.binding.port)
          }
          CONNECTION_POSTGRES_USERNAME: {
            value: udtpgexisting.properties.status.binding.username
          }
          CONNECTION_POSTGRES_DATABASE: {
            value: udtpgexisting.properties.status.binding.database
          }
          CONNECTION_POSTGRES_PASSWORD: {
            value: udtpgexisting.properties.status.binding.password
          }
        }
    }
  
    }
}

resource udtpgexisting 'MyCompany2.Datastores/postgres@2023-10-01-preview' existing= {
  name: 'existing-postgres'
}

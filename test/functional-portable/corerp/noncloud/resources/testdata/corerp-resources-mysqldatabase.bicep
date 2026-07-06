extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-mysqldb'
  location: location
  properties: {
    environment: environment
  }
}

resource dbSecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'mysqldb-secret'
  location: location
  properties: {
    environment: environment
    application: app.id
    data: {
      USERNAME: {
        value: 'admin'
      }
      PASSWORD: {
        value: 'password'
      }
    }
  }
}

resource mysql 'Radius.Data/mySqlDatabases@2025-08-01-preview' = {
  name: 'mysqldb-db'
  location: location
  properties: {
    environment: environment
    application: app.id
    secretName: dbSecret.name
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'mysqldb-ctnr'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      mysqldbctnr: {
        image: magpieimage
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
    connections: {
      mysqldb: {
        source: mysql.id
      }
    }
  }
}

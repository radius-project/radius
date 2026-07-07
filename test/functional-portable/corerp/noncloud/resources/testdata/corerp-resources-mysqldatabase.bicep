extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

@secure()
@description('Administrator password for the MySQL database.')
param password string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-mysqldb'
  location: location
  properties: {
    environment: environment
  }
}

resource mysql 'Radius.Data/mySqlDatabases@2025-08-01-preview' = {
  name: 'mysqldb-db'
  location: location
  properties: {
    environment: environment
    application: app.id
    username: 'admin'
    password: password
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

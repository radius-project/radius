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

// Single source of truth for the database name: authored on the mySqlDatabases
// resource below and passed to the container as MYSQL_DB.
var databaseName = 'appdb'

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
    database: databaseName
    username: 'admin'
    password: password
  }
}

// NOTE — no `connections` entry. The Radius.Compute/containers recipe builds
// CONNECTION_MYSQLDB_* env vars from every property of the connected resource,
// which now includes the x-radius-sensitive `password`. That property is redacted
// to null on reads, so the recipe's `string(...)` over the connection properties
// fails with "value is of type Null". The database connection details are set
// directly on the container instead, and the `mysql.properties.host` reference
// still creates the deploy-ordering edge so the database exists before the
// container starts.
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
        env: {
          MYSQL_HOST: {
            value: mysql.properties.host
          }
          MYSQL_DB: {
            value: databaseName
          }
          MYSQL_USER: {
            value: 'admin'
          }
          MYSQL_PASSWORD: {
            value: password
          }
        }
      }
    }
  }
}

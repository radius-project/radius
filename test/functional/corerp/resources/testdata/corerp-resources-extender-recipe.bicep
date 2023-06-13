import radius as rad

param application string
param environment string

resource s3Extender 'Applications.Link/extenders' = {
  name: 's3'
  properties: {
    environment: environment
    application: application
    recipe: {
      name: 's3'
    }
  }
}

resource container 'Applications.Core/containers' = {
  name: 'mycontainer'
  properties: {
    application: application
    container: {
      image: '*****'
      // In this case, I need to set the bucketname to an env name I already use
      env: {
        // Access property
        // User needs to know name of the property (tooling does not)
        BUCKETNAME: s3.properties.bucketName
        // Access secrets (permissioned separately than properties and not part of the resource body)
        DBSECRET: s3.secrets('databaseSecret')
      }
    }
    connections: {
      // This sets environment variable(s) on the container
      // CONNECTION_S3_BUCKETNAME
      // CONNECTION_S3_DATABASESECRET
      s3: {
        source: s3.id
      }
    }
  }
}

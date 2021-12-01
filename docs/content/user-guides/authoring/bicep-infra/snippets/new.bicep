param location string = 'westus2'

resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: 'myaccount'
  location: location
  properties: {
    databaseAccountOfferType: 'Standard'
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
      }
    ]
  }

  resource db 'mongodbDatabases' = {
    name: 'mydb'
    properties: {
      resource: {
        id: 'mydatabase'
      }
      options: {
        throughput: 400
      }
    }
  }
}

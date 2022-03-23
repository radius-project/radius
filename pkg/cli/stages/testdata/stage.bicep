param param1 string

// Sample resource
resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: param1
  location: 'westus2'
  properties: {
    databaseAccountOfferType: 'Standard'
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: 'westus2'
      }
    ]
  }
}

import radius as radius

@description('ID of the Radius environment. Passed in automatically via the rad CLI')
param environment string

resource demoApplication 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-failure-app'
  properties: {
    environment: environment
  }
}

resource demoSecretStore 'Applications.Core/secretStores@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-failure-secretstore'
  properties: {
    application: demoApplication.id
    type: 'certificate'
    
    // Reference the existing mynamespace/secret Kubernetes secret
    resource: 'mynamespace/secret'
    data: {
      // Make the tls.crt and tls.key secrets available to the application
      'tls.crt': {}
      'tls.key': {}
    }
  }
}

resource demoGateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-failure-gateway'
  properties: {
    application: demoApplication.id
    hostname: {
       fullyQualifiedHostname: 'a.example.com' // Replace with your domain name.
    }
    routes: [
      {
        path: '/'
        destination: 'http://demo-container:3000'
      }
    ]
    tls: {
      certificateFrom: demoSecretStore.id
      minimumProtocolVersion: '1.2'
    }
  }
}

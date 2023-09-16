import radius as radius

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

@description('Specifies tls cert secret values.')
@secure()
param tlscrt string
@secure()
param tlskey string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-gateway-tlstermination'
  properties: {
    environment: environment
  }
}

// This is not being referenced by any other resource. Should it be deleted?
resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'tls-gtwy-gtwy'
  properties: {
    application: app.id
    tls: {
      certificateFrom: certificate.id
    } 
    routes: [
      {
        path: '/'
        destination: frontendRoute.id
      }
    ]
  }
}

resource certificate 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'tls-gtwy-cert'
  properties: {
    application: app.id
    type: 'certificate'
    data: {
      'tls.key': {
        value: tlskey
      }
      'tls.crt': {
        value: tlscrt
      }
    }
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'tls-gtwy-front-rte'
  properties: {
    application: app.id
    port: 443
  }
}

resource frontendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'tls-gtwy-front-ctnr'
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          provides: frontendRoute.id
        }
      }
      readinessProbe: {
        kind: 'tcp'
        containerPort: port
      }
    }
  }
}

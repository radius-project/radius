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

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-tlstermination'
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-gtwy'
  properties: {
    application: app.id
    tls: {
      certificateFrom: certificate.id
    } 
    routes: [
      {
        path: '/'
        destination: 'http://tls-gtwy-front-ctnr:443'
      }
    ]
  }
}

resource certificate 'Applications.Core/secretStores@2022-03-15-privatepreview' = {
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

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-front-ctnr'
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          port: 443
        }
      }
      readinessProbe: {
        kind: 'tcp'
        containerPort: port
      }
    }
  }
}

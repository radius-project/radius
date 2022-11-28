import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: 'default'
}

import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

param cert string
param key string


resource secret 'core/Secret@v1' = {
  metadata: {
    name: 'mysecret'
  }
  stringData: {
    key:key
    cert: cert
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'gtwy-gtwy'
  location: location
  properties: {
    application: app.id
    routes: [
      {
        destination: frontendRoute.id
      }
    ]
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'gtwy-front-rte'
  location: location
  properties: {
    application: app.id
    port: 81
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'gtwy-front-ctnr'
  location: location
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
      env: {
        KEY: base64ToString(secret.data.key)
        CERT: base64ToString(secret.data.cert)
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
  }
}

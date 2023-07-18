import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-httproute'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-httproute-app'
      }
    ]
  }
}

resource containeruuu 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containeruuu'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
    connections: {
      containerzzm: {
        source: 'http://containerzzm:42'
      }
      containerkkj: {
        source: 'http://containerkkj:934'
      }
    }
  }
}

// canonically accurate ports :)
resource containerzzm 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerzzm'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
        wonderland: {
          containerPort: 42
        }
        vegas: {
          containerPort: 777
        }
      }
    }
  }
}

resource containerkkj 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerkkj'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
        hogwarts: {
          containerPort: 934
        }
        narnia: {
          containerPort: 7
        }
        asgard: {
          containerPort: 9
        }
      }
    }
  }
}

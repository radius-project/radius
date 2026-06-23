extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Specifies the Kubernetes namespace for the environment.')
param kubernetesNamespace string = 'mcluster-resources-container-tf'

resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'mcluster-resources-container-tf-pack'
  location: location
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'terraform'
        source: '${moduleServer}/kubernetes-container.zip//modules'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'mcluster-resources-container-tf-env'
  location: location
  properties: {
    recipePacks: [
      recipePack.id
    ]
    providers: {
      kubernetes: {
        namespace: kubernetesNamespace
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'mcluster-resources-container-tf'
  location: location
  properties: {
    environment: env.id
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'mcluster-tf-ctnr'
  location: location
  properties: {
    application: app.id
    environment: env.id
    containers: {
      mclustertfctnr: {
        image: magpieimage
        ports: {
          web: {
            containerPort: port
          }
        }
      }
    }
    connections: {}
  }
}

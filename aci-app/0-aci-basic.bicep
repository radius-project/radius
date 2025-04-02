extension radius
param aciscope string = '/subscriptions/<>/resourceGroups/<>'


resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'aci-env'
  properties: {
    compute: {
      kind: 'aci'
      resourceGroup: aciscope
    }
    providers: {
      azure: {
        scope: aciscope
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'aci-app'
  properties: {
    environment: env.id
  }
}

resource acimag 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'aci-magpie'
  properties: {
    application: app.id
    container: {
      image: 'ghcr.io/radius-project/magpiego:latest'
      env: {
        MAGPIE_PORT: {
          value: '8080'
        }
      }
    }
  }
}

resource acidemo 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'aci-demo'
  properties: {
    application: app.id
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      env: {
        DEMO_ENV: {
          value: 'test'
        }
      }
    }
  }
}
